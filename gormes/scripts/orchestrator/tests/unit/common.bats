#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
}

@test "classify_worker_failure maps 124 to timeout" {
  run classify_worker_failure 124
  assert_success
  assert_output "timeout"
}

@test "classify_worker_failure maps 137 to killed" {
  run classify_worker_failure 137
  assert_output "killed"
}

@test "classify_worker_failure maps 1 to contract_or_test_failure" {
  run classify_worker_failure 1
  assert_output "contract_or_test_failure"
}

@test "classify_worker_failure maps other to worker_error" {
  run classify_worker_failure 42
  assert_output "worker_error"
}

@test "safe_path_token strips unsafe characters" {
  run safe_path_token "Feat/sub phase: X_Y.Z@v1"
  assert_output "Feat-sub-phase-X_Y.Z-v1"
}

@test "safe_path_token trims leading and trailing dashes" {
  run safe_path_token "///foo///"
  assert_output "foo"
}

@test "require_cmd succeeds for a real command" {
  run require_cmd bash
  assert_success
}

@test "require_cmd fails for a bogus command" {
  run require_cmd bogus_cmd_that_does_not_exist_xyz
  assert_failure
}

@test "available_mem_mb returns a positive integer" {
  run available_mem_mb
  assert_success
  [[ "$output" =~ ^[0-9]+$ ]]
  (( output > 0 ))
}
