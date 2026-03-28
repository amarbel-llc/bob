# Spinclass Session Entrypoint Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Replace zmx dependency with a sweatfile-configured session entrypoint
and XDG_STATE-based session tracking, hard-forked as `spinclass2`/`sc-dev`.

**Architecture:** Fork `packages/spinclass` → `packages/spinclass2` with new
module path `github.com/amarbel-llc/spinclass2`. Replace `ZmxExecutor` with
`SessionExecutor` that execs the entrypoint from `[session]` sweatfile table.
Session state tracked in `~/.local/state/spinclass/sessions/<hash>-state.json`.
Rename `new` → `attach`, merge `status` into `list`.

**Tech Stack:** Go 1.26, Cobra CLI, TOML (tommy), TAP-14 output, Nix
(mkGoWorkspaceModule)

**Rollback:** Delete `packages/spinclass2/` and its references in `go.work`,
`flake.nix`, `justfile`. Original `packages/spinclass/` is untouched throughout.

**FDR:** `docs/features/0002-spinclass-session-entrypoint.md`

--------------------------------------------------------------------------------

### Task 1: Hard fork spinclass → spinclass2

**Files:** - Create: `packages/spinclass2/` (full copy of
`packages/spinclass/`) - Modify: `packages/spinclass2/go.mod` (change module
path) - Modify: `go.work` (add `./packages/spinclass2`)

**Step 1: Copy the package**

``` bash
cp -r packages/spinclass packages/spinclass2
```

**Step 2: Update the Go module path**

In `packages/spinclass2/go.mod`, change:

    module github.com/amarbel-llc/spinclass

to:

    module github.com/amarbel-llc/spinclass2

**Step 3: Update all internal imports**

Find-and-replace across all `.go` files in `packages/spinclass2/`:

    github.com/amarbel-llc/spinclass/internal/ → github.com/amarbel-llc/spinclass2/internal/

**Step 4: Add to go.work**

Add `./packages/spinclass2` to the `use` block in `go.work`.

**Step 5: Verify it compiles**

Run: `nix develop --command go build ./packages/spinclass2/...` Expected: builds
without errors.

**Step 6: Commit**

    feat(spinclass2): hard fork spinclass for session entrypoint work

--------------------------------------------------------------------------------

### Task 2: Nix build for spinclass2

**Files:** - Create: `lib/packages/spinclass2.nix` - Modify: `flake.nix` (add
spinclass2Pkg build + export) - Modify: `justfile` (add build-spinclass2,
test-spinclass2)

**Step 1: Create `lib/packages/spinclass2.nix`**

Copy `lib/packages/spinclass.nix` and change: - `pname = "spinclass2"` -
`subPackages = [ "packages/spinclass2/cmd/spinclass" ]` - Completion file paths:
`spinclass2.bash-completion`, `spinclass2.fish`, `sc-dev.bash-completion`,
`sc-dev.fish` - Symlink: `ln -s spinclass2 $out/bin/sc-dev` (instead of `sc`)

Note: completion files don't exist yet --- skip the `shellCompletions`
derivation for now (just build the binary). Completions will be added in a later
task after the CLI is reworked.

**Step 2: Add to `flake.nix`**

In `buildPackages`, add:

``` nix
spinclass2Pkg = import ./lib/packages/spinclass2.nix {
  inherit pkgs goWorkspaceSrc goVendorHash go;
  src = ./packages/spinclass2;
};
```

Add to `nonPluginPkgs` list and named exports:

``` nix
spinclass2 = localPkgs.spinclass2Pkg;
```

**Step 3: Add justfile targets**

``` just
build-spinclass2:
    nix build .#spinclass2

test-spinclass2:
    {{cmd_nix_dev}} {{tap-dancer-go-test}} ./packages/spinclass2/...
```

**Step 4: Build it**

Run: `just build-spinclass2` Expected: builds successfully,
`result/bin/spinclass2` and `result/bin/sc-dev` exist.

**Step 5: Commit**

    feat(spinclass2): add Nix build and justfile targets

--------------------------------------------------------------------------------

### Task 3: Add `[session]` table to sweatfile

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/sweatfile.go` -
Test: `packages/spinclass2/internal/sweatfile/sweatfile_test.go`

**Step 1: Write the failing test**

``` go
func TestSweatfileSessionTable(t *testing.T) {
    input := `
[session]
start = ["zellij", "-s", "test"]
resume = ["zellij", "attach", "test"]
`
    var sf Sweatfile
    if err := tommy.Decode([]byte(input), &sf); err != nil {
        t.Fatal(err)
    }
    if sf.Session == nil {
        t.Fatal("expected Session to be non-nil")
    }
    if len(sf.Session.Start) != 3 || sf.Session.Start[0] != "zellij" {
        t.Errorf("Start = %v, want [zellij -s test]", sf.Session.Start)
    }
    if len(sf.Session.Resume) != 3 || sf.Session.Resume[0] != "zellij" {
        t.Errorf("Resume = %v, want [zellij attach test]", sf.Session.Resume)
    }
}

func TestSweatfileSessionDefault(t *testing.T) {
    var sf Sweatfile
    if err := tommy.Decode([]byte(""), &sf); err != nil {
        t.Fatal(err)
    }
    if sf.Session != nil {
        t.Error("expected Session to be nil for empty sweatfile")
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestSweatfileSession ./packages/spinclass2/internal/sweatfile/...`
Expected: FAIL --- `Session` field doesn't exist yet.

**Step 3: Add the Session struct and field**

In `sweatfile.go`, add:

``` go
type Session struct {
    Start  []string `toml:"start"`
    Resume []string `toml:"resume"`
}
```

Add to `Sweatfile` struct:

``` go
Session *Session `toml:"session"`
```

Add accessor methods:

``` go
func (sf Sweatfile) SessionStart() []string {
    if sf.Session != nil && len(sf.Session.Start) > 0 {
        return sf.Session.Start
    }
    shell := os.Getenv("SHELL")
    if shell == "" {
        shell = "/bin/sh"
    }
    return []string{shell}
}

func (sf Sweatfile) SessionResume() []string {
    if sf.Session != nil && len(sf.Session.Resume) > 0 {
        return sf.Session.Resume
    }
    return nil
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestSweatfileSession ./packages/spinclass2/internal/sweatfile/...`
Expected: PASS

**Step 5: Regenerate tommy code if needed**

Run:
`nix develop --command go generate ./packages/spinclass2/internal/sweatfile/...`

**Step 6: Commit**

    feat(spinclass2): add [session] table to sweatfile (start/resume)

--------------------------------------------------------------------------------

### Task 4: Add sweatfile `[session]` merge semantics

**Files:** - Modify: `packages/spinclass2/internal/sweatfile/hierarchy.go` -
Test: `packages/spinclass2/internal/sweatfile/sweatfile_test.go`

**Step 1: Write the failing test**

Test that `[session]` fields use override semantics (deepest level wins):

``` go
func TestSweatfileSessionMerge(t *testing.T) {
    base := Sweatfile{
        Session: &Session{
            Start:  []string{"bash"},
            Resume: []string{"tmux", "attach"},
        },
    }
    override := Sweatfile{
        Session: &Session{
            Start: []string{"zellij"},
        },
    }
    merged := Merge(base, override)
    // Start overridden
    if merged.Session.Start[0] != "zellij" {
        t.Errorf("Start = %v, want [zellij]", merged.Session.Start)
    }
    // Resume inherited (override didn't set it)
    if merged.Session.Resume[0] != "tmux" {
        t.Errorf("Resume = %v, want [tmux attach]", merged.Session.Resume)
    }
}

func TestSweatfileSessionMergeNilInherit(t *testing.T) {
    base := Sweatfile{
        Session: &Session{Start: []string{"fish"}},
    }
    override := Sweatfile{} // no [session] table
    merged := Merge(base, override)
    if merged.Session == nil || merged.Session.Start[0] != "fish" {
        t.Errorf("expected Session.Start to be inherited, got %v", merged.Session)
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestSweatfileSessionMerge ./packages/spinclass2/internal/sweatfile/...`
Expected: FAIL

**Step 3: Add merge logic for `[session]`**

In `hierarchy.go`, in the `Merge` function (or wherever sweatfile fields are
merged), add Session handling with override semantics:

``` go
// Session: override semantics (deepest wins, nil = inherit)
if override.Session != nil {
    if merged.Session == nil {
        merged.Session = &Session{}
    }
    if len(override.Session.Start) > 0 {
        merged.Session.Start = override.Session.Start
    }
    if len(override.Session.Resume) > 0 {
        merged.Session.Resume = override.Session.Resume
    }
} else if base.Session != nil && merged.Session == nil {
    cp := *base.Session
    merged.Session = &cp
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestSweatfileSessionMerge ./packages/spinclass2/internal/sweatfile/...`
Expected: PASS

**Step 5: Commit**

    feat(spinclass2): add [session] override merge semantics in hierarchy

--------------------------------------------------------------------------------

### Task 5: Session state directory and types

**Files:** - Create: `packages/spinclass2/internal/session/session.go` - Create:
`packages/spinclass2/internal/session/session_test.go`

**Step 1: Write the failing test**

``` go
func TestSessionStateRoundTrip(t *testing.T) {
    dir := t.TempDir()
    t.Setenv("XDG_STATE_HOME", dir)

    s := State{
        PID:          12345,
        SessionState: StateActive,
        RepoPath:     "/home/user/repos/bob",
        WorktreePath: "/home/user/repos/bob/.worktrees/my-branch",
        Branch:       "my-branch",
        SessionKey:   "bob/my-branch",
        Entrypoint:   []string{"zellij"},
        Env:          map[string]string{"SPINCLASS_SESSION": "bob/my-branch"},
        StartedAt:    time.Now().UTC().Truncate(time.Second),
    }

    if err := Write(s); err != nil {
        t.Fatal(err)
    }

    loaded, err := Read(s.RepoPath, s.Branch)
    if err != nil {
        t.Fatal(err)
    }

    if loaded.PID != s.PID {
        t.Errorf("PID = %d, want %d", loaded.PID, s.PID)
    }
    if loaded.SessionState != StateActive {
        t.Errorf("State = %s, want active", loaded.SessionState)
    }
    if loaded.SessionKey != s.SessionKey {
        t.Errorf("SessionKey = %s, want %s", loaded.SessionKey, s.SessionKey)
    }
}

func TestSessionStateHash(t *testing.T) {
    h1 := stateFilename("/home/user/repos/bob", "my-branch")
    h2 := stateFilename("/home/user/repos/bob", "other-branch")
    if h1 == h2 {
        t.Error("different branches should produce different hashes")
    }
    if !strings.HasSuffix(h1, "-state.json") {
        t.Errorf("expected -state.json suffix, got %s", h1)
    }
}
```

**Step 2: Run test to verify it fails**

Run:
`nix develop --command go test -run TestSessionState ./packages/spinclass2/internal/session/...`
Expected: FAIL --- package doesn't exist.

**Step 3: Implement session state types and I/O**

``` go
package session

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "syscall"
    "time"
)

const (
    StateActive    = "active"
    StateInactive  = "inactive"
    StateAbandoned = "abandoned"
)

type State struct {
    PID          int               `json:"pid"`
    SessionState string            `json:"state"`
    RepoPath     string            `json:"repo_path"`
    WorktreePath string            `json:"worktree_path"`
    Branch       string            `json:"branch"`
    SessionKey   string            `json:"session_key"`
    Entrypoint   []string          `json:"entrypoint"`
    Env          map[string]string `json:"env"`
    StartedAt    time.Time         `json:"started_at"`
    ExitedAt     *time.Time        `json:"exited_at,omitempty"`
}

func stateDir() string {
    base := os.Getenv("XDG_STATE_HOME")
    if base == "" {
        home, _ := os.UserHomeDir()
        base = filepath.Join(home, ".local", "state")
    }
    return filepath.Join(base, "spinclass", "sessions")
}

func stateFilename(repoPath, branch string) string {
    h := sha256.Sum256([]byte(repoPath + "/" + branch))
    return fmt.Sprintf("%x-state.json", h[:8])
}

func statePath(repoPath, branch string) string {
    return filepath.Join(stateDir(), stateFilename(repoPath, branch))
}

func Write(s State) error {
    dir := stateDir()
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    data, err := json.MarshalIndent(s, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(statePath(s.RepoPath, s.Branch), data, 0o644)
}

func Read(repoPath, branch string) (*State, error) {
    data, err := os.ReadFile(statePath(repoPath, branch))
    if err != nil {
        return nil, err
    }
    var s State
    if err := json.Unmarshal(data, &s); err != nil {
        return nil, err
    }
    return &s, nil
}

func Remove(repoPath, branch string) error {
    return os.Remove(statePath(repoPath, branch))
}

func IsAlive(pid int) bool {
    if pid <= 0 {
        return false
    }
    err := syscall.Kill(pid, 0)
    return err == nil
}

// ResolveState checks the actual state, handling crash recovery.
// If state file says "active" but PID is dead, returns StateInactive.
// If worktree dir doesn't exist, returns StateAbandoned.
func (s *State) ResolveState() string {
    if _, err := os.Stat(s.WorktreePath); os.IsNotExist(err) {
        return StateAbandoned
    }
    if s.SessionState == StateActive && !IsAlive(s.PID) {
        return StateInactive
    }
    return s.SessionState
}

func ListAll() ([]State, error) {
    dir := stateDir()
    entries, err := os.ReadDir(dir)
    if os.IsNotExist(err) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    var states []State
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        data, err := os.ReadFile(filepath.Join(dir, e.Name()))
        if err != nil {
            continue
        }
        var s State
        if err := json.Unmarshal(data, &s); err != nil {
            continue
        }
        states = append(states, s)
    }
    return states, nil
}
```

**Step 4: Run test to verify it passes**

Run:
`nix develop --command go test -run TestSessionState ./packages/spinclass2/internal/session/...`
Expected: PASS

**Step 5: Commit**

    feat(spinclass2): add session state types and XDG_STATE I/O

--------------------------------------------------------------------------------

### Task 6: SessionExecutor (replace ZmxExecutor)

**Files:** - Create: `packages/spinclass2/internal/executor/session.go` -
Modify: `packages/spinclass2/internal/executor/executor.go` (no changes needed
to interface) - Delete: `packages/spinclass2/internal/executor/zmx.go` - Delete:
`packages/spinclass2/internal/executor/sessions.go` - Delete:
`packages/spinclass2/internal/executor/sessions_test.go` - Test:
`packages/spinclass2/internal/executor/session_test.go`

**Step 1: Delete zmx files**

Remove `zmx.go`, `sessions.go`, `sessions_test.go` from
`packages/spinclass2/internal/executor/`.

**Step 2: Write the failing test**

``` go
func TestSessionExecutorAttachDryRun(t *testing.T) {
    exec := SessionExecutor{
        Entrypoint: []string{"zellij", "-s", "test"},
    }
    tp := tap.TestPoint{}
    err := exec.Attach("/tmp/test", "repo/branch", nil, true, &tp)
    if err != nil {
        t.Fatal(err)
    }
    if tp.Skip != "dry run" {
        t.Errorf("Skip = %q, want 'dry run'", tp.Skip)
    }
}
```

**Step 3: Run test to verify it fails**

Run:
`nix develop --command go test -run TestSessionExecutor ./packages/spinclass2/internal/executor/...`
Expected: FAIL

**Step 4: Implement SessionExecutor**

``` go
package executor

import (
    "fmt"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "strings"
    "syscall"
    "time"

    "github.com/amarbel-llc/spinclass2/internal/session"
    tap "github.com/amarbel-llc/bob/packages/tap-dancer/go"
)

type SessionExecutor struct {
    Entrypoint []string
}

func (s SessionExecutor) Attach(dir string, key string, command []string, dryRun bool, tp *tap.TestPoint) error {
    entrypoint := s.Entrypoint
    if len(command) > 0 {
        entrypoint = command
    }
    if len(entrypoint) == 0 {
        shell := os.Getenv("SHELL")
        if shell == "" {
            shell = "/bin/sh"
        }
        entrypoint = []string{shell}
    }

    if dryRun {
        tp.Skip = "dry run"
        tp.Diagnostics = &tap.Diagnostics{
            Extras: map[string]any{
                "command": strings.Join(entrypoint, " "),
            },
        }
        return nil
    }

    tmpDir := filepath.Join(dir, ".tmp")

    cmd := exec.Command(entrypoint[0], entrypoint[1:]...)
    cmd.Dir = dir
    cmd.Env = append(os.Environ(),
        "SPINCLASS_SESSION="+key,
        "TMPDIR="+tmpDir,
        "CLAUDE_CODE_TMPDIR="+tmpDir,
    )
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin

    // Install SIGHUP handler to forward to child
    sighup := make(chan os.Signal, 1)
    signal.Notify(sighup, syscall.SIGHUP)

    if err := cmd.Start(); err != nil {
        return err
    }

    // Forward SIGHUP to child in background
    go func() {
        <-sighup
        if cmd.Process != nil {
            cmd.Process.Signal(syscall.SIGHUP)
            // Wait with timeout, then SIGTERM
            timer := time.NewTimer(10 * time.Second)
            defer timer.Stop()
            <-timer.C
            if cmd.Process != nil {
                cmd.Process.Signal(syscall.SIGTERM)
            }
        }
    }()

    err := cmd.Wait()
    signal.Stop(sighup)
    return err
}

func (s SessionExecutor) Detach() error {
    // Detach is called from merge when outside a session.
    // It's a no-op for SessionExecutor — state file cleanup
    // is handled by the caller.
    return nil
}

// RequestClose sends SIGHUP to the PID in the session state file.
// Used by `sc-dev merge` when an active session exists.
func RequestClose(repoPath, branch string) error {
    st, err := session.Read(repoPath, branch)
    if err != nil {
        return nil // no state file, nothing to close
    }
    if !session.IsAlive(st.PID) {
        return nil // already dead
    }
    return syscall.Kill(st.PID, syscall.SIGHUP)
}
```

**Step 5: Run test to verify it passes**

Run:
`nix develop --command go test -run TestSessionExecutor ./packages/spinclass2/internal/executor/...`
Expected: PASS

**Step 6: Verify full package compiles**

Run: `nix develop --command go build ./packages/spinclass2/...` Expected: build
succeeds (will have compile errors from status/shop referencing deleted zmx code
--- those are fixed in later tasks).

**Step 7: Commit**

    feat(spinclass2): replace ZmxExecutor with SessionExecutor

--------------------------------------------------------------------------------

### Task 7: Rename `new` → `attach` and wire session state

**Files:** - Modify: `packages/spinclass2/cmd/spinclass/main.go` - Modify:
`packages/spinclass2/internal/shop/shop.go`

**Step 1: Rename `newCmd` to `attachCmd` in `main.go`**

- Change `Use: "new [name parts...]"` → `Use: "attach [name parts...]"`
- Change `Short` and `Long` descriptions
- Replace `executor.ZmxExecutor{}` with `SessionExecutor` constructed from
  sweatfile
- Load the sweatfile hierarchy to get `[session].start` / `[session].resume`
- Pass entrypoint to `SessionExecutor`

**Step 2: Update `shop.New` to handle session state**

Rename `shop.New` → `shop.Attach`. Before exec'ing the entrypoint: 1. Check
state dir for existing session 2. If active + resume available → use resume
command 3. If active + no resume → warn, use start 4. Inactive or none → use
start 5. Write state file (`state: active`)

After entrypoint exits: 1. Update state file (`state: inactive`, clear PID, set
`exited_at`)

**Step 3: Update `closeShop`**

Remove references to `executor.ListSessions()`. Session state update happens in
`shop.Attach` after the entrypoint exits.

**Step 4: Verify it compiles**

Run: `nix develop --command go build ./packages/spinclass2/cmd/spinclass/`
Expected: compiles.

**Step 5: Commit**

    feat(spinclass2): rename new → attach, wire session state lifecycle

--------------------------------------------------------------------------------

### Task 8: Merge `status` into `list` command

**Files:** - Modify: `packages/spinclass2/cmd/spinclass/main.go` (remove
statusCmd, rewrite listCmd) - Modify:
`packages/spinclass2/internal/status/status.go` (remove zmx dependency) -
Create: `packages/spinclass2/internal/status/status_test.go` (update tests)

**Step 1: Rewrite `listCmd`**

The new `list` command: 1. Reads all state files from
`~/.local/state/spinclass/sessions/` 2. For each: resolve state
(active/inactive/abandoned via `ResolveState()`) 3. For active/inactive: compute
dirty state live via `git -C <worktree_path> status --porcelain` 4. Auto-clean
abandoned state files 5. Display in table or TAP format

**Step 2: Remove `statusCmd` and `status` package zmx dependency**

- Remove `statusCmd` from `init()` and command registration
- In `status.go`, remove `executor.ListSessions()` call from `CollectStatus`
- Remove `Session bool` from `BranchStatus` and session column from rendering
- Or: if status is fully subsumed by list, remove the `status` package entirely
  and build list rendering into the new `list` command handler

**Step 3: Remove the `"● zmx"` session indicator from rendering**

Replace with state-based indicators from the session state files: - `● active`
for active sessions - `○ inactive` for inactive sessions - (abandoned sessions
are auto-cleaned, not shown)

**Step 4: Verify it compiles and test**

Run: `nix develop --command go build ./packages/spinclass2/cmd/spinclass/` Run:
`nix develop --command go test ./packages/spinclass2/...`

**Step 5: Commit**

    feat(spinclass2): merge status into list, read from XDG_STATE

--------------------------------------------------------------------------------

### Task 9: Update `fork` command

**Files:** - Modify: `packages/spinclass2/cmd/spinclass/main.go` (update
forkCmd)

**Step 1: Update `forkCmd`**

- Remove `SPINCLASS_SESSION` requirement
- Resolve source worktree from cwd (detect repo + branch from git)
- Add `--from <dir>` flag for explicit source
- Cross-reference state dir for session metadata (optional, for richer output)

**Step 2: Verify it compiles**

Run: `nix develop --command go build ./packages/spinclass2/cmd/spinclass/`

**Step 3: Commit**

    feat(spinclass2): update fork to resolve from cwd or --from flag

--------------------------------------------------------------------------------

### Task 10: Update `merge` to remove state files

**Files:** - Modify: `packages/spinclass2/internal/merge/merge.go`

**Step 1: Add state file cleanup to `merge.Resolved`**

After successful merge, call `session.Remove(repoPath, branch)`.

Replace the `exec.Detach()` call at the end of `Resolved` with: 1. If active
session exists (PID alive): call `executor.RequestClose()` to send SIGHUP 2.
Then `session.Remove()` to clean up state file

**Step 2: Update `isInsideSession` to check state dir**

Keep `SPINCLASS_SESSION` env var check as primary (still set on entrypoint
process), but add state dir as fallback.

**Step 3: Verify tests pass**

Run: `nix develop --command go test ./packages/spinclass2/internal/merge/...`

**Step 4: Commit**

    feat(spinclass2): remove session state files on merge

--------------------------------------------------------------------------------

### Task 11: Update `clean` to handle abandoned sessions

**Files:** - Modify: `packages/spinclass2/internal/clean/clean.go`

**Step 1: Add abandoned session cleanup**

After cleaning merged worktrees, scan state files and remove any with
`ResolveState() == StateAbandoned`.

**Step 2: Verify tests pass**

Run: `nix develop --command go test ./packages/spinclass2/internal/clean/...`

**Step 3: Commit**

    feat(spinclass2): auto-clean abandoned session state files

--------------------------------------------------------------------------------

### Task 12: Update completions to use state dir

**Files:** - Modify: `packages/spinclass2/internal/completions/completions.go` -
Create: `packages/spinclass2/completions/spinclass2.bash-completion` - Create:
`packages/spinclass2/completions/spinclass2.fish` - Create:
`packages/spinclass2/completions/sc-dev.bash-completion` - Create:
`packages/spinclass2/completions/sc-dev.fish`

**Step 1: Update completion logic**

Replace worktree directory scanning with state file scanning: 1. Read all
`~/.local/state/spinclass/sessions/*.json` 2. Extract repo name + branch for
completion entries 3. Include both active and inactive sessions

**Step 2: Create shell completion files**

Copy from spinclass originals, rename `spinclass` → `spinclass2` and `sc` →
`sc-dev`.

**Step 3: Update `lib/packages/spinclass2.nix`**

Re-enable the `shellCompletions` derivation now that the files exist.

**Step 4: Build and verify**

Run: `just build-spinclass2` Expected: `result/bin/spinclass2`,
`result/bin/sc-dev` exist, completions installed.

**Step 5: Commit**

    feat(spinclass2): completions from XDG_STATE session files

--------------------------------------------------------------------------------

### Task 13: Update BATS integration tests

**Files:** - Modify: `packages/spinclass2/zz-tests_bats/lifecycle.bats` -
Modify: `packages/spinclass2/zz-tests_bats/fork.bats` - Modify:
`packages/spinclass2/zz-tests_bats/common.bash`

**Step 1: Update `common.bash`**

- Set `XDG_STATE_HOME` to a temp dir for test isolation
- Replace any `zmx` references with state dir checks
- Update binary name from `spinclass` to `spinclass2` (or use `SPINCLASS_BIN`
  env var)

**Step 2: Update lifecycle tests**

- Replace `sc new` with `sc-dev attach`
- Remove `sc list` zmx-specific assertions
- Add assertions for state files in `$XDG_STATE_HOME/spinclass/sessions/`
- Test the active → inactive transition

**Step 3: Update fork tests**

- Remove `SPINCLASS_SESSION` requirement from fork tests
- Test `--from` flag

**Step 4: Add justfile target**

``` just
test-spinclass2-bats: build-batman
    nix build .#spinclass2
    SPINCLASS_BIN={{justfile_directory()}}/result/bin/spinclass2 PATH="{{justfile_directory()}}/result-batman/bin:$PATH" {{cmd_nix_dev}} just packages/spinclass2/zz-tests_bats/test
```

**Step 5: Run BATS tests**

Run: `just test-spinclass2-bats` Expected: all tests pass.

**Step 6: Commit**

    test(spinclass2): update BATS tests for attach/list/state-dir

--------------------------------------------------------------------------------

### Task 14: Update CLAUDE.md and docs

**Files:** - Create: `packages/spinclass2/CLAUDE.md` (update from spinclass) -
Modify: `docs/features/0002-spinclass-session-entrypoint.md` (status → proposed)

**Step 1: Update CLAUDE.md**

Copy `packages/spinclass/CLAUDE.md` to `packages/spinclass2/CLAUDE.md` and
update: - Module name references - Remove zmx references - Document `[session]`
table - Document XDG_STATE session tracking - Update command names (`new` →
`attach`, `status` removed, etc.) - Update alias (`sc` → `sc-dev`)

**Step 2: Update FDR status**

Change `status: exploring` → `status: proposed` in FDR-0002 frontmatter.

**Step 3: Commit**

    docs(spinclass2): update CLAUDE.md and promote FDR to proposed

--------------------------------------------------------------------------------

### Task 15: Full test run and Nix build verification

**Step 1: Run all spinclass2 Go tests**

Run: `just test-spinclass2` Expected: all pass.

**Step 2: Run BATS integration tests**

Run: `just test-spinclass2-bats` Expected: all pass.

**Step 3: Nix build**

Run: `just build-spinclass2` Expected: builds successfully.

**Step 4: Nix flake check**

Run: `nix flake check` Expected: passes.

**Step 5: Manual smoke test**

``` bash
result/bin/sc-dev attach test-session
# Verify: state file created in ~/.local/state/spinclass/sessions/
# Verify: SPINCLASS_SESSION env var is set
# Exit shell
# Verify: state file updated to inactive
result/bin/sc-dev list
# Verify: shows the test-session as inactive
```
