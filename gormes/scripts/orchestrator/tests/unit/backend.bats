#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib backend
  unset BACKEND
  unset MODE
}

@test "BACKEND=codexu MODE=safe build_backend_cmd emits codexu argv" {
  BACKEND=codexu MODE=safe run bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd | tr "\0" "\n"'
  assert_success
  # First line should be the binary; then the translated flags.
  assert_line --index 0 'codexu'
  assert_line --index 1 'exec'
  assert_line --index 2 '--json'
  assert_line --index 3 '-m'
  assert_line --index 4 'gpt-5.5'
  assert_line --index 5 '-c'
  assert_line --index 6 'approval_policy=never'
  assert_line --index 7 '--sandbox'
  assert_line --index 8 'workspace-write'
}

@test "BACKEND=codexu MODE=full uses danger-full-access sandbox" {
  BACKEND=codexu MODE=full run bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd | tr "\0" "\n"'
  assert_success
  assert_line --index 0 'codexu'
  assert_line --index 4 'gpt-5.5'
  assert_line --index 8 'danger-full-access'
}

@test "BACKEND=claudeu MODE=safe emits claudeu-prefixed argv" {
  BACKEND=claudeu MODE=safe run bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd | tr "\0" "\n"'
  assert_success
  assert_line --index 0 'claudeu'
  assert_line --index 1 'exec'
  assert_line --index 2 '--json'
  assert_line --index 6 'workspace-write'
}

@test "BACKEND=opencode emits opencode run --no-interactive" {
  BACKEND=opencode MODE=safe run bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd | tr "\0" "\n"'
  assert_success
  assert_line --index 0 'opencode'
  assert_line --index 1 'run'
  assert_line --index 2 '--no-interactive'
}

@test "BACKEND=bogus build_backend_cmd fails with informative error" {
  BACKEND=bogus MODE=safe run bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd'
  assert_failure
  assert_output --partial 'unknown BACKEND=bogus'
}

@test "BACKEND unset defaults to codexu" {
  unset BACKEND
  MODE=safe run bash -c 'unset BACKEND; source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd | tr "\0" "\n"'
  assert_success
  assert_line --index 0 'codexu'
  assert_line --index 4 'gpt-5.5'
}

@test "MODE invalid returns non-zero for codexu backend" {
  BACKEND=codexu MODE=wacky run bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd'
  assert_failure
  assert_output --partial 'invalid MODE=wacky'
}

@test "build_codex_cmd alias produces identical output to build_backend_cmd" {
  local alias_out direct_out
  alias_out="$(BACKEND=codexu MODE=safe bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_codex_cmd | tr "\0" "|"')"
  direct_out="$(BACKEND=codexu MODE=safe bash -c 'source "'"$ORCHESTRATOR_LIB_DIR"'/backend.sh"; build_backend_cmd | tr "\0" "|"')"
  [[ "$alias_out" == "$direct_out" ]]
}
