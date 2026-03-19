# Resource-Aware Hook Mappings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Intercept git read-only bash commands in grit's PreToolUse hook and deny them with a message pointing to the corresponding MCP resource.

**Architecture:** New `internal/hooks/` package in grit with shell parsing (duplicated from go-mcp), git command normalization, and a resource mapping table. The hook handler in `main.go` buffers stdin, tries resource mappings first, then falls through to `app.HandleHook()` for existing tool mappings.

**Tech Stack:** Go, `mvdan.cc/sh/v3` (already indirect dep in grit)

**Rollback:** Revert `main.go` to call `app.HandleHook(os.Stdin, os.Stdout)` directly. Purely additive — no existing behavior modified.

---

### Task 1: Shell Parsing and Git Normalization

**Files:**
- Create: `packages/grit/internal/hooks/shellparse.go`
- Create: `packages/grit/internal/hooks/gitnormalize.go`
- Create: `packages/grit/internal/hooks/shellparse_test.go`
- Create: `packages/grit/internal/hooks/gitnormalize_test.go`

**Step 1: Write the shell parsing function and test**

Create `packages/grit/internal/hooks/shellparse.go`:

```go
package hooks

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// extractSimpleCommands parses a bash command string and returns the
// individual simple commands found in the AST. Handles &&, ||, ;, |,
// subshells, and strips redirections. On parse failure, returns the
// original command string as a single-element slice.
func extractSimpleCommands(command string) []string {
	if command == "" {
		return []string{""}
	}

	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return []string{command}
	}

	var commands []string
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}

		if len(call.Args) == 0 {
			return false
		}

		var parts []string
		printer := syntax.NewPrinter()
		for _, word := range call.Args {
			var sb strings.Builder
			printer.Print(&sb, word)
			parts = append(parts, sb.String())
		}

		commands = append(commands, strings.Join(parts, " "))
		return false
	})

	if len(commands) == 0 {
		return []string{command}
	}

	return commands
}
```

Create `packages/grit/internal/hooks/shellparse_test.go`:

```go
package hooks

import (
	"testing"
)

func TestExtractSimpleCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single command", "git status", []string{"git status"}},
		{"compound and", "git status && git log", []string{"git status", "git log"}},
		{"compound or", "git status || git log", []string{"git status", "git log"}},
		{"pipe", "git log | head", []string{"git log", "head"}},
		{"empty", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSimpleCommands(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d commands, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, cmd := range result {
				if cmd != tt.expected[i] {
					t.Errorf("command[%d]: expected %q, got %q", i, tt.expected[i], cmd)
				}
			}
		})
	}
}
```

**Step 2: Write the git normalization function and test**

Create `packages/grit/internal/hooks/gitnormalize.go`:

```go
package hooks

import "strings"

// normalizeGitCommand strips global git options (-C <path>, --no-pager,
// -c key=val, --git-dir, --work-tree, --bare) from between "git" and
// the subcommand. Non-git commands are returned unchanged.
func normalizeGitCommand(cmd string) string {
	tokens := strings.Fields(cmd)
	if len(tokens) == 0 || tokens[0] != "git" {
		return cmd
	}

	var kept []string
	i := 1
	for i < len(tokens) {
		tok := tokens[i]

		if strings.HasPrefix(tok, "-C=") ||
			strings.HasPrefix(tok, "-c=") ||
			strings.HasPrefix(tok, "--git-dir=") ||
			strings.HasPrefix(tok, "--work-tree=") {
			i++
			continue
		}

		if tok == "-C" || tok == "-c" || tok == "--git-dir" || tok == "--work-tree" {
			i += 2
			continue
		}

		if tok == "--no-pager" || tok == "--bare" {
			i++
			continue
		}

		break
	}

	kept = append(kept, "git")
	kept = append(kept, tokens[i:]...)

	return strings.Join(kept, " ")
}
```

Create `packages/grit/internal/hooks/gitnormalize_test.go`:

```go
package hooks

import (
	"testing"
)

func TestNormalizeGitCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain", "git status", "git status"},
		{"with -C", "git -C /path status", "git status"},
		{"with --git-dir", "git --git-dir=/path status", "git status"},
		{"with -c", "git -c core.pager=cat log", "git log"},
		{"with --no-pager", "git --no-pager log", "git log"},
		{"non-git", "ls -la", "ls -la"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeGitCommand(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
```

**Step 3: Run tests to verify they pass**

Run: `nix develop --command go test ./packages/grit/internal/hooks/...`
Expected: PASS

**Step 4: Commit**

```
git add packages/grit/internal/hooks/
git commit -m "feat(grit): add shell parsing and git normalization for resource hooks"
```

---

### Task 2: Resource Hook Handler

**Files:**
- Create: `packages/grit/internal/hooks/resource_mappings.go`
- Create: `packages/grit/internal/hooks/resource_mappings_test.go`

**Step 1: Write the failing test**

Create `packages/grit/internal/hooks/resource_mappings_test.go`:

```go
package hooks

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func makeHookInput(toolName string, toolInput map[string]any) []byte {
	input := map[string]any{
		"tool_name":  toolName,
		"tool_input": toolInput,
	}
	data, _ := json.Marshal(input)
	return data
}

func TestResourceHookBasicMatch(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git status"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle git status")
	}
	if !strings.Contains(out.String(), "grit://status") {
		t.Errorf("expected grit://status in output, got %q", out.String())
	}
}

func TestResourceHookNormalizedMatch(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git -C /some/path status"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle normalized git status")
	}
	if !strings.Contains(out.String(), "grit://status") {
		t.Errorf("expected grit://status in output, got %q", out.String())
	}
}

func TestResourceHookCompoundCommand(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git status && git log"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected hook to handle compound command")
	}
}

func TestResourceHookNoMatch(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git commit -m 'foo'"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle git commit")
	}
	if out.Len() != 0 {
		t.Errorf("expected no output, got %q", out.String())
	}
}

func TestResourceHookNonBashTool(t *testing.T) {
	input := makeHookInput("Read", map[string]any{"file_path": "/foo/bar"})
	var out bytes.Buffer
	handled, err := HandleResourceHook(input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected hook to not handle non-Bash tool")
	}
}

func TestResourceHookDenyMessageFormat(t *testing.T) {
	input := makeHookInput("Bash", map[string]any{"command": "git status"})
	var out bytes.Buffer
	HandleResourceHook(input, &out)

	output := out.String()
	if !strings.Contains(output, "grit://status") {
		t.Error("deny message should contain resource URI")
	}
	if !strings.Contains(output, "resource-read") {
		t.Error("deny message should contain resource-read tool for subagents")
	}
	if !strings.Contains(output, "deny") {
		t.Error("deny message should contain deny decision")
	}
}

func TestResourceHookAllMappings(t *testing.T) {
	tests := []struct {
		command     string
		resourceURI string
	}{
		{"git status", "grit://status"},
		{"git branch", "grit://branches"},
		{"git branch -a", "grit://branches"},
		{"git remote", "grit://remotes"},
		{"git remote -v", "grit://remotes"},
		{"git tag", "grit://tags"},
		{"git tag --list", "grit://tags"},
		{"git log", "grit://log"},
		{"git log --oneline", "grit://log"},
		{"git show abc123", "grit://commits/{ref}"},
		{"git blame foo.go", "grit://blame/{path}"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			input := makeHookInput("Bash", map[string]any{"command": tt.command})
			var out bytes.Buffer
			handled, err := HandleResourceHook(input, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !handled {
				t.Fatalf("expected hook to handle %q", tt.command)
			}
			if !strings.Contains(out.String(), tt.resourceURI) {
				t.Errorf("expected %s in output, got %q", tt.resourceURI, out.String())
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `nix develop --command go test ./packages/grit/internal/hooks/... -run TestResourceHook`
Expected: FAIL — `HandleResourceHook` not defined

**Step 3: Write the implementation**

Create `packages/grit/internal/hooks/resource_mappings.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `nix develop --command go test ./packages/grit/internal/hooks/... -v`
Expected: PASS

**Step 5: Commit**

```
git add packages/grit/internal/hooks/resource_mappings.go packages/grit/internal/hooks/resource_mappings_test.go
git commit -m "feat(grit): add resource hook handler with mapping table"
```

---

### Task 3: Wire Into main.go

**Files:**
- Modify: `packages/grit/cmd/grit/main.go:1-15` (imports), `packages/grit/cmd/grit/main.go:45-50` (hook handler)

**Step 1: Update main.go to buffer stdin and try resource hook first**

In `packages/grit/cmd/grit/main.go`, add imports `"bytes"`, `"io"`, and `"github.com/friedenberg/grit/internal/hooks"`. Replace lines 45-50:

```go
	if flag.NArg() >= 1 && flag.Arg(0) == "hook" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("reading hook input: %v", err)
		}

		handled, err := hooks.HandleResourceHook(input, os.Stdout)
		if err != nil {
			log.Fatalf("handling resource hook: %v", err)
		}

		if !handled {
			if err := app.HandleHook(bytes.NewReader(input), os.Stdout); err != nil {
				log.Fatalf("handling hook: %v", err)
			}
		}

		return
	}
```

**Step 2: Run all grit tests**

Run: `nix develop --command go test ./packages/grit/...`
Expected: PASS

**Step 3: Manual verification**

Run: `echo '{"tool_name":"Bash","tool_input":{"command":"git status"}}' | nix develop --command go run ./packages/grit/cmd/grit hook`
Expected: JSON output with `"permissionDecision": "deny"` and `grit://status` in the reason.

Run: `echo '{"tool_name":"Bash","tool_input":{"command":"git commit -m test"}}' | nix develop --command go run ./packages/grit/cmd/grit hook`
Expected: JSON output with `"permissionDecision": "deny"` pointing to `mcp__plugin_grit_grit__commit` (existing tool mapping).

**Step 4: Commit**

```
git add packages/grit/cmd/grit/main.go
git commit -m "feat(grit): wire resource hook mappings into PreToolUse handler"
```

---

### Task 4: Build Verification

**Step 1: Run nix build**

Run: `nix build .#grit`
Expected: builds successfully

**Step 2: Run full test suite**

Run: `just test-grit`
Expected: PASS

**Step 3: Commit (if any fixups needed)**

Only if build/test issues required changes.
