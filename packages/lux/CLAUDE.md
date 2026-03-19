# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with this repository.

## Overview

Lux is an LSP multiplexer written in Go that routes LSP requests to multiple language servers based on file type. It functions as an MCP server, exposing LSP capabilities as resources for AI assistants. Lux launches as a short-lived per-worktree process, starts LSP subprocesses on demand via Nix flakes, and terminates when the parent disconnects.

## Build & Development Commands

```sh
just build            # Nix build (produces ./result)
just build-go         # Quick go build for dev iteration (runs gomod2nix first)
just test             # Run all Go tests
just test-v           # Verbose test output
just fmt              # Format Go + shell code
just lint             # go vet
just deps             # go mod tidy + gomod2nix regeneration
just build-gomod2nix  # Regenerate gomod2nix.toml only
```

Run a single test:
```sh
nix develop --command go test -v -run TestName ./internal/config/
```

After changing `go.mod`, always run `just deps` to regenerate `gomod2nix.toml` (required for Nix builds).

## Architecture

### Per-Worktree Model

Lux runs as a one-off process per worktree (initiated by editor or Claude). It starts LSP subprocesses on demand for that workspace and terminates when the parent disconnects. There is no daemon, no sockets, no service installation, and no session management.

### Request Flow

Client (editor/Claude) connects via MCP over stdio. The **Bridge** (`internal/tools/bridge.go`) routes LSP requests through the **Router** (`internal/server/router.go`) which determines which LSP owns each file. The **Pool** (`internal/subprocess/pool.go`) starts LSP subprocesses on-demand via **NixExecutor** (`internal/subprocess/nix.go`), which runs `nix build <flake>` and caches the result path. Requests are forwarded over stdin/stdout pipes to the subprocess; responses are relayed back.

### MCP Resources

LSP capabilities are exposed as MCP resource templates, accessed via a single `read_resource` tool. This enables progressive disclosure — clients list available resource templates to discover capabilities, then read specific resources. Resource URI patterns:

- `lux://lsp/hover?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/definition?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/references?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/completion?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/document-symbols?uri={file_uri}`
- `lux://lsp/diagnostics?uri={file_uri}`
- `lux://lsp/format?uri={file_uri}`
- `lux://lsp/code-action?uri={file_uri}&start_line={sl}&start_character={sc}&end_line={el}&end_character={ec}`
- `lux://lsp/rename?uri={file_uri}&line={line}&character={character}&new_name={name}`
- `lux://lsp/workspace-symbols?uri={file_uri}&query={pattern}`
- `lux://status`, `lux://languages`, `lux://files`

### Key Packages

| Package | Role |
|---------|------|
| `cmd/lux` | CLI: `mcp-stdio`, `init`, `add`, `list`, `fmt`, `hook` |
| `internal/server` | LSP server, handler, and file-type router |
| `internal/subprocess` | LSP process pool, lifecycle state machine (Idle→Starting→Running→Stopping→Stopped), Nix executor |
| `internal/mcp` | MCP server, resource provider, document manager, diagnostics store, prompts |
| `internal/tools` | Bridge (adapts LSP operations to MCP), tool registry (for hook/artifact generation) |
| `internal/config` | TOML config parsing (`lsps.toml`, `formatters.toml`), per-project overrides, config merging |
| `internal/formatter` | External formatter routing and execution (separate from LSP formatting) |
| `internal/capabilities` | Auto-discovery and caching of LSP capabilities during `lux add` |
| `internal/lsp` | LSP protocol types, capability aggregation, URI utilities |
| `pkg/filematch` | File matching by extension, glob pattern, or language ID (priority: languageID > extension > pattern) |

### Configuration

- User config: `~/.config/lux/lsps.toml` (TOML, `[[lsp]]` entries)
- Formatter config: `~/.config/lux/formatters.toml`
- Cached capabilities: `~/.local/share/lux/capabilities/`
- Per-project overrides load from the project root directory

### LSP Config Fields

Each `[[lsp]]` entry supports: `name`, `flake`, `binary` (optional, for multi-binary flakes), `extensions`, `patterns`, `language_ids`, `args`, `env`, `init_options`, `settings`, `settings_key`, and `capabilities` (with `disable`/`enable` lists). At least one of `extensions`/`patterns`/`language_ids` is required.

## Key Dependencies

- `github.com/amarbel-llc/purse-first/libs/go-mcp` - MCP protocol library (JSON-RPC handler, transport interfaces)
- `github.com/BurntSushi/toml` - Config parsing
- `github.com/gobwas/glob` - Glob pattern matching for file routing
