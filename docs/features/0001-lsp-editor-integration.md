---
date: 2026-03-21
promotion-criteria: |
  Phase 1 (exploring → proposed): design validated, implementation plan written.
  Phase 1 (proposed → experimental): formatting works in neovim headless tests.
  Phase 1 (experimental → testing): 7 days real neovim usage without fallback. B
  vs C decision: ADR written after Phase 1 testing with real usage data.
status: experimental
---

# LSP Editor Integration

## Problem Statement

Lux centralizes LSP and formatter configuration in `lsps.toml`,
`formatters.toml`, and filetype configs, but editors can't use it --- lux only
speaks MCP over stdio. This forces duplication: formatting config (e.g., pandoc
with `--standalone` for YAML frontmatter) lives in both lux config and
editor-specific configs like `conform.lua`. Adding a new language or formatter
means updating two places.

## Interface

`lux lsp` subcommand that speaks LSP JSON-RPC over stdio.

**Multiplexing mode** (single process handles all filetypes):

    cmd = { "lux", "lsp" }

**Single-language mode** (one backend LSP):

    cmd = { "lux", "lsp", "--lang", "go" }

### Phase 1: Formatting

Supported methods:

- `textDocument/formatting`
- `textDocument/rangeFormatting`
- `initialize` / `initialized` / `shutdown` / `exit` (lifecycle)
- `textDocument/didOpen` / `textDocument/didClose` (document sync)

All other methods return LSP `MethodNotFound` (`-32601`).

### Phase 2: Diagnostics

- `textDocument/publishDiagnostics` (server → client push)
- `textDocument/diagnostic` (client pull model)

### Phase 3: Navigation

- `textDocument/hover`
- `textDocument/definition`
- `textDocument/references`
- `textDocument/completion`

### Phase 4: Workspace

- `workspace/symbol`
- `textDocument/rename`
- `workspace/didChangeConfiguration`

### Phase 5: Full proxy

Forward any method the backend supports. Dynamic capability advertisement from
the union of connected backends.

## Examples

Editor config (neovim):

``` lua
-- Single config for all lux-managed languages
vim.lsp.config("lux", {
  cmd = { "lux", "lsp" },
})
vim.lsp.enable("lux")
```

Or per-language:

``` lua
vim.lsp.config("lux-go", {
  cmd = { "lux", "lsp", "--lang", "go" },
  filetypes = { "go" },
})
vim.lsp.enable("lux-go")
```

Format a file:

    nvim --headless -c "edit test.go" -c "lua vim.lsp.buf.format()" -c "write" -c "quit"

## Limitations

- Phase 1 supports formatting only. Other LSP methods return `MethodNotFound`.
- Multiplexing mode advertises the union of backend capabilities, which means
  some methods may not be supported for all filetypes. Errors are returned
  per-request when a backend doesn't support a method.
- Workspace-scoped features (workspace/symbol, rename across filetypes) are
  deferred to Phase 4.
- A previous daemon-based approach was removed (`d460e81`). This design
  intentionally avoids sockets, SSE, HTTP transports, and service management.

## Testing

### Current tests (`zz-tests_bats/lux_lsp.bats`)

Protocol-level:

1.  Initialize response has formatting capabilities and serverInfo
2.  Phase 1 advertises only formatting providers (no hover/definition)
3.  Go formatting end-to-end via gopls (didOpen → formatting → apply edits)
4.  Non-formatting methods return MethodNotFound (-32601)

Neovim integration:

5.  Format Go file through neovim headless (full editor → lux → gopls pipeline)
6.  Client attaches to Go buffer with formatting capability
7.  Client does NOT attach to non-matching filetype (.txt)
8.  Clean shutdown via LspStop

### Future tests (add as phases are implemented)

- **Capability detection timing** --- verify `documentFormattingProvider`
  becomes true within a bounded window, not just eventually. Catches regressions
  in initialization ordering.
- **Multiplexing: two filetypes in one session** --- open `.go` and `.nix` files
  in the same neovim session, format both. Validates routing when multiple
  backends are active simultaneously.
- **No-op format on already-formatted file** --- verify formatting returns empty
  edits (not an error) when the file is already correct.
- **Diagnostic forwarding** (Phase 2) --- verify `publishDiagnostics` from gopls
  propagates through lux to neovim and appears in the quickfix list.
- **Hover content** (Phase 3) --- verify `textDocument/hover` returns type info
  from gopls through lux.
- **Backend crash resilience** --- kill a backend LSP mid-session, verify lux
  stays alive and reports errors gracefully rather than crashing.

## More Information

- Design doc: `docs/plans/2026-03-21-lux-lsp-editor-integration-design.md`
- GitHub issue: amarbel-llc/bob#21
- Related: amarbel-llc/bob#22 (migrate mutations to tools, prerequisite for
  clean LSP behavior)
- Related: amarbel-llc/eng#11 (switch neovim to lux, blocked on this feature)
- Removed daemon approach: commit `d460e81`
