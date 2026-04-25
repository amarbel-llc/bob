#!/usr/bin/env bats

function fence_runs_a_command_inside_sandbox { # @test
  run fence --settings "$BATS_TEST_DIRNAME/fence.jsonc" -- echo hello
  echo "status=$status output=$output"
  [ "$status" -eq 0 ]
  [[ "$output" == *hello* ]]
}

function fence_passes_curl_to_allowed_domain { # @test
  run fence --settings "$BATS_TEST_DIRNAME/fence.jsonc" -- curl -fsSL --max-time 5 https://example.com
  echo "status=$status"
  [ "$status" -eq 0 ]
}
