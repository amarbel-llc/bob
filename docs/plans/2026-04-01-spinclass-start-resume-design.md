# Spinclass: Split attach into start / resume / update-description

## Problem

The recent refactor made `sc attach` always generate a random worktree name and
treat all positional args as a freeform description. This broke the ability to
reattach to an existing session --- every invocation creates a new worktree.

## Design

Split `attach` into three focused commands, each doing one thing.

### `sc start [description...]`

Creates a new worktree session. Direct rename of the current `attach` command.

- Random worktree name via `RandomName()`
- All positional args joined as session description
- Keeps existing flags: `--merge-on-close`, `--no-attach`

### `sc resume [id]`

Reattaches to an existing session.

- **Zero args:** auto-detect by scanning session state files for one whose
  `WorktreePath` is a prefix of the current working directory. Fail if no match
  or not inside a worktree.
- **One arg:** the worktree ID (directory name under `.worktrees/`, e.g.
  `plain-spruce`). This is NOT the git branch --- the branch inside the worktree
  may differ from the worktree directory name.
- No description args, no extra flags beyond global `--format` / `--verbose`
- Uses `SessionResume` sweatfile entrypoint if defined, falls back to
  `SessionStart`
- Validates the worktree directory exists, errors if not found

### `sc update-description [description...] [--id <id>]`

Updates the description on an existing session.

- `--id` optional: if omitted, auto-detect from session state matching current
  directory, or fail
- All positional args joined as the new description
- Writes updated description to the session state file

## Session State Lookup

Two new functions in `internal/session/`:

- **`FindByWorktreePath(path string) (*State, error)`** --- scans all session
  state files, returns the one whose `WorktreePath` is a prefix of `path`. Used
  for auto-detection when no ID is provided.
- **`FindByID(repoPath, id string) (*State, error)`** --- scans for a session
  whose `WorktreePath` ends in `/.worktrees/<id>`. Used when the user provides
  an explicit worktree ID.

## Worktree ID vs Branch

The worktree ID is the directory name under `.worktrees/`. It is assigned by
`RandomName()` at creation time. The git branch checked out inside the worktree
starts with the same name but may diverge if the user checks out a different
branch. Session lookup uses `WorktreePath`, not `Branch`, to avoid this
mismatch.

## Changes

| File | Change |
|------|--------|
| `cmd/spinclass/main.go` | Rename `attachCmd` → `startCmd`, add `resumeCmd`, add `updateDescriptionCmd` |
| `internal/session/session.go` | Add `FindByWorktreePath`, `FindByID` |
| `CLAUDE.md` | Update CLI command table |

`internal/worktree/worktree.go` is unchanged --- `resume` bypasses `ResolvePath`
entirely and builds `ResolvedPath` from the session state.

## Rollback

Rename `startCmd` back to `attachCmd`, remove the two new commands. Single
commit revert.
