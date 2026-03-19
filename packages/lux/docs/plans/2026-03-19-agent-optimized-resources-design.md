# Agent-Optimized Resource Extensions for Lux

**Date:** 2026-03-19
**Status:** approved
**Relates to:** amarbel-llc/eng#8 (cleanup), amarbel-llc/eng#9 (tuning levers)

## Problem

Agent workflows using lux resources are bottlenecked by round-trips. Analyzing
the lux codebase via 3 parallel subagents required ~76 tool calls. Key gaps:

- No call graph traversal — agents simulate it with references + hover
- No batch diagnostics — agents open files one at a time
- References return bare locations — agents need N follow-up calls for context
- All resources return human-readable text — agents must parse unstructured output

## Design

### 1. Call Hierarchy Resources

Two new single-level resources. The agent walks the graph by feeding results
back as inputs.

```
lux://lsp/incoming-calls?uri={uri}&line={line}&character={character}
lux://lsp/outgoing-calls?uri={uri}&line={line}&character={character}
```

**Protocol flow** (internal to bridge):
1. `textDocument/prepareCallHierarchy` at position → `CallHierarchyItem`
2. `callHierarchy/incomingCalls` or `outgoingCalls` with that item
3. Return flat list of callers/callees with locations

**Response shape:**
```json
{
  "symbol": {"name": "Route", "kind": "Method", "uri": "file:///...router.go", "line": 37},
  "calls": [
    {"name": "handleDefault", "kind": "Method", "uri": "file:///...handler.go", "line": 181, "character": 29},
    {"name": "TestRouteByURI", "kind": "Function", "uri": "file:///...router_test.go", "line": 98, "character": 16}
  ]
}
```

**Error cases:**
- LSP doesn't support call hierarchy → `"error": "gopls does not support callHierarchy"`
- No callable symbol at position → `"error": "No callable symbol at this position"`

### 2. Batch Diagnostics Resource

```
lux://lsp/diagnostics-batch?glob={glob}
```

**Flow:**
1. Expand glob against working directory
2. Group matched files by extension → LSP name via router
3. For each LSP group: start LSP if needed, open all files, collect push
   diagnostics, close files
4. Aggregate and return

**Response shape:**
```json
{
  "lsps": [
    {
      "name": "gopls",
      "files_scanned": 12,
      "diagnostics": [
        {
          "uri": "file:///...bridge.go",
          "line": 598, "character": 21,
          "severity": "hint",
          "code": "omitzero",
          "source": "gopls",
          "message": "Omitempty has no effect on nested struct fields"
        }
      ]
    },
    {
      "name": "rust-analyzer",
      "files_scanned": 3,
      "diagnostics": []
    }
  ]
}
```

A single glob can fan out to multiple LSPs — `packages/**/*.{go,rs}` routes
`.go` to gopls and `.rs` to rust-analyzer automatically via extension-based
inference.

**Tuning lever — batch open strategy:** Initial implementation opens all files
at once against each LSP (approach A). If this causes memory pressure on large
globs, switch to chunked opening (e.g., 20 files at a time). No cap initially;
let real usage inform whether one is needed.

### 3. Enriched References

Modified existing resource with new default:

```
lux://lsp/references?uri={uri}&line={line}&character={character}&context={context}
```

- `context` defaults to `3` (lines of surrounding source)
- `context=0` restores location-only behavior

**Per-reference enrichment:**
1. Hover at reference position → type/signature
2. Read N surrounding lines from source file

**Response shape:**
```json
{
  "symbol": "Router.Route",
  "count": 5,
  "references": [
    {
      "uri": "file:///...handler.go",
      "line": 181,
      "character": 29,
      "hover": "func (*Router).Route(method string, params json.RawMessage) string",
      "context": {
        "before": [
          "// Route to appropriate LSP",
          "func (h *Handler) handleDefault(ctx context.Context, msg *jsonrpc.Message) (*jsonrpc.Message, error) {"
        ],
        "line": "\tlspName := h.server.router.Route(msg.Method, msg.Params)",
        "after": [
          "\tif lspName == \"\" {",
          "\t\treturn nil, &jsonrpc.Error{Code: -32601, Message: \"no LSP configured\"}"
        ]
      }
    }
  ]
}
```

**Tuning levers:**
- Default context lines: `3` — adjust based on agent tool-use patterns (we
  observe frequent `grep -B`/`-A` calls suggesting agents want surrounding
  context)
- Hover per reference: adds N LSP round-trips. Start always-on; if slow, make
  opt-in via `&hover=false`

### 4. JSON Output for All Resources

All resources migrate from `text/plain` to `application/json`. Controlled by a
`format` parameter:

- `format=json` (default) — structured JSON
- `format=text` — current human-readable text

**Token cost trade-off:** JSON is approximately 30% more tokens than text for
equivalent content (estimated from 2-reference enriched example: ~210 tokens
JSON vs ~160 tokens text). However, JSON eliminates downstream round-trips —
one enriched-references call replaces 15+ follow-up calls. Net token savings
are large. This estimate needs real-world validation.

**Affected resources:**

| Resource | JSON shape |
|----------|-----------|
| hover | `{content: string}` |
| definition | `[{uri, line, character}]` |
| references | `[{uri, line, character, hover?, context?}]` |
| completion | `[{label, detail, kind, documentation}]` |
| format | `[{range, newText}]` |
| document-symbols | `[{name, kind, range, children}]` |
| diagnostics | `[{range, severity, code, source, message}]` |
| code-action | `[{title, kind, edit?, command?}]` |
| rename | `{changes: {uri: [TextEdit]}}` |
| workspace-symbols | `[{name, kind, location}]` |

**Cap removal:** Text format capped completions at 20, diagnostics at 30. JSON
consumers handle their own truncation. Caps removed for JSON; retained for text.
Flagged as tuning lever — may add optional `limit` parameter if responses get
unwieldy.

**Bridge pattern:** Add `*Raw` methods (like existing `DocumentSymbolsRaw`)
that return parsed Go structs without text formatting. Resources serialize
these directly to JSON.

### 5. Rollback Strategy

The `format` parameter is the dual-architecture mechanism. `format=text`
preserves current behavior entirely.

**Promotion criteria for removing text format:**
- 4 weeks with no `format=text` usage in agent sessions
- JSON token overhead confirmed acceptable (< 40% increase) through real-world
  measurement

**Rollback procedure:** Set `format=text` as default — single-line change in
`readLSPResource()`.

## Tuning Levers Summary

| Lever | Default | Signal to adjust |
|-------|---------|-----------------|
| Reference context lines | 3 | Agent tool-use patterns (grep -B/-A frequency) |
| Batch open strategy | All at once | LSP memory pressure on large globs |
| Batch file cap | None | Timeout or OOM on very large repos |
| Completion/diagnostic caps | None (JSON), 20/30 (text) | Response size vs utility |
| Hover per reference | Always on | Latency on high-reference-count symbols |
| JSON vs text default | JSON | Token cost measurement in real agent sessions |
| JSON token overhead | ~30% estimate | Real-world measurement needed |
