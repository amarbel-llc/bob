# BATS Integration Tests for spinclass start/resume

## Problem

No BATS tests exercise real session attachment (state file creation) or the
`resume` command. The `--no-attach` flag used in existing lifecycle tests
explicitly skips writing session state, so the start→resume round-trip is
untested.

## Approach

Use a global sweatfile with `[session-entry] start = ["true"]` so `start`
(without `--no-attach`) writes session state and exits immediately when the
entrypoint completes. Resume tests use a corresponding
`[session-entry] resume = ["true"]`.

All state is sandboxed via `setup_test_home` which redirects `$HOME`,
`$XDG_STATE_HOME`, and `$XDG_CONFIG_HOME` into `$BATS_TEST_TMPDIR`.

## Test Cases

**File: `packages/spinclass/zz-tests_bats/session.bats`**

### 1. `spinclass_start_writes_session_state`

- `start` without `--no-attach` in a test repo
- Verify state JSON exists in `$XDG_STATE_HOME/spinclass/sessions/`
- Verify state fields: `worktree_path`, `branch`, `state` = `inactive` (since
  `true` exits immediately)

### 2. `spinclass_resume_by_id`

- `start` a session (writes state, exits)
- Extract worktree dirname as ID
- `resume <id>` with sweatfile `resume = ["true"]`
- Assert success

### 3. `spinclass_resume_from_cwd`

- `start` a session
- `cd` into the worktree path
- `resume` (no args, auto-detect from cwd)
- Assert success

### 4. `spinclass_resume_no_session_fails`

- Create a worktree with `--no-attach` (no state written)
- `cd` into the worktree
- `resume` (no args)
- Assert failure with "no session found" error

## Helpers

Add to `common.bash`:

- `create_session_sweatfile` --- writes `$HOME/.config/spinclass/sweatfile` with
  `[session-entry] start = ["true"]` and optionally `resume = ["true"]`
- `run_sc_session` --- like `run_sc` but with longer timeout (session attach
  involves subprocess spawn)

## Files Changed

- `packages/spinclass/zz-tests_bats/session.bats` (new)
- `packages/spinclass/zz-tests_bats/common.bash` (add helpers)
