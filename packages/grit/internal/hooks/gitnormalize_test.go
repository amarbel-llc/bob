package hooks

import (
	"testing"
)

func TestNormalizeGitCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain", "git status", "git status"},
		{"with -C", "git -C /path status", "git status"},
		{"with --git-dir", "git --git-dir=/path status", "git status"},
		{"with -c", "git -c core.pager=cat log", "git log"},
		{"with --no-pager", "git --no-pager log", "git log"},
		{"non-git", "ls -la", "ls -la"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeGitCommand(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
