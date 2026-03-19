package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amarbel-llc/purse-first/libs/go-mcp/command"
	"github.com/amarbel-llc/purse-first/libs/go-mcp/jsonrpc"
	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/formatter"
	"github.com/amarbel-llc/lux/internal/logfile"
	"github.com/amarbel-llc/lux/internal/lsp"
	"github.com/amarbel-llc/lux/internal/server"
	"github.com/amarbel-llc/lux/internal/subprocess"
	"github.com/gobwas/glob"
)

// DocumentTracker tracks open documents for persistent LSP sessions.
type DocumentTracker interface {
	IsOpen(uri lsp.DocumentURI) bool
	Open(ctx context.Context, uri lsp.DocumentURI) error
}

type Bridge struct {
	pool             *subprocess.Pool
	router           *server.Router
	fmtRouter        *formatter.Router
	executor         subprocess.Executor
	docMgr           DocumentTracker
	progressReporter func(lspName, message string)
}

func NewBridge(pool *subprocess.Pool, router *server.Router, fmtRouter *formatter.Router, executor subprocess.Executor, progressReporter func(lspName, message string)) *Bridge {
	return &Bridge{
		pool:             pool,
		router:           router,
		fmtRouter:        fmtRouter,
		executor:         executor,
		progressReporter: progressReporter,
	}
}

func (b *Bridge) SetDocumentManager(dm DocumentTracker) {
	b.docMgr = dm
}

func (b *Bridge) waitForLSPReady(ctx context.Context, inst *subprocess.LSPInstance) error {
	if !inst.WaitForReady || inst.Progress == nil || inst.Progress.IsReady() {
		return nil
	}

	fmt.Fprintf(logfile.Writer(), "[lux] %s: waiting for LSP to finish indexing...\n", inst.Name)

	done := make(chan error, 1)
	go func() {
		done <- inst.Progress.WaitForReady(ctx, inst.ActivityTimeout, inst.ReadyTimeout, inst.IsFailed)
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if err == nil {
				fmt.Fprintf(logfile.Writer(), "[lux] %s: LSP ready\n", inst.Name)
			}
			return err
		case <-ticker.C:
			active := inst.Progress.ActiveProgress()
			for _, tok := range active {
				logMsg := tok.Title
				if tok.Message != "" {
					logMsg += ": " + tok.Message
				}
				if tok.Pct != nil {
					logMsg += fmt.Sprintf(" (%d%%)", *tok.Pct)
				}
				fmt.Fprintf(logfile.Writer(), "[lux] %s: %s\n", inst.Name, logMsg)
				if b.progressReporter != nil {
					b.progressReporter(inst.Name, logMsg)
				}
			}
		}
	}
}

func isRetryableLSPError(err error) bool {
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) {
		return rpcErr.Code == 0 && strings.Contains(rpcErr.Message, "no views")
	}
	return false
}

func (b *Bridge) callWithRetry(ctx context.Context, fn func() (json.RawMessage, error)) (json.RawMessage, error) {
	const maxAttempts = 8
	delay := 500 * time.Millisecond

	for attempt := 1; ; attempt++ {
		result, err := fn()
		if err == nil || !isRetryableLSPError(err) || attempt >= maxAttempts {
			return result, err
		}

		fmt.Fprintf(logfile.Writer(), "[lux] retrying LSP call (attempt %d/%d, waiting %v): %v\n", attempt, maxAttempts, delay, err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
	}
}

func (b *Bridge) withDocument(ctx context.Context, uri lsp.DocumentURI, method string, params any) (json.RawMessage, error) {
	lspName := b.router.RouteByURI(uri)
	if lspName == "" {
		return nil, fmt.Errorf("no LSP configured for %s", uri)
	}

	initParams := b.DefaultInitParams(uri)
	inst, err := b.pool.GetOrStart(ctx, lspName, initParams)
	if err != nil {
		return nil, fmt.Errorf("starting LSP %s: %w", lspName, err)
	}

	// Wait for LSP to finish indexing before making calls
	if err := b.waitForLSPReady(ctx, inst); err != nil {
		return nil, fmt.Errorf("waiting for LSP %s readiness: %w", lspName, err)
	}

	projectRoot := b.ProjectRootForPath(uri.Path())
	if err := inst.EnsureWorkspaceFolder(projectRoot); err != nil {
		return nil, fmt.Errorf("adding workspace folder: %w", err)
	}

	// Use DocumentManager for persistent tracking if available
	if b.docMgr != nil {
		if !b.docMgr.IsOpen(uri) {
			if err := b.docMgr.Open(ctx, uri); err != nil {
				return nil, fmt.Errorf("opening document: %w", err)
			}
		}
		return b.callWithRetry(ctx, func() (json.RawMessage, error) {
			return inst.Call(ctx, method, params)
		})
	}

	// Fallback: ephemeral open/close when no DocumentManager
	content, err := b.readFile(uri)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	langID := b.InferLanguageID(uri)

	if err := inst.Notify(lsp.MethodTextDocumentDidOpen, lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{
			URI:        uri,
			LanguageID: langID,
			Version:    1,
			Text:       content,
		},
	}); err != nil {
		return nil, fmt.Errorf("opening document: %w", err)
	}

	defer func() {
		inst.Notify(lsp.MethodTextDocumentDidClose, lsp.DidCloseTextDocumentParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		})
	}()

	return b.callWithRetry(ctx, func() (json.RawMessage, error) {
		return inst.Call(ctx, method, params)
	})
}

func (b *Bridge) Hover(ctx context.Context, uri lsp.DocumentURI, line, character int) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentHover, lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	if result == nil || string(result) == "null" {
		return command.TextResult("No hover information available"), nil
	}

	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return command.TextErrorResult(fmt.Sprintf("parsing hover result: %v", err)), nil
	}

	text := extractMarkdownContent(hover.Contents)
	return command.TextResult(text), nil
}

func (b *Bridge) Definition(ctx context.Context, uri lsp.DocumentURI, line, character int) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentDefinition, lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	locations := parseLocations(result)
	if len(locations) == 0 {
		return command.TextResult("No definition found"), nil
	}

	text := formatLocations(locations)
	return command.TextResult(text), nil
}

func (b *Bridge) References(ctx context.Context, uri lsp.DocumentURI, line, character int, includeDecl bool) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentReferences, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"position":     lsp.Position{Line: line, Character: character},
		"context":      map[string]any{"includeDeclaration": includeDecl},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	locations := parseLocations(result)
	if len(locations) == 0 {
		return command.TextResult("No references found"), nil
	}

	text := formatLocations(locations)
	return command.TextResult(text), nil
}

func (b *Bridge) Completion(ctx context.Context, uri lsp.DocumentURI, line, character int) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentCompletion, lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	items := parseCompletionItems(result)
	if len(items) == 0 {
		return command.TextResult("No completions available"), nil
	}

	text := formatCompletionItems(items)
	return command.TextResult(text), nil
}

func (b *Bridge) Format(ctx context.Context, uri lsp.DocumentURI) (*command.Result, error) {
	if result, handled := b.tryExternalFormat(ctx, uri); handled {
		return result, nil
	}

	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentFormatting, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"options": map[string]any{
			"tabSize":      4,
			"insertSpaces": true,
		},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	var edits []lsp.TextEdit
	if err := json.Unmarshal(result, &edits); err != nil {
		return command.TextErrorResult(fmt.Sprintf("parsing edits: %v", err)), nil
	}

	if len(edits) == 0 {
		return command.TextResult("No formatting changes needed"), nil
	}

	text := formatTextEdits(edits)
	return command.TextResult(text), nil
}

func (b *Bridge) tryExternalFormat(ctx context.Context, uri lsp.DocumentURI) (*command.Result, bool) {
	if b.fmtRouter == nil {
		return nil, false
	}

	filePath := uri.Path()
	match := b.fmtRouter.Match(filePath)
	if match == nil {
		return nil, false
	}

	if match.LSPFormat == "prefer" {
		return nil, false
	}

	content, err := b.readFile(uri)
	if err != nil {
		return command.TextErrorResult(fmt.Sprintf("reading file: %v", err)), true
	}

	var result *formatter.Result
	switch match.Mode {
	case "chain":
		result, err = formatter.FormatChain(ctx, match.Formatters, filePath, []byte(content), b.executor)
	case "fallback":
		result, err = formatter.FormatFallback(ctx, match.Formatters, filePath, []byte(content), b.executor)
	default:
		return command.TextErrorResult(fmt.Sprintf("unknown formatter mode: %s", match.Mode)), true
	}

	if err != nil {
		if match.LSPFormat == "fallback" {
			return nil, false
		}
		return command.TextErrorResult(fmt.Sprintf("formatter failed: %v", err)), true
	}

	if !result.Changed {
		return command.TextResult("No formatting changes needed"), true
	}

	lines := strings.Count(content, "\n")
	edit := lsp.TextEdit{
		Range: lsp.Range{
			Start: lsp.Position{Line: 0, Character: 0},
			End:   lsp.Position{Line: lines + 1, Character: 0},
		},
		NewText: result.Formatted,
	}

	text := formatTextEdits([]lsp.TextEdit{edit})
	return command.TextResult(text), true
}

func (b *Bridge) DocumentSymbols(ctx context.Context, uri lsp.DocumentURI) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentDocumentSymbol, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	symbols := parseSymbols(result)
	if len(symbols) == 0 {
		return command.TextResult("No symbols found"), nil
	}

	text := formatSymbols(symbols, 0)
	return command.TextResult(text), nil
}

func (b *Bridge) DocumentSymbolsRaw(ctx context.Context, uri lsp.DocumentURI) ([]Symbol, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentDocumentSymbol, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		return nil, err
	}

	return parseSymbols(result), nil
}

func (b *Bridge) HoverRaw(ctx context.Context, uri lsp.DocumentURI, line, character int) (*HoverResult, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentHover, lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return nil, err
	}

	if result == nil || string(result) == "null" {
		return nil, nil
	}

	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return nil, fmt.Errorf("parsing hover result: %w", err)
	}

	return &HoverResult{Content: extractMarkdownContent(hover.Contents)}, nil
}

func locationsToLocationResults(locs []lsp.Location) []LocationResult {
	results := make([]LocationResult, len(locs))
	for i, loc := range locs {
		results[i] = LocationResult{
			URI:       string(loc.URI),
			Line:      loc.Range.Start.Line,
			Character: loc.Range.Start.Character,
		}
	}
	return results
}

func (b *Bridge) DefinitionRaw(ctx context.Context, uri lsp.DocumentURI, line, character int) ([]LocationResult, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentDefinition, lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return nil, err
	}

	locations := parseLocations(result)
	return locationsToLocationResults(locations), nil
}

// SourceContext holds surrounding source lines for an enriched reference location.
type SourceContext struct {
	Before []string `json:"before"`
	Line   string   `json:"line"`
	After  []string `json:"after"`
}

// EnrichedLocation is a reference location optionally enriched with hover info and source context.
type EnrichedLocation struct {
	URI       string         `json:"uri"`
	Line      int            `json:"line"`
	Character int            `json:"character"`
	Hover     string         `json:"hover,omitempty"`
	Context   *SourceContext `json:"context,omitempty"`
}

// EnrichedReferencesResult wraps enriched reference locations with the queried symbol info.
type EnrichedReferencesResult struct {
	Symbol string             `json:"symbol"`
	Count  int                `json:"count"`
	Refs   []EnrichedLocation `json:"references"`
}

func extractSourceContext(content string, line, contextLines int) *SourceContext {
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return nil
	}

	startBefore := line - contextLines
	if startBefore < 0 {
		startBefore = 0
	}
	endAfter := line + contextLines + 1
	if endAfter > len(lines) {
		endAfter = len(lines)
	}

	return &SourceContext{
		Before: lines[startBefore:line],
		Line:   lines[line],
		After:  lines[line+1 : endAfter],
	}
}

func (b *Bridge) ReferencesRaw(ctx context.Context, uri lsp.DocumentURI, line, character int, includeDecl bool, contextLines int) (*EnrichedReferencesResult, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentReferences, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"position":     lsp.Position{Line: line, Character: character},
		"context":      map[string]any{"includeDeclaration": includeDecl},
	})
	if err != nil {
		return nil, err
	}

	locations := parseLocations(result)

	// Get symbol name from hover at the queried position
	var symbol string
	if hover, err := b.HoverRaw(ctx, uri, line, character); err == nil && hover != nil {
		symbol = hover.Content
	}

	refs := make([]EnrichedLocation, len(locations))
	// Cache file contents to avoid re-reading the same file for multiple refs
	fileCache := make(map[string]string)

	for i, loc := range locations {
		refs[i] = EnrichedLocation{
			URI:       string(loc.URI),
			Line:      loc.Range.Start.Line,
			Character: loc.Range.Start.Character,
		}

		if contextLines > 0 {
			// Enrich with hover info (gracefully handle errors)
			if hover, err := b.HoverRaw(ctx, loc.URI, loc.Range.Start.Line, loc.Range.Start.Character); err == nil && hover != nil {
				refs[i].Hover = hover.Content
			}

			// Enrich with source context
			content, ok := fileCache[string(loc.URI)]
			if !ok {
				if c, err := b.readFile(loc.URI); err == nil {
					content = c
					fileCache[string(loc.URI)] = content
				}
			}
			if content != "" {
				refs[i].Context = extractSourceContext(content, loc.Range.Start.Line, contextLines)
			}
		}
	}

	return &EnrichedReferencesResult{
		Symbol: symbol,
		Count:  len(refs),
		Refs:   refs,
	}, nil
}

func (b *Bridge) CompletionRaw(ctx context.Context, uri lsp.DocumentURI, line, character int) ([]CompletionItem, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentCompletion, lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Position:     lsp.Position{Line: line, Character: character},
	})
	if err != nil {
		return nil, err
	}

	return parseCompletionItems(result), nil
}

func (b *Bridge) DiagnosticsRaw(ctx context.Context, uri lsp.DocumentURI) ([]DiagnosticItem, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentDiagnostic, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		return nil, err
	}

	return parseDiagnostics(result), nil
}

func (b *Bridge) CodeActionRaw(ctx context.Context, uri lsp.DocumentURI, startLine, startChar, endLine, endChar int) ([]CodeAction, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentCodeAction, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"range": lsp.Range{
			Start: lsp.Position{Line: startLine, Character: startChar},
			End:   lsp.Position{Line: endLine, Character: endChar},
		},
		"context": map[string]any{
			"diagnostics": []any{},
		},
	})
	if err != nil {
		return nil, err
	}

	return parseCodeActions(result), nil
}

func (b *Bridge) RenameRaw(ctx context.Context, uri lsp.DocumentURI, line, character int, newName string) (*WorkspaceEdit, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentRename, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"position":     lsp.Position{Line: line, Character: character},
		"newName":      newName,
	})
	if err != nil {
		return nil, err
	}

	var edit WorkspaceEdit
	if err := json.Unmarshal(result, &edit); err != nil {
		return nil, fmt.Errorf("parsing workspace edit: %w", err)
	}

	return &edit, nil
}

func (b *Bridge) WorkspaceSymbolsRaw(ctx context.Context, uri lsp.DocumentURI, query string) ([]WorkspaceSymbol, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodWorkspaceSymbol, map[string]any{
		"query": query,
	})
	if err != nil {
		return nil, err
	}

	return parseWorkspaceSymbols(result), nil
}

func (b *Bridge) FormatRaw(ctx context.Context, uri lsp.DocumentURI) (json.RawMessage, error) {
	if result, handled := b.tryExternalFormatRaw(ctx, uri); handled {
		return result, nil
	}

	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentFormatting, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"options": map[string]any{
			"tabSize":      4,
			"insertSpaces": true,
		},
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (b *Bridge) tryExternalFormatRaw(ctx context.Context, uri lsp.DocumentURI) (json.RawMessage, bool) {
	if b.fmtRouter == nil {
		return nil, false
	}

	filePath := uri.Path()
	match := b.fmtRouter.Match(filePath)
	if match == nil {
		return nil, false
	}

	if match.LSPFormat == "prefer" {
		return nil, false
	}

	content, err := b.readFile(uri)
	if err != nil {
		return nil, false
	}

	var fmtResult *formatter.Result
	switch match.Mode {
	case "chain":
		fmtResult, err = formatter.FormatChain(ctx, match.Formatters, filePath, []byte(content), b.executor)
	case "fallback":
		fmtResult, err = formatter.FormatFallback(ctx, match.Formatters, filePath, []byte(content), b.executor)
	default:
		return nil, false
	}

	if err != nil {
		if match.LSPFormat == "fallback" {
			return nil, false
		}
		return nil, true
	}

	if !fmtResult.Changed {
		raw, _ := json.Marshal([]lsp.TextEdit{})
		return raw, true
	}

	lines := strings.Count(content, "\n")
	edit := lsp.TextEdit{
		Range: lsp.Range{
			Start: lsp.Position{Line: 0, Character: 0},
			End:   lsp.Position{Line: lines + 1, Character: 0},
		},
		NewText: fmtResult.Formatted,
	}

	raw, _ := json.Marshal([]lsp.TextEdit{edit})
	return raw, true
}

func (b *Bridge) CodeAction(ctx context.Context, uri lsp.DocumentURI, startLine, startChar, endLine, endChar int) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentCodeAction, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"range": lsp.Range{
			Start: lsp.Position{Line: startLine, Character: startChar},
			End:   lsp.Position{Line: endLine, Character: endChar},
		},
		"context": map[string]any{
			"diagnostics": []any{},
		},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	actions := parseCodeActions(result)
	if len(actions) == 0 {
		return command.TextResult("No code actions available"), nil
	}

	text := formatCodeActions(actions)
	return command.TextResult(text), nil
}

func (b *Bridge) Rename(ctx context.Context, uri lsp.DocumentURI, line, character int, newName string) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentRename, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
		"position":     lsp.Position{Line: line, Character: character},
		"newName":      newName,
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	var edit WorkspaceEdit
	if err := json.Unmarshal(result, &edit); err != nil {
		return command.TextErrorResult(fmt.Sprintf("parsing workspace edit: %v", err)), nil
	}

	text := formatWorkspaceEdit(edit)
	return command.TextResult(text), nil
}

func (b *Bridge) WorkspaceSymbols(ctx context.Context, uri lsp.DocumentURI, query string) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodWorkspaceSymbol, map[string]any{
		"query": query,
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	symbols := parseWorkspaceSymbols(result)
	if len(symbols) == 0 {
		return command.TextResult("No symbols found matching: " + query), nil
	}

	text := formatWorkspaceSymbols(symbols)
	return command.TextResult(text), nil
}

func (b *Bridge) Diagnostics(ctx context.Context, uri lsp.DocumentURI) (*command.Result, error) {
	result, err := b.withDocument(ctx, uri, lsp.MethodTextDocumentDiagnostic, map[string]any{
		"textDocument": lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		return command.TextErrorResult(err.Error()), nil
	}

	diagnostics := parseDiagnostics(result)
	if len(diagnostics) == 0 {
		return command.TextResult("No diagnostics (errors, warnings) found"), nil
	}

	text := formatDiagnostics(diagnostics, uri)
	return command.TextResult(text), nil
}

// Call hierarchy types

type CallHierarchyItem struct {
	Name           string          `json:"name"`
	Kind           int             `json:"kind"`
	URI            string          `json:"uri"`
	Range          json.RawMessage `json:"range"`
	SelectionRange json.RawMessage `json:"selectionRange"`
	Detail         string          `json:"detail,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
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

func (b *Bridge) IncomingCallsRaw(ctx context.Context, uri lsp.DocumentURI, line, character int) (*CallHierarchyResult, error) {
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

	lspName := b.router.RouteByURI(uri)
	inst, ok := b.pool.Get(lspName)
	if !ok {
		return nil, fmt.Errorf("LSP %s not running", lspName)
	}

	incomingRaw, err := inst.Call(ctx, lsp.MethodCallHierarchyIncomingCalls, map[string]any{
		"item": items[0],
	})
	if err != nil {
		return nil, err
	}

	return parseIncomingCalls(items[0], incomingRaw), nil
}

func (b *Bridge) OutgoingCallsRaw(ctx context.Context, uri lsp.DocumentURI, line, character int) (*CallHierarchyResult, error) {
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

	lspName := b.router.RouteByURI(uri)
	inst, ok := b.pool.Get(lspName)
	if !ok {
		return nil, fmt.Errorf("LSP %s not running", lspName)
	}

	outgoingRaw, err := inst.Call(ctx, lsp.MethodCallHierarchyOutgoingCalls, map[string]any{
		"item": items[0],
	})
	if err != nil {
		return nil, err
	}

	return parseOutgoingCalls(items[0], outgoingRaw), nil
}

func callHierarchyItemToCall(item CallHierarchyItem) CallHierarchyCall {
	call := CallHierarchyCall{
		Name: item.Name,
		Kind: symbolKindName(item.Kind),
		URI:  item.URI,
	}

	var selRange struct {
		Start struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"start"`
	}
	if err := json.Unmarshal(item.SelectionRange, &selRange); err == nil {
		call.Line = selRange.Start.Line
		call.Character = selRange.Start.Character
	}

	return call
}

func parseIncomingCalls(prepared CallHierarchyItem, raw json.RawMessage) *CallHierarchyResult {
	result := &CallHierarchyResult{
		Symbol: callHierarchyItemToCall(prepared),
	}

	var entries []struct {
		From       CallHierarchyItem `json:"from"`
		FromRanges json.RawMessage   `json:"fromRanges"`
	}
	if err := json.Unmarshal(raw, &entries); err == nil {
		for _, entry := range entries {
			result.Calls = append(result.Calls, callHierarchyItemToCall(entry.From))
		}
	}

	return result
}

func parseOutgoingCalls(prepared CallHierarchyItem, raw json.RawMessage) *CallHierarchyResult {
	result := &CallHierarchyResult{
		Symbol: callHierarchyItemToCall(prepared),
	}

	var entries []struct {
		To         CallHierarchyItem `json:"to"`
		FromRanges json.RawMessage   `json:"fromRanges"`
	}
	if err := json.Unmarshal(raw, &entries); err == nil {
		for _, entry := range entries {
			result.Calls = append(result.Calls, callHierarchyItemToCall(entry.To))
		}
	}

	return result
}

func (b *Bridge) readFile(uri lsp.DocumentURI) (string, error) {
	path := uri.Path()
	if path == "" {
		return "", fmt.Errorf("invalid URI: %s", uri)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (b *Bridge) ProjectRootForPath(path string) string {
	root, err := config.FindProjectRoot(path)
	if err != nil {
		return filepath.Dir(path)
	}
	return root
}

func (b *Bridge) DefaultInitParams(uri lsp.DocumentURI) *lsp.InitializeParams {
	path := uri.Path()
	rootPath := b.ProjectRootForPath(path)
	rootURI := lsp.URIFromPath(rootPath)

	pid := os.Getpid()
	return &lsp.InitializeParams{
		ProcessID: &pid,
		RootURI:   &rootURI,
		RootPath:  &rootPath,
		ClientInfo: &lsp.ClientInfo{
			Name:    "lux-mcp",
			Version: "0.1.0",
		},
		Capabilities: lsp.ClientCapabilities{
			Workspace: &lsp.WorkspaceClientCapabilities{
				WorkspaceFolders: true,
			},
			TextDocument: &lsp.TextDocumentClientCapabilities{
				Hover:          &lsp.HoverClientCaps{},
				Definition:     &lsp.DefinitionClientCaps{},
				References:     &lsp.ReferencesClientCaps{},
				Completion:     &lsp.CompletionClientCaps{},
				DocumentSymbol: &lsp.DocumentSymbolClientCaps{},
				CodeAction:     &lsp.CodeActionClientCaps{},
				Formatting:     &lsp.FormattingClientCaps{},
				Rename:             &lsp.RenameClientCaps{},
				CallHierarchy:     &lsp.CallHierarchyClientCaps{},
				PublishDiagnostics: &lsp.PublishDiagnosticsClientCaps{},
			},
			Window: &lsp.WindowClientCapabilities{
				WorkDoneProgress: true,
			},
		},
		WorkspaceFolders: []lsp.WorkspaceFolder{
			{
				URI:  rootURI,
				Name: filepath.Base(rootPath),
			},
		},
	}
}

func (b *Bridge) InferLanguageID(uri lsp.DocumentURI) string {
	ext := uri.Extension()
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".jsx":
		return "javascriptreact"
	case ".rs":
		return "rust"
	case ".nix":
		return "nix"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "cpp"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".cs":
		return "csharp"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".lua":
		return "lua"
	case ".sh", ".bash":
		return "shellscript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".md":
		return "markdown"
	default:
		return "plaintext"
	}
}

// Raw result types for JSON-serializable output

type HoverResult struct {
	Content string `json:"content"`
}

type LocationResult struct {
	URI       string `json:"uri"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

// Helper types and functions

type WorkspaceEdit struct {
	Changes         map[string][]lsp.TextEdit `json:"changes,omitempty"`
	DocumentChanges json.RawMessage           `json:"documentChanges,omitempty"`
}

type CompletionItem struct {
	Label      string `json:"label"`
	Kind       int    `json:"kind,omitempty"`
	Detail     string `json:"detail,omitempty"`
	InsertText string `json:"insertText,omitempty"`
}

type Symbol struct {
	Name     string        `json:"name"`
	Kind     int           `json:"kind"`
	Range    lsp.Range     `json:"range,omitempty"`
	Location *lsp.Location `json:"location,omitempty"`
	Children []Symbol      `json:"children,omitempty"`
}

type CodeAction struct {
	Title       string `json:"title"`
	Kind        string `json:"kind,omitempty"`
	IsPreferred bool   `json:"isPreferred,omitempty"`
}

func extractMarkdownContent(raw json.RawMessage) string {
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}

	var markupContent struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &markupContent); err == nil {
		return markupContent.Value
	}

	var contents []json.RawMessage
	if err := json.Unmarshal(raw, &contents); err == nil && len(contents) > 0 {
		var parts []string
		for _, c := range contents {
			parts = append(parts, extractMarkdownContent(c))
		}
		return strings.Join(parts, "\n\n")
	}

	return string(raw)
}

func parseLocations(raw json.RawMessage) []lsp.Location {
	if raw == nil || string(raw) == "null" {
		return nil
	}

	var single lsp.Location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []lsp.Location{single}
	}

	var multiple []lsp.Location
	if err := json.Unmarshal(raw, &multiple); err == nil {
		return multiple
	}

	return nil
}

func formatLocations(locs []lsp.Location) string {
	var sb strings.Builder
	for i, loc := range locs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%s:%d:%d",
			loc.URI.Path(),
			loc.Range.Start.Line+1,
			loc.Range.Start.Character+1))
	}
	return sb.String()
}

func parseCompletionItems(raw json.RawMessage) []CompletionItem {
	if raw == nil || string(raw) == "null" {
		return nil
	}

	var items []CompletionItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items
	}

	var list struct {
		Items []CompletionItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err == nil {
		return list.Items
	}

	return nil
}

func formatCompletionItems(items []CompletionItem) string {
	var sb strings.Builder
	for i, item := range items {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("\n... and %d more", len(items)-20))
			break
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(item.Label)
		if item.Detail != "" {
			sb.WriteString(" - ")
			sb.WriteString(item.Detail)
		}
	}
	return sb.String()
}

func parseSymbols(raw json.RawMessage) []Symbol {
	if raw == nil || string(raw) == "null" {
		return nil
	}

	var symbols []Symbol
	if err := json.Unmarshal(raw, &symbols); err == nil {
		return symbols
	}

	return nil
}

func formatSymbols(symbols []Symbol, indent int) string {
	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)
	for i, sym := range symbols {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(prefix)
		sb.WriteString(symbolKindName(sym.Kind))
		sb.WriteString(" ")
		sb.WriteString(sym.Name)
		if len(sym.Children) > 0 {
			sb.WriteString("\n")
			sb.WriteString(formatSymbols(sym.Children, indent+1))
		}
	}
	return sb.String()
}

func symbolKindName(kind int) string {
	kinds := map[int]string{
		1: "File", 2: "Module", 3: "Namespace", 4: "Package", 5: "Class",
		6: "Method", 7: "Property", 8: "Field", 9: "Constructor", 10: "Enum",
		11: "Interface", 12: "Function", 13: "Variable", 14: "Constant", 15: "String",
		16: "Number", 17: "Boolean", 18: "Array", 19: "Object", 20: "Key",
		21: "Null", 22: "EnumMember", 23: "Struct", 24: "Event", 25: "Operator",
		26: "TypeParameter",
	}
	if name, ok := kinds[kind]; ok {
		return name
	}
	return "Symbol"
}

func parseCodeActions(raw json.RawMessage) []CodeAction {
	if raw == nil || string(raw) == "null" {
		return nil
	}

	var actions []CodeAction
	if err := json.Unmarshal(raw, &actions); err == nil {
		return actions
	}

	return nil
}

func formatCodeActions(actions []CodeAction) string {
	var sb strings.Builder
	for i, action := range actions {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("- ")
		sb.WriteString(action.Title)
		if action.Kind != "" {
			sb.WriteString(" (")
			sb.WriteString(action.Kind)
			sb.WriteString(")")
		}
	}
	return sb.String()
}

func formatTextEdits(edits []lsp.TextEdit) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d edit(s) to apply:\n", len(edits)))
	for i, edit := range edits {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("... and %d more", len(edits)-10))
			break
		}
		sb.WriteString(fmt.Sprintf("- Line %d-%d: replace with %q\n",
			edit.Range.Start.Line+1,
			edit.Range.End.Line+1,
			truncate(edit.NewText, 50)))
	}
	return sb.String()
}

func formatWorkspaceEdit(edit WorkspaceEdit) string {
	var sb strings.Builder
	total := 0
	for uri, edits := range edit.Changes {
		total += len(edits)
		sb.WriteString(fmt.Sprintf("%s: %d edit(s)\n", uri, len(edits)))
	}
	if total == 0 {
		return "No changes to apply"
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d edit(s)", total))
	return sb.String()
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

type WorkspaceSymbol struct {
	Name          string       `json:"name"`
	Kind          int          `json:"kind"`
	Location      lsp.Location `json:"location"`
	ContainerName string       `json:"containerName,omitempty"`
}

func parseWorkspaceSymbols(raw json.RawMessage) []WorkspaceSymbol {
	if raw == nil || string(raw) == "null" {
		return nil
	}

	var symbols []WorkspaceSymbol
	if err := json.Unmarshal(raw, &symbols); err == nil {
		return symbols
	}

	return nil
}

func formatWorkspaceSymbols(symbols []WorkspaceSymbol) string {
	var sb strings.Builder
	for i, sym := range symbols {
		if i >= 50 {
			sb.WriteString(fmt.Sprintf("\n... and %d more", len(symbols)-50))
			break
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%s %s", symbolKindName(sym.Kind), sym.Name))
		if sym.ContainerName != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", sym.ContainerName))
		}
		sb.WriteString(fmt.Sprintf(" - %s:%d",
			sym.Location.URI.Path(),
			sym.Location.Range.Start.Line+1))
	}
	return sb.String()
}

type DiagnosticItem struct {
	URI      string    `json:"uri,omitempty"`
	Range    lsp.Range `json:"range"`
	Severity int       `json:"severity,omitempty"`
	Source   string    `json:"source,omitempty"`
	Message  string    `json:"message"`
}

type LSPDiagnosticGroup struct {
	Name         string           `json:"name"`
	FilesScanned int              `json:"files_scanned"`
	Diagnostics  []DiagnosticItem `json:"diagnostics"`
}

type BatchDiagnosticsResult struct {
	LSPs []LSPDiagnosticGroup `json:"lsps"`
}

func (b *Bridge) BatchDiagnostics(ctx context.Context, pattern string) (*BatchDiagnosticsResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	g, err := glob.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}

	// Group matched files by LSP name
	lspFiles := make(map[string][]string)
	err = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(cwd, path)
		if !g.Match(relPath) {
			return nil
		}

		ext := filepath.Ext(path)
		lspName := b.router.RouteByExtension(ext)
		if lspName == "" {
			return nil
		}

		lspFiles[lspName] = append(lspFiles[lspName], path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	result := &BatchDiagnosticsResult{}

	for lspName, files := range lspFiles {
		group := LSPDiagnosticGroup{
			Name:         lspName,
			FilesScanned: len(files),
		}

		for _, absPath := range files {
			uri := lsp.DocumentURI("file://" + absPath)
			diags, err := b.DiagnosticsRaw(ctx, uri)
			if err != nil {
				fmt.Fprintf(logfile.Writer(), "[lux] batch diagnostics: skipping %s: %v\n", absPath, err)
				continue
			}

			for i := range diags {
				diags[i].URI = string(uri)
			}
			group.Diagnostics = append(group.Diagnostics, diags...)
		}

		result.LSPs = append(result.LSPs, group)
	}

	return result, nil
}

func parseDiagnostics(raw json.RawMessage) []DiagnosticItem {
	if raw == nil || string(raw) == "null" {
		return nil
	}

	// Try full diagnostic response format
	var fullResp struct {
		Kind  string           `json:"kind"`
		Items []DiagnosticItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &fullResp); err == nil && len(fullResp.Items) > 0 {
		return fullResp.Items
	}

	// Try direct array of diagnostics
	var items []DiagnosticItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items
	}

	return nil
}

func formatDiagnostics(diags []DiagnosticItem, uri lsp.DocumentURI) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d diagnostic(s) in %s:\n", len(diags), uri.Path()))
	for i, d := range diags {
		if i >= 30 {
			sb.WriteString(fmt.Sprintf("\n... and %d more", len(diags)-30))
			break
		}
		severity := "info"
		switch d.Severity {
		case 1:
			severity = "error"
		case 2:
			severity = "warning"
		case 3:
			severity = "info"
		case 4:
			severity = "hint"
		}
		sb.WriteString(fmt.Sprintf("\n[%s] Line %d: %s",
			severity,
			d.Range.Start.Line+1,
			d.Message))
		if d.Source != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", d.Source))
		}
	}
	return sb.String()
}
