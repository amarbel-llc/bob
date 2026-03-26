# Tool-Use Log Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Log every tool invocation during a spinclass session to a JSONL file
at `<worktree>/.claude/tool-use.log` using the PostToolUse hook, controlled by a
sweatfile `tool-use-log` boolean.

**Architecture:** Add `ToolUseLog *bool` to the `Hooks` struct in sweatfile,
re-run tommy codegen, add merge logic, conditionally register `PostToolUse` hook
in `apply.go`, and handle the event in `hooks.go` by appending the raw JSON
payload to the log file.

**Tech Stack:** Go stdlib, tommy codegen, TOML sweatfile config.

**Rollback:** `git revert` the commits. Purely additive.

--------------------------------------------------------------------------------

### Task 1: Add `ToolUseLog` Field to Sweatfile Hooks

**Promotion criteria:** N/A

**Files:**

- Modify: `packages/spinclass/internal/sweatfile/sweatfile.go:12-17`
- Modify: `packages/spinclass/internal/sweatfile/hierarchy.go:181-197`
- Regenerate: `packages/spinclass/internal/sweatfile/sweatfile_tommy.go`

**Step 1: Add `ToolUseLog` field and accessor to `sweatfile.go`**

In `packages/spinclass/internal/sweatfile/sweatfile.go`, add `ToolUseLog` to the
`Hooks` struct:

``` go
type Hooks struct {
    Create               *string `toml:"create"`
    Stop                 *string `toml:"stop"`
    PreMerge             *string `toml:"pre-merge"`
    DisallowMainWorktree *bool   `toml:"disallow-main-worktree"`
    ToolUseLog           *bool   `toml:"tool-use-log"`
}
```

Add an accessor method after `DisallowMainWorktreeEnabled()`:

``` go
func (sf Sweatfile) ToolUseLogEnabled() bool {
    return sf.Hooks != nil &&
        sf.Hooks.ToolUseLog != nil &&
        *sf.Hooks.ToolUseLog
}
```

**Step 2: Add merge logic in `hierarchy.go`**

In `packages/spinclass/internal/sweatfile/hierarchy.go`, inside the
`if other.Hooks != nil` block (after the `DisallowMainWorktree` merge at line
195), add:

``` go
        if other.Hooks.ToolUseLog != nil {
            merged.Hooks.ToolUseLog = other.Hooks.ToolUseLog
        }
```

**Step 3: Re-run tommy codegen**

Run from the sweatfile package directory:

    cd packages/spinclass/internal/sweatfile && nix develop <root> --command go generate ./...

This regenerates `sweatfile_tommy.go` to include `ToolUseLog` decode/encode.

**Step 4: Write merge test**

Append to `packages/spinclass/internal/sweatfile/sweatfile_test.go`:

``` go
func TestMergeToolUseLogInherit(t *testing.T) {
    enabled := true
    base := Sweatfile{Hooks: &Hooks{ToolUseLog: &enabled}}
    overlay := Sweatfile{}
    merged := base.MergeWith(overlay)
    if !merged.ToolUseLogEnabled() {
        t.Error("expected ToolUseLog to be inherited")
    }
}

func TestMergeToolUseLogOverride(t *testing.T) {
    enabled := true
    disabled := false
    base := Sweatfile{Hooks: &Hooks{ToolUseLog: &enabled}}
    overlay := Sweatfile{Hooks: &Hooks{ToolUseLog: &disabled}}
    merged := base.MergeWith(overlay)
    if merged.ToolUseLogEnabled() {
        t.Error("expected ToolUseLog to be overridden to false")
    }
}

func TestParseToolUseLog(t *testing.T) {
    doc, err := Parse([]byte("[hooks]\ntool-use-log = true\n"))
    if err != nil {
        t.Fatalf("parse error: %v", err)
    }
    if !doc.Data().ToolUseLogEnabled() {
        t.Error("expected ToolUseLog to be true")
    }
    undecoded := doc.Undecoded()
    for _, key := range undecoded {
        if key == "hooks.tool-use-log" {
            t.Error("tool-use-log should be decoded, not undecoded")
        }
    }
}
```

**Step 5: Run tests**

Run: `nix develop --command go test ./packages/spinclass/internal/sweatfile/...`
Expected: PASS

**Step 6: Commit**

``` bash
git add packages/spinclass/internal/sweatfile/sweatfile.go \
       packages/spinclass/internal/sweatfile/hierarchy.go \
       packages/spinclass/internal/sweatfile/sweatfile_tommy.go \
       packages/spinclass/internal/sweatfile/sweatfile_test.go
git commit -m "feat(spinclass): add tool-use-log sweatfile config field"
```

--------------------------------------------------------------------------------

### Task 2: Register PostToolUse Hook in `apply.go`

**Promotion criteria:** N/A

**Files:**

- Modify: `packages/spinclass/internal/sweatfile/apply.go:236-265`

**Step 1: Add PostToolUse registration**

In `packages/spinclass/internal/sweatfile/apply.go`, after the Stop hook
registration block (after line 263), add:

``` go
        if sweatfile.ToolUseLogEnabled() {
            hooksMap["PostToolUse"] = []any{
                map[string]any{
                    "matcher": "*",
                    "hooks": []any{
                        map[string]any{
                            "type":    "command",
                            "command": "spinclass hooks",
                        },
                    },
                },
            }
        }
```

**Step 2: Run tests**

Run: `nix develop --command go test ./packages/spinclass/internal/sweatfile/...`
Expected: PASS

**Step 3: Commit**

``` bash
git add packages/spinclass/internal/sweatfile/apply.go
git commit -m "feat(spinclass): register PostToolUse hook when tool-use-log enabled"
```

--------------------------------------------------------------------------------

### Task 3: Handle PostToolUse in `hooks.go`

**Promotion criteria:** N/A

**Files:**

- Modify: `packages/spinclass/internal/hooks/hooks.go:29-34`
- Modify: `packages/spinclass/internal/hooks/hooks_test.go`

**Step 1: Write test for PostToolUse logging**

Append to `packages/spinclass/internal/hooks/hooks_test.go`:

``` go
func TestPostToolUseWritesLog(t *testing.T) {
    worktree := t.TempDir()
    claudeDir := filepath.Join(worktree, ".claude")
    os.MkdirAll(claudeDir, 0o755)

    input, _ := json.Marshal(map[string]any{
        "hook_event_name": "PostToolUse",
        "session_id":      "test-session",
        "tool_name":       "Edit",
        "tool_input":      map[string]any{"file_path": "/some/file.go"},
        "cwd":             worktree,
    })

    var out bytes.Buffer
    err := Run(bytes.NewReader(input), &out, "", "", false)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // PostToolUse produces no stdout
    if out.Len() != 0 {
        t.Errorf("expected no output, got %q", out.String())
    }

    // Log file should exist with the payload
    logPath := filepath.Join(claudeDir, "tool-use.log")
    data, err := os.ReadFile(logPath)
    if err != nil {
        t.Fatalf("expected log file at %s: %v", logPath, err)
    }

    lines := strings.Split(strings.TrimSpace(string(data)), "\n")
    if len(lines) != 1 {
        t.Fatalf("expected 1 log line, got %d", len(lines))
    }

    var logged map[string]any
    if err := json.Unmarshal([]byte(lines[0]), &logged); err != nil {
        t.Fatalf("expected valid JSON log line: %v", err)
    }
    if logged["tool_name"] != "Edit" {
        t.Errorf("expected tool_name Edit, got %v", logged["tool_name"])
    }
}

func TestPostToolUseAppendsToLog(t *testing.T) {
    worktree := t.TempDir()
    claudeDir := filepath.Join(worktree, ".claude")
    os.MkdirAll(claudeDir, 0o755)

    // Write two tool uses
    for _, tool := range []string{"Edit", "Bash"} {
        input, _ := json.Marshal(map[string]any{
            "hook_event_name": "PostToolUse",
            "session_id":      "test-session",
            "tool_name":       tool,
            "tool_input":      map[string]any{},
            "cwd":             worktree,
        })
        var out bytes.Buffer
        if err := Run(bytes.NewReader(input), &out, "", "", false); err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
    }

    logPath := filepath.Join(claudeDir, "tool-use.log")
    data, err := os.ReadFile(logPath)
    if err != nil {
        t.Fatalf("expected log file: %v", err)
    }

    lines := strings.Split(strings.TrimSpace(string(data)), "\n")
    if len(lines) != 2 {
        t.Fatalf("expected 2 log lines, got %d", len(lines))
    }
}

func TestPostToolUseNoClaudeDirIsSilent(t *testing.T) {
    // CWD with no .claude/ directory — should silently succeed
    cwd := t.TempDir()

    input, _ := json.Marshal(map[string]any{
        "hook_event_name": "PostToolUse",
        "session_id":      "test-session",
        "tool_name":       "Read",
        "tool_input":      map[string]any{},
        "cwd":             cwd,
    })

    var out bytes.Buffer
    err := Run(bytes.NewReader(input), &out, "", "", false)
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
}

func TestPostToolUseSubdirFindsClaudeDir(t *testing.T) {
    worktree := t.TempDir()
    claudeDir := filepath.Join(worktree, ".claude")
    os.MkdirAll(claudeDir, 0o755)

    // CWD is a subdirectory of the worktree
    subdir := filepath.Join(worktree, "src", "pkg")
    os.MkdirAll(subdir, 0o755)

    input, _ := json.Marshal(map[string]any{
        "hook_event_name": "PostToolUse",
        "session_id":      "test-session",
        "tool_name":       "Grep",
        "tool_input":      map[string]any{},
        "cwd":             subdir,
    })

    var out bytes.Buffer
    err := Run(bytes.NewReader(input), &out, "", "", false)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    logPath := filepath.Join(claudeDir, "tool-use.log")
    if _, err := os.Stat(logPath); os.IsNotExist(err) {
        t.Fatal("expected log file to be created when CWD is a subdirectory")
    }
}
```

**Step 2: Run tests to verify they fail**

Run:
`nix develop --command go test -run 'TestPostToolUse' ./packages/spinclass/internal/hooks/`
Expected: FAIL --- PostToolUse case not handled (falls through to PreToolUse
handler which is a no-op, so tests fail on missing log file).

**Step 3: Implement PostToolUse handler**

In `packages/spinclass/internal/hooks/hooks.go`, update the `Run` function's
switch to add the PostToolUse case. Change lines 29-34 to:

``` go
    switch input.HookEventName {
    case "Stop":
        return runStopHook(input, w)
    case "PostToolUse":
        return runPostToolUseLog(input)
    default:
        return runPreToolUse(input, w, mainRepoRoot, sessionWorktree, disallowMainWorktree)
    }
```

Add the implementation at the end of `hooks.go`:

``` go
// runPostToolUseLog appends the raw hook payload as a JSONL line to the
// tool-use log in the worktree's .claude/ directory. Fails silently — a
// logging failure must never block Claude.
func runPostToolUseLog(input hookInput) error {
    claudeDir := findClaudeDir(input.CWD)
    if claudeDir == "" {
        return nil
    }

    logPath := filepath.Join(claudeDir, "tool-use.log")

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

// findClaudeDir walks up from dir looking for a .claude/ directory containing
// settings.local.json. Returns the .claude/ path or empty string if not found.
func findClaudeDir(dir string) string {
    current := filepath.Clean(dir)
    for {
        candidate := filepath.Join(current, ".claude")
        if _, err := os.Stat(filepath.Join(candidate, "settings.local.json")); err == nil {
            return candidate
        }

        parent := filepath.Dir(current)
        if parent == current {
            return ""
        }
        current = parent
    }
}
```

**Step 4: Run tests to verify they pass**

Run:
`nix develop --command go test -run 'TestPostToolUse' ./packages/spinclass/internal/hooks/`
Expected: FAIL --- `TestPostToolUseSubdirFindsClaudeDir` will fail because
`findClaudeDir` looks for `settings.local.json` but the test only creates
`.claude/` without that file.

**Step 5: Fix tests --- add `settings.local.json` to test fixtures**

In each test that creates a `.claude/` directory, also create the sentinel file.
Add after each `os.MkdirAll(claudeDir, 0o755)` line:

``` go
    os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte("{}"), 0o644)
```

This applies to `TestPostToolUseWritesLog`, `TestPostToolUseAppendsToLog`, and
`TestPostToolUseSubdirFindsClaudeDir`.

**Step 6: Run all hooks tests**

Run: `nix develop --command go test ./packages/spinclass/internal/hooks/`
Expected: PASS

**Step 7: Commit**

``` bash
git add packages/spinclass/internal/hooks/hooks.go \
       packages/spinclass/internal/hooks/hooks_test.go
git commit -m "feat(spinclass): log tool invocations via PostToolUse hook"
```

--------------------------------------------------------------------------------

### Task 4: Build and Manual Integration Test

**Promotion criteria:** N/A

**Files:** None (testing only)

**Step 1: Build spinclass**

Run: `nix build .#spinclass`

**Step 2: Verify hook registration**

Create a test sweatfile with `tool-use-log = true` and verify that
`ApplyClaudeSettings` produces a `settings.local.json` containing a
`PostToolUse` hook entry. This can be verified by reading the generated file:

``` bash
# In a test worktree with a sweatfile containing:
# [hooks]
# tool-use-log = true
cat .claude/settings.local.json | jq '.hooks.PostToolUse'
```

Expected: non-null PostToolUse hook array with `spinclass hooks` command.
