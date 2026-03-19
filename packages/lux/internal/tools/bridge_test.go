package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/amarbel-llc/lux/internal/lsp"
)

func TestNewBridge_SetsLocalFields(t *testing.T) {
	b := NewBridge(nil, nil, nil, nil, nil)

	if b.pool != nil {
		t.Error("expected pool to be nil when not provided")
	}
	if b.router != nil {
		t.Error("expected router to be nil when not provided")
	}
}

func TestLocationsToLocationResults(t *testing.T) {
	locs := []lsp.Location{
		{
			URI:   "file:///home/user/foo.go",
			Range: lsp.Range{Start: lsp.Position{Line: 10, Character: 5}},
		},
		{
			URI:   "file:///home/user/bar.go",
			Range: lsp.Range{Start: lsp.Position{Line: 20, Character: 3}},
		},
	}

	results := locationsToLocationResults(locs)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].URI != "file:///home/user/foo.go" {
		t.Errorf("expected URI file:///home/user/foo.go, got %s", results[0].URI)
	}
	if results[0].Line != 10 {
		t.Errorf("expected line 10, got %d", results[0].Line)
	}
	if results[0].Character != 5 {
		t.Errorf("expected character 5, got %d", results[0].Character)
	}

	if results[1].URI != "file:///home/user/bar.go" {
		t.Errorf("expected URI file:///home/user/bar.go, got %s", results[1].URI)
	}
	if results[1].Line != 20 {
		t.Errorf("expected line 20, got %d", results[1].Line)
	}
	if results[1].Character != 3 {
		t.Errorf("expected character 3, got %d", results[1].Character)
	}
}

func TestLocationsToLocationResults_Empty(t *testing.T) {
	results := locationsToLocationResults(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(results))
	}

	results = locationsToLocationResults([]lsp.Location{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestHoverResult_JSONStructure(t *testing.T) {
	h := HoverResult{Content: "func Foo() string"}

	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["content"] != "func Foo() string" {
		t.Errorf("expected content field, got %v", decoded)
	}
}

func TestLocationResult_JSONStructure(t *testing.T) {
	lr := LocationResult{
		URI:       "file:///test.go",
		Line:      5,
		Character: 10,
	}

	data, err := json.Marshal(lr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["uri"] != "file:///test.go" {
		t.Errorf("expected uri field, got %v", decoded["uri"])
	}
	if decoded["line"] != float64(5) {
		t.Errorf("expected line 5, got %v", decoded["line"])
	}
	if decoded["character"] != float64(10) {
		t.Errorf("expected character 10, got %v", decoded["character"])
	}
}

func TestRawMethodSignatures_Exist(t *testing.T) {
	// Verify that all Raw methods exist with correct signatures by
	// referencing them as method values. This is a compile-time check
	// that also runs as a test.
	b := NewBridge(nil, nil, nil, nil, nil)
	ctx := context.Background()
	uri := lsp.DocumentURI("file:///test.go")

	// Each variable assignment verifies the method signature at compile time.
	_ = func() (*HoverResult, error) { return b.HoverRaw(ctx, uri, 0, 0) }
	_ = func() ([]LocationResult, error) { return b.DefinitionRaw(ctx, uri, 0, 0) }
	_ = func() ([]LocationResult, error) { return b.ReferencesRaw(ctx, uri, 0, 0, true) }
	_ = func() ([]CompletionItem, error) { return b.CompletionRaw(ctx, uri, 0, 0) }
	_ = func() ([]DiagnosticItem, error) { return b.DiagnosticsRaw(ctx, uri) }
	_ = func() ([]CodeAction, error) { return b.CodeActionRaw(ctx, uri, 0, 0, 0, 0) }
	_ = func() (*WorkspaceEdit, error) { return b.RenameRaw(ctx, uri, 0, 0, "new") }
	_ = func() ([]WorkspaceSymbol, error) { return b.WorkspaceSymbolsRaw(ctx, uri, "q") }
	_ = func() (json.RawMessage, error) { return b.FormatRaw(ctx, uri) }
	_ = func() ([]Symbol, error) { return b.DocumentSymbolsRaw(ctx, uri) }
}

// Helpers

type stubDocTracker struct {
	open   bool
	opened bool
}

func (s *stubDocTracker) IsOpen(_ lsp.DocumentURI) bool {
	return s.open
}

func (s *stubDocTracker) Open(_ context.Context, _ lsp.DocumentURI) error {
	s.opened = true
	s.open = true
	return nil
}

func createTestFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	return path
}
