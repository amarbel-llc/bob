# Lux PostToolUse Formatting Hook — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Auto-format files after Claude edits them by adding a PostToolUse hook to lux's generate-plugin output.

**Architecture:** New `internal/hooks/` package in lux with a `GeneratePostToolUseHooks()` function that post-processes the hooks directory created by go-mcp's `GenerateHooks()`. Adds PostToolUse entries to hooks.json and writes a `format-file` shell script. Called from `main.go` after `HandleGeneratePlugin()`.

**Tech Stack:** Go, shell (bash), Claude Code hooks API (PostToolUse event)

**Rollback:** Delete `packages/lux/internal/hooks/` and revert the generate-plugin call in `main.go`. Purely additive.

---

## Reference Files

| File | Role |
|------|------|
| `packages/lux/cmd/lux/main.go` | Entry point — calls `generate-plugin` |
| `packages/lux/cmd/lux/app.go` | `lux fmt` CLI command (lines 206-297) |
| `packages/chix/.claude-plugin/hooks/format-nix` | Reference PostToolUse hook script |
| `go-mcp@v0.0.4/command/generate_hooks.go` | Go-mcp's PreToolUse hook generation |
| `purse-first/skills/claude-plugins/references/claude-plugin-specification.md` | Official hooks.json format |

## hooks.json Format Reference

The official Claude Code plugin hooks.json format (from the spec):

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/format-file",
            "timeout": 30
          }
        ]
      }
    ]
  }
}
```

---

### Task 1: Hook Generation Function

**Files:**
- Create: `packages/lux/internal/hooks/generate.go`
- Test: `packages/lux/internal/hooks/generate_test.go`

**Step 1: Write the failing test — PostToolUse added to empty hooks dir**

Create `packages/lux/internal/hooks/generate_test.go`:

```go
package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePostToolUseHooks_CreatesHooksJSON(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "lux", "hooks")

	if err := GeneratePostToolUseHooks(filepath.Join(dir, "lux")); err != nil {
		t.Fatalf("GeneratePostToolUseHooks: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
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

	postToolUse, ok := hooks["PostToolUse"].([]any)
	if !ok {
		t.Fatal("hooks.json missing 'PostToolUse' key")
	}

	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 PostToolUse entry, got %d", len(postToolUse))
	}

	entry := postToolUse[0].(map[string]any)
	if entry["matcher"] != "Edit|Write" {
		t.Errorf("matcher = %q, want %q", entry["matcher"], "Edit|Write")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/sfriedenberg/eng/repos/bob/.worktrees/lux-push-diag && nix develop --command go test -run TestGeneratePostToolUseHooks_CreatesHooksJSON ./packages/lux/internal/hooks/`

Expected: FAIL — package does not exist

**Step 3: Write the failing test — merges with existing PreToolUse hooks**

Add to `generate_test.go`:

```go
func TestGeneratePostToolUseHooks_MergesWithExisting(t *testing.T) {
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

	if err := GeneratePostToolUseHooks(pluginDir); err != nil {
		t.Fatalf("GeneratePostToolUseHooks: %v", err)
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

	// PreToolUse preserved
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse was lost during merge")
	}

	// PostToolUse added
	if _, ok := hooks["PostToolUse"]; !ok {
		t.Error("PostToolUse was not added")
	}
}
```

**Step 4: Write the failing test — format-file script is written and executable**

Add to `generate_test.go`:

```go
func TestGeneratePostToolUseHooks_WritesFormatScript(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "lux")

	if err := GeneratePostToolUseHooks(pluginDir); err != nil {
		t.Fatalf("GeneratePostToolUseHooks: %v", err)
	}

	scriptPath := filepath.Join(pluginDir, "hooks", "format-file")

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat format-file: %v", err)
	}

	if info.Mode()&0o111 == 0 {
		t.Error("format-file is not executable")
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}

	script := string(content)
	if !strings.Contains(script, "#!/usr/bin/env bash") {
		t.Error("missing shebang")
	}
	if !strings.Contains(script, "lux fmt") {
		t.Error("missing lux fmt invocation")
	}
	if !strings.Contains(script, "file_path") {
		t.Error("missing file_path extraction")
	}
}
```

Add `"strings"` to the imports.

**Step 5: Write minimal implementation**

Create `packages/lux/internal/hooks/generate.go`:

```go
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
[[ -n "$file_path" ]] && lux fmt "$file_path" 2>/dev/null || true
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

	if data, err := os.ReadFile(hooksJSONPath); err == nil {
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

	data, err := json.MarshalIndent(manifest, "", "  ")
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
```

**Step 6: Run tests to verify they pass**

Run: `cd /Users/sfriedenberg/eng/repos/bob/.worktrees/lux-push-diag && nix develop --command go test -v -run TestGeneratePostToolUseHooks ./packages/lux/internal/hooks/`

Expected: all 3 tests PASS

**Step 7: Commit**

```
feat(lux): add PostToolUse hook generation for auto-formatting

Adds internal/hooks package with GeneratePostToolUseHooks() that writes
PostToolUse hook entries to hooks.json and a format-file shell script.
Merges with existing PreToolUse hooks if present.
```

---

### Task 2: Wire Into generate-plugin

**Files:**
- Modify: `packages/lux/cmd/lux/main.go:25-32`

**Step 1: Add the post-processing call**

After `app.HandleGeneratePlugin(os.Args[2:], os.Stdout)`, add the PostToolUse
hook generation. The generate-plugin output directory is determined by the args:
0 args = ".", 1 arg = the given path. We need to match this logic.

Modify `packages/lux/cmd/lux/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/amarbel-llc/lux/internal/hooks"
	"github.com/amarbel-llc/lux/internal/logfile"
	"github.com/amarbel-llc/lux/internal/tools"
)

var version = "dev"

func main() {
	cleanup := logfile.Init()
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := buildApp()

	if len(os.Args) >= 2 && os.Args[1] == "generate-plugin" {
		tools.RegisterAll(app, nil)
		if err := app.HandleGeneratePlugin(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(logfile.Writer(), "Error: %v\n", err)
			os.Exit(1)
		}

		// Add PostToolUse formatting hook to generated artifacts.
		// HandleGeneratePlugin writes to "." (0 remaining args) or the
		// given directory (1 remaining arg, skip "-" which is stdout-only).
		outDir := "."
		remaining := os.Args[2:]
		for _, a := range remaining {
			if a == "--skills-dir" || a == "-skills-dir" {
				remaining = remaining[2:] // skip flag + value
				break
			}
		}
		if len(remaining) == 1 && remaining[0] != "-" {
			outDir = remaining[0]
		}
		if outDir != "-" {
			pluginDir := filepath.Join(outDir, "share", "purse-first", "lux")
			if err := hooks.GeneratePostToolUseHooks(pluginDir); err != nil {
				fmt.Fprintf(logfile.Writer(), "Error generating PostToolUse hooks: %v\n", err)
				os.Exit(1)
			}
		}

		return
	}

	if err := app.RunCLI(ctx, os.Args[1:], nil); err != nil {
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintf(logfile.Writer(), "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 2: Build and verify generate-plugin output**

Run: `cd /Users/sfriedenberg/eng/repos/bob/.worktrees/lux-push-diag && nix develop --command go build -o build/lux ./packages/lux/cmd/lux && build/lux generate-plugin build/test-output`

Then verify:

Run: `jq . build/test-output/share/purse-first/lux/hooks/hooks.json`

Expected: JSON with `PostToolUse` entry containing matcher `Edit|Write` and
command `${CLAUDE_PLUGIN_ROOT}/hooks/format-file`.

Run: `cat build/test-output/share/purse-first/lux/hooks/format-file`

Expected: bash script with `lux fmt` invocation.

Run: `test -x build/test-output/share/purse-first/lux/hooks/format-file && echo executable`

Expected: `executable`

**Step 3: Commit**

```
feat(lux): wire PostToolUse hook generation into generate-plugin

After HandleGeneratePlugin writes standard artifacts, add PostToolUse
formatting hook entries to hooks.json and write the format-file script.
```

---

### Task 3: Nix Build and Plugin Validation

**Step 1: Build with Nix**

Run: `cd /Users/sfriedenberg/eng/repos/bob/.worktrees/lux-push-diag && nix build .#lux`

Expected: successful build

**Step 2: Verify hooks in Nix output**

Run: `jq . result/share/purse-first/lux/hooks/hooks.json`

Expected: hooks.json with PostToolUse entry.

Run: `cat result/share/purse-first/lux/hooks/format-file`

Expected: format-file script.

**Step 3: Validate plugin**

Run: `claude plugin validate result/share/purse-first/lux/.claude-plugin/plugin.json`

Expected: validation passes

**Step 4: Run existing tests**

Run: `cd /Users/sfriedenberg/eng/repos/bob/.worktrees/lux-push-diag && just test-lux`

Expected: all existing tests still pass

**Step 5: Clean up build artifacts**

Run: `rm -rf build/lux build/test-output`

---

### Task 4: Design Doc Commit

**Step 1: Commit design and plan docs**

```
docs: add design and plan for lux PostToolUse formatting hook
```
