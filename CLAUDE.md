# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Bob is the purse-first marketplace package containing MCP servers, CLI tools, and development workflow skills. It consumes `purse-first.lib.mkMarketplace` as a flake input to assemble 8 packages and 22 skills into a single installable marketplace.

## Build & Test Commands

```sh
just build              # nix build (marketplace bundle with all packages)
just test               # Run ALL tests (Go + Rust + BATS integration)
just fmt                # Format code (Go, shell, Nix)
nix flake check         # Nix-level validation
just lint               # go vet ./...
just vendor             # Regenerate go workspace vendor after dep changes
just vendor-hash        # Recompute goVendorHash in flake.nix from vendor/
just deps               # go work sync + go work vendor
```

### Running Individual Tests

```sh
# Per-package Go tests:
just test-grit          # packages/grit/...
just test-lux           # packages/lux/...
just test-get-hubbed    # packages/get-hubbed/...
just test-go-mcp        # libs/go-mcp/... (from purse-first, not local)
just test-chix          # packages/chix (Rust, via cargo test)

# Single Go test function:
nix develop --command go test -run TestFunctionName ./packages/grit/...

# Single BATS file:
nix develop --command bats --tap zz-tests_bats/validate_marketplace.bats

# Integration tests (requires nix build first):
just test-integration
```

### Building Individual Packages

```sh
nix build .#grit
nix build .#lux
nix build .#get-hubbed
nix build .#chix
nix build .#robin       # skill-only package from batman
nix build .#tap-dancer
```

## Terminology

- **Package** (not "plugin") --- the user-facing term. Three flavors:
  - **MCP package** --- MCP server only (grit, get-hubbed, lux, mgp)
  - **Skill package** --- Skill only (robin, tap-dancer, bob skills)
  - **MCP + Skill package** --- Both (chix)
- **Marketplace** --- aggregated `symlinkJoin` output with `marketplace.json`

## Architecture

### Go Workspace

All Go packages share a single `go.work` workspace. Modules: `packages/{grit,get-hubbed,lux,mgp,potato,spinclass}`, `packages/tap-dancer/go`, `dummies/go`.

In Nix, all Go packages share a single `goWorkspaceSrc` and `goVendorHash` in `flake.nix`. The vendor hash only covers external dependencies --- local code changes never require recomputing it. Run `just vendor-hash` only after adding/removing external dependencies.

### Rust Workspace

`packages/chix` and `packages/tap-dancer/rust` share a Cargo workspace.

### Package Lifecycle (Three-Mode Main)

Every Go MCP package's `main.go` dispatches on its first argument:

1. **`generate-plugin <dir>`** --- build-time: writes `plugin.json`, `mappings.json`, and `hooks/`
2. **`hook`** --- Claude Code PreToolUse handler: denies built-in tools when an MCP tool should be used instead
3. **no args** --- runtime: starts the MCP server

### Nix Build

Uses `purse-first.lib.mkMarketplace` as a flake input to assemble the marketplace. The `flake.nix` imports package build expressions from `lib/packages/`, builds all Go and Rust packages, then passes them to `mkMarketplace` which runs `symlinkJoin` and generates `marketplace.json`.

### Skill Documents

Skills live in `skills/<name>/SKILL.md` with YAML frontmatter. Skills MAY have `references/` and `examples/` subdirectories. Discovery is automatic --- any `skills/*/SKILL.md` is a skill.

## Repository Layout

| Directory | Purpose |
|-----------|---------|
| `packages/` | All packages (grit, get-hubbed, lux, mgp, chix, batman, tap-dancer, spinclass, potato, sandcastle, and-so-can-you-repo) |
| `skills/` | 22 general-purpose skills (workflow, documentation, debugging) |
| `lib/packages/` | Nix build expressions for each package |
| `devenvs/` | Dev shells (go, rust, shell, bats) |
| `dummies/go/` | Fake MCP servers for testing |
| `zz-tests_bats/` | BATS integration tests |

## Key Conventions

### Stable-First Nixpkgs

Every flake uses this pattern --- do not deviate:

- `nixpkgs` → stable branch (runtimes, core tools)
- `nixpkgs-master` → master/unstable (LSPs, linters, formatters)
- `utils` → `flake-utils` from FlakeHub
- Both follow purse-first's nixpkgs pins

### Build Artifacts

Nix builds output to `result`/`result-*` symlinks (gitignored). All other toolchain builds (go, cargo) must output to the `build/` directory.

### Code Style

- **Go**: `goimports` + `gofumpt`
- **Rust**: `cargo fmt` + `cargo clippy`
- **Nix**: `nixfmt-rfc-style`
- **Shell**: `set -euo pipefail`, 2-space indent, `shfmt -s -i=2`
- **Tests**: TAP-14 output format when reasonable, BATS for CLI integration tests

### Git

- GPG signing is required for commits. If signing fails, ask user to unlock their agent rather than skipping signatures

### Dependencies

- Go MCP packages depend on `github.com/amarbel-llc/purse-first/libs/go-mcp` (published module, not workspace local)
- Rust packages depend on `mcp-server` crate from the purse-first repo via git dependency
- `packages/spinclass` has a workspace `replace` directive for `packages/tap-dancer/go` (unpublished module)
