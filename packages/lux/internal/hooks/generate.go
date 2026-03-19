package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const formatScript = `#!/usr/bin/env bash
set -euo pipefail
input=$(cat)
file_path=$(jq -r '.tool_input.file_path // empty' <<< "$input")
if [[ -n "$file_path" ]]; then
  lux fmt "$file_path" 2>/dev/null || true
fi
`

// GeneratePostToolUseHooks adds PostToolUse hook entries to the hooks directory
// under pluginDir. If hooks/hooks.json already exists (e.g. from go-mcp's
// PreToolUse generation), the PostToolUse entry is merged in. If it doesn't
// exist, a new hooks.json is created.
func GeneratePostToolUseHooks(pluginDir string) error {
	hooksDir := filepath.Join(pluginDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	hooksJSONPath := filepath.Join(hooksDir, "hooks.json")

	manifest := make(map[string]any)

	data, err := os.ReadFile(hooksJSONPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading existing hooks.json: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("parsing existing hooks.json: %w", err)
		}
	}

	hooks, ok := manifest["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		manifest["hooks"] = hooks
	}

	hooks["PostToolUse"] = []any{
		map[string]any{
			"matcher": "Edit|Write",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "${CLAUDE_PLUGIN_ROOT}/hooks/format-file",
					"timeout": float64(30),
				},
			},
		},
	}

	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks.json: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(hooksJSONPath, data, 0o644); err != nil {
		return fmt.Errorf("writing hooks.json: %w", err)
	}

	scriptPath := filepath.Join(hooksDir, "format-file")
	if err := os.WriteFile(scriptPath, []byte(formatScript), 0o755); err != nil {
		return fmt.Errorf("writing format-file: %w", err)
	}

	return nil
}
