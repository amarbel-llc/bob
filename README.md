# bob

**Purse-first package repo of MCP servers and CLI tools.**

> **Status:** `caldav` is the only active MCP package. Batman (the
> fence-based BATS test runner and bats helper libs) was extracted to
> [`amarbel-llc/bats`](https://github.com/amarbel-llc/bats) and is consumed
> here as a flake input; the other packages are small standalone tools or
> dormant.

Each package's Nix derivation self-produces its
`share/purse-first/<name>/` layout (plugin manifest + hooks) via its own
`generate-plugin` subcommand; clown's circus loads `caldav` from that path.
The flake's `default` output is a `symlinkJoin` of `caldav` plus the
non-MCP packages.

## Packages

| Package | What it is | Status |
|---------|-----------|--------|
| `caldav` | CalDAV MCP server — tasks, calendars, and VTODO management (Go) | **active** |
| `potato` | Pomodoro break timer CLI; counts down a rest break, 5 minutes by default (Go) | built, standalone |
| `sandcastle` | Sandbox runtime wrapping filesystem/network restrictions around processes — bubblewrap + seccomp on Linux, `sandbox-exec` on macOS (Node) | dormant — no longer wired into the bats test wrapper (superseded by `fence`) |
| `and-so-can-you-repo` | Interactive script that scaffolds a new nix-flake-backed repo (bash + gum + gh) | built, standalone |
| `polkadots` | Section-7 man pages documenting dev conventions (justfile design patterns, nix flake conventions, man pages, nix binary wrapping, dev test workspaces) | built, docs only |

The flake also re-exports `batman` / `bats-libs` from the
`amarbel-llc/bats` input, and builds `probe-fence-sandbox`, a regression
check that fence's bubblewrap keeps working inside Nix's build sandbox.

`packages/spinclass/` contains only empty directories — leftover scaffolding,
not built by the flake.

## Install / build

```sh
nix build                  # default bundle: caldav + non-MCP packages
nix build .#caldav         # individual packages: caldav, potato, sandcastle,
                           # and-so-can-you-repo, polkadots, batman, ...
./result/bin/caldav --help
```

The caldav MCP server speaks stdio and is configured via environment
variables: `CALDAV_URL`, `CALDAV_USERNAME`, `CALDAV_PASSWORD`.

## Three-mode main

Every Go MCP package's `main.go` dispatches on its first argument:

1. `generate-plugin <dir>` — build-time: writes `plugin.json`,
   `mappings.json`, and `hooks/`
2. `hook` — Claude Code PreToolUse handler: denies built-in tools when an
   MCP tool should be used instead
3. *no args* — runtime: starts the MCP server

Currently `caldav` is the only package using this convention; its
`generate-plugin` runs as a `postInstall` step so the built derivation
already carries its plugin layout.

## Development

Justfile recipes are the paved paths:

```sh
just build        # nix build (caldav + non-MCP packages)
just test         # all tests: Go + BATS lanes + validate-mcp
just test-caldav  # Go tests for packages/caldav only
just test-bats    # BATS integration lanes (run inside the nix sandbox)
just fmt          # format Go code
just lint         # go vet across every workspace module
just vendor       # after ANY Go module change: re-vendor + recompute
                  # the goVendorHash in flake.nix
```

All Go packages share a single `go.work` workspace and a single
`goVendorHash` in `flake.nix`. The `vendor/` directory is intentionally
gitignored — it exists only for local tooling and for computing the Nix
vendor hash. See `CLAUDE.md` for the full Go-workspace and vendor-hash
workflow.
