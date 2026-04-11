#!/usr/bin/env bats

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output
  purse_first="$(purse_first_bin)"
}

teardown() {
  teardown_test_home
}

function claude_validates_lux { # @test
  run claude plugin validate "$(plugin_share_dir lux)/.claude-plugin/plugin.json"
  assert_success
}

function claude_validates_bob { # @test
  run claude plugin validate "$(plugin_share_dir bob)/.claude-plugin/plugin.json"
  assert_success
}

function claude_validates_robin { # @test
  run claude plugin validate "$(plugin_share_dir robin)/.claude-plugin/plugin.json"
  assert_success
}

function claude_validates_tap_dancer { # @test
  run claude plugin validate "$(plugin_share_dir tap-dancer)/.claude-plugin/plugin.json"
  assert_success
}

function purse_first_validates_lux { # @test
  run "$purse_first" validate "$(plugin_share_dir lux)"
  assert_success
}

function purse_first_validates_bob { # @test
  run "$purse_first" validate "$(plugin_share_dir bob)"
  assert_success
}

function purse_first_validates_robin { # @test
  run "$purse_first" validate "$(plugin_share_dir robin)"
  assert_success
}

function purse_first_validates_tap_dancer { # @test
  run "$purse_first" validate "$(plugin_share_dir tap-dancer)"
  assert_success
}
