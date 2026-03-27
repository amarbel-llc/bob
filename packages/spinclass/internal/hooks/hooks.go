package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/amarbel-llc/spinclass/internal/sweatfile"
)

type hookInput struct {
	HookEventName string         `json:"hook_event_name"`
	SessionID     string         `json:"session_id"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	CWD           string         `json:"cwd"`
}

func Run(r io.Reader, w io.Writer, mainRepoRoot, sessionWorktree string, disallowMainWorktree bool) error {
	var input hookInput
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		return fmt.Errorf("decoding hook input: %w", err)
	}

	switch input.HookEventName {
	case "Stop":
		return runStopHook(input, w)
	case "PostToolUse":
		return runPostToolUseLog(input)
	default:
		return runPreToolUse(input, w, mainRepoRoot, sessionWorktree, disallowMainWorktree)
	}
}

func runStopHook(input hookInput, w io.Writer) error {
	tmpDir := os.TempDir()
	sentinelPath := filepath.Join(tmpDir, "stop-hook-"+input.SessionID)

	if _, err := os.Stat(sentinelPath); err == nil {
		return nil // second invocation -> approve
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil // can't load sweatfile -> approve
	}

	result, err := sweatfile.LoadHierarchy(home, input.CWD)
	stopCmd := result.Merged.StopHookCommand()
	if err != nil || stopCmd == nil || *stopCmd == "" {
		return nil // no stop hook configured -> approve
	}

	cmd := exec.Command("sh", "-c", *stopCmd)
	cmd.Dir = input.CWD
	output, cmdErr := cmd.CombinedOutput()

	if cmdErr == nil {
		return nil // command passed -> approve
	}

	// Command failed -> write output to sentinel and block
	os.WriteFile(sentinelPath, output, 0o644)

	reason := fmt.Sprintf("stop hook failed: %s", *stopCmd)
	systemMsg := fmt.Sprintf(
		"Stop hook failed. Output written to %s. Review the failures and address them before completing.",
		sentinelPath,
	)

	decision := map[string]any{
		"decision":      "block",
		"reason":        reason,
		"systemMessage": systemMsg,
	}

	return json.NewEncoder(w).Encode(decision)
}

func runPreToolUse(input hookInput, w io.Writer, mainRepoRoot, sessionWorktree string, disallowMainWorktree bool) error {
	if !disallowMainWorktree || mainRepoRoot == "" {
		return nil
	}

	mainRepoRoot = resolvePath(mainRepoRoot)
	sessionWorktree = resolvePath(sessionWorktree)

	paths := extractPaths(input)
	if paths == nil {
		return nil
	}

	for _, p := range paths {
		if isInsideMainWorktree(p, mainRepoRoot, sessionWorktree) {
			output := map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":      "PreToolUse",
					"permissionDecision": "deny",
					"permissionDecisionReason": fmt.Sprintf(
						"Path %s is in the main worktree (%s). Restrict operations to the session worktree (%s).",
						p, mainRepoRoot, sessionWorktree,
					),
				},
			}
			return json.NewEncoder(w).Encode(output)
		}
	}

	return nil
}

func extractPaths(input hookInput) []string {
	switch input.ToolName {
	case "Read", "Write", "Edit":
		if fp, ok := input.ToolInput["file_path"].(string); ok && fp != "" {
			return []string{fp}
		}
	case "Glob", "Grep", "Find":
		if p, ok := input.ToolInput["path"].(string); ok && p != "" {
			return []string{p}
		}
	case "Bash":
		return extractAbsolutePathsFromCommand(input)
	case "Task":
		return nil
	}
	return nil
}

func extractAbsolutePathsFromCommand(input hookInput) []string {
	cmd, ok := input.ToolInput["command"].(string)
	if !ok || cmd == "" {
		return nil
	}

	var paths []string
	for _, token := range strings.Fields(cmd) {
		if strings.HasPrefix(token, "/") {
			paths = append(paths, token)
		}
	}
	return paths
}

func resolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}

	// Path doesn't fully exist — walk up until we find an existing ancestor,
	// resolve symlinks there, then re-append the non-existent suffix.
	cleaned := filepath.Clean(path)
	var suffix []string
	current := cleaned
	for {
		parent := filepath.Dir(current)
		suffix = append(suffix, filepath.Base(current))
		if parent == current {
			break
		}
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			// Reverse suffix and join.
			for i, j := 0, len(suffix)-1; i < j; i, j = i+1, j-1 {
				suffix[i], suffix[j] = suffix[j], suffix[i]
			}
			return filepath.Join(append([]string{resolved}, suffix...)...)
		}
		current = parent
	}

	return cleaned
}

func isInsideMainWorktree(path, mainRepoRoot, sessionWorktree string) bool {
	resolved := resolvePath(path)

	if sessionWorktree != "" &&
		(resolved == sessionWorktree || strings.HasPrefix(resolved, sessionWorktree+string(filepath.Separator))) {
		return false
	}

	return resolved == mainRepoRoot || strings.HasPrefix(resolved, mainRepoRoot+string(filepath.Separator))
}

// runPostToolUseLog appends the raw hook payload as a JSONL line to the
// tool-use log in the worktree's .spinclass/ directory. Fails silently — a
// logging failure must never block Claude.
func runPostToolUseLog(input hookInput) error {
	spinclassDir := findSpinclassDir(input.CWD)
	if spinclassDir == "" {
		return nil
	}

	logPath := filepath.Join(spinclassDir, "tool-use.log")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil // fail silently
	}
	defer f.Close()

	data, err := json.Marshal(input)
	if err != nil {
		return nil
	}

	data = append(data, '\n')
	f.Write(data)

	return nil
}

// findSpinclassDir walks up from dir looking for a .spinclass/ directory.
// Returns the .spinclass/ path or empty string if not found.
func findSpinclassDir(dir string) string {
	current := filepath.Clean(dir)
	for {
		candidate := filepath.Join(current, ".spinclass")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}
