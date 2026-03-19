# Agent-Optimized Resource Extensions — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add call hierarchy, batch diagnostics, enriched references, and JSON output to all lux MCP resources.

**Architecture:** Add `*Raw` bridge methods returning Go structs. Resources serialize to JSON by default, text via `format=text`. New resources follow the existing `readLSPResource()` dispatch pattern. Batch diagnostics uses `gobwas/glob` (already a dependency) for file matching.

**Tech Stack:** Go, LSP protocol (call hierarchy extension), gobwas/glob, go-mcp resource templates.

**Rollback:** `format=text` query parameter on any resource restores current behavior. New resources can be unregistered without affecting existing ones.

---

### Task 1: Add JSON output infrastructure to bridge

Add `*Raw` methods to bridge that return parsed Go structs instead of formatted
text. Follow the `DocumentSymbolsRaw` pattern at `bridge.go:369-378`.

**Files:**
- Modify: `internal/tools/bridge.go:190-455` (add Raw variants)
- Test: `internal/tools/bridge_test.go`

**Step 1: Write failing tests for HoverRaw**

```go
func TestHoverRaw_ReturnsStruct(t *testing.T) {
	// Verify HoverRaw returns a HoverResult struct, not *command.Result
	// This is a compile-time check more than runtime — the method must exist
	var b *Bridge
	_ = (func(context.Context, lsp.DocumentURI, int, int) (*HoverResult, error))(b.HoverRaw)
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestHoverRaw ./internal/tools/`
Expected: FAIL — `HoverRaw` undefined, `HoverResult` undefined

**Step 3: Define JSON response structs and Raw methods**

Add to `bridge.go` after the existing structs:

```go
type HoverResult struct {
	Content string `json:"content"`
}

type LocationResult struct {
	URI       string `json:"uri"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}
```

Add Raw methods following the DocumentSymbolsRaw pattern. Each one calls
`withDocument` and returns the parsed struct:

- `HoverRaw(ctx, uri, line, char) (*HoverResult, error)`
- `DefinitionRaw(ctx, uri, line, char) ([]LocationResult, error)`
- `ReferencesRaw(ctx, uri, line, char, includeDecl) ([]LocationResult, error)`
- `CompletionRaw(ctx, uri, line, char) ([]CompletionItem, error)` (CompletionItem already exists at bridge.go)
- `DiagnosticsRaw(ctx, uri) ([]DiagnosticItem, error)` (DiagnosticItem already exists)
- `CodeActionRaw(ctx, uri, startLine, startChar, endLine, endChar) ([]CodeAction, error)` (CodeAction already exists)
- `RenameRaw(ctx, uri, line, char, newName) (*WorkspaceEdit, error)` (WorkspaceEdit already exists)
- `WorkspaceSymbolsRaw(ctx, uri, query) ([]WorkspaceSymbol, error)` (WorkspaceSymbol already exists)
- `FormatRaw(ctx, uri) (json.RawMessage, error)` (return raw TextEdits)

**Step 4: Run tests to verify they pass**

Run: `nix develop --command go test -run TestHoverRaw ./internal/tools/`
Expected: PASS

**Step 5: Commit**

```
feat(lux): add Raw bridge methods returning Go structs for JSON serialization
```

---

### Task 2: Add format parameter to resource dispatch

Add `format` query parameter support to `readLSPResource()` at
`resources.go:67`. Default to `json`, fall back to `text` for existing behavior.

**Files:**
- Modify: `internal/mcp/resources.go:67-236`
- Create: `internal/mcp/resources_test.go`

**Step 1: Write failing test**

```go
func TestReadLSPResource_FormatParam(t *testing.T) {
	// format=json should return application/json MIME type
	// format=text should return text/plain MIME type
	// default (no format) should return application/json
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestReadLSPResource_FormatParam ./internal/mcp/`
Expected: FAIL

**Step 3: Implement format dispatch**

In `readLSPResource()`, after parsing the URL at line 71:

```go
outputFormat := q.Get("format")
if outputFormat == "" {
	outputFormat = "json"
}
```

For each case in the switch block, branch on `outputFormat`:
- `"json"`: call `bridge.*Raw()` method, `json.Marshal` the result, return with
  `mimeType: "application/json"`
- `"text"`: call existing `bridge.*()` method, return with `mimeType: "text/plain"`
  (current behavior)

**Step 4: Run tests**

Run: `nix develop --command go test ./internal/mcp/`
Expected: PASS

**Step 5: Commit**

```
feat(lux): add format=json|text parameter to all LSP resources

Defaults to JSON. format=text preserves current human-readable output.
```

---

### Task 3: Add LSP call hierarchy protocol support

Add call hierarchy constants, capability declaration, and bridge methods.
The LSP call hierarchy is a 2-step protocol: prepare → incoming/outgoing.

**Files:**
- Modify: `internal/lsp/protocol.go` (add method constants)
- Modify: `internal/tools/bridge.go` (add call hierarchy methods)
- Modify: `internal/tools/bridge.go:477-517` (add capability to DefaultInitParams)
- Test: `internal/tools/bridge_test.go`

**Step 1: Write failing test**

```go
func TestIncomingCallsRaw_MethodExists(t *testing.T) {
	var b *Bridge
	_ = (func(context.Context, lsp.DocumentURI, int, int) (*CallHierarchyResult, error))(b.IncomingCallsRaw)
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestIncomingCallsRaw ./internal/tools/`
Expected: FAIL — `IncomingCallsRaw` undefined

**Step 3: Add protocol constants**

In `internal/lsp/protocol.go`, add:

```go
const (
	MethodTextDocumentPrepareCallHierarchy = "textDocument/prepareCallHierarchy"
	MethodCallHierarchyIncomingCalls       = "callHierarchy/incomingCalls"
	MethodCallHierarchyOutgoingCalls       = "callHierarchy/outgoingCalls"
)
```

**Step 4: Add call hierarchy types and bridge methods**

In `bridge.go`, add types:

```go
type CallHierarchyItem struct {
	Name   string          `json:"name"`
	Kind   int             `json:"kind"`
	URI    string          `json:"uri"`
	Range  json.RawMessage `json:"range"`
	Detail string          `json:"detail,omitempty"`
}

type CallHierarchyCall struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	URI       string `json:"uri"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

type CallHierarchyResult struct {
	Symbol CallHierarchyCall   `json:"symbol"`
	Calls  []CallHierarchyCall `json:"calls"`
}
```

Add `IncomingCallsRaw` method:

```go
func (b *Bridge) IncomingCallsRaw(ctx context.Context, uri lsp.DocumentURI, line, character int) (*CallHierarchyResult, error) {
	// Step 1: prepareCallHierarchy
	prepareResult, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentPrepareCallHierarchy, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"position":     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return nil, err
	}

	var items []CallHierarchyItem
	if err := json.Unmarshal(prepareResult, &items); err != nil || len(items) == 0 {
		return nil, fmt.Errorf("no callable symbol at this position")
	}

	// Step 2: incomingCalls with first item
	incomingResult, err := b.callOnRunningLSP(ctx, uri, lsp.MethodCallHierarchyIncomingCalls, map[string]any{
		"item": items[0],
	})
	if err != nil {
		return nil, err
	}

	// Parse and format result
	return parseIncomingCalls(items[0], incomingResult), nil
}
```

Add `OutgoingCallsRaw` following the same pattern with `MethodCallHierarchyOutgoingCalls`.

Note: The second call (`incomingCalls`/`outgoingCalls`) must go to the same LSP
instance that handled `prepareCallHierarchy`, without re-opening the document.
Add a `callOnRunningLSP` helper that calls an already-started LSP by name
without the document open/close lifecycle.

**Step 5: Add callHierarchy to DefaultInitParams**

In `DefaultInitParams()` at line 491, add to the capabilities struct:

```go
"callHierarchyProvider": true,
```

**Step 6: Run tests**

Run: `nix develop --command go test ./internal/tools/`
Expected: PASS

**Step 7: Commit**

```
feat(lux): add call hierarchy bridge methods and LSP protocol support
```

---

### Task 4: Register call hierarchy resources

Wire `IncomingCallsRaw` and `OutgoingCallsRaw` into the MCP resource layer.

**Files:**
- Modify: `internal/mcp/resources.go:292-378` (register templates)
- Modify: `internal/mcp/resources.go:105-221` (add switch cases)

**Step 1: Write failing test**

```go
func TestReadResource_IncomingCalls(t *testing.T) {
	// Verify lux://lsp/incoming-calls URI is recognized and dispatched
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestReadResource_IncomingCalls ./internal/mcp/`
Expected: FAIL

**Step 3: Register templates**

In `registerResources()` at line 292, add two new templates:

```go
registry.RegisterTemplate(
	protocol.ResourceTemplate{
		URITemplate: "lux://lsp/incoming-calls?uri={uri}&line={line}&character={character}",
		Name:        "Incoming Calls",
		Description: "Find all callers of a function at a position. Returns one level; walk the graph by passing results back.",
		MimeType:    "application/json",
	},
	nil,
)

registry.RegisterTemplate(
	protocol.ResourceTemplate{
		URITemplate: "lux://lsp/outgoing-calls?uri={uri}&line={line}&character={character}",
		Name:        "Outgoing Calls",
		Description: "Find all functions called by the function at a position. Returns one level; walk the graph by passing results back.",
		MimeType:    "application/json",
	},
	nil,
)
```

**Step 4: Add switch cases**

In `readLSPResource()` switch block, add:

```go
case "incoming-calls":
	fileURI, line, char, err := getPosition()
	if err != nil {
		return nil, err
	}
	raw, err := p.bridge.IncomingCallsRaw(ctx, fileURI, line, char)
	if err != nil {
		return nil, err
	}
	data, _ := json.MarshalIndent(raw, "", "  ")
	result = string(data)
	mimeType = "application/json"

case "outgoing-calls":
	fileURI, line, char, err := getPosition()
	if err != nil {
		return nil, err
	}
	raw, err := p.bridge.OutgoingCallsRaw(ctx, fileURI, line, char)
	if err != nil {
		return nil, err
	}
	data, _ := json.MarshalIndent(raw, "", "  ")
	result = string(data)
	mimeType = "application/json"
```

**Step 5: Run tests**

Run: `nix develop --command go test ./internal/mcp/`
Expected: PASS

**Step 6: Commit**

```
feat(lux): register incoming-calls and outgoing-calls MCP resources
```

---

### Task 5: Implement enriched references

Modify `ReferencesRaw` to accept a `contextLines` parameter. When > 0, enrich
each reference with hover info and surrounding source lines.

**Files:**
- Modify: `internal/tools/bridge.go` (extend ReferencesRaw)
- Modify: `internal/mcp/resources.go` (parse context param, pass to bridge)

**Step 1: Write failing test**

```go
func TestEnrichedReference_HasContextFields(t *testing.T) {
	// Verify EnrichedLocation struct has Hover and Context fields
	loc := EnrichedLocation{
		URI: "file:///test.go", Line: 10, Character: 5,
		Hover: "func Foo()",
		Context: &SourceContext{
			Before: []string{"line 1", "line 2"},
			Line:   "call Foo()",
			After:  []string{"line 3", "line 4"},
		},
	}
	data, _ := json.Marshal(loc)
	if !strings.Contains(string(data), `"hover"`) {
		t.Error("expected hover field in JSON")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestEnrichedReference ./internal/tools/`
Expected: FAIL — `EnrichedLocation`, `SourceContext` undefined

**Step 3: Add enrichment types**

```go
type SourceContext struct {
	Before []string `json:"before"`
	Line   string   `json:"line"`
	After  []string `json:"after"`
}

type EnrichedLocation struct {
	URI       string         `json:"uri"`
	Line      int            `json:"line"`
	Character int            `json:"character"`
	Hover     string         `json:"hover,omitempty"`
	Context   *SourceContext `json:"context,omitempty"`
}

type EnrichedReferencesResult struct {
	Symbol string             `json:"symbol"`
	Count  int                `json:"count"`
	Refs   []EnrichedLocation `json:"references"`
}
```

**Step 4: Implement enrichment in ReferencesRaw**

Modify `ReferencesRaw` to accept `contextLines int`. When > 0, for each
reference location:

1. Call `HoverRaw` at that position (reuses the same running LSP)
2. Call `readFile` on the URI and extract surrounding lines
3. Populate `EnrichedLocation.Hover` and `EnrichedLocation.Context`

When `contextLines == 0`, return `EnrichedLocation` with only uri/line/character
(no hover, no context).

**Step 5: Wire context parameter in resources.go**

In the `"references"` case of `readLSPResource()`, parse the context parameter:

```go
case "references":
	fileURI, line, char, err := getPosition()
	if err != nil {
		return nil, err
	}
	includeDecl := q.Get("include_declaration") != "false"
	contextLines := 3 // default
	if v := q.Get("context"); v != "" {
		contextLines, _ = strconv.Atoi(v)
	}
```

**Step 6: Run tests**

Run: `nix develop --command go test ./internal/tools/ ./internal/mcp/`
Expected: PASS

**Step 7: Commit**

```
feat(lux): enrich references with hover and context lines (default 3)
```

---

### Task 6: Implement batch diagnostics

New resource that expands a glob, groups files by LSP, opens them all, collects
diagnostics.

**Files:**
- Modify: `internal/mcp/resources.go` (register template, add read handler)
- Modify: `internal/tools/bridge.go` (add BatchDiagnostics method)

**Step 1: Write failing test**

```go
func TestBatchDiagnosticsResult_JSONShape(t *testing.T) {
	result := BatchDiagnosticsResult{
		LSPs: []LSPDiagnosticGroup{
			{Name: "gopls", FilesScanned: 5, Diagnostics: []DiagnosticItem{}},
		},
	}
	data, _ := json.Marshal(result)
	if !strings.Contains(string(data), `"files_scanned"`) {
		t.Error("expected files_scanned field")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test -run TestBatchDiagnosticsResult ./internal/tools/`
Expected: FAIL — types undefined

**Step 3: Add batch diagnostics types**

```go
type LSPDiagnosticGroup struct {
	Name         string           `json:"name"`
	FilesScanned int              `json:"files_scanned"`
	Diagnostics  []DiagnosticItem `json:"diagnostics"`
}

type BatchDiagnosticsResult struct {
	LSPs []LSPDiagnosticGroup `json:"lsps"`
}
```

**Step 4: Implement BatchDiagnostics bridge method**

```go
func (b *Bridge) BatchDiagnostics(ctx context.Context, pattern string) (*BatchDiagnosticsResult, error) {
	// 1. Expand glob using gobwas/glob against working directory files
	// 2. Group matched files by extension → LSP name via router.RouteByExtension
	// 3. For each LSP group:
	//    a. GetOrStart the LSP
	//    b. Open all files (textDocument/didOpen)
	//    c. Wait for diagnostics (subscribe to publishDiagnostics notifications)
	//    d. Close all files
	// 4. Aggregate into BatchDiagnosticsResult
}
```

Use `filepath.WalkDir` + `gobwas/glob` (already in go.mod) for file matching.
Use `router.RouteByExtension` to determine LSP per file.

**Step 5: Register resource template**

```go
registry.RegisterTemplate(
	protocol.ResourceTemplate{
		URITemplate: "lux://lsp/diagnostics-batch?glob={glob}",
		Name:        "Batch Diagnostics",
		Description: "Run diagnostics on all files matching a glob pattern. Groups by extension and fans out to multiple LSPs automatically.",
		MimeType:    "application/json",
	},
	nil,
)
```

**Step 6: Add dispatch case**

In `ReadResource()` at line 45, add a new prefix check before the `lux://lsp/`
match, or add `"diagnostics-batch"` to the `readLSPResource()` switch.

```go
case "diagnostics-batch":
	pattern := q.Get("glob")
	if pattern == "" {
		return nil, fmt.Errorf("missing required parameter 'glob'")
	}
	raw, err := p.bridge.BatchDiagnostics(ctx, pattern)
	if err != nil {
		return nil, err
	}
	data, _ := json.MarshalIndent(raw, "", "  ")
	result = string(data)
	mimeType = "application/json"
```

**Step 7: Run tests**

Run: `nix develop --command go test ./internal/tools/ ./internal/mcp/`
Expected: PASS

**Step 8: Commit**

```
feat(lux): add batch diagnostics resource with glob-based multi-LSP fan-out
```

---

### Task 7: Integration test with real LSPs

Verify all new resources work against real gopls and nil.

**Files:**
- Create: `zz-tests_bats/lux_resources.bats` (in bob repo root)

**Step 1: Write BATS test**

```bash
@test "lux incoming-calls returns JSON for Go function" {
  run lux resource-read "lux://lsp/incoming-calls?uri=file://${PWD}/packages/lux/internal/server/router.go&line=36&character=18"
  [ "$status" -eq 0 ]
  echo "$output" | jq '.symbol.name' | grep -q "Route"
}

@test "lux diagnostics-batch returns results for Go files" {
  run lux resource-read "lux://lsp/diagnostics-batch?glob=packages/lux/internal/tools/*.go"
  [ "$status" -eq 0 ]
  echo "$output" | jq '.lsps[0].name' | grep -q "gopls"
}

@test "lux enriched references include context" {
  run lux resource-read "lux://lsp/references?uri=file://${PWD}/packages/lux/internal/server/router.go&line=36&character=18&context=3"
  [ "$status" -eq 0 ]
  echo "$output" | jq '.references[0].context.line' | grep -q "Route"
}

@test "lux hover returns JSON by default" {
  run lux resource-read "lux://lsp/hover?uri=file://${PWD}/packages/lux/internal/server/router.go&line=81&character=24"
  [ "$status" -eq 0 ]
  echo "$output" | jq '.content' | grep -q "Match"
}

@test "lux hover returns text with format=text" {
  run lux resource-read "lux://lsp/hover?uri=file://${PWD}/packages/lux/internal/server/router.go&line=81&character=24&format=text"
  [ "$status" -eq 0 ]
  # Should NOT be valid JSON object
  ! echo "$output" | jq '.' 2>/dev/null || echo "$output" | grep -q '```'
}
```

**Step 2: Run integration tests**

Run: `nix develop --command bats --tap zz-tests_bats/lux_resources.bats`
Expected: All 5 tests pass

**Step 3: Commit**

```
test(lux): add BATS integration tests for agent-optimized resources
```

---

### Task 8: Update CLAUDE.md and resource template descriptions

Update lux's CLAUDE.md to document the new resources and JSON default.

**Files:**
- Modify: `packages/lux/CLAUDE.md` (MCP Resources section)

**Step 1: Update the MCP Resources section**

Add new resource URIs to the list:
- `lux://lsp/incoming-calls?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/outgoing-calls?uri={file_uri}&line={line}&character={character}`
- `lux://lsp/diagnostics-batch?glob={pattern}`

Note that references now defaults to `context=3`.
Note that all resources default to `format=json`.

**Step 2: Commit**

```
docs(lux): update CLAUDE.md with new agent-optimized resources
```
