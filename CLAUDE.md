# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Overview

Bob is the purse-first marketplace package containing MCP servers, CLI tools,
and development workflow skills. It consumes `purse-first.lib.mkMarketplace` as
a flake input to assemble 6 packages and 22 skills into a single installable
marketplace.

## Build & Test Commands

``` sh
just build              # nix build (marketplace bundle with all packages)
just test               # Run ALL tests (Go + Rust + BATS integration)
just fmt                # Format code (Go, shell, Nix)
nix flake check         # Nix-level validation
just lint               # go vet ./...
just go-mod-sync        # After ANY Go module change (see below)
just vendor             # Regenerate go workspace vendor after dep changes
just vendor-hash        # Recompute goVendorHash in flake.nix from vendor/
just deps               # go work sync + go work vendor

# EXPERIMENTAL
just release-tap-dancer 0.2.0  # Bump versions, build, commit, tag (does not push)
```

### Running Individual Tests

``` sh
# Per-package Go tests:
just test-lux           # packages/lux/...
just test-caldav        # packages/caldav/...
just test-go-mcp        # libs/go-mcp/... (from purse-first, not local)
just test-chix          # packages/chix (Rust, via cargo test)

# Single Go test function:
nix develop --command go test -run TestFunctionName ./packages/lux/...

# Single BATS file:
nix develop --command bats --tap zz-tests_bats/validate_plugin_repos.bats

# Integration tests (requires nix build first):
just test-integration
```

### Building Individual Packages

``` sh
nix build .#lux
nix build .#chix
nix build .#robin       # skill-only package from batman
nix build .#tap-dancer
```

## Terminology

- **Package** (not "plugin") --- the user-facing term. Three flavors:
  - **MCP package** --- MCP server only (lux, caldav)
  - **Skill package** --- Skill only (robin, tap-dancer, bob skills)
  - **MCP + Skill package** --- Both (chix)
- **Marketplace** --- aggregated `symlinkJoin` output with `marketplace.json`

## Architecture

### Go Workspace

All Go packages share a single `go.work` workspace. Modules:
`packages/{caldav,lux,potato}`, `packages/tap-dancer/go`, `dummies/go`.

The `vendor/` directory is **intentionally gitignored**. It exists only for
local IDE/tooling use and for computing the Nix vendor hash via
`nix hash path vendor/`. It will never appear in `git status` or `git diff` ---
this is expected. The Nix build independently re-vendors inside the sandbox
using `go work vendor -e` (see `mkGoWorkspaceModule.nix`); the local vendor
directory is never copied into the Nix store.

All Go packages share a single `goWorkspaceSrc` and `goVendorHash` in
`flake.nix`. The vendor hash only covers external dependencies --- local code
changes never require recomputing it. Run `just vendor-hash` only after
adding/removing external dependencies.

**After any Go module change** (adding/removing deps, changing module paths,
editing `go.mod` or `go.work`), run `just go-mod-sync`. This syncs the
workspace, re-vendors, recomputes the Nix vendor hash, and verifies the build.
It uses the standalone `#go` devShell to avoid a chicken-and-egg problem: the
default devShell builds all packages (requiring a valid vendor hash), but the
vendor hash can't be computed until vendoring is done. The individual
`just vendor`, `just deps`, and `just vendor-hash` recipes also use `#go` for
the same reason.

### Rust Workspace

`packages/chix` and `packages/tap-dancer/rust` share a Cargo workspace.

### Package Lifecycle (Three-Mode Main)

Every Go MCP package's `main.go` dispatches on its first argument:

1.  **`generate-plugin <dir>`** --- build-time: writes `plugin.json`,
    `mappings.json`, and `hooks/`
2.  **`hook`** --- Claude Code PreToolUse handler: denies built-in tools when an
    MCP tool should be used instead
3.  **no args** --- runtime: starts the MCP server

### Nix Build

Uses `purse-first.lib.mkMarketplace` as a flake input to assemble the
marketplace. The `flake.nix` imports package build expressions from
`lib/packages/`, builds all Go and Rust packages, then passes them to
`mkMarketplace` which runs `symlinkJoin` and generates `marketplace.json`.

### Skill Documents

Skills live in `skills/<name>/SKILL.md` with YAML frontmatter. Skills MAY have
`references/` and `examples/` subdirectories. Discovery is automatic --- any
`skills/*/SKILL.md` is a skill.

## Repository Layout

  -----------------------------------------------------------------------------
  Directory                                  Purpose
  ------------------------------------------ ----------------------------------
  `packages/`                                All packages (caldav, lux, chix,
                                             batman, tap-dancer, potato,
                                             sandcastle, and-so-can-you-repo)

  `skills/`                                  22 general-purpose skills
                                             (workflow, documentation,
                                             debugging)

  `lib/packages/`                            Nix build expressions for each
                                             package

  `flake.nix` (devShell)                     Dev shell packages inlined (go,
                                             rust, shell, bats)

  `dummies/go/`                              Fake MCP servers for testing

  `zz-tests_bats/`                           BATS integration tests
  -----------------------------------------------------------------------------

## Key Conventions

### Stable-First Nixpkgs

Every flake uses this pattern --- do not deviate:

- `nixpkgs` → stable branch (runtimes, core tools); follows purse-first's pin
- `nixpkgs-master` → master/unstable (LSPs, linters, formatters); pinned
  directly
- `utils` → `flake-utils` from FlakeHub

### Build Artifacts

Nix builds output to `result`/`result-*` symlinks (gitignored). All other
toolchain builds (go, cargo) must output to the `build/` directory.

### Code Style

- **Go**: `goimports` + `gofumpt`
- **Rust**: `cargo fmt` + `cargo clippy`
- **Nix**: `nixfmt-rfc-style`
- **Shell**: `set -euo pipefail`, 2-space indent, `shfmt -s -i=2`
- **Tests**: TAP-14 output format when reasonable, BATS for CLI integration
  tests

### Git

- GPG signing is required for commits. If signing fails, ask user to unlock
  their agent rather than skipping signatures

### Dependencies

- Go MCP packages depend on `github.com/amarbel-llc/purse-first/libs/go-mcp`
  (published module, not workspace local)
- Rust packages depend on `mcp-server` crate from the purse-first repo via git
  dependency
- `tap-dancer/go` is published as
  `github.com/amarbel-llc/bob/packages/tap-dancer/go` (tagged v0.1.0)
