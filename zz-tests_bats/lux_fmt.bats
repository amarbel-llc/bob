#!/usr/bin/env bats

# Integration tests for lux fmt with default formatter configs.
# Each test writes a deliberately mis-formatted file and verifies
# lux fmt --stdout produces corrected output.
#
# Requires: nix build .#lux

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  lux="$(result_dir)/bin/lux"
  [[ -x "$lux" ]] || skip "lux binary not found at $lux — run: nix build .#lux"

  # Initialize default config so all formatters and filetypes are registered.
  "$lux" init --default --force
}

# --- Go: goimports + gofumpt chain ---

# @test "go: gofumpt removes empty line at start of function body" {
  local f="$BATS_TEST_TMPDIR/test.go"
  cat > "$f" <<'GO'
package main

func main() {

	println("hi")
}
GO
  run "$lux" fmt --stdout "$f"
  assert_success
  refute_output --partial $'\nfunc main() {\n\n'
  assert_output --partial $'func main() {\n\tprintln("hi")'
}

# --- Shell: shfmt ---

# @test "shell: shfmt normalizes indent to 2 spaces" {
  local f="$BATS_TEST_TMPDIR/test.sh"
  cat > "$f" <<'SH'
#!/bin/bash
if true; then
    echo "4 spaces"
fi
SH
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial '  echo "4 spaces"'
}

# --- JSON: jq ---

# @test "json: jq formats compact json" {
  local f="$BATS_TEST_TMPDIR/test.json"
  echo '{"a":1,"b":[2,3]}' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial '"a": 1'
  assert_output --partial '"b": ['
}

# --- Rust: rustfmt ---

# @test "rust: rustfmt formats function" {
  local f="$BATS_TEST_TMPDIR/test.rs"
  echo 'fn main(){println!("hi");}' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'fn main() {'
}

# --- TOML: tommy ---

# @test "toml: tommy formats spacing" {
  local f="$BATS_TEST_TMPDIR/test.toml"
  echo 'name="test"' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'name = "test"'
}

# --- CSS: prettier ---

# @test "css: prettier formats compact css" {
  local f="$BATS_TEST_TMPDIR/test.css"
  echo 'body{color:red;margin:0}' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'body {'
  assert_output --partial '  color: red;'
}

# --- HTML: prettier ---

# @test "html: prettier formats compact html" {
  local f="$BATS_TEST_TMPDIR/test.html"
  echo '<html><body><p>hi</p></body></html>' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial '<html>'
  assert_output --partial '    <p>hi</p>'
}

# --- YAML: prettier ---

# @test "yaml: prettier formats yaml" {
  local f="$BATS_TEST_TMPDIR/test.yaml"
  printf 'a:   1\nb: 2\n' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'a: 1'
  assert_output --partial 'b: 2'
}

# --- Lua: stylua ---

# @test "lua: stylua formats indentation" {
  local f="$BATS_TEST_TMPDIR/test.lua"
  echo '  local x=1' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'local x = 1'
}

# --- Zig: zig fmt ---

# @test "zig: zig fmt adds spacing" {
  local f="$BATS_TEST_TMPDIR/test.zig"
  echo 'const x:u32=42;' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'const x: u32 = 42;'
}

# --- C: clang-format ---

# @test "c: clang-format formats function" {
  local f="$BATS_TEST_TMPDIR/test.c"
  echo 'int main(){return 0;}' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'int main() {'
}

# --- Java: google-java-format ---

# @test "java: google-java-format formats class" {
  local f="$BATS_TEST_TMPDIR/Test.java"
  echo 'public class Test{public static void main(String[] args){System.out.println("hi");}}' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'public class Test {'
  assert_output --partial '  public static void main'
}

# --- Swift: swift-format ---

# @test "swift: swift-format formats function" {
  local f="$BATS_TEST_TMPDIR/test.swift"
  echo 'func greet(name:String){print("Hello")}' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial 'func greet(name: String)'
}

# --- Nix: nixfmt ---

# @test "nix: nixfmt formats expression" {
  local f="$BATS_TEST_TMPDIR/test.nix"
  echo '{pkgs}:pkgs.hello' > "$f"
  run "$lux" fmt --stdout "$f"
  assert_success
  assert_output --partial '{ pkgs }:'
}
