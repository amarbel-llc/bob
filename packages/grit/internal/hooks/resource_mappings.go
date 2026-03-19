package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

type resourceMapping struct {
	commandPrefix string
	resourceURI   string
	description   string
}

var resourceMappings = []resourceMapping{
	{"git status", "grit://status", "working tree status"},
	{"git branch", "grit://branches", "branch listing"},
	{"git remote", "grit://remotes", "remote listing"},
	{"git tag", "grit://tags", "tag listing"},
	{"git log", "grit://log", "commit history"},
	{"git show", "grit://commits/{ref}", "commit detail"},
	{"git blame", "grit://blame/{path}", "line authorship"},
}

type hookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

// HandleResourceHook checks whether a hook input matches a resource mapping.
// Returns (true, nil) if the hook was handled (deny written to w),
// (false, nil) if no match (caller should fall through to tool mappings).
// Follows fail-open: parse errors return (false, nil).
func HandleResourceHook(input []byte, w io.Writer) (bool, error) {
	var hi hookInput
	if err := json.Unmarshal(input, &hi); err != nil {
		log.Printf("resource hook: ignoring decode error (fail-open): %v", err)
		return false, nil
	}

	if hi.ToolName != "Bash" {
		return false, nil
	}

	command, _ := hi.ToolInput["command"].(string)
	if command == "" {
		return false, nil
	}

	commands := extractSimpleCommands(command)
	normalized := make([]string, len(commands))
	for i, cmd := range commands {
		normalized[i] = normalizeGitCommand(cmd)
	}

	for _, mapping := range resourceMappings {
		for _, cmd := range normalized {
			if strings.HasPrefix(cmd, mapping.commandPrefix) {
				return true, writeDeny(w, mapping)
			}
		}
	}

	return false, nil
}

func writeDeny(w io.Writer, m resourceMapping) error {
	reason := fmt.Sprintf(
		"Read the %s resource instead (%s).\nSubagents: use mcp__plugin_grit_grit__resource-read with uri %s",
		m.resourceURI, m.description, m.resourceURI,
	)

	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}

	return json.NewEncoder(w).Encode(output)
}
