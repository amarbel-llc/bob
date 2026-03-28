#! /usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  export output
  setup_test_home
  setup_stubs
  create_repo
}

function fork_creates_new_branch { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"
  "$bin" --format tap attach --no-attach source_branch

  # Use --from flag (cwd-based fork hits #65: relative .git path bug)
  run_sc fork --from "$TEST_REPO/.worktrees/source_branch" new_branch
  assert_success

  # New worktree should exist
  assert [ -d "$TEST_REPO/.worktrees/new_branch" ]
  assert [ -f "$TEST_REPO/.worktrees/new_branch/.git" ]
}

function fork_creates_branch_with_from_flag { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"
  "$bin" --format tap attach --no-attach from_src

  # Fork using --from flag (can run from anywhere)
  run_sc fork --from "$TEST_REPO/.worktrees/from_src" from_dst
  assert_success

  assert [ -d "$TEST_REPO/.worktrees/from_dst" ]
  assert [ -f "$TEST_REPO/.worktrees/from_dst/.git" ]
}

function fork_auto_names_branch { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"
  "$bin" --format tap attach --no-attach auto_src

  run_sc fork --from "$TEST_REPO/.worktrees/auto_src"
  assert_success

  # Should have created auto_src-1
  assert [ -d "$TEST_REPO/.worktrees/auto_src-1" ]
}

function fork_fails_outside_worktree { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"
  "$bin" --format tap attach --no-attach fork_test

  # Running from main repo (not a worktree) without --from should fail
  # because the main branch won't have a .worktrees/<branch> layout
  cd "$TEST_REPO"
  run_sc fork some_branch
  assert_failure
}
