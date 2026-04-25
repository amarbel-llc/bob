#!/usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export XDG_LOG_HOME="$BATS_TEST_TMPDIR/.xdg/log"
  FIXTURES="$(dirname "$BATS_TEST_FILE")/fixtures"
}

teardown() {
  teardown_test_home
}

function batman_stub_runs { # @test
  run "$BATMAN_BIN" foo bar
  assert_success
  assert_output --partial "received 2 positional args"
}
