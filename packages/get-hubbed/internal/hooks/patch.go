package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func PatchHooksMatcher(dir string, extraMatcher string) error {
	hooksPath := filepath.Join(dir, "hooks", "hooks.json")

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		return fmt.Errorf("reading hooks.json: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing hooks.json: %w", err)
	}

	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		return fmt.Errorf("hooks.json missing 'hooks' object")
	}

	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		return fmt.Errorf("hooks.json missing 'PreToolUse' array")
	}

	for _, raw := range preToolUse {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		matcher, ok := entry["matcher"].(string)
		if !ok {
			continue
		}

		if matcherContains(matcher, extraMatcher) {
			continue
		}

		entry["matcher"] = matcher + "|" + extraMatcher
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks.json: %w", err)
	}

	out = append(out, '\n')

	if err := os.WriteFile(hooksPath, out, 0o644); err != nil {
		return fmt.Errorf("writing hooks.json: %w", err)
	}

	return nil
}

func matcherContains(matcher, target string) bool {
	for _, part := range strings.Split(matcher, "|") {
		if part == target {
			return true
		}
	}

	return false
}
