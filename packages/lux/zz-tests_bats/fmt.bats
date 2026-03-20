#!/usr/bin/env bats

# Integration tests for `lux fmt` with invalid Go syntax.

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  local lux_bin="${LUX_BIN:-lux}"

  # Create a fake formatter that rejects invalid syntax (mimics gofumpt).
  fake_gofumpt="$BATS_TEST_TMPDIR/fake-gofumpt"
  cat > "$fake_gofumpt" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
input=$(cat)
if echo "$input" | grep -q 'func main('; then
  echo "$input"
else
  echo "1:1: expected declaration, found invalid_token" >&2
  exit 1
fi
SCRIPT
  chmod +x "$fake_gofumpt"

  # Write filetype config for .go files with a formatter reference.
  local lux_config_dir="$XDG_CONFIG_HOME/lux"
  mkdir -p "$lux_config_dir/filetype"

  cat > "$lux_config_dir/filetype/go.toml" <<'TOML'
extensions = [".go"]
language_ids = ["go"]
formatters = ["gofumpt"]
formatter_mode = "chain"
TOML

  cat > "$lux_config_dir/formatters.toml" <<TOML
[[formatter]]
name = "gofumpt"
path = "${fake_gofumpt}"
TOML
}

teardown() {
  teardown_test_home
}

function fmt_fails_on_invalid_go_syntax { # @test
  local bad_file="$BATS_TEST_TMPDIR/bad.go"
  cat > "$bad_file" <<'GO'
package main

func {{{ invalid syntax
GO

  run lux fmt --file "$bad_file"
  assert_failure
  assert_output --partial "expected declaration"
}

function fmt_succeeds_on_valid_go_syntax { # @test
  local good_file="$BATS_TEST_TMPDIR/good.go"
  cat > "$good_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$good_file" --stdout
  assert_success
  assert_output --partial "func main()"
}

function fmt_does_not_modify_file_on_failure { # @test
  local bad_file="$BATS_TEST_TMPDIR/bad.go"
  cat > "$bad_file" <<'GO'
package main

func {{{ invalid syntax
GO

  local original
  original=$(cat "$bad_file")

  run lux fmt --file "$bad_file"
  assert_failure

  # File contents must be unchanged after a formatter failure.
  run cat "$bad_file"
  assert_output "$original"
}
