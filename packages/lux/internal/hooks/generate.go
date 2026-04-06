package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const postToolUseScript = `#!/usr/bin/env bash
set -euo pipefail
input=$(cat)
file_path=$(jq -r '.tool_input.file_path // empty' <<< "$input")
session_id=$(jq -r '.session_id // empty' <<< "$input")
if [[ -n "$file_path" && -n "$session_id" ]]; then
  state_dir="${XDG_STATE_HOME:-${HOME}/.local/state}/lux"
  mkdir -p "$state_dir"
  printf '%s\n' "$file_path" >> "$state_dir/edited-files-${session_id}"
fi
`

const stopFmtScript = `#!/usr/bin/env bash
set -euo pipefail
input=$(cat)
session_id=$(jq -r '.session_id // empty' <<< "$input")
if [[ -z "$session_id" ]]; then
  exit 0
fi
state_file="${XDG_STATE_HOME:-${HOME}/.local/state}/lux/edited-files-${session_id}"
if [[ -f "$state_file" ]]; then
  mapfile -t files < <(sort -u "$state_file")
  if [[ ${#files[@]} -gt 0 ]]; then
    lux fmt-all -- "${files[@]}"
  fi
  rm -f "$state_file"
fi
`

// GenerateHooks adds PostToolUse and Stop hook entries to the hooks directory
// under pluginDir. If hooks/hooks.json already exists (e.g. from go-mcp's
// PreToolUse generation), the new entries are merged in.
//
// PostToolUse Edit|Write accumulates edited file paths to a state file.
// Stop reads the state file and runs lux fmt-all on only those files.
func GenerateHooks(pluginDir string) error {
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

	// PostToolUse: accumulate edited file paths to state file
	hooks["PostToolUse"] = []any{
		map[string]any{
			"matcher": "Edit|Write",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "${CLAUDE_PLUGIN_ROOT}/hooks/post-tool-use",
					"timeout": float64(5),
				},
			},
		},
	}

	// Stop: format only the accumulated files
	hooks["Stop"] = []any{
		map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "${CLAUDE_PLUGIN_ROOT}/hooks/stop-fmt",
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

	// Write PostToolUse accumulator script
	postToolUsePath := filepath.Join(hooksDir, "post-tool-use")
	if err := os.WriteFile(postToolUsePath, []byte(postToolUseScript), 0o755); err != nil {
		return fmt.Errorf("writing post-tool-use script: %w", err)
	}

	// Write Stop formatter script
	stopFmtPath := filepath.Join(hooksDir, "stop-fmt")
	if err := os.WriteFile(stopFmtPath, []byte(stopFmtScript), 0o755); err != nil {
		return fmt.Errorf("writing stop-fmt script: %w", err)
	}

	// Clean up old format-file script
	oldScriptPath := filepath.Join(hooksDir, "format-file")
	if err := os.Remove(oldScriptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing old format-file script: %w", err)
	}

	return nil
}
