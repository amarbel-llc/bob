# Perms Review Editor Redesign

## Problem

The current `spinclass perms review` has two issues:

1.  **Wrong diff baseline** --- diffs worktree `settings.local.json` against a
    snapshot taken at worktree creation, instead of against the global Claude
    settings. This means rules already in the user's global config appear as
    "new".

2.  **Poor TUI** --- uses `charmbracelet/huh` for one-at-a-time select per rule.
    No overview, no undo, no batch editing. Tedious with many rules.

## Design

### Diff Logic

Rules to review = worktree `settings.local.json` rules minus:

- Global Claude settings (`~/.claude/settings.local.json`)
- Already-curated tier rules (`global.json` + `repos/<repo>.json`)
- Auto-injected worktree-scoped rules (`Read/Edit/Write(<worktree>/*)`,
  `Read(~/.claude/*)`)

If no rules remain, print "no new permissions to review" and exit 0.

### Editor Format (git-rebase-i style)

    # spinclass perms review — change the action word for each permission
    # Actions: global | repo | keep | discard (unique prefixes OK: g/r/k/d)
    # Lines starting with # are ignored. Empty lines are ignored.
    # Repo: bob

    discard Bash(cargo test:*)
    discard Bash(nix build:*)
    discard mcp__plugin_chix_chix__build                    # chix:build
    discard mcp__plugin_grit_grit__add                      # grit:add
    discard mcp__plugin_lux_lux__hover                      # lux:hover

- Rules sorted alphabetically
- Default action: `discard`
- MCP tools get an inline `# server:tool` comment
- Action prefix matching: `g` = global, `r` = repo, `k` = keep, `d` = discard

### Parsing

1.  Strip lines starting with `#` and blank lines
2.  Split each line on first whitespace into `(action, rest)`
3.  Strip trailing `# comment` from `rest` to get the rule
4.  Resolve action prefix to full action name
5.  Unknown or ambiguous prefix is a parse error

### Post-Editor Review Loop

After the editor closes:

1.  Parse the file
2.  If parse errors, print them and re-open the editor
3.  Print the full parsed list with resolved action names
4.  Prompt: **accept / edit / abort**
    - accept: execute RouteDecisions (or dry-run output)
    - edit: re-open \$EDITOR
    - abort: exit with no changes

### Friendly Name Derivation

Parse `mcp__plugin_<server>_<server>__<tool>` to `server:tool` via string
splitting. No external metadata lookup.

### CLI Flags

    spinclass perms review [worktree-path]
      --worktree-dir <path>   Override worktree path
      --dry-run               Show what would change without writing

`--worktree-dir` takes precedence over the positional arg and cwd detection.

### Dry Run Output

    would promote to global tier (~/.config/spinclass/permissions/global.json):
      mcp__plugin_grit_grit__add
    would promote to repo tier (bob):
      Bash(cargo test:*)
    would discard (remove from settings.local.json):
      mcp__plugin_lux_lux__hover
    would keep (no change):
      Bash(nix build:*)

### Snapshot Removal

The `.settings-snapshot.json` mechanism is no longer needed. It can be removed
from `ApplyClaudeSettings` and the review flow in a follow-up cleanup.

### Rollback

Revert the commit. No dual-architecture needed --- the old huh TUI has no
external consumers.

## Files Changed

- `packages/spinclass/internal/perms/cmd.go` --- new flags, replace
  RunReviewInteractive
- `packages/spinclass/internal/perms/review.go` --- new editor-based review flow
- `packages/spinclass/internal/perms/settings.go` --- add global settings
  loading, worktree rule filtering
- `packages/spinclass/internal/perms/review_test.go` --- update tests
- New: `packages/spinclass/internal/perms/editor.go` --- editor format
  generation, parsing, friendly names
- New: `packages/spinclass/internal/perms/editor_test.go`
