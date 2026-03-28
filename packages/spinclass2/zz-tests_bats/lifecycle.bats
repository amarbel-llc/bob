#! /usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  export output
  setup_test_home
  setup_stubs
  create_repo
}

function spinclass_attach_creates_worktree { # @test
  cd "$TEST_REPO"
  run_sc attach test_branch --no-attach

  assert_success
  assert [ -d "$TEST_REPO/.worktrees/test_branch" ]
  # Should be a git worktree (has .git file, not directory)
  assert [ -f "$TEST_REPO/.worktrees/test_branch/.git" ]
  # Branch should exist
  run git -C "$TEST_REPO" rev-parse --verify refs/heads/test_branch
  assert_success
}

function spinclass_attach_auto_name { # @test
  cd "$TEST_REPO"
  run_sc attach --no-attach

  assert_success
  # Should have created a worktree dir — at least one entry in .worktrees/
  run ls "$TEST_REPO/.worktrees/"
  assert_success
  assert [ -n "$output" ]
}

function spinclass_attach_no_attach_skips_session { # @test
  cd "$TEST_REPO"
  run_sc attach --no-attach test_noattach

  assert_success
  assert [ -d "$TEST_REPO/.worktrees/test_noattach" ]
  # No session state file should be created with --no-attach
  local state_dir="$XDG_STATE_HOME/spinclass/sessions"
  if [ -d "$state_dir" ]; then
    local count
    count="$(find "$state_dir" -name '*-state.json' | wc -l)"
    assert [ "$count" -eq 0 ]
  fi
}

function spinclass_attach_idempotent { # @test
  cd "$TEST_REPO"
  run_sc attach --no-attach test_idem
  assert_success

  # Second run should succeed with SKIP
  run_sc attach --no-attach test_idem
  assert_success
  assert_output --partial "SKIP"
}

function spinclass_list_shows_sessions { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"

  # Create some worktrees
  "$bin" --format tap attach --no-attach branch_a
  "$bin" --format tap attach --no-attach branch_b

  # list without active sessions should produce empty output (no-attach doesn't write state)
  run_sc list
  assert_success
}

function spinclass_merge_fast_forwards { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"
  "$bin" --format tap attach --no-attach merge_test

  local wt="$TEST_REPO/.worktrees/merge_test"

  # Make a commit on the worktree branch
  echo "new content" > "$wt/new-file.txt"
  git -C "$wt" add new-file.txt
  git -C "$wt" commit -m "add new file"

  # Clean untracked files created by sweatfile apply so worktree remove succeeds
  git -C "$wt" clean -fd

  # Merge from the main repo
  run_sc merge merge_test
  assert_success

  # Commit should now be on main
  run git -C "$TEST_REPO" log --oneline --all
  assert_output --partial "add new file"

  # Worktree should be removed
  assert [ ! -d "$wt" ]
}

function spinclass_clean_removes_merged { # @test
  cd "$TEST_REPO"
  local bin="${SPINCLASS_BIN:-spinclass2}"
  "$bin" --format tap attach --no-attach clean_test

  # Clean untracked files so worktree remove succeeds
  git -C "$TEST_REPO/.worktrees/clean_test" clean -fd

  # Merge the worktree first (makes the branch fully merged)
  "$bin" --format tap merge clean_test

  # Create another worktree that IS merged (no extra commits)
  "$bin" --format tap attach --no-attach clean_noop

  # Clean untracked files from sweatfile apply
  git -C "$TEST_REPO/.worktrees/clean_noop" clean -fd

  run_sc clean
  assert_success
  # The noop worktree with zero commits ahead should be cleaned
  assert [ ! -d "$TEST_REPO/.worktrees/clean_noop" ]
}
