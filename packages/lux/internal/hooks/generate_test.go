package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateStopHook_CreatesHooksJSON(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(pluginDir, "hooks", "hooks.json"))
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

	stop, ok := hooks["Stop"].([]any)
	if !ok {
		t.Fatal("hooks.json missing 'Stop' key")
	}

	if len(stop) != 1 {
		t.Fatalf("expected 1 Stop entry, got %d", len(stop))
	}

	entry := stop[0].(map[string]any)
	innerHooks := entry["hooks"].([]any)
	hook := innerHooks[0].(map[string]any)

	if hook["command"] != "lux fmt-all" {
		t.Errorf("command = %q, want %q", hook["command"], "lux fmt-all")
	}

	timeout, ok := hook["timeout"].(float64)
	if !ok || timeout != 60 {
		t.Errorf("timeout = %v, want 60", hook["timeout"])
	}
}

func TestGenerateStopHook_MergesWithExistingPreToolUse(t *testing.T) {
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

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(result, &manifest); err != nil {
		t.Fatal(err)
	}

	hooks := manifest["hooks"].(map[string]any)

	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse was lost during merge")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("Stop was not added")
	}
	if _, ok := hooks["PostToolUse"]; ok {
		t.Error("PostToolUse should not be present")
	}
}

func TestGenerateStopHook_NoFormatFileScript(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	scriptPath := filepath.Join(pluginDir, "hooks", "format-file")
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("format-file script should not exist")
	}
}

func TestGenerateStopHook_RemovesExistingPostToolUse(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")
	hooksDir := filepath.Join(pluginDir, "hooks")

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	existing := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{
					"matcher": "Edit|Write",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "${CLAUDE_PLUGIN_ROOT}/hooks/format-file",
							"timeout": 30,
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0o644)
	os.WriteFile(filepath.Join(hooksDir, "format-file"), []byte("#!/bin/bash\n"), 0o755)

	if err := GenerateStopHook(pluginDir); err != nil {
		t.Fatalf("GenerateStopHook: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	var manifest map[string]any
	json.Unmarshal(result, &manifest)
	hooks := manifest["hooks"].(map[string]any)

	if _, ok := hooks["PostToolUse"]; ok {
		t.Error("PostToolUse should have been removed")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("Stop should have been added")
	}

	if _, err := os.Stat(filepath.Join(hooksDir, "format-file")); !os.IsNotExist(err) {
		t.Error("format-file script should have been deleted")
	}
}
