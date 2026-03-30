package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchHooksMatcherAddWebFetch(t *testing.T) {
	dir := t.TempDir()

	hooksDir := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initial := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "${CLAUDE_PLUGIN_ROOT}/hooks/pre-tool-use",
							"timeout": 5,
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := PatchHooksMatcher(dir, "WebFetch"); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	hooks := parsed["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)
	entry := preToolUse[0].(map[string]any)
	matcher := entry["matcher"].(string)

	if matcher != "Bash|WebFetch" {
		t.Errorf("expected matcher %q, got %q", "Bash|WebFetch", matcher)
	}
}

func TestPatchHooksMatcherAlreadyPresent(t *testing.T) {
	dir := t.TempDir()

	hooksDir := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initial := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash|WebFetch",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "${CLAUDE_PLUGIN_ROOT}/hooks/pre-tool-use",
							"timeout": 5,
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := PatchHooksMatcher(dir, "WebFetch"); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	hooks := parsed["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)
	entry := preToolUse[0].(map[string]any)
	matcher := entry["matcher"].(string)

	if matcher != "Bash|WebFetch" {
		t.Errorf("expected matcher %q, got %q", "Bash|WebFetch", matcher)
	}
}

func TestPatchHooksMatcherNoFile(t *testing.T) {
	dir := t.TempDir()

	err := PatchHooksMatcher(dir, "WebFetch")
	if err == nil {
		t.Fatal("expected error when hooks.json does not exist")
	}
}
