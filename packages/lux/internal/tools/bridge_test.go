package tools

import (
	"context"
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
