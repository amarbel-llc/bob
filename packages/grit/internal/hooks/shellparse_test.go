package hooks

import (
	"testing"
)

func TestExtractSimpleCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single command", "git status", []string{"git status"}},
		{"compound and", "git status && git log", []string{"git status", "git log"}},
		{"compound or", "git status || git log", []string{"git status", "git log"}},
		{"pipe", "git log | head", []string{"git log", "head"}},
		{"empty", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSimpleCommands(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d commands, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, cmd := range result {
				if cmd != tt.expected[i] {
					t.Errorf("command[%d]: expected %q, got %q", i, tt.expected[i], cmd)
				}
			}
		})
	}
}
