#!/usr/bin/env bash
# Shared helpers available to all bats files.
# Source this via `load '../lib/test_env'` inside bats setup().

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORCHESTRATOR_SCRIPTS_DIR="$(cd "$TESTS_DIR/../.." && pwd)"
ORCHESTRATOR_LIB_DIR="$ORCHESTRATOR_SCRIPTS_DIR/orchestrator/lib"
ENTRY_SCRIPT="$ORCHESTRATOR_SCRIPTS_DIR/gormes-auto-codexu-orchestrator.sh"
FIXTURES_DIR="$TESTS_DIR/fixtures"

load_helpers() {
  load "$TESTS_DIR/vendor/bats-support/load"
  load "$TESTS_DIR/vendor/bats-assert/load"
}

mktmp_workspace() {
  local base="${BATS_TEST_TMPDIR:-$(mktemp -d)}"
  local dir
  dir="$(mktemp -d "$base/ws.XXXXXX")"
  echo "$dir"
}

source_lib() {
  local name="$1"
  # shellcheck disable=SC1090
  source "$ORCHESTRATOR_LIB_DIR/${name}.sh"
}

export -f load_helpers mktmp_workspace source_lib
