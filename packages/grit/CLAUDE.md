# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Overview

**grit** is an MCP (Model Context Protocol) server that exposes git operations
over JSON-RPC via stdin/stdout. It's designed to be launched by MCP clients like
Claude Code. Built in Go, packaged with Nix using `gomod2nix`.

## Build & Dev Commands

All commands use the justfile and run inside a Nix dev shell:

``` sh
just build              # Nix build -> ./result/bin/grit
just build-go           # Go build via nix develop -> ./grit
just test               # go test ./...
just test-v             # go test -v ./...
just fmt                # go fmt ./...
just lint               # go vet ./...
just deps               # go mod tidy + gomod2nix
just install-claude     # Register as Claude Code MCP server
just clean              # Remove build artifacts
```

## Architecture

Single external dependency: `github.com/amarbel-llc/purse-first/libs/go-mcp`
(MCP server framework providing protocol types, transport, tool registry, and
resource registry).

### Entry Point

`cmd/grit/main.go` --- Sets up signal handling, creates a stdio JSON-RPC
transport, registers tools and resources, and runs the MCP server loop.

### Git Execution Layer

`internal/git/exec.go` --- Single function `Run(ctx, dir, args...)` that shells
out to `git`, captures stdout/stderr, and returns output or a formatted error.
Every tool and resource handler calls through this.

### MCP Resources

Read-only git operations are exposed as native MCP resources (auto-approved by
Claude Code, no permission dialog). The `resourceProvider` in
`internal/tools/resources.go` registers resources and dispatches reads by URI.

  Resource   URI                      Description
  ---------- ------------------------ ------------------------------------------
  Status     `grit://status`          Working tree status with branch info
  Branches   `grit://branches`        Local/remote branches with tracking info
  Remotes    `grit://remotes`         Remotes with URLs
  Tags       `grit://tags`            Tags with metadata
  Stashes    `grit://stashes`         Stash entries with index and message
  Log        `grit://log`             Commit history (template)
  Show       `grit://commits/{ref}`   Commit/tag/object detail (template)
  Blame      `grit://blame/{path}`    Line-by-line authorship (template)

All resources accept an optional `repo_path` query parameter (defaults to cwd).

**Subagent access:** Subagents cannot use MCP resources directly. Two tool
wrappers provide equivalent access: - `resource-templates` --- lists available
resources and templates - `resource-read` --- reads a resource by URI

### Tool System

`internal/tools/registry.go` --- `RegisterAll()` returns
`(*command.App, *resourceProvider)`. Tools are registered from category files.
Resources are registered via the resource provider.

**Tools** (mutation and complex query operations):

  Category    Tools
  ----------- -------------------------------------------------------------------
  Diff        `diff`
  Staging     `add`, `reset`, `rm`
  Commit      `commit`
  Branch      `branch-create`, `branch-delete`, `checkout`
  Remote      `fetch`, `pull`, `push`
  Rev parse   `git-rev-parse`
  Rebase      `rebase`, `interactive-rebase-plan`, `interactive-rebase-execute`
  Reset       `hard-reset`
  Cherry pick `cherry-pick`
  Tag         `tag-verify`
  Stash       `stash-save`, `stash-apply`, `stash-drop`
  Merge       `merge`
  Worktree    `worktree-list`, `worktree-remove`
  Resources   `resource-templates`, `resource-read`

### Safety Constraints

- Force push is blocked on `main`/`master` branches
- Merge into `main`/`master` is blocked for safety
- `reset` is soft-only (no working tree modifications)

## Nix Flake

Follows the stable-first nixpkgs convention (`nixpkgs` = stable,
`nixpkgs-master` = unstable). Uses devenv flakes from `github:friedenberg/eng`
for Go and shell environments. Built with `pkgs.buildGoApplication` via
`gomod2nix`.
