---
date: 2026-03-28
promotion-criteria: |
  exploring → proposed: design validated, implementation plan written. proposed
  → experimental: spinclass2 package builds, sc-dev attach/list/merge/clean work
  end-to-end. experimental → testing: 7 days of sc-dev use with no regressions
  vs sc. testing → accepted: delete packages/spinclass, rename spinclass2 →
  spinclass, alias sc-dev → sc.
status: exploring
---

# Spinclass Session Entrypoint

## Problem Statement

Spinclass currently depends on `zmx` (an external session manager binary) to
attach to worktree sessions. This creates a hard dependency on a specific tool
for session multiplexing, couples session tracking to zmx's output format, and
limits session lifecycle visibility to what zmx exposes.

## Design

Replace zmx with two changes:

1.  A sweatfile `[session]` table that specifies the entrypoint command.
2.  A session state directory at `~/.local/state/spinclass/sessions/` that
    spinclass owns directly.

### Sweatfile `[session]` Table

New table in the sweatfile TOML:

``` toml
[session]
start = ["zellij", "-s", "myproject"]
resume = ["zellij", "attach", "myproject"]
```

- `start`: command exec'd when creating a new session or restarting an inactive
  one. Default: `[$SHELL]`.
- `resume`: command exec'd when attaching to an active session (PID alive).
  Default: nil (falls back to `start` with a warning about second instance).

Both fields are `[]string` (TOML arrays). Merge semantics: override (deepest
sweatfile level wins, nil = inherit from parent).

### Session State Directory

    ~/.local/state/spinclass/sessions/
      <hash>-state.json
      <hash>-state.json

`<hash>` = SHA-256 truncated to 16 hex chars of `<abs-repo-path>/<branch>`.

State file schema:

``` json
{
  "pid": 12345,
  "state": "active",
  "repo_path": "/home/sasha/eng/repos/bob",
  "worktree_path": "/home/sasha/eng/repos/bob/.worktrees/eager-aspen",
  "branch": "eager-aspen",
  "session_key": "bob/eager-aspen",
  "entrypoint": ["zellij", "-s", "bob-eager-aspen"],
  "env": {"SPINCLASS_SESSION": "bob/eager-aspen"},
  "started_at": "2026-03-28T14:30:00Z",
  "exited_at": "2026-03-28T16:00:00Z"
}
```

### Session States

  State         PID alive   Worktree exists   Meaning
  ------------- ----------- ----------------- ----------------------------------------
  `active`      yes         yes               Entrypoint running
  `inactive`    no          yes               Entrypoint exited, work not yet merged
  `abandoned`   no          no                Worktree removed, state file lingers

State transitions:

- `sc-dev attach` (new worktree) → create state file, `state: active`
- `sc-dev attach` (resume active) → update PID/started_at, `state: active`
- Entrypoint exits normally → update: clear PID, set exited_at,
  `state: inactive`
- Crash / kill -9 → no update; readers detect via `kill -0` (fallback)
- `sc-dev merge` → remove state file
- `sc-dev clean` → remove state file (for merged worktrees and abandoned
  sessions)

Dirty state is not stored --- `sc-dev list` computes it live via
`git -C <worktree_path> status --porcelain`.

### Executor Refactor

The `Executor` interface is preserved:

``` go
type Executor interface {
    Attach(dir, key string, command []string, dryRun bool, tp *tap.TestPoint) error
    Detach() error
}
```

Implementations:

- **`ZmxExecutor`** --- deleted.
- **`SessionExecutor`** (new) --- `Attach` execs the entrypoint with session env
  vars. `Detach` reads the state file and sends SIGHUP to the PID.
- **`ShellExecutor`** --- unchanged (merge's non-session exec, `Detach` is
  no-op).

### SIGHUP Handling

When the spinclass process (running `sc-dev attach`) receives SIGHUP:

1.  Forward SIGHUP to the entrypoint child process.
2.  Wait up to a timeout for graceful exit.
3.  SIGTERM if still alive after timeout.
4.  Run closeShop (state file update, optional merge-on-close).
5.  Exit.

`Detach()` (called from `sc-dev merge` on external merge) handles two cases:

- **Active session** (PID alive): send SIGHUP, triggering the above.
- **Inactive session** (PID dead): remove state file directly.

### CLI Changes

  -------------------------------------------------------------------------------
  Current                           New                Notes
  --------------------------------- ------------------ --------------------------
  `sc new`                          `sc-dev attach`    Auto-detects:
                                                       create+start, start, or
                                                       resume

  `sc status`                       removed            Merged into `list`

  `sc list`                         `sc-dev list`      Reads from state dir,
                                                       shows all sessions with
                                                       live dirty state

  `sc merge`                        `sc-dev merge`     Unchanged behavior,
                                                       removes state file on
                                                       completion

  `sc clean`                        `sc-dev clean`     Also removes state files;
                                                       auto-cleans abandoned
                                                       sessions

  `sc fork`                         `sc-dev fork`      Resolves source from cwd
                                                       or `--from <dir>`, reads
                                                       state dir
  -------------------------------------------------------------------------------

### `sc-dev attach` Flow

1.  Resolve worktree path.
2.  Pull main worktree (if clean).
3.  Create worktree if needed.
4.  Check state dir for existing session:
    - Active + `[session].resume` set → exec resume command.
    - Active + no resume → warn, exec start (second instance).
    - Inactive or no state file → exec start command.
5.  Write/update state file (`state: active`).
6.  Install SIGHUP handler.
7.  Exec entrypoint.
8.  On exit: update state file (`state: inactive`, clear PID, set exited_at),
    run closeShop.

### `sc-dev fork`

- `sc-dev fork [new-branch]` --- fork from current directory.
- `sc-dev fork [new-branch] --from <dir>` --- fork from a specific worktree.
- No longer requires `SPINCLASS_SESSION` env var.

### Completions

All commands generate completions by scanning
`~/.local/state/spinclass/sessions/*.json`. Entries include repo name + branch.

## Rollback Strategy

Hard fork: `packages/spinclass2/` coexists with `packages/spinclass/`.

- `sc` (current zmx-based) and `sc-dev` (new) run side by side.
- Promotion: delete `packages/spinclass/`, rename `spinclass2` → `spinclass`,
  alias back to `sc`.
- Rollback: delete `packages/spinclass2/`. Original is untouched.

## Limitations

- PID recycling: theoretically a stale PID could match a new unrelated process.
  Acceptable for session liveness checks; not used for anything destructive.
- Crash cleanup: if spinclass is killed -9, state file stays `active` with a
  dead PID. Readers detect this via `kill -0` fallback and treat as `inactive`.
- No cross-machine sessions: state dir is local to the machine.
