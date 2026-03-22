#!/usr/bin/env bats

# Integration tests for lux lsp subcommand.
# Layer 1: Protocol-level tests (JSON-RPC over stdio with Content-Length framing).
# Layer 2: Neovim integration tests (real editor, headless).
#
# Requires: nix build .#lux, gopls on PATH (devShell).

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  lux="$(result_dir)/bin/lux"
  [[ -x "$lux" ]] || skip "lux binary not found at $lux — run: nix build .#lux"

  # Resolve gopls store path from devShell to avoid network access.
  local gopls_bin
  gopls_bin="$(command -v gopls 2>/dev/null)" || skip "gopls not on PATH"
  gopls_store_path="$(dirname "$(dirname "$gopls_bin")")"

  # Write minimal lux config.
  local lux_config_dir="$XDG_CONFIG_HOME/lux"
  mkdir -p "$lux_config_dir/filetype"

  cat > "$lux_config_dir/lsps.toml" <<TOML
[[lsp]]
name = "gopls"
flake = "${gopls_store_path}"
TOML

  cat > "$lux_config_dir/formatters.toml" <<'TOML'
TOML

  cat > "$lux_config_dir/filetype/go.toml" <<'TOML'
extensions = ["go"]
language_ids = ["go"]
lsp = "gopls"
TOML

  fixtures_dir="${BATS_TEST_DIRNAME}/fixtures/lsp"

  # Create a minimal Go project for gopls.
  mkdir -p "${BATS_TEST_TMPDIR}/project"
  echo 'module test' > "${BATS_TEST_TMPDIR}/project/go.mod"
}

teardown() {
  teardown_test_home
}

# Send a Content-Length framed JSON-RPC message to stdout.
lsp_frame() {
  local msg="$1"
  printf 'Content-Length: %d\r\n\r\n%s' "${#msg}" "$msg"
}

# Run lux lsp with timed input messages, capture output to a file.
# Messages are sent with sleep delays so lux has time to process each one.
# Usage: lux_lsp_session <output_file> <timeout_secs> [lux_args...]
# Reads message functions from the caller via the LSP_SEND_MESSAGES function.
#
# Example:
#   send_messages() {
#     lsp_frame "$init_msg"
#     sleep 1
#     lsp_frame "$shutdown_msg"
#   }
#   LSP_SEND_MESSAGES=send_messages lux_lsp_session "$outfile" 5 lsp
lux_lsp_session() {
  local output_file="$1"
  local timeout_secs="$2"
  shift 2

  # Use process substitution: feed messages via a subshell, capture output to file.
  # timeout wraps the entire pipeline so nothing hangs.
  timeout --signal=KILL "${timeout_secs}s" bash -c '
    eval "$(declare -f lsp_frame)"
    eval "$LSP_SEND_MESSAGES_BODY"
    send_messages
  ' | "$lux" "$@" > "$output_file" 2>/dev/null || true
}

# Extract a JSON-RPC response from a Content-Length framed output file.
# Usage: extract_lsp_response <file> <id>
extract_lsp_response() {
  local file="$1"
  local target_id="$2"
  python3 -c "
import json, re

with open('$file', 'rb') as f:
    data = f.read()
text = data.decode('utf-8', errors='replace')

for m in re.finditer(r'Content-Length:\s*(\d+)\r?\n\r?\n', text):
    length = int(m.group(1))
    body = text[m.end():m.end() + length]
    try:
        obj = json.loads(body)
        if obj.get('id') == $target_id:
            print(json.dumps(obj))
            exit(0)
    except (json.JSONDecodeError, ValueError):
        continue
exit(1)
"
}

# --- Layer 1: Protocol-level tests ---

function lux_lsp_responds_to_initialize { # @test
  local init_msg='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":"file:///tmp","capabilities":{}}}'
  local shutdown_msg='{"jsonrpc":"2.0","id":99,"method":"shutdown","params":null}'
  local exit_msg='{"jsonrpc":"2.0","method":"exit","params":null}'

  local output_file="${BATS_TEST_TMPDIR}/output.bin"

  (
    lsp_frame "$init_msg"
    sleep 2
    lsp_frame "$shutdown_msg"
    sleep 1
    lsp_frame "$exit_msg"
  ) | timeout --signal=KILL 10s "$lux" lsp > "$output_file" 2>/dev/null || true

  local response
  response="$(extract_lsp_response "$output_file" 1)"

  # Should have a result with capabilities
  run jq -e '.result.capabilities.documentFormattingProvider' <<< "$response"
  assert_success
  assert_output "true"

  # Should report serverInfo
  run jq -r '.result.serverInfo.name' <<< "$response"
  assert_success
  assert_output "lux"
}

function lux_lsp_advertises_formatting_only_in_phase_1 { # @test
  local init_msg='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":"file:///tmp","capabilities":{}}}'
  local shutdown_msg='{"jsonrpc":"2.0","id":99,"method":"shutdown","params":null}'
  local exit_msg='{"jsonrpc":"2.0","method":"exit","params":null}'

  local output_file="${BATS_TEST_TMPDIR}/output.bin"

  (
    lsp_frame "$init_msg"
    sleep 2
    lsp_frame "$shutdown_msg"
    sleep 1
    lsp_frame "$exit_msg"
  ) | timeout --signal=KILL 10s "$lux" lsp > "$output_file" 2>/dev/null || true

  local response
  response="$(extract_lsp_response "$output_file" 1)"

  # Phase 1: formatting providers should be true
  run jq -e '.result.capabilities.documentFormattingProvider' <<< "$response"
  assert_success
  assert_output "true"

  run jq -e '.result.capabilities.documentRangeFormattingProvider' <<< "$response"
  assert_success
  assert_output "true"

  # Phase 1: other providers should NOT be present
  run jq -e '.result.capabilities.hoverProvider // null' <<< "$response"
  assert_output "null"

  run jq -e '.result.capabilities.definitionProvider // null' <<< "$response"
  assert_output "null"
}

function lux_lsp_formats_go_file_via_gopls { # @test
  local go_file="${BATS_TEST_TMPDIR}/project/main.go"
  cp "${fixtures_dir}/unformatted.go" "$go_file"

  local file_uri="file://${go_file}"
  local file_content
  file_content="$(cat "$go_file")"

  # Escape the content for JSON (newlines become \n, tabs become \t)
  local escaped_content
  escaped_content="$(python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))" < "$go_file")"

  local init_msg='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":"file://'"${BATS_TEST_TMPDIR}/project"'","capabilities":{"textDocument":{"formatting":{"dynamicRegistration":false}}}}}'
  local initialized_msg='{"jsonrpc":"2.0","method":"initialized","params":{}}'
  local did_open_msg='{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"'"$file_uri"'","languageId":"go","version":1,"text":'"$escaped_content"'}}}'
  local format_msg='{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":"'"$file_uri"'"},"options":{"tabSize":4,"insertSpaces":false}}}'
  local shutdown_msg='{"jsonrpc":"2.0","id":99,"method":"shutdown","params":null}'
  local exit_msg='{"jsonrpc":"2.0","method":"exit","params":null}'

  local output_file="${BATS_TEST_TMPDIR}/output.bin"

  (
    lsp_frame "$init_msg"
    sleep 1
    lsp_frame "$initialized_msg"
    sleep 5
    lsp_frame "$did_open_msg"
    sleep 5
    lsp_frame "$format_msg"
    sleep 5
    lsp_frame "$shutdown_msg"
    sleep 1
    lsp_frame "$exit_msg"
  ) | timeout --signal=KILL 30s "$lux" lsp > "$output_file" 2>/dev/null || true

  local response
  response="$(extract_lsp_response "$output_file" 2)"

  # Should have a result (array of text edits)
  run jq -e '.result' <<< "$response"
  assert_success

  # Save response to file and apply edits
  echo "$response" > "${BATS_TEST_TMPDIR}/response.json"

  python3 -c "
import json, sys

with open(sys.argv[1], 'r') as f:
    lines = f.read().split('\n')

with open(sys.argv[2], 'r') as f:
    response = json.load(f)

edits = response.get('result', [])
edits.sort(key=lambda e: (e['range']['start']['line'], e['range']['start']['character']), reverse=True)

for edit in edits:
    s = edit['range']['start']
    e = edit['range']['end']
    before = lines[s['line']][:s['character']] if s['line'] < len(lines) else ''
    after = lines[e['line']][e['character']:] if e['line'] < len(lines) else ''
    replacement_lines = (before + edit['newText'] + after).split('\n')
    lines[s['line']:e['line'] + 1] = replacement_lines

sys.stdout.write('\n'.join(lines))
" "$go_file" "${BATS_TEST_TMPDIR}/response.json" > "${BATS_TEST_TMPDIR}/formatted.go"

  run diff -u "${fixtures_dir}/expected.go" "${BATS_TEST_TMPDIR}/formatted.go"
  assert_success
}

# --- Layer 2: Neovim integration tests ---

function lux_lsp_neovim_formats_go_file { # @test
  command -v nvim >/dev/null 2>&1 || skip "nvim not on PATH"

  local go_file="${BATS_TEST_TMPDIR}/project/main.go"
  cp "${fixtures_dir}/unformatted.go" "$go_file"

  local output_go="${BATS_TEST_TMPDIR}/project/formatted.go"

  LUX_CMD="$lux lsp" \
  LUX_INPUT="$go_file" \
  LUX_OUTPUT="$output_go" \
    timeout --signal=KILL 120s \
    nvim --headless --clean -c "luafile ${fixtures_dir}/format.lua" 2>"${BATS_TEST_TMPDIR}/nvim_stderr.log" || true

  # Check nvim produced output
  [[ -f "$output_go" ]] || {
    echo "nvim stderr:"
    cat "${BATS_TEST_TMPDIR}/nvim_stderr.log" >&2
    fail "output file not created"
  }

  run diff -u "${fixtures_dir}/expected.go" "$output_go"
  assert_success
}

function lux_lsp_neovim_attaches_to_go_file { # @test
  command -v nvim >/dev/null 2>&1 || skip "nvim not on PATH"

  local go_file="${BATS_TEST_TMPDIR}/project/main.go"
  cp "${fixtures_dir}/expected.go" "$go_file"

  run timeout --signal=KILL 60s env \
    LUX_CMD="$lux lsp" \
    LUX_FILE="$go_file" \
    nvim --headless --clean -c "luafile ${fixtures_dir}/check_attach.lua"

  assert_success
}

function lux_lsp_neovim_does_not_attach_to_non_matching_filetype { # @test
  command -v nvim >/dev/null 2>&1 || skip "nvim not on PATH"

  local txt_file="${BATS_TEST_TMPDIR}/project/readme.txt"
  echo "hello" > "$txt_file"

  run timeout --signal=KILL 15s env \
    LUX_CMD="$lux lsp" \
    LUX_FILE="$txt_file" \
    nvim --headless --clean -c "luafile ${fixtures_dir}/check_no_attach.lua"

  assert_success
}

function lux_lsp_neovim_clean_shutdown { # @test
  command -v nvim >/dev/null 2>&1 || skip "nvim not on PATH"

  local go_file="${BATS_TEST_TMPDIR}/project/main.go"
  cp "${fixtures_dir}/expected.go" "$go_file"

  run timeout --signal=KILL 60s env \
    LUX_CMD="$lux lsp" \
    LUX_FILE="$go_file" \
    nvim --headless --clean -c "luafile ${fixtures_dir}/check_shutdown.lua"

  assert_success
}

function lux_lsp_returns_method_not_found_for_hover { # @test
  local init_msg='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":"file:///tmp","capabilities":{}}}'
  local hover_msg='{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":"file:///tmp/test.go"},"position":{"line":0,"character":0}}}'
  local shutdown_msg='{"jsonrpc":"2.0","id":99,"method":"shutdown","params":null}'
  local exit_msg='{"jsonrpc":"2.0","method":"exit","params":null}'

  local output_file="${BATS_TEST_TMPDIR}/output.bin"

  (
    lsp_frame "$init_msg"
    sleep 1
    lsp_frame "$hover_msg"
    sleep 2
    lsp_frame "$shutdown_msg"
    sleep 1
    lsp_frame "$exit_msg"
  ) | timeout --signal=KILL 10s "$lux" lsp > "$output_file" 2>/dev/null || true

  local response
  response="$(extract_lsp_response "$output_file" 2)"

  # Should return MethodNotFound error
  run jq -e '.error.code' <<< "$response"
  assert_success
  assert_output -- "-32601"
}
