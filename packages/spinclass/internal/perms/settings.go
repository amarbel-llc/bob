package perms

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type claudeSettings struct {
	Permissions struct {
		Allow []string `json:"allow"`
	} `json:"permissions"`
}

// LoadClaudeSettings reads the allow list from a Claude settings.local.json
// file. Returns nil and no error when the file does not exist.
func LoadClaudeSettings(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var settings claudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return settings.Permissions.Allow, nil
}

// SaveClaudeSettings writes the allow list back to a Claude settings.local.json
// file, preserving any other top-level keys. Creates parent directories as needed.
func SaveClaudeSettings(path string, rules []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Read existing file to preserve non-permission fields
	var doc map[string]any
	if existing, err := os.ReadFile(path); err == nil {
		json.Unmarshal(existing, &doc)
	}
	if doc == nil {
		doc = make(map[string]any)
	}

	permsMap, _ := doc["permissions"].(map[string]any)
	if permsMap == nil {
		permsMap = make(map[string]any)
	}
	permsMap["allow"] = rules
	doc["permissions"] = permsMap

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')

	return os.WriteFile(path, data, 0o644)
}

// DiffRules returns rules present in after but not in before, preserving the
// order from after.
func DiffRules(before, after []string) []string {
	beforeSet := make(map[string]bool, len(before))
	for _, r := range before {
		beforeSet[r] = true
	}

	var diff []string
	for _, r := range after {
		if !beforeSet[r] {
			diff = append(diff, r)
		}
	}

	if diff == nil {
		diff = []string{}
	}

	return diff
}

// RemoveRules returns rules with toRemove entries filtered out, preserving the
// original order.
func RemoveRules(rules, toRemove []string) []string {
	removeSet := make(map[string]bool, len(toRemove))
	for _, r := range toRemove {
		removeSet[r] = true
	}

	var result []string
	for _, r := range rules {
		if !removeSet[r] {
			result = append(result, r)
		}
	}

	if result == nil {
		result = []string{}
	}

	return result
}

// ComputeReviewableRules returns worktree rules that are not already covered by
// global Claude settings, curated tier files, or auto-injected worktree-scoped
// rules.
func ComputeReviewableRules(
	worktreeSettingsPath, globalSettingsPath, tiersDir, repo, worktreePath string,
) ([]string, error) {
	worktreeRules, err := LoadClaudeSettings(worktreeSettingsPath)
	if err != nil {
		return nil, err
	}

	globalRules, err := LoadClaudeSettings(globalSettingsPath)
	if err != nil {
		return nil, err
	}

	tierRules := LoadTiers(tiersDir, repo)

	exclude := make(map[string]bool)
	for _, r := range globalRules {
		exclude[r] = true
	}
	for _, r := range tierRules {
		exclude[r] = true
	}

	// Auto-injected worktree-scoped rules
	home, _ := os.UserHomeDir()
	if home != "" {
		exclude[fmt.Sprintf("Read(%s/.claude/*)", home)] = true
	}
	if worktreePath != "" {
		exclude[fmt.Sprintf("Read(%s/*)", worktreePath)] = true
		exclude[fmt.Sprintf("Edit(%s/*)", worktreePath)] = true
		exclude[fmt.Sprintf("Write(%s/*)", worktreePath)] = true
	}

	var result []string
	for _, r := range worktreeRules {
		if !exclude[r] {
			result = append(result, r)
		}
	}

	if result == nil {
		result = []string{}
	}

	return result, nil
}

// GlobalClaudeSettingsPath returns the path to the user-level Claude
// settings.local.json file.
func GlobalClaudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.local.json")
}
