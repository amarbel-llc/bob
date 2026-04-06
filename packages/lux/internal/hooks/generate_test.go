package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateHooks_CreatesHooksJSON(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GenerateHooks(pluginDir); err != nil {
		t.Fatalf("GenerateHooks: %v", err)
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

	// PostToolUse
	postToolUse, ok := hooks["PostToolUse"].([]any)
	if !ok {
		t.Fatal("hooks.json missing 'PostToolUse' key")
	}
	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 PostToolUse entry, got %d", len(postToolUse))
	}
	ptuEntry := postToolUse[0].(map[string]any)
	if ptuEntry["matcher"] != "Edit|Write" {
		t.Errorf("PostToolUse matcher = %q, want %q", ptuEntry["matcher"], "Edit|Write")
	}
	ptuHooks := ptuEntry["hooks"].([]any)
	ptuHook := ptuHooks[0].(map[string]any)
	if ptuHook["command"] != "${CLAUDE_PLUGIN_ROOT}/hooks/post-tool-use" {
		t.Errorf("PostToolUse command = %q", ptuHook["command"])
	}

	// Stop
	stop, ok := hooks["Stop"].([]any)
	if !ok {
		t.Fatal("hooks.json missing 'Stop' key")
	}
	if len(stop) != 1 {
		t.Fatalf("expected 1 Stop entry, got %d", len(stop))
	}
	stopEntry := stop[0].(map[string]any)
	stopHooks := stopEntry["hooks"].([]any)
	stopHook := stopHooks[0].(map[string]any)
	if stopHook["command"] != "${CLAUDE_PLUGIN_ROOT}/hooks/stop-fmt" {
		t.Errorf("Stop command = %q", stopHook["command"])
	}
	timeout, ok := stopHook["timeout"].(float64)
	if !ok || timeout != 60 {
		t.Errorf("Stop timeout = %v, want 60", stopHook["timeout"])
	}
}

func TestGenerateHooks_MergesWithExistingPreToolUse(t *testing.T) {
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

	if err := GenerateHooks(pluginDir); err != nil {
		t.Fatalf("GenerateHooks: %v", err)
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
	if _, ok := hooks["PostToolUse"]; !ok {
		t.Error("PostToolUse was not added")
	}
	if _, ok := hooks["Stop"]; !ok {
		t.Error("Stop was not added")
	}
}

func TestGenerateHooks_WritesScripts(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GenerateHooks(pluginDir); err != nil {
		t.Fatalf("GenerateHooks: %v", err)
	}

	hooksDir := filepath.Join(pluginDir, "hooks")

	for _, script := range []struct {
		name     string
		contains []string
	}{
		{
			name:     "post-tool-use",
			contains: []string{"#!/usr/bin/env bash", "file_path", "session_id", "edited-files-${session_id}", "XDG_STATE_HOME"},
		},
		{
			name:     "stop-fmt",
			contains: []string{"#!/usr/bin/env bash", "lux fmt-all", "session_id", "edited-files-${session_id}", "sort -u", "XDG_STATE_HOME"},
		},
	} {
		path := filepath.Join(hooksDir, script.name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", script.name, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable", script.name)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", script.name, err)
		}

		for _, s := range script.contains {
			if !strings.Contains(string(content), s) {
				t.Errorf("%s missing %q", script.name, s)
			}
		}
	}
}

func TestGenerateHooks_CleansUpOldFormatFile(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")
	hooksDir := filepath.Join(pluginDir, "hooks")

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create old format-file script
	os.WriteFile(filepath.Join(hooksDir, "format-file"), []byte("#!/bin/bash\n"), 0o755)

	if err := GenerateHooks(pluginDir); err != nil {
		t.Fatalf("GenerateHooks: %v", err)
	}

	if _, err := os.Stat(filepath.Join(hooksDir, "format-file")); !os.IsNotExist(err) {
		t.Error("format-file script should have been deleted")
	}
}
