# Session BATS Tests Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add BATS integration tests that exercise the
start→session-state→resume round-trip, covering the untested code paths where
`--no-attach` is not used.

**Architecture:** A new `session.bats` file with 4 tests. A global sweatfile
sets `[session-entry] start = ["true"]` and `resume = ["true"]` so sessions
start, write state, and exit immediately. Two new helpers in `common.bash`
support this.

**Tech Stack:** BATS, jq, spinclass CLI, sweatfile TOML config

**Rollback:** N/A --- purely additive test file, no production code changes.

--------------------------------------------------------------------------------

### Task 1: Add helper functions to common.bash

**Files:** - Modify: `packages/spinclass/zz-tests_bats/common.bash` (append
after line 95)

**Step 1: Add `create_session_sweatfile` and `run_sc_session` helpers**

Append these two functions after the existing `assert_session_state` function:

``` bash
# Write a global sweatfile with fast-exiting entrypoints for session tests.
# Both start and resume use "true" so the session writes state and exits
# immediately.
create_session_sweatfile() {
  local sweatfile_dir="$HOME/.config/spinclass"
  mkdir -p "$sweatfile_dir"
  cat >"$sweatfile_dir/sweatfile" <<'EOF'
[session-entry]
start = ["true"]
resume = ["true"]
EOF
}

# Run spinclass with a longer timeout for session attach tests.
# The subprocess spawn + closeShop workflow needs more headroom than
# the 5s used by run_sc.
# Usage: run_sc_session <subcommand> [args...]
run_sc_session() {
  local bin="${SPINCLASS_BIN:-spinclass}"
  run timeout --preserve-status 10s "$bin" --format tap "$@"
}
```

**Step 2: Verify helpers are syntactically valid**

Run: `bash -n packages/spinclass/zz-tests_bats/common.bash` Expected: no output
(clean parse)

**Step 3: Commit**

    feat(spinclass/tests): add session sweatfile and timeout helpers to common.bash

--------------------------------------------------------------------------------

### Task 2: Write `spinclass_start_writes_session_state` test

**Files:** - Create: `packages/spinclass/zz-tests_bats/session.bats`

**Step 1: Create session.bats with first test**

``` bash
#! /usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  export output
  setup_test_home
  create_session_sweatfile
  create_repo
}

function spinclass_start_writes_session_state { # @test
  cd "$TEST_REPO"
  run_sc_session start
  assert_success

  # Session state dir should exist with exactly one state file
  local state_dir="$XDG_STATE_HOME/spinclass/sessions"
  assert [ -d "$state_dir" ]
  local state_file
  state_file=$(find "$state_dir" -name '*-state.json' | head -1)
  assert [ -n "$state_file" ]

  # Verify key fields in the state JSON
  run jq -r '.state' "$state_file"
  assert_output "inactive"

  run jq -r '.worktree_path' "$state_file"
  assert_success
  # Worktree path should be under .worktrees/
  assert_output --partial ".worktrees/"

  run jq -r '.branch' "$state_file"
  assert_success
  assert [ -n "$output" ]
}
```

**Step 2: Build spinclass and run the test**

Run: `just test-spinclass-bats` (or from the test dir: `bats session.bats`)
Expected: 1 test, PASS

**Step 3: Commit**

    test(spinclass): add session state test for start without --no-attach

--------------------------------------------------------------------------------

### Task 3: Write `spinclass_resume_by_id` test

**Files:** - Modify: `packages/spinclass/zz-tests_bats/session.bats` (append)

**Step 1: Add resume-by-id test**

``` bash
function spinclass_resume_by_id { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass}"

  # Start a session — writes state and exits (entrypoint is "true")
  local start_output
  start_output=$(timeout --preserve-status 10s "$bin" --format tap start 2>&1)

  # Extract the worktree dirname (the ID used by resume)
  local wt_path
  wt_path=$(extract_wt_path "$start_output")
  local wt_id
  wt_id=$(basename "$wt_path")

  # Resume by ID should succeed
  run_sc_session resume "$wt_id"
  assert_success
}
```

**Step 2: Run the test**

Run: `just test-spinclass-bats` Expected: 2 tests in session.bats, both PASS

**Step 3: Commit**

    test(spinclass): add resume-by-id integration test

--------------------------------------------------------------------------------

### Task 4: Write `spinclass_resume_from_cwd` test

**Files:** - Modify: `packages/spinclass/zz-tests_bats/session.bats` (append)

**Step 1: Add resume-from-cwd test**

``` bash
function spinclass_resume_from_cwd { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass}"

  # Start a session — writes state and exits
  local start_output
  start_output=$(timeout --preserve-status 10s "$bin" --format tap start 2>&1)

  local wt_path
  wt_path=$(extract_wt_path "$start_output")

  # cd into the worktree and resume with no args (auto-detect from cwd)
  cd "$wt_path"
  run_sc_session resume
  assert_success
}
```

**Step 2: Run the test**

Run: `just test-spinclass-bats` Expected: 3 tests in session.bats, all PASS

**Step 3: Commit**

    test(spinclass): add resume-from-cwd integration test

--------------------------------------------------------------------------------

### Task 5: Write `spinclass_resume_no_session_fails` test

**Files:** - Modify: `packages/spinclass/zz-tests_bats/session.bats` (append)

**Step 1: Add resume-no-session test**

``` bash
function spinclass_resume_no_session_fails { # @test
  cd "$TEST_REPO"

  # Create a worktree with --no-attach (no session state written)
  run_sc start --no-attach
  assert_success

  local wt_path
  wt_path=$(extract_wt_path "$output")

  # cd into the worktree and try to resume — should fail
  cd "$wt_path"
  run_sc_session resume
  assert_failure
  assert_output --partial "no session found"
}
```

**Step 2: Run all session tests**

Run: `just test-spinclass-bats` Expected: 4 tests in session.bats, all PASS

**Step 3: Commit**

    test(spinclass): add resume-no-session failure test

--------------------------------------------------------------------------------

### Task 6: Run full test suite

**Step 1: Run all spinclass BATS tests**

Run: `just test-spinclass-bats` Expected: All tests pass (session.bats +
lifecycle.bats + hooks.bats + sweatfile.bats + validate.bats + fork.bats)

**Step 2: Verify no sandbox escape**

Run: `ls ~/.local/state/spinclass/sessions/ 2>/dev/null` Expected: No state
files leaked from tests (all should be in `$BATS_TEST_TMPDIR`)
