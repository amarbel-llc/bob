#!/usr/bin/env bats

# Integration tests for lux MCP resources against real LSPs.
# These tests start lux in MCP-stdio mode and send JSON-RPC requests
# to exercise the lux://lsp/* resource templates with actual gopls responses.
#
# Requires: nix build .#lux (binary at result/bin/lux), gopls on PATH (devShell).

setup() {
  load "$(dirname "$BATS_TEST_FILE")/common.bash"
  setup_test_home
  export output

  lux="$(result_dir)/bin/lux"
  [[ -x "$lux" ]] || skip "lux binary not found at $lux — run: nix build .#lux"

  # Resolve gopls store path from the devShell PATH so lux does not need
  # network access (sandcastle blocks it) to nix-build gopls at runtime.
  local gopls_bin
  gopls_bin="$(command -v gopls 2>/dev/null)" || skip "gopls not on PATH"
  local gopls_store_path
  gopls_store_path="$(dirname "$(dirname "$gopls_bin")")"

  # Write minimal lux config pointing to the pre-built gopls store path.
  local lux_config_dir="$XDG_CONFIG_HOME/lux"
  mkdir -p "$lux_config_dir/filetype"

  cat > "$lux_config_dir/lsps.toml" <<TOML
[[lsp]]
name = "gopls"
flake = "${gopls_store_path}"

[lsp.settings]
  [lsp.settings.gopls]
    staticcheck = false
TOML

  cat > "$lux_config_dir/filetype/go.toml" <<'TOML'
extensions = [".go"]
language_ids = ["go"]
lsp = "gopls"
TOML

  # Use the lux package source as the Go project for gopls to analyze.
  test_project="${BATS_CWD}/packages/lux"
  test_file="${test_project}/internal/server/router.go"
  test_file_uri="file://${test_file}"
}

teardown() {
  teardown_test_home
}

# Send a JSON-RPC resources/read request to lux mcp-stdio.
# Sends initialize, waits briefly for it to complete, then sends the resource
# read. Keeps stdin open with sleep so lux stays alive while gopls starts.
# Usage: read_lux_resource <uri> [timeout_secs]
read_lux_resource() {
  local uri="$1"
  local timeout_secs="${2:-90}"

  local init_request='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"bats-test","version":"0.0.1"}}}'
  local initialized_notification='{"jsonrpc":"2.0","method":"notifications/initialized"}'
  local read_request
  read_request=$(jq -cn --arg uri "$uri" \
    '{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":$uri}}')

  local response
  # Send init, wait for it to be processed, then send the read request.
  # The trailing sleep keeps stdin open while lux processes the request
  # (gopls needs time to initialize). grep -m1 exits on first match,
  # which tears down the pipeline.
  response=$( (
    printf '%s\n%s\n' "$init_request" "$initialized_notification"
    sleep 1
    printf '%s\n' "$read_request"
    sleep "$timeout_secs"
  ) | timeout --preserve-status "$((timeout_secs + 10))s" "$lux" mcp-stdio 2>/dev/null \
    | grep -m1 -F '"id":2')

  if [ -z "$response" ]; then
    echo "no response from lux"
    return 1
  fi

  # Check for JSON-RPC error
  local err
  err=$(echo "$response" | jq -r '.error.message // empty')
  if [ -n "$err" ]; then
    echo "JSON-RPC error: $err"
    return 1
  fi

  echo "$response" | jq -r '.result.contents[0].text'
}

# --- Hover tests ---

function hover_returns_json_by_default { # @test
  # NewRouter is at line 19 (0-indexed: 18), character 5
  local uri="lux://lsp/hover?uri=${test_file_uri}&line=18&character=5"
  run read_lux_resource "$uri" 90
  assert_success

  # Output should be valid JSON with a content field (hover result)
  run jq -e '.content' <<< "$output"
  assert_success
}

function hover_returns_text_with_format_param { # @test
  local uri="lux://lsp/hover?uri=${test_file_uri}&line=18&character=5&format=text"
  run read_lux_resource "$uri" 90
  assert_success

  # Text format should contain markdown code fence from gopls hover output
  assert_output --partial '```'
}

# --- References tests ---

function references_returns_enriched_json { # @test
  # Route method at line 37 (0-indexed: 36), character 18 (on "Route")
  local uri="lux://lsp/references?uri=${test_file_uri}&line=36&character=18&context=3"
  run read_lux_resource "$uri" 90
  assert_success

  local refs_output="$output"

  # Should be valid JSON with references array
  run jq -e '.references' <<< "$refs_output"
  assert_success

  # Each ref should have context with line when context > 0
  run jq -e '.references[0].context.line' <<< "$refs_output"
  assert_success
}

# --- Incoming calls tests ---

function incoming_calls_returns_json { # @test
  # NewRouter at line 19 (0-indexed: 18), character 5
  local uri="lux://lsp/incoming-calls?uri=${test_file_uri}&line=18&character=5"
  run read_lux_resource "$uri" 90
  assert_success

  local calls_output="$output"

  # Should be valid JSON with symbol and calls fields
  run jq -e '.symbol' <<< "$calls_output"
  assert_success
  run jq -e '.calls' <<< "$calls_output"
  assert_success
}

# --- Batch diagnostics tests ---

function diagnostics_batch_returns_results_for_go_files { # @test
  local uri="lux://lsp/diagnostics-batch?glob=packages/lux/internal/tools/*.go"
  run read_lux_resource "$uri" 120
  assert_success

  local diag_output="$output"

  # Should be valid JSON with lsps array
  run jq -e '.lsps' <<< "$diag_output"
  assert_success

  # First LSP should be gopls
  run jq -r '.lsps[0].name' <<< "$diag_output"
  assert_success
  assert_output "gopls"
}
