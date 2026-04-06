#! /usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  export output
}

teardown() {
  teardown_test_home
}

# --- stash_save ---

function stash_save_creates_stash_from_staged_changes { # @test
  setup_test_repo
  echo "modified" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt

  run run_grit_mcp "stash-save" "$(printf '{"repo_path":"%s","message":"test stash"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "stashed"

  local name
  name=$(echo "$output" | jq -r '.name')
  assert_equal "$name" "test stash"
}

function stash_save_creates_stash_from_unstaged_changes { # @test
  setup_test_repo
  echo "modified" >"$TEST_REPO/file.txt"

  run run_grit_mcp "stash-save" "$(printf '{"repo_path":"%s","message":"unstaged"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "stashed"
}

function stash_save_no_changes_returns_no_changes_status { # @test
  setup_test_repo
  # Working tree is clean — no modifications at all

  run run_grit_mcp "stash-save" "$(printf '{"repo_path":"%s","message":"nothing"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "no_changes"
}

function stash_save_include_untracked { # @test
  setup_test_repo
  echo "new file" >"$TEST_REPO/untracked.txt"

  run run_grit_mcp "stash-save" "$(printf '{"repo_path":"%s","message":"with untracked","include_untracked":true}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "stashed"

  # Verify the untracked file was actually stashed (removed from worktree)
  [ ! -f "$TEST_REPO/untracked.txt" ]
}

function stash_save_without_include_untracked_leaves_untracked_files { # @test
  setup_test_repo
  echo "new file" >"$TEST_REPO/untracked.txt"
  # Also modify a tracked file so stash has something to save
  echo "modified" >"$TEST_REPO/file.txt"

  run run_grit_mcp "stash-save" "$(printf '{"repo_path":"%s","message":"no untracked"}' "$TEST_REPO")"
  assert_success

  # Untracked file should still be present
  [ -f "$TEST_REPO/untracked.txt" ]
}

# --- stash_apply ---

function stash_apply_restores_stashed_changes { # @test
  setup_test_repo
  echo "stashed content" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "to apply"

  run run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "applied"

  # Verify the change was actually restored
  local content
  content=$(cat "$TEST_REPO/file.txt")
  assert_equal "$content" "stashed content"
}

function stash_apply_with_explicit_ref { # @test
  setup_test_repo
  # Create two stashes
  echo "first" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "first stash"
  echo "second" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "second stash"

  # Apply the older stash (stash@{1})
  run run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s","stash_ref":"stash@{1}"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "applied"

  # Should have the first stash's content
  local content
  content=$(cat "$TEST_REPO/file.txt")
  assert_equal "$content" "first"
}

function stash_apply_with_conflict_returns_json_not_text_error { # @test
  setup_test_repo
  # Stash a change
  echo "stashed version" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "will conflict"

  # Make a different change and commit it
  echo "committed version" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" commit -m "conflicting change"

  # Applying the stash should conflict
  local result
  result=$(run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s"}' "$TEST_REPO")")

  # The result must be valid JSON with status "conflict", not a plain text error
  run jq -e -r '.status' <<<"$result"
  assert_success
  assert_output "conflict"
}

function stash_apply_with_conflict_lists_conflicted_files { # @test
  setup_test_repo
  echo "stashed version" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "will conflict"

  echo "committed version" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" commit -m "conflicting change"

  local result
  result=$(run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s"}' "$TEST_REPO")")

  # Should list file.txt as conflicted (depends on the bug above being fixed)
  run jq -e -r '.conflicts[]' <<<"$result"
  assert_success
  assert_output "file.txt"
}

function stash_apply_nonexistent_ref_returns_error { # @test
  setup_test_repo

  run run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s","stash_ref":"stash@{99}"}' "$TEST_REPO")"
  assert_success

  # Should return an error, not crash
  echo "$output" | grep -qi "error\|stash"
}

function stash_apply_preserves_stash_in_list { # @test
  setup_test_repo
  echo "stashed" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "should remain"

  run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s"}' "$TEST_REPO")"

  # Stash should still exist after apply (unlike pop)
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success

  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "1"
}

# --- stash_drop ---

function stash_drop_removes_stash { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  run run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","stash_ref":"stash@{0}"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "dropped"

  # Verify stash list is empty
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success

  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "0"
}

function stash_drop_nonexistent_ref_returns_error { # @test
  setup_test_repo

  run run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","stash_ref":"stash@{99}"}' "$TEST_REPO")"
  assert_success

  # Should contain an error message
  echo "$output" | grep -qi "error\|stash"
}

function stash_drop_with_index_instead_of_stash_ref { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73 bug 1: LLMs naturally pass {"index": 0} instead of
  # {"stash_ref": "stash@{0}"}. The tool silently ignores the unknown
  # "index" param and calls "git stash drop" with an empty stash_ref,
  # producing "refs/stash@{} is not a valid reference".
  # The tool should either accept an index param or fail with a clear
  # error about the missing stash_ref.
  run run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","index":0}' "$TEST_REPO")"
  assert_success

  # If the tool doesn't understand "index", it should return a clear error
  # about the missing required stash_ref param, not a cryptic git ref error.
  # Verify the stash was NOT dropped (since the param was wrong)
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  # Stash should still be there — the drop should have failed gracefully
  assert_equal "$count" "1"
}

function stash_drop_with_bare_integer_ref { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73: LLM passes just "0" instead of "stash@{0}"
  run run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","stash_ref":"0"}' "$TEST_REPO")"
  assert_success

  # Should either work (by constructing the proper ref) or return a
  # clear error — not a cryptic git ref parse failure
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  # If it worked, count is 0; if it failed gracefully, count is 1.
  # Either way, it shouldn't have crashed.
}

function stash_drop_with_empty_stash_ref_does_not_drop_silently { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73: stash_ref is required but go-mcp doesn't enforce it at runtime.
  # When empty string arrives, git stash drop "" is called.
  local result
  result=$(run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","stash_ref":""}' "$TEST_REPO")")

  # Empty stash_ref should not silently drop stash@{0}
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "1"
}

function stash_drop_with_missing_stash_ref_does_not_drop_silently { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73: stash_ref omitted entirely. Go json.Unmarshal gives zero value "".
  # "git stash drop" with no ref arg drops stash@{0}.
  # Since Required is schema-only with no runtime enforcement, the tool
  # should validate this itself.
  local result
  result=$(run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s"}' "$TEST_REPO")")

  # Missing stash_ref should not silently drop stash@{0}
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "1"
}

function stash_drop_with_stash_at_empty_braces { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73: The exact error was "refs/stash@{} is not a valid reference"
  # This means the ref "stash@{}" (empty braces) was passed to git.
  # Reproduce by sending stash_ref with empty braces.
  local result
  result=$(run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","stash_ref":"stash@{}"}' "$TEST_REPO")")

  # Should return an error, and stash should remain
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "1"
}

function stash_drop_ref_with_special_chars_in_printf { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73: The curly braces in stash@{0} could be mangled by printf
  # or shell interpolation. Test with the exact JSON the MCP client sends.
  # Use a heredoc to avoid any shell interpolation of {0}
  local args
  args=$(
    cat <<ARGS
{"repo_path":"$TEST_REPO","stash_ref":"stash@{0}"}
ARGS
  )
  run run_grit_mcp "stash-drop" "$args"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "dropped"

  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "0"
}

function stash_drop_ref_survives_json_encoding { # @test
  setup_test_repo
  echo "to drop" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "drop me"

  # gh#73: Build the JSON with jq to ensure proper encoding of stash@{0},
  # mimicking how a real MCP client (e.g. Claude Code SDK) would serialize it.
  local args
  args=$(jq -cn --arg repo "$TEST_REPO" --arg ref 'stash@{0}' \
    '{repo_path: $repo, stash_ref: $ref}')

  run run_grit_mcp "stash-drop" "$args"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "dropped"

  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "0"
}

function stash_apply_with_index_instead_of_stash_ref { # @test
  setup_test_repo
  echo "stashed" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "apply me"

  # Same issue as stash_drop: LLMs pass index instead of stash_ref.
  # "git stash apply" with empty ref defaults to stash@{0} which happens
  # to work, but the intent is wrong — the tool should handle index params.
  run run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s","index":0}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "applied"
}

function stash_apply_with_bare_integer_ref { # @test
  setup_test_repo
  echo "stashed" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "apply me"

  # gh#73: LLM passes just "0" instead of "stash@{0}"
  run run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s","stash_ref":"0"}' "$TEST_REPO")"
  assert_success

  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "applied"
}

# --- grit://stashes resource ---

function stashes_resource_empty_when_no_stashes { # @test
  setup_test_repo

  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success

  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "0"
}

function stashes_resource_lists_stash_entries { # @test
  setup_test_repo
  echo "first" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "first stash"
  echo "second" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "second stash"

  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success

  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "2"

  # Most recent stash is index 0
  local idx0
  idx0=$(echo "$output" | jq -r '.[0].index')
  assert_equal "$idx0" "0"

  local idx1
  idx1=$(echo "$output" | jq -r '.[1].index')
  assert_equal "$idx1" "1"
}

function stashes_resource_includes_message { # @test
  setup_test_repo
  echo "change" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt
  git -C "$TEST_REPO" stash push -m "my test message"

  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success

  local message
  message=$(echo "$output" | jq -r '.[0].message')
  # The message should contain our stash message
  echo "$message" | grep -q "my test message"
}

# --- round-trip: save → list → apply → drop ---

function stash_round_trip_save_list_apply_drop { # @test
  setup_test_repo
  echo "round trip content" >"$TEST_REPO/file.txt"
  git -C "$TEST_REPO" add file.txt

  # Save
  run run_grit_mcp "stash-save" "$(printf '{"repo_path":"%s","message":"round trip"}' "$TEST_REPO")"
  assert_success
  local status
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "stashed"

  # Verify file is restored to committed state
  local content
  content=$(cat "$TEST_REPO/file.txt")
  assert_equal "$content" "initial"

  # List via resource
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  local count
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "1"

  # Apply
  run run_grit_mcp "stash-apply" "$(printf '{"repo_path":"%s"}' "$TEST_REPO")"
  assert_success
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "applied"

  # Verify content restored
  content=$(cat "$TEST_REPO/file.txt")
  assert_equal "$content" "round trip content"

  # Drop
  run run_grit_mcp "stash-drop" "$(printf '{"repo_path":"%s","stash_ref":"stash@{0}"}' "$TEST_REPO")"
  assert_success
  status=$(echo "$output" | jq -r '.status')
  assert_equal "$status" "dropped"

  # Verify stash list is empty
  run read_grit_resource "grit://stashes?repo_path=$TEST_REPO"
  assert_success
  count=$(echo "$output" | jq 'length')
  assert_equal "$count" "0"
}
