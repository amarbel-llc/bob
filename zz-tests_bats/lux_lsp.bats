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
