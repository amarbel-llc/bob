# Lux LSP Editor Integration Design

## Problem

Lux currently only speaks MCP over stdio (`mcp-stdio`). Editors like neovim
can't use it as an LSP server. The neovim config at
`rcm/config/nvim/lsp/lux.lua` has `cmd = { "lux", "lsp" }` but the subcommand
doesn't exist.

This forces config duplication: formatting config (e.g., pandoc with
`--standalone` for YAML frontmatter preservation) lives in both
`formatters.toml` and editor-specific configs like `conform.lua`.

A previous daemon-based approach (socket activation, SSE/HTTP transports, Unix
sockets, LSP proxy client) was removed in `d460e81` because the architecture was
too complex. This design takes a simpler approach.

## Decision

Add a `lux lsp` subcommand that speaks LSP JSON-RPC over stdio, acting as a thin
proxy to backend LSPs. Two modes, tested in parallel:

- `lux lsp` --- multiplexing mode. Single process, routes requests by filetype
  using existing `internal/server/router.go`.
- `lux lsp --lang <name>` --- single-language mode. Thin proxy to one backend
  LSP.

Phase 1 scope: `textDocument/formatting` and `textDocument/rangeFormatting`
only. All other methods return `MethodNotFound`.

## Architecture

### Shared infrastructure (already exists)

- `internal/subprocess/pool.go` --- LSP lifecycle (start, initialize, shutdown)
- `internal/server/router.go` --- filetype → LSP routing
- `internal/config/` --- `lsps.toml` + filetype configs
- `internal/formatter/` --- formatter routing and execution

### New components

- `internal/lsp/proxy.go` --- JSON-RPC stdio server. Accepts editor requests,
  routes to backends via the subprocess pool, returns responses. Handles
  `initialize`/`initialized`/`shutdown`/`exit` lifecycle.
- `cmd/lux/lsp.go` --- CLI subcommand. Parses `--lang` flag, creates proxy, runs
  stdio loop.

### Capability advertisement

Phase 1 hardcodes `ServerCapabilities` to formatting-only in both modes:

``` json
{
  "documentFormattingProvider": true,
  "documentRangeFormattingProvider": true
}
```

Later phases will dynamically build capabilities from the union of connected
backends (multiplexing mode) or forward backend capabilities directly
(single-lang mode).

### Document tracking

Editor sends `didOpen`/`didClose`. Proxy forwards to the appropriate backend LSP
using the existing `DocumentManager` logic. In `--lang` mode, all documents go
to the one backend. In multiplexing mode, the router determines which backend
based on file extension/language ID.

## Integration testing

Tests run against real editors speaking real LSP protocol, using BATS.

### Editors

- **neovim** (`nvim --headless`) --- primary target. LSP client configured
  programmatically via `-l script.lua`. Added as a nix devShell dependency.
- vim and helix considered for later phases.

### Test matrix (Phase 1)

  Test                                         Multiplexing   Single-lang
  -------------------------------------------- -------------- -------------
  Format `.go` file via neovim                 x              x
  Format `.md` file via neovim                 x              x
  Format `.go` then `.md` in same session      x              n/a
  Backend LSP not available (graceful error)   x              x

### Test fixtures

Test files with known-bad formatting in a fixture directory. After formatting,
assert file contents match expected output.

### Dependencies in test devShell

neovim, gopls, pandoc, and any other LSPs/formatters needed for test filetypes.

## Rollback

- **Multiplexing doesn't work:** `--lang` mode is the fallback.
- **Whole feature is bad:** remove `lux lsp` subcommand. Editors fall back to
  native LSP configs, which remain functional throughout the dual-architecture
  period.
- No existing behavior changes --- purely additive.

## Phased rollout

Tracked in FDR `docs/features/0001-lsp-editor-integration.md`:

1.  Formatting (`textDocument/formatting`, `textDocument/rangeFormatting`)
2.  Diagnostics (`textDocument/publishDiagnostics`, pull diagnostics)
3.  Navigation (hover, definition, references, completion)
4.  Workspace (workspace/symbol, rename, didChangeConfiguration)
5.  Full proxy (forward any method, dynamic capability advertisement)

Promotion between phases requires integration tests passing, 7 days real usage
without fallback, and no MCP regressions.
