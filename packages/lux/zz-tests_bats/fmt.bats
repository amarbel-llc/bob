#!/usr/bin/env bats

# Integration tests for `lux fmt` — error handling, truncation safety, config edge cases.

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  lux_config_dir="$XDG_CONFIG_HOME/lux"
  mkdir -p "$lux_config_dir/filetype"

  # Create fake formatters in BATS_TEST_TMPDIR.

  # fake-gofumpt: rejects invalid syntax, echoes valid input.
  fake_gofumpt="$BATS_TEST_TMPDIR/fake-gofumpt"
  cat >"$fake_gofumpt" <<'SCRIPT'
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

  # fake-empty: exits 0 but produces no output (truncation bug).
  fake_empty="$BATS_TEST_TMPDIR/fake-empty"
  cat >"$fake_empty" <<'SCRIPT'
#!/usr/bin/env bash
cat > /dev/null
exit 0
SCRIPT
  chmod +x "$fake_empty"

  # fake-ignores-stdin: consumes stdin, writes nothing to stdout, exits 0.
  # Mimics a filepath-mode formatter misconfigured as stdin mode — it expects
  # a file path argument and ignores stdin, producing no stdout.
  fake_ignores_stdin="$BATS_TEST_TMPDIR/fake-ignores-stdin"
  cat >"$fake_ignores_stdin" <<'SCRIPT'
#!/usr/bin/env bash
cat > /dev/null
exit 0
SCRIPT
  chmod +x "$fake_ignores_stdin"

  # fake-missing: a path that does not exist.
  fake_missing="$BATS_TEST_TMPDIR/no-such-formatter"
}

# Write filetype + formatter config for a single formatter.
# Usage: write_config <formatter_name> <formatter_path> [filetype_mode] [formatter_mode]
write_config() {
  local name="$1"
  local path="$2"
  local filetype_mode="${3:-chain}"
  local formatter_mode="${4:-}"

  cat >"$lux_config_dir/filetype/go.toml" <<TOML
extensions = [".go"]
language_ids = ["go"]
formatters = ["${name}"]
formatter_mode = "${filetype_mode}"
TOML

  local mode_line=""
  if [[ -n $formatter_mode ]]; then
    mode_line="mode = \"${formatter_mode}\""
  fi

  cat >"$lux_config_dir/formatters.toml" <<TOML
[[formatter]]
name = "${name}"
path = "${path}"
${mode_line}
TOML
}

# Write filetype + formatter config for two chained formatters.
# Usage: write_chain_config <name1> <path1> <name2> <path2>
write_chain_config() {
  cat >"$lux_config_dir/filetype/go.toml" <<TOML
extensions = [".go"]
language_ids = ["go"]
formatters = ["${1}", "${3}"]
formatter_mode = "chain"
TOML

  cat >"$lux_config_dir/formatters.toml" <<TOML
[[formatter]]
name = "${1}"
path = "${2}"

[[formatter]]
name = "${3}"
path = "${4}"
TOML
}

teardown() {
  teardown_test_home
}

# --- Basic formatter behavior ---

function fmt_fails_on_invalid_go_syntax { # @test
  write_config "gofumpt" "$fake_gofumpt"

  local bad_file="$BATS_TEST_TMPDIR/bad.go"
  cat >"$bad_file" <<'GO'
package main

func {{{ invalid syntax
GO

  run lux fmt --file "$bad_file"
  assert_failure
  assert_output --partial "expected declaration"
}

function fmt_succeeds_on_valid_go_syntax { # @test
  write_config "gofumpt" "$fake_gofumpt"

  local good_file="$BATS_TEST_TMPDIR/good.go"
  cat >"$good_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$good_file" --stdout
  assert_success
  assert_output --partial "func main()"
}

function fmt_does_not_modify_file_on_failure { # @test
  write_config "gofumpt" "$fake_gofumpt"

  local bad_file="$BATS_TEST_TMPDIR/bad.go"
  cat >"$bad_file" <<'GO'
package main

func {{{ invalid syntax
GO

  local original
  original=$(cat "$bad_file")

  run lux fmt --file "$bad_file"
  assert_failure

  run cat "$bad_file"
  assert_output "$original"
}

# --- Truncation safety ---

function fmt_with_empty_stdout_formatter_does_not_truncate_file { # @test
  write_config "empty-fmt" "$fake_empty"

  local go_file="$BATS_TEST_TMPDIR/truncate.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  local original
  original=$(cat "$go_file")
  local original_size=${#original}

  run lux fmt --file "$go_file"

  # Whether lux treats this as success or failure, the file must not be empty.
  local after
  after=$(cat "$go_file")
  local after_size=${#after}

  [[ $after_size -gt 0 ]] || {
    echo "file was truncated to 0 bytes"
    return 1
  }
}

function fmt_stdout_with_empty_formatter_does_not_produce_empty_output { # @test
  write_config "empty-fmt" "$fake_empty"

  local go_file="$BATS_TEST_TMPDIR/truncate.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$go_file" --stdout

  # If the formatter produces empty output, lux should either error or
  # preserve the original content — never emit nothing.
  if [[ $status -eq 0 ]]; then
    [[ -n $output ]] || {
      echo "stdout was empty on success — content would be lost"
      return 1
    }
  fi
}

function fmt_chain_with_empty_first_formatter_does_not_truncate { # @test
  write_chain_config "empty-fmt" "$fake_empty" "gofumpt" "$fake_gofumpt"

  local go_file="$BATS_TEST_TMPDIR/chain.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  local original
  original=$(cat "$go_file")

  run lux fmt --file "$go_file"

  local after
  after=$(cat "$go_file")
  local after_size=${#after}

  [[ $after_size -gt 0 ]] || {
    echo "file was truncated to 0 bytes after chain formatting"
    return 1
  }
}

# --- Stdin/filepath mode mismatch ---

function fmt_filepath_formatter_in_stdin_mode_does_not_truncate { # @test
  # A filepath-mode formatter misconfigured as stdin mode: it consumes stdin,
  # produces no stdout, and exits 0. Previously lux would treat the empty
  # stdout as the formatted result and truncate the file to 0 bytes.
  write_config "ignores-stdin" "$fake_ignores_stdin" "chain" "stdin"

  local go_file="$BATS_TEST_TMPDIR/mismatch.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$go_file"
  assert_failure
  assert_output --partial "produced empty output"
  assert_output --partial 'mode = "filepath"'

  # File must be preserved.
  local after
  after=$(cat "$go_file")
  [[ ${#after} -gt 0 ]] || {
    echo "file was truncated to 0 bytes"
    return 1
  }
}

# --- Missing/broken formatter ---

function fmt_fails_when_formatter_binary_missing { # @test
  write_config "missing-fmt" "$fake_missing"

  local go_file="$BATS_TEST_TMPDIR/missing.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  local original
  original=$(cat "$go_file")

  run lux fmt --file "$go_file"
  assert_failure

  # File must be preserved when formatter cannot be found.
  run cat "$go_file"
  assert_output "$original"
}

function fmt_fails_when_no_formatter_configured { # @test
  # Write filetype config without any formatters.
  cat >"$lux_config_dir/filetype/go.toml" <<'TOML'
extensions = [".go"]
language_ids = ["go"]
TOML
  cat >"$lux_config_dir/formatters.toml" <<'TOML'
TOML

  local go_file="$BATS_TEST_TMPDIR/noconfig.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$go_file"
  assert_failure
  assert_output --partial "no formatter configured"
}

# --- goimports invocation ---

function fmt_invokes_goimports_when_configured { # @test
  # Fake goimports: records that it was called by writing a sentinel file,
  # then echoes stdin back (acts as identity formatter).
  local fake_goimports="$BATS_TEST_TMPDIR/fake-goimports"
  local sentinel="$BATS_TEST_TMPDIR/goimports-was-called"
  cat >"$fake_goimports" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
touch "$sentinel"
cat
SCRIPT
  chmod +x "$fake_goimports"

  write_config "goimports" "$fake_goimports"

  local go_file="$BATS_TEST_TMPDIR/test.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$go_file"
  assert_success

  [[ -f $sentinel ]] || {
    echo "goimports was never invoked"
    return 1
  }
}

function fmt_silently_skips_formatter_not_in_formatters_toml { # @test
  # Configure goimports in the filetype config but do NOT define it in
  # formatters.toml. The router silently drops unknown formatter names,
  # resulting in "no formatter configured" instead of a clear error.
  local fake_goimports="$BATS_TEST_TMPDIR/fake-goimports"
  local sentinel="$BATS_TEST_TMPDIR/goimports-was-called"
  cat >"$fake_goimports" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
touch "$sentinel"
cat
SCRIPT
  chmod +x "$fake_goimports"

  # Filetype references goimports...
  cat >"$lux_config_dir/filetype/go.toml" <<'TOML'
extensions = [".go"]
language_ids = ["go"]
formatters = ["goimports"]
TOML

  # ...but formatters.toml is empty (goimports not defined).
  cat >"$lux_config_dir/formatters.toml" <<'TOML'
TOML

  local go_file="$BATS_TEST_TMPDIR/test.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$go_file"
  assert_failure
  assert_output --partial 'references formatter "goimports"'
  assert_output --partial "not defined in formatters.toml"

  # goimports was never called.
  [[ ! -f $sentinel ]] || {
    echo "goimports should not have been called"
    return 1
  }
}

function fmt_silently_drops_undefined_formatter_from_chain { # @test
  # Configure a chain of [goimports, gofumpt] in the filetype but only define
  # gofumpt in formatters.toml. The router silently drops goimports from the
  # chain without warning — the user thinks both are running but only one is.
  local sentinel="$BATS_TEST_TMPDIR/goimports-was-called"

  cat >"$lux_config_dir/filetype/go.toml" <<'TOML'
extensions = [".go"]
language_ids = ["go"]
formatters = ["goimports", "gofumpt"]
formatter_mode = "chain"
TOML

  # Only gofumpt is defined; goimports is missing.
  cat >"$lux_config_dir/formatters.toml" <<TOML
[[formatter]]
name = "gofumpt"
path = "$fake_gofumpt"
TOML

  local go_file="$BATS_TEST_TMPDIR/test.go"
  cat >"$go_file" <<'GO'
package main

func main() {}
GO

  run lux fmt --file "$go_file" --stdout
  assert_failure
  assert_output --partial 'references formatter "goimports"'
  assert_output --partial "not defined in formatters.toml"

  [[ ! -f $sentinel ]] || {
    echo "goimports should not have been called (it's not even defined)"
    return 1
  }
}

function fmt_goimports_adds_missing_import { # @test
  # Fake goimports: simulates adding a missing import for fmt.Println.
  local fake_goimports="$BATS_TEST_TMPDIR/fake-goimports"
  cat >"$fake_goimports" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
input=$(cat)
# If input uses fmt.Println but doesn't import "fmt", add it.
if echo "$input" | grep -q 'fmt\.Println' && ! echo "$input" | grep -q '"fmt"'; then
  cat <<'GO'
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
GO
else
  echo "$input"
fi
SCRIPT
  chmod +x "$fake_goimports"

  write_config "goimports" "$fake_goimports"

  local go_file="$BATS_TEST_TMPDIR/needs-import.go"
  cat >"$go_file" <<'GO'
package main

func main() {
	fmt.Println("hello")
}
GO

  run lux fmt --file "$go_file" --stdout
  assert_success
  assert_output --partial 'import "fmt"'
}
