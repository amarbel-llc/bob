# `lux fmt-all` and Stop-Hook Formatting

**Date:** 2026-04-06
**Status:** approved
**Relates to:** amarbel-llc/bob#87 (LSP fallback for `lux fmt`), amarbel-llc/bob#88 (default exclusions for generated files)
**Defers to:** FDR `0001-claude-hooks-actions-config` (exploring)

## Problem

Lux's Claude Code package generates a `PostToolUse` hook on `Edit|Write` that
runs `lux fmt` against the just-edited file. This formats the file *between*
the agent's edit landing and the agent's next tool call, which causes
edit-loop pathologies:

- The agent's next `Edit` carries an `old_string` computed against
  pre-format content. After formatting, that string no longer matches.
- The agent retries, re-reads, re-edits, and may loop several times before
  recovering.
- Markdown formatters (line rewrapping, list renumbering) are the worst
  offender because every edit can shift unrelated lines.

The cross-turn variant exists too: edits in one turn get formatted at turn
end, the next turn starts with stale agent context. Less frequent but still
real.

## Decision

Replace the per-edit `PostToolUse` hook with a single `Stop` hook that runs a
new subcommand `lux fmt-all`. The subcommand walks the project tree and
formats every recognized file in one pass after the agent finishes its
response.

### `lux fmt-all`

**Signature:** `lux fmt-all [path...]`

**Inputs:**

- Zero paths → walk PWD
- One or more paths → for each entry:
  - file → format if recognized by `formatter.Router`, else silently skip
  - directory → walk recursively, applying the same per-file rule

**Walk strategy** (configurable, default `"git"`):

- `"git"` → use `git ls-files` semantics (tracked + untracked-not-ignored).
  Falls back to `"all"` outside a git repository.
- `"all"` → walk every file under the root, skipping only `.git/` and
  symlinks.

**Per-file behavior:** identical to `lux fmt <file>` today — load merged
filetype + formatter configs, route via `formatter.Router`, run the matched
formatter chain or fallback, write back in-place. Files with no formatter
match are silently skipped (no error). Files where the formatter produces no
change are also silent.

**Output:**

- Silent on per-file success
- Per-file failures logged to stderr, walk continues
- Hard errors (unreadable PWD, config load failure) abort and return non-zero

**Exit code:**

- `0` if the walk completed, regardless of per-file failures
- non-zero only on hard errors

This bias is deliberate: a single broken file should not kill every Stop-hook
invocation in the repo.

**No flags in v1.** No `--stdout`, no `--dry-run`, no `--filetype`, no
parallelism flag, no `--exclude`. Single-threaded walk. Add later if a real
need surfaces.

### Configuration: `fmt-all.toml`

New config file `~/.config/lux/fmt-all.toml` with per-project override at
`.lux/fmt-all.toml`. Same merge semantics as `formatters.toml`: local entries
shallow-replace global keys.

```toml
# ~/.config/lux/fmt-all.toml

# Walk strategy.
#   "git" → use git's view of the tree (tracked + untracked-not-ignored).
#           Falls back to "all" outside a git repository.
#   "all" → walk every file under the root, skipping only .git/ and symlinks.
walk = "git"

# Glob patterns (relative to the walk root) to skip even when git would
# include them. Use this for committed-but-generated files where running a
# formatter would cause spurious churn.
exclude_globs = [
  "flake.lock",
  "**/flake.lock",
  "**/go.sum",
  "**/Cargo.lock",
]
```

**Defaults if the file is missing:**

- `walk = "git"`
- `exclude_globs = []`

**Validation:**

- `walk` must be `"git"` or `"all"` (error otherwise)
- `exclude_globs` is a list of strings; bad glob patterns log a warning at
  load time and are dropped, walk continues with the remaining patterns
- Schema is intentionally minimal — no `exclude_filetypes`, no per-event
  knobs. Anything more complex is deferred to the FDR.

**Why a new file rather than extending `formatters.toml`:**

- `formatters.toml` is `[[formatter]]` array-of-tables describing formatter
  programs. `fmt-all` config is about traversal and selection, not formatter
  definitions.
- Per the lux convention "one file per concept," new concept → new file.
- Easier to delete or replace once the deferred `[[action]]` config layer
  subsumes it.

### Hook generation change

`internal/hooks/generate.go` currently writes a `PostToolUse` `Edit|Write`
matcher pointing at `${CLAUDE_PLUGIN_ROOT}/hooks/format-file`, which is a
bash script that extracts `tool_input.file_path` via `jq` and runs
`lux fmt`. Replace with:

```json
{
  "hooks": {
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "lux fmt-all",
        "timeout": 60
      }]
    }]
  }
}
```

The standalone `format-file` script and its `jq`-based extraction are
deleted. The `PreToolUse` deny-builtins hook from `go-mcp` is untouched (it
lives in a separate generation path).

`lux fmt-all` is invoked as a bare command — no `${CLAUDE_PLUGIN_ROOT}`
indirection, because the lux binary is on `$PATH` once the bob marketplace
is installed.

**Hook timeout:** 60 seconds. Cold-start of nix-built formatters can dominate
the wall-clock; 60s is conservative for typical worktrees but may be too low
for the very first Stop-hook fire after a clean nix store. Bumping the
timeout (or making it configurable) is deferred to the FDR.

## Tracer-bullet validation

Run against the bob worktree using the user's existing
`~/.config/lux/` configuration to verify the design holds in practice.

| Ext | Files | Filetype config | Formatter chain | `fmt-all` action |
|---|---:|---|---|---|
| `.go` | 165 | `go.toml` | goimports → gofumpt | format |
| `.rs` | 65 | `rust.toml` | rustfmt | format |
| `.bash` | 49 | `bash.toml` | shfmt | format |
| `.bats` | 15 | `bats.toml` | shfmt | format |
| `.ts` | 18 | `typescript.toml` | prettier | format |
| `.toml` | 8 | `toml.toml` | tommy | format |
| `.json` | 140 | `json.toml` | jq | format |
| `.yml` | 2 | `yaml.toml` | prettier | format |
| `.md` | 118 | `markdown.toml` | (commented out) | skip |
| `.nix` | 15 | `nix.toml` | nixfmt (added during this design) | format |

**Findings the tracer surfaced:**

1. **Markdown loop is already moot** in this user's global config: pandoc is
   commented out in `markdown.toml`, so `lux fmt-all` silently skips every
   `.md` file with no work needed. The original motivation for this
   redesign — markdown edit loops — was real but the user had already
   self-healed by disabling the markdown formatter. The redesign still has
   value: it eliminates the failure mode for *anyone* who re-enables a
   markdown formatter, and removes the per-edit interference for non-markdown
   loops.

2. **Nix files were silently un-formatted**: `nix.toml` declared the `nil`
   LSP but no external formatter, so `lux fmt` returned "no formatter
   configured" for every `.nix` file. Fixed during this design by adding
   `nixfmt-rfc-style` to `formatters.toml` and referencing it in `nix.toml`.
   The deeper issue (LSP-driven formatting fallback) is tracked in #87.

3. **`flake.lock` would be touched by jq** because `json.toml` routes all
   `.json` to `jq .`. Mitigated user-side by the `exclude_globs` defaults in
   `~/.config/lux/fmt-all.toml`. The lux-side fix (ship default exclusions
   for known generated files) is tracked in #88.

## Rollback

The change is small: one new subcommand, one new config loader, one
generate.go edit, deletion of `format-file` script. To roll back:

1. Revert the commit
2. Rebuild bob marketplace: `just build` from bob root
3. Reinstall: `just install-bob`
4. Restart any running Claude Code sessions

Rollback restores the per-edit `PostToolUse` `Edit|Write` hook exactly as it
was. No data migration, no state to clean up. The new `fmt-all.toml` file
becomes inert if present after rollback (lux will read it but nothing
consumes it).

**No dual-architecture period.** The two hooks are mutually exclusive
(running both means double-formatting on every edit, which defeats the
point) and the rollback procedure is one revert away. The brainstorming
skill flags this — accepted because the change is small and the
worst-case outcome of a bad rollout is reverting one commit.

## Testing

**Unit tests:**

- `internal/config/fmtall_test.go` — load defaults, load with values,
  validate `walk` enum, malformed glob handling, missing-file → defaults,
  per-project override merge
- `cmd/lux/fmtall_test.go` — tmp dir with mixed recognized and unrecognized
  files, assert recognized files get formatted and unrecognized are skipped;
  exclude_globs honored; per-file failure does not abort the walk; explicit
  paths bypass git-walk semantics

**Integration tests (BATS):**

- `zz-tests_bats/lux_fmt_all.bats` — fixture directory with .go + .md +
  generated .json; assert exit 0, .go formatted, .md skipped, generated .json
  excluded
- Update `internal/hooks/generate_test.go` to assert the new `Stop` hook
  shape and the absence of the old `format-file` script

**Manual verification before commit:**

- Build the new lux package
- Install via `just install-bob`
- Run `lux fmt-all` in this worktree, assert no spurious changes to
  `flake.lock`, no churn on `.md` files, .go files reformatted as expected
- Trigger a Claude Code Stop event (any short prompt) and verify the hook
  fires within the 60s budget

## Out of scope

- The `[[action]]` config layer for binding arbitrary lux subcommands to
  arbitrary Claude hook events. Tracked in FDR `0001-claude-hooks-actions-config`.
- Per-filetype enable/disable for `fmt-all`. Deferred to the same FDR.
- LSP-driven formatting fallback. Tracked in #87.
- Default exclusions for generated files. Tracked in #88.
- Parallelism, dry-run, stdin mode, alternate output formats for `lux fmt-all`.
- Configurable hook timeout.
