# Spinclass start/resume/update-description Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Split `sc attach` into `sc start`, `sc resume`, and
`sc update-description` so users can create new sessions and reattach to
existing ones.

**Architecture:** Rename `attachCmd` to `startCmd` (same behavior). Add
`resumeCmd` that looks up session state by worktree ID or auto-detects from cwd.
Add `updateDescriptionCmd` that modifies session description. Two new lookup
functions in `internal/session/` power the auto-detection.

**Tech Stack:** Go 1.26, Cobra CLI, TAP-14 output

**Rollback:** Rename `startCmd` back to `attachCmd`, remove the two new
commands. Single commit revert.

---

### Task 1: Add session lookup functions

**Files:**

- Modify: `packages/spinclass/internal/session/session.go`
- Modify: `packages/spinclass/internal/session/session_test.go`

**Step 1: Write failing tests for FindByWorktreePath and FindByID**

Add to `session_test.go`:

```go
func TestFindByWorktreePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	s := State{
		PID:          12345,
		SessionState: StateActive,
		RepoPath:     "/home/user/repos/bob",
		WorktreePath: "/home/user/repos/bob/.worktrees/plain-spruce",
		Branch:       "plain-spruce",
		SessionKey:   "bob/plain-spruce",
		Entrypoint:   []string{"/bin/sh"},
		StartedAt:    time.Now().UTC(),
	}
	if err := Write(s); err != nil {
		t.Fatal(err)
	}

	// Exact match
	found, err := FindByWorktreePath(s.WorktreePath)
	if err != nil {
		t.Fatal(err)
	}
	if found.SessionKey != s.SessionKey {
		t.Errorf("SessionKey = %s, want %s", found.SessionKey, s.SessionKey)
	}

	// Subdirectory match
	found, err = FindByWorktreePath(s.WorktreePath + "/src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if found.SessionKey != s.SessionKey {
		t.Errorf("subdirectory: SessionKey = %s, want %s", found.SessionKey, s.SessionKey)
	}

	// No match
	_, err = FindByWorktreePath("/completely/different/path")
	if err == nil {
		t.Error("expected error for non-matching path")
	}
}

func TestFindByID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	s := State{
		PID:          12345,
		SessionState: StateActive,
		RepoPath:     "/home/user/repos/bob",
		WorktreePath: "/home/user/repos/bob/.worktrees/plain-spruce",
		Branch:       "different-branch",
		SessionKey:   "bob/different-branch",
		Entrypoint:   []string{"/bin/sh"},
		StartedAt:    time.Now().UTC(),
	}
	if err := Write(s); err != nil {
		t.Fatal(err)
	}

	// Match by worktree directory name, not branch
	found, err := FindByID("plain-spruce")
	if err != nil {
		t.Fatal(err)
	}
	if found.WorktreePath != s.WorktreePath {
		t.Errorf("WorktreePath = %s, want %s", found.WorktreePath, s.WorktreePath)
	}

	// No match
	_, err = FindByID("nonexistent")
	if err == nil {
		t.Error("expected error for non-matching ID")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
`nix develop --command go test -run 'TestFindBy' ./packages/spinclass/internal/session/`

Expected: compilation error — `FindByWorktreePath` and `FindByID` undefined.

**Step 3: Implement FindByWorktreePath and FindByID**

Add to `session.go`:

```go
// FindByWorktreePath scans all session state files and returns the one
// whose WorktreePath is a prefix of path. Returns an error if no match.
func FindByWorktreePath(path string) (*State, error) {
	states, err := ListAll()
	if err != nil {
		return nil, err
	}
	for i := range states {
		s := &states[i]
		if strings.HasPrefix(path, s.WorktreePath) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no session found for path %s", path)
}

// FindByID scans all session state files and returns the one whose
// WorktreePath ends in /.worktrees/<id>. The id is the worktree directory
// name, which may differ from the git branch.
func FindByID(id string) (*State, error) {
	suffix := "/.worktrees/" + id
	states, err := ListAll()
	if err != nil {
		return nil, err
	}
	for i := range states {
		s := &states[i]
		if strings.HasSuffix(s.WorktreePath, suffix) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no session found for worktree ID %q", id)
}
```

Add `"strings"` to the import block if not already present.

**Step 4: Run tests to verify they pass**

Run:
`nix develop --command go test -run 'TestFindBy' ./packages/spinclass/internal/session/`

Expected: PASS

**Step 5: Commit**

```
feat(spinclass): add FindByWorktreePath and FindByID session lookups

Support resuming sessions by worktree directory name or by auto-detecting
from the current working directory.
```

---

### Task 2: Rename attach to start

**Files:**

- Modify: `packages/spinclass/cmd/spinclass/main.go`

**Step 1: Rename attachCmd to startCmd**

In `main.go`:

- Rename `attachCmd` variable to `startCmd`
- Change `Use` from `"attach [description...]"` to `"start [description...]"`
- Change `Short` to `"Create and start a new worktree session"`
- Update `Long` description to reference `start` instead of `attach`
- Rename `attachMergeOnClose` to `startMergeOnClose` and `attachNoAttach` to
  `startNoAttach` (in the `var` block and all references)
- In `init()`, change `rootCmd.AddCommand(attachCmd)` to
  `rootCmd.AddCommand(startCmd)` and update flag registrations to use `startCmd`

**Step 2: Remove session-resume logic from startCmd**

Delete lines 85-94 from `startCmd.RunE` (the block that checks for existing
sessions and swaps entrypoints). `start` always creates new sessions — it never
resumes.

**Step 3: Run existing tests**

Run: `nix develop --command go test ./packages/spinclass/...`

Expected: PASS (no test references `attachCmd` by name)

**Step 4: Commit**

```
refactor(spinclass): rename attach to start

start always creates new sessions. Removes the session-resume logic that
was inlined in the old attach command — that moves to the new resume
command.
```

---

### Task 3: Add resume command

**Files:**

- Modify: `packages/spinclass/cmd/spinclass/main.go`

**Step 1: Add resumeCmd**

Add after `startCmd`:

```go
var resumeCmd = &cobra.Command{
	Use:   "resume [id]",
	Short: "Resume an existing worktree session",
	Long:  `Resume an existing worktree session. With no arguments, auto-detects the session from the current working directory. With one argument, resumes the session identified by the worktree directory name (the name under .worktrees/).`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format := outputFormat
		if format == "" {
			format = "tap"
		}

		var state *session.State
		var err error

		if len(args) == 1 {
			state, err = session.FindByID(args[0])
		} else {
			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				return cwdErr
			}
			state, err = session.FindByWorktreePath(cwd)
		}
		if err != nil {
			return err
		}

		hierarchy, err := sweatfile.LoadWorktreeHierarchy(
			os.Getenv("HOME"), state.RepoPath, state.WorktreePath,
		)
		if err != nil {
			hierarchy, err = sweatfile.LoadHierarchy(os.Getenv("HOME"), state.RepoPath)
			if err != nil {
				return err
			}
		}

		merged := hierarchy.Merged
		entrypoint := merged.SessionStart()
		if resume := merged.SessionResume(); resume != nil {
			entrypoint = resume
		}

		rp := worktree.ResolvedPath{
			AbsPath:    state.WorktreePath,
			RepoPath:   state.RepoPath,
			SessionKey: state.SessionKey,
			Branch:     state.Branch,
		}

		exec := executor.SessionExecutor{
			Entrypoint: entrypoint,
		}

		return shop.Attach(
			os.Stdout,
			exec,
			rp,
			format,
			false, // mergeOnClose
			false, // noAttach
			verbose,
		)
	},
}
```

Register in `init()`:

```go
rootCmd.AddCommand(resumeCmd)
```

**Step 2: Build and verify**

Run: `nix develop --command go build ./packages/spinclass/cmd/spinclass/`

Expected: compiles without errors.

**Step 3: Commit**

```
feat(spinclass): add resume command for reattaching to existing sessions

Looks up session state by worktree ID or auto-detects from cwd. Uses
SessionResume entrypoint if defined in sweatfile, falls back to
SessionStart.
```

---

### Task 4: Add update-description command

**Files:**

- Modify: `packages/spinclass/cmd/spinclass/main.go`

**Step 1: Add updateDescriptionCmd**

```go
var updateDescriptionID string

var updateDescriptionCmd = &cobra.Command{
	Use:   "update-description [description...]",
	Short: "Update the description of a session",
	Long:  `Update the freeform description of an existing session. With --id, targets a specific worktree by directory name. Without --id, auto-detects from the current working directory.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var state *session.State
		var err error

		if updateDescriptionID != "" {
			state, err = session.FindByID(updateDescriptionID)
		} else {
			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				return cwdErr
			}
			state, err = session.FindByWorktreePath(cwd)
		}
		if err != nil {
			return err
		}

		state.Description = strings.Join(args, " ")
		return session.Write(*state)
	},
}
```

Add `"strings"` to the import block in `main.go`.

Register in `init()`:

```go
updateDescriptionCmd.Flags().StringVar(
	&updateDescriptionID,
	"id",
	"",
	"worktree ID to update (auto-detects from cwd if omitted)",
)
rootCmd.AddCommand(updateDescriptionCmd)
```

**Step 2: Build and verify**

Run: `nix develop --command go build ./packages/spinclass/cmd/spinclass/`

Expected: compiles without errors.

**Step 3: Commit**

```
feat(spinclass): add update-description command

Updates the freeform description on an existing session state file.
Auto-detects session from cwd or accepts --id for explicit targeting.
```

---

### Task 5: Update completions for resume

**Files:**

- Modify: `packages/spinclass/internal/completions/completions.go`
- Modify: `packages/spinclass/cmd/spinclass/main.go`

**Step 1: Update Sessions() to output worktree ID instead of branch**

In `completions.go`, change the `fmt.Fprintf` line in `Sessions()` from:

```go
fmt.Fprintf(w, "%s\t%s\n", s.Branch, label)
```

to:

```go
wtID := filepath.Base(s.WorktreePath)
fmt.Fprintf(w, "%s\t%s\n", wtID, label)
```

**Step 2: Wire completions into resumeCmd**

Add a `ValidArgsFunction` to `resumeCmd` that calls `completions.Sessions` and
parses the output for cobra completion. Alternatively, register a custom
completion for the first positional arg.

**Step 3: Run tests**

Run: `nix develop --command go test ./packages/spinclass/...`

Expected: PASS

**Step 4: Commit**

```
feat(spinclass): update completions for resume command

Completions now output worktree directory name (ID) instead of branch
name, matching the resume command's argument semantics.
```

---

### Task 6: Update documentation

**Files:**

- Modify: `packages/spinclass/CLAUDE.md`

**Step 1: Update CLI commands table**

Replace the `sc attach [name]` row with:

```
  sc start [desc...]        Create and start a new worktree session
  sc resume [id]            Resume an existing session (auto-detects from cwd)
  sc update-description     Update session description (--id or auto-detect)
```

**Step 2: Commit**

```
docs(spinclass): update CLI table for start/resume/update-description
```
