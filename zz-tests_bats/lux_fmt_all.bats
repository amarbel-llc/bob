#!/usr/bin/env bats

# Integration tests for lux fmt-all subcommand.
# Requires: nix build .#lux
#
# These tests use walk="all" mode because the sandcastle sandbox blocks
# git init (.git/config writes are denied by default). The git walk mode
# is covered by Go unit tests in fmtall_test.go.
#
# Formatter execution is not available in the sandcastle sandbox (nix
# build is blocked), so tests verify command behavior without asserting
# on formatter output. Actual formatting is covered by lux_fmt.bats
# when run outside the sandbox.

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  lux="$(result_dir)/bin/lux"
  [[ -x $lux ]] || skip "lux binary not found at $lux — run: nix build .#lux"

  "$lux" init --default --force

  # Configure fmt-all to use "all" walk strategy (no git required)
  mkdir -p "$XDG_CONFIG_HOME/lux"
  cat >"$XDG_CONFIG_HOME/lux/fmt-all.toml" <<'TOML'
walk = "all"
TOML

  # Create a test project directory
  export PROJECT_DIR="$BATS_TEST_TMPDIR/project"
  mkdir -p "$PROJECT_DIR"
  cd "$PROJECT_DIR"
}

function fmt_all_exits_0_with_recognized_file { # @test
  cat >"$PROJECT_DIR/main.go" <<'GO'
package main

func main() {
	println("hi")
}
GO

  run "$lux" fmt-all
  assert_success
}

function fmt_all_skips_unrecognized_files { # @test
  echo "some random content" >"$PROJECT_DIR/readme.txt"

  run "$lux" fmt-all
  assert_success

  # unrecognized files are left untouched
  run cat "$PROJECT_DIR/readme.txt"
  assert_output "some random content"
}

function fmt_all_respects_exclude_globs { # @test
  echo '{"a":1}' >"$PROJECT_DIR/flake.lock"

  # Override config with exclude_globs
  cat >"$XDG_CONFIG_HOME/lux/fmt-all.toml" <<'TOML'
walk = "all"
exclude_globs = ["flake.lock", "**/flake.lock"]
TOML

  run "$lux" fmt-all
  assert_success

  # flake.lock should NOT have been reformatted
  run cat "$PROJECT_DIR/flake.lock"
  assert_output '{"a":1}'
}

function fmt_all_with_explicit_file_path_limits_scope { # @test
  echo '{"a":1}' >"$PROJECT_DIR/a.json"
  echo '{"b":2}' >"$PROJECT_DIR/b.json"

  run "$lux" fmt-all a.json
  assert_success
}

function fmt_all_exits_0_even_when_a_formatter_fails_on_one_file { # @test
  echo "not valid go at all {{{" >"$PROJECT_DIR/bad.go"
  echo '{"ok":true}' >"$PROJECT_DIR/good.json"

  run "$lux" fmt-all
  assert_success
}
