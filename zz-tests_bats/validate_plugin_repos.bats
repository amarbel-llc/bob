#!/usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output
  purse_first="$(purse_first_bin)"
  claude_bin="${CLAUDE_BIN:-claude}"
}

teardown() {
  teardown_test_home
}

function claude_validates_caldav { # @test
  run "$claude_bin" plugin validate "$(plugin_share_dir caldav)/.claude-plugin/plugin.json"
  assert_success
}

function purse_first_validates_caldav { # @test
  run "$purse_first" validate "$(plugin_share_dir caldav)"
  assert_success
}
