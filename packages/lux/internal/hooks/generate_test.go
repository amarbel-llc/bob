package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePostToolUseHooks_CreatesHooksJSON(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GeneratePostToolUseHooks(pluginDir); err != nil {
		t.Fatalf("GeneratePostToolUseHooks: %v", err)
	}

	hooksDir := filepath.Join(pluginDir, "hooks")
	data, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	if err != nil {
		t.Fatalf("reading hooks.json: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parsing hooks.json: %v", err)
	}

	hooks, ok := manifest["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks.json missing 'hooks' key")
	}

	postToolUse, ok := hooks["PostToolUse"].([]any)
	if !ok {
		t.Fatal("hooks.json missing 'PostToolUse' key")
	}

	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 PostToolUse entry, got %d", len(postToolUse))
	}

	entry := postToolUse[0].(map[string]any)
	if entry["matcher"] != "Edit|Write" {
		t.Errorf("matcher = %q, want %q", entry["matcher"], "Edit|Write")
	}
}

func TestGeneratePostToolUseHooks_MergesWithExisting(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")
	hooksDir := filepath.Join(pluginDir, "hooks")

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	existing := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash|Read",
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

	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := GeneratePostToolUseHooks(pluginDir); err != nil {
		t.Fatalf("GeneratePostToolUseHooks: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(result, &manifest); err != nil {
		t.Fatal(err)
	}

	hooks, ok := manifest["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks.json missing 'hooks' key after merge")
	}

	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse was lost during merge")
	}

	if _, ok := hooks["PostToolUse"]; !ok {
		t.Error("PostToolUse was not added")
	}
}

func TestGeneratePostToolUseHooks_WritesFormatScript(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GeneratePostToolUseHooks(pluginDir); err != nil {
		t.Fatalf("GeneratePostToolUseHooks: %v", err)
	}

	scriptPath := filepath.Join(pluginDir, "hooks", "format-file")

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat format-file: %v", err)
	}

	if info.Mode()&0o111 == 0 {
		t.Error("format-file is not executable")
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}

	script := string(content)
	if !strings.Contains(script, "#!/usr/bin/env bash") {
		t.Error("missing shebang")
	}
	if !strings.Contains(script, "lux fmt") {
		t.Error("missing lux fmt invocation")
	}
	if !strings.Contains(script, "file_path") {
		t.Error("missing file_path extraction")
	}
}
