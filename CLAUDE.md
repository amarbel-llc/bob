# CLAUDE.md

## Overview

MCP servers, CLI tools, and development workflow skills — built as a purse-first marketplace package.

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

## Architecture

### Go Workspace

All Go packages share a single `go.work` workspace. Modules: `packages/{grit,get-hubbed,lux,mgp,potato,spinclass}`, `packages/tap-dancer/go`, `dummies/go`.

### Rust Workspace

`packages/chix` and `packages/tap-dancer/rust` share a Cargo workspace.

### Nix

Uses `purse-first.lib.mkMarketplace` as a flake input to assemble the marketplace. All Go packages share `goWorkspaceSrc` and `goVendorHash`.

## Key Conventions

### Stable-First Nixpkgs

- `nixpkgs` → stable branch
- `nixpkgs-master` → master/unstable
- Both follow purse-first's pins

### Code Style

- **Go**: `goimports` + `gofumpt`
- **Rust**: `cargo fmt` + `cargo clippy`
- **Nix**: `nixfmt-rfc-style`
- **Shell**: `set -euo pipefail`, 2-space indent, `shfmt -s -i=2`

### Git

- GPG signing is required for commits
