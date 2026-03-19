package hooks

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func makeHookInput(toolName string, toolInput map[string]any) []byte {
	input := map[string]any{
		"tool_name":  toolName,
		"tool_input": toolInput,
	}
	data, _ := json.Marshal(input)
	return data
}

func TestResourceHookBasicMatch(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git status"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle git status")
	}
	if !strings.Contains(out.String(), "grit://status") {
		t.Errorf("expected grit://status in output, got %q", out.String())
	}
}

func TestResourceHookNormalizedMatch(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git -C /some/path status"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle normalized git status")
	}
	if !strings.Contains(out.String(), "grit://status") {
		t.Errorf("expected grit://status in output, got %q", out.String())
	}
}

func TestResourceHookCompoundCommand(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git status && git log"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle compound command")
	}
}

func TestResourceHookCatchAllGit(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git commit -m 'foo'"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to deny git commit via catch-all")
	}
	if !strings.Contains(out.String(), "All git commands are blocked") {
		t.Errorf("expected catch-all deny message, got %q", out.String())
	}
}

func TestResourceHookNoMatchNonGit(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "ls -la"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle non-git command")
	}
	if out.Len() != 0 {
		t.Errorf("expected no output, got %q", out.String())
	}
}

func TestResourceHookNonBashTool(t *testing.T) {
	input := makeHookInput("Read", map[string]any{"file_path": "/foo/bar"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle non-Bash tool")
	}
}

func TestResourceHookDenyMessageFormat(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git status"})
	var out bytes.Buffer
	HandleResourceHook(input, &out)

	output := out.String()
	if !strings.Contains(output, "grit://status") {
		t.Error("deny message should contain resource URI")
	}
	if !strings.Contains(output, "resource-read") {
		t.Error("deny message should contain resource-read tool for subagents")
	}
	if !strings.Contains(output, "deny") {
		t.Error("deny message should contain deny decision")
	}
}

func TestResourceHookAllMappings(t *testing.T) {
	tests := []struct {
		command     string
		resourceURI string
	}{
		{"git status", "grit://status"},
		{"git branch", "grit://branches"},
		{"git branch -a", "grit://branches"},
		{"git remote", "grit://remotes"},
		{"git remote -v", "grit://remotes"},
		{"git tag", "grit://tags"},
		{"git tag --list", "grit://tags"},
		{"git log", "grit://log"},
		{"git log --oneline", "grit://log"},
		{"git show abc123", "grit://commits/{ref}"},
		{"git blame foo.go", "grit://blame/{path}"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			input := makeHookInput("Bash", map[string]any{"command": tt.command})
			var out bytes.Buffer
			handled, err := HandleResourceHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %q", tt.command)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}
