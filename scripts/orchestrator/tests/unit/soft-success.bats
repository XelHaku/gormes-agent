#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export TMP_WS
}

@test "legacy entry script can be source-loaded for direct soft-success tests" {
  run env \
    GORMES_ORCHESTRATOR_SOURCE_ONLY=1 \
    REPO_ROOT="$TMP_WS/repo" \
    RUN_ROOT="$TMP_WS/run" \
    PATH="$FIXTURES_DIR/bin:$PATH" \
    bash -c 'source "$1"; declare -F try_soft_success_nonzero >/dev/null' _ "$ENTRY_SCRIPT"

  assert_success
}
