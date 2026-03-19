package mcp

import (
	"context"
	"testing"

	mcpserver "github.com/amarbel-llc/purse-first/libs/go-mcp/server"
	"github.com/amarbel-llc/lux/internal/server"
	"github.com/amarbel-llc/lux/internal/tools"
)

func newTestResourceProvider() *resourceProvider {
	registry := mcpserver.NewResourceRegistry()
	router, _ := server.NewRouter(nil)
	bridge := tools.NewBridge(nil, router, nil, nil, nil)
	return newResourceProvider(registry, bridge, NewDiagnosticsStore())
}

func TestReadLSPResourceFormatParameterDefaultJSON(t *testing.T) {
	p := newTestResourceProvider()

	// The bridge will fail (no pool/router), but the error message tells us
	// the format was parsed and the correct code path was taken.
	// With format=json (default), it calls HoverRaw which goes through withDocument.
	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/hover?uri=file:///test.go&line=1&character=1")
	if err == nil {
		t.Fatal("expected error from bridge call (no LSP configured)")
	}
	// The error should be from the bridge, not from parameter parsing
	if err.Error() == "missing required parameter 'uri'" {
		t.Error("URI parameter should have been parsed successfully")
	}
}

func TestReadLSPResourceFormatText(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/hover?uri=file:///test.go&line=1&character=1&format=text")
	if err == nil {
		t.Fatal("expected error from bridge call (no LSP configured)")
	}
	// The error should be from the bridge, not from parameter parsing
	if err.Error() == "missing required parameter 'uri'" {
		t.Error("URI parameter should have been parsed successfully")
	}
}

func TestReadLSPResourceFormatJSON(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/hover?uri=file:///test.go&line=1&character=1&format=json")
	if err == nil {
		t.Fatal("expected error from bridge call (no LSP configured)")
	}
	if err.Error() == "missing required parameter 'uri'" {
		t.Error("URI parameter should have been parsed successfully")
	}
}

func TestReadLSPResourceMissingOperation(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(), "lux://lsp/")
	if err == nil {
		t.Fatal("expected error for missing operation")
	}
	expected := "missing operation in resource URI"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceUnknownOperation(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(), "lux://lsp/bogus?uri=file:///test.go")
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
	expected := "unknown LSP operation: bogus"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceMissingURI(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(), "lux://lsp/hover?line=1&character=1")
	if err == nil {
		t.Fatal("expected error for missing URI")
	}
	expected := "missing required parameter 'uri'"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceMissingLineParam(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/hover?uri=file:///test.go&character=1")
	if err == nil {
		t.Fatal("expected error for missing line")
	}
	expected := "invalid or missing 'line' parameter"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceMissingCharacterParam(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/hover?uri=file:///test.go&line=1")
	if err == nil {
		t.Fatal("expected error for missing character")
	}
	expected := "invalid or missing 'character' parameter"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceRenameMissingNewName(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/rename?uri=file:///test.go&line=1&character=1")
	if err == nil {
		t.Fatal("expected error for missing new_name")
	}
	expected := "missing required parameter 'new_name'"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceWorkspaceSymbolsMissingQuery(t *testing.T) {
	p := newTestResourceProvider()

	_, err := p.readLSPResource(context.Background(),
		"lux://lsp/workspace-symbols?uri=file:///test.go")
	if err == nil {
		t.Fatal("expected error for missing query")
	}
	expected := "missing required parameter 'query'"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestReadLSPResourceAllOperationsReachBridge(t *testing.T) {
	// Verify that all operations with valid parameters reach the bridge
	// (get past URL parsing). With a nil pool, bridge methods panic or
	// return errors about no LSP configured.
	p := newTestResourceProvider()

	tests := []struct {
		name string
		uri  string
	}{
		{"hover-json", "lux://lsp/hover?uri=file:///t.go&line=0&character=0"},
		{"hover-text", "lux://lsp/hover?uri=file:///t.go&line=0&character=0&format=text"},
		{"definition-json", "lux://lsp/definition?uri=file:///t.go&line=0&character=0"},
		{"definition-text", "lux://lsp/definition?uri=file:///t.go&line=0&character=0&format=text"},
		{"references-json", "lux://lsp/references?uri=file:///t.go&line=0&character=0"},
		{"references-text", "lux://lsp/references?uri=file:///t.go&line=0&character=0&format=text"},
		{"completion-json", "lux://lsp/completion?uri=file:///t.go&line=0&character=0"},
		{"completion-text", "lux://lsp/completion?uri=file:///t.go&line=0&character=0&format=text"},
		{"format-json", "lux://lsp/format?uri=file:///t.go"},
		{"format-text", "lux://lsp/format?uri=file:///t.go&format=text"},
		{"document-symbols-json", "lux://lsp/document-symbols?uri=file:///t.go"},
		{"document-symbols-text", "lux://lsp/document-symbols?uri=file:///t.go&format=text"},
		{"diagnostics-json", "lux://lsp/diagnostics?uri=file:///t.go"},
		{"diagnostics-text", "lux://lsp/diagnostics?uri=file:///t.go&format=text"},
		{"code-action-json", "lux://lsp/code-action?uri=file:///t.go&start_line=0&start_character=0&end_line=0&end_character=0"},
		{"code-action-text", "lux://lsp/code-action?uri=file:///t.go&start_line=0&start_character=0&end_line=0&end_character=0&format=text"},
		{"rename-json", "lux://lsp/rename?uri=file:///t.go&line=0&character=0&new_name=foo"},
		{"rename-text", "lux://lsp/rename?uri=file:///t.go&line=0&character=0&new_name=foo&format=text"},
		{"workspace-symbols-json", "lux://lsp/workspace-symbols?uri=file:///t.go&query=foo"},
		{"workspace-symbols-text", "lux://lsp/workspace-symbols?uri=file:///t.go&query=foo&format=text"},
		{"incoming-calls", "lux://lsp/incoming-calls?uri=file:///t.go&line=0&character=0"},
		{"outgoing-calls", "lux://lsp/outgoing-calls?uri=file:///t.go&line=0&character=0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.readLSPResource(context.Background(), tt.uri)
			if err == nil {
				t.Fatal("expected error from bridge (no LSP configured)")
			}
			// Should NOT be a parameter-parsing error
			paramErrors := []string{
				"missing required parameter",
				"invalid or missing",
				"missing operation",
				"unknown LSP operation",
			}
			for _, pe := range paramErrors {
				if err.Error() == pe || len(err.Error()) > len(pe) && err.Error()[:len(pe)] == pe {
					t.Errorf("got parameter-parsing error instead of bridge error: %s", err.Error())
				}
			}
		})
	}
}
