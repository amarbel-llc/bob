# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
this repository.

## Overview

Lux is an LSP multiplexer written in Go that routes LSP requests to multiple
language servers based on file type. It functions as an MCP server, exposing LSP
capabilities as resources for AI assistants. Lux launches as a short-lived
per-worktree process, starts LSP subprocesses on demand via Nix flakes, and
terminates when the parent disconnects.

## Build & Development Commands

``` sh
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

``` sh
nix develop --command go test -v -run TestName ./internal/config/
```

After changing `go.mod`, always run `just deps` to regenerate `gomod2nix.toml`
(required for Nix builds).

## Architecture

### Per-Worktree Model

Lux runs as a one-off process per worktree (initiated by editor or Claude). It
starts LSP subprocesses on demand for that workspace and terminates when the
parent disconnects. There is no daemon, no sockets, no service installation, and
no session management.

### LSP Server Mode (`lux lsp`)

Lux can also run as an LSP server over stdio, using standard JSON-RPC with
Content-Length framing. This allows editors to use lux directly as a language
server instead of going through MCP.

- `lux lsp` --- multiplexes across all configured LSPs
- `lux lsp --lang gopls` --- restricts to a single LSP backend

Phase 1 supports formatting only: `documentFormattingProvider` and
`documentRangeFormattingProvider`. All non-formatting methods return
`MethodNotFound` (`-32601`). Integration tests are in
`zz-tests_bats/lux_lsp.bats`.

See `docs/features/0001-lsp-editor-integration.md` for the full feature design
and phased roadmap.

### Request Flow

Client (editor/Claude) connects via MCP over stdio. The **Bridge**
(`internal/tools/bridge.go`) routes LSP requests through the **Router**
(`internal/server/router.go`) which determines which LSP owns each file. The
**Pool** (`internal/subprocess/pool.go`) starts LSP subprocesses on-demand via
**NixExecutor** (`internal/subprocess/nix.go`), which runs `nix build <flake>`
and caches the result path. Requests are forwarded over stdin/stdout pipes to
the subprocess; responses are relayed back.

### MCP Resources

LSP capabilities are exposed as MCP resource templates. The main conversation
should use `ReadMcpResourceTool` (server `plugin:lux:lux`) to access static
resources directly. Subagents use the `resource-templates` and `resource-read`
tools since they cannot access MCP resources directly.

All resources default to JSON output (`format=json`). Pass `&format=text` for
human-readable text output.

Resource URI patterns:

- `lux://lsp/hover?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/definition?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/references?uri={file_uri}&line={line}&character={character}&context={n}`
  (context defaults to 3, enriches with hover + surrounding lines)
- `lux://lsp/completion?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/document-symbols?uri={file_uri}`
- `lux://lsp/diagnostics?uri={file_uri}`
- `lux://lsp/format?uri={file_uri}`
- `lux://lsp/code-action?uri={file_uri}&start_line={sl}&start_character={sc}&end_line={el}&end_character={ec}`
- `lux://lsp/rename?uri={file_uri}&line={line}&character={character}&new_name={name}`
- `lux://lsp/workspace-symbols?uri={file_uri}&query={pattern}`
- `lux://lsp/incoming-calls?uri={file_uri}&line={line}&character={character}`
  (call hierarchy --- who calls this?)
- `lux://lsp/outgoing-calls?uri={file_uri}&line={line}&character={character}`
  (call hierarchy --- what does this call?)
- `lux://lsp/diagnostics-batch?glob={pattern}` (multi-file, multi-LSP
  diagnostics via glob)
- `lux://lsp/packages?uri={file_uri}&recursive={bool}` (Go package metadata,
  requires gopls)
- `lux://lsp/package-symbols?uri={file_uri}` (all symbols in a Go package,
  requires gopls)
- `lux://lsp/imports?uri={file_uri}` (file and package imports, requires gopls)
- `lux://lsp/modules?uri={dir_uri}&max_depth={n}` (module info in directory,
  requires gopls)
- `lux://status`, `lux://languages`, `lux://files`, `lux://commands`

### MCP Tools

In addition to `resource-templates` and `resource-read`, lux exposes:

- `execute-command` --- generic `workspace/executeCommand` passthrough. Params:
  `lsp` (LSP name), `command` (command ID), `arguments` (JSON string, optional).
  Use `lux://commands` to discover available commands.

### Key Packages

  ---------------------------------------------------------------------------------
  Package                               Role
  ------------------------------------- -------------------------------------------
  `cmd/lux`                             CLI: `mcp-stdio`, `lsp`, `init`, `add`,
                                        `list`, `fmt`, `hook`

  `internal/server`                     LSP server, handler, and file-type router

  `internal/subprocess`                 LSP process pool, lifecycle state machine
                                        (Idle→Starting→Running→Stopping→Stopped),
                                        Nix executor

  `internal/mcp`                        MCP server, resource provider, document
                                        manager, diagnostics store, prompts

  `internal/tools`                      Bridge (adapts LSP operations to MCP), tool
                                        registry (for hook/artifact generation)

  `internal/config`                     TOML config parsing (`lsps.toml`,
                                        `formatters.toml`), per-project overrides,
                                        config merging

  `internal/formatter`                  External formatter routing and execution
                                        (separate from LSP formatting)

  `internal/capabilities`               Auto-discovery and caching of LSP
                                        capabilities during `lux add`

  `internal/lsp`                        LSP protocol types, capability aggregation,
                                        URI utilities

  `pkg/filematch`                       File matching by extension, glob pattern,
                                        or language ID (priority: languageID \>
                                        extension \> pattern)
  ---------------------------------------------------------------------------------

### Configuration

- User config: `~/.config/lux/lsps.toml` (TOML, `[[lsp]]` entries)
- Formatter config: `~/.config/lux/formatters.toml`
- Cached capabilities: `~/.local/share/lux/capabilities/`
- Per-project overrides load from the project root directory

### LSP Config Fields

Each `[[lsp]]` entry supports: `name`, `flake`, `binary` (optional, for
multi-binary flakes), `extensions`, `patterns`, `language_ids`, `args`, `env`,
`init_options`, `settings`, `settings_key`, and `capabilities` (with
`disable`/`enable` lists). At least one of
`extensions`/`patterns`/`language_ids` is required.

## Key Dependencies

- `github.com/amarbel-llc/purse-first/libs/go-mcp` - MCP protocol library
  (JSON-RPC handler, transport interfaces)
- `github.com/BurntSushi/toml` - Config parsing
- `github.com/gobwas/glob` - Glob pattern matching for file routing
