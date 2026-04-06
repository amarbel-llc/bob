package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateStopHook adds a Stop hook entry to the hooks directory under
// pluginDir. If hooks/hooks.json already exists (e.g. from go-mcp's
// PreToolUse generation), the Stop entry is merged in and any existing
// PostToolUse entry is removed. The old format-file script is deleted
// if present.
func GenerateStopHook(pluginDir string) error {
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

	// Remove old PostToolUse formatter hook
	delete(hooks, "PostToolUse")

	// Add Stop hook
	hooks["Stop"] = []any{
		map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "lux fmt-all",
					"timeout": float64(60),
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

	// Clean up old format-file script
	scriptPath := filepath.Join(hooksDir, "format-file")
	if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing old format-file script: %w", err)
	}

	return nil
}
