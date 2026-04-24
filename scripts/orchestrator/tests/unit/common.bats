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

@test "worker_status_outcome does not count failed soft-success task slug as success" {
  run worker_status_outcome "worker[3]: failed(1) -> 1__1.c__soft-success-nonzero-bats-coverage"
  assert_success
  assert_output "failed"
}

@test "worker_status_outcome maps explicit soft-success status to success" {
  run worker_status_outcome "worker[3]: soft-success(nonzero=1) -> task-slug (abcdef1)"
  assert_success
  assert_output "success"
}

@test "worker_status_outcome maps fail-fast abort status to aborted" {
  run worker_status_outcome "worker[4]: aborted-fail-fast -> prior worker failure"
  assert_success
  assert_output "aborted"
}

@test "abort_worker_pids terminates live worker pids" {
  sleep 30 &
  local pid="$!"

  run abort_worker_pids "unit-test" "$pid"
  assert_success

  local i
  for i in {1..20}; do
    if ! proc_alive "$pid"; then
      wait "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 0.05
  done

  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  fail "pid $pid was still alive after abort_worker_pids"
}

@test "abort_worker_pids terminates worker process trees" {
  bash -c 'trap "" TERM; sleep 30 & wait' &
  local pid="$!"

  local child=""
  local i
  for i in {1..20}; do
    child="$(pgrep -P "$pid" 2>/dev/null | head -n1 || true)"
    [[ -n "$child" ]] && break
    sleep 0.05
  done
  [[ -n "$child" ]] || {
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
    fail "test worker child never started"
  }

  run env FAIL_FAST_ABORT_GRACE_SECONDS=0 bash -c 'source "$1"; abort_worker_pids "unit-test-tree" "$2"' _ "$ORCHESTRATOR_LIB_DIR/common.sh" "$pid"
  assert_success

  for i in {1..20}; do
    if ! proc_alive "$pid" && ! proc_alive "$child"; then
      wait "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 0.05
  done

  kill -KILL "$pid" "$child" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  fail "worker process tree was still alive after abort_worker_pids"
}

@test "find_stale_orchestrator_pids tolerates no matching process under pipefail" {
  run bash -c 'set -Eeuo pipefail; source "$1"; find_stale_orchestrator_pids >/dev/null' _ "$ORCHESTRATOR_LIB_DIR/common.sh"
  assert_success
}

@test "should_pause_after_cycle pauses non-quota failures by default" {
  run should_pause_after_cycle 1
  assert_success
}

@test "should_pause_after_cycle does not pause success or quota backoff" {
  run should_pause_after_cycle 0
  assert_failure

  run should_pause_after_cycle 75
  assert_failure
}

@test "should_pause_after_cycle respects PAUSE_ON_RUN_FAILURE=0" {
  run env PAUSE_ON_RUN_FAILURE=0 bash -c 'source "$1"; should_pause_after_cycle 1' _ "$ORCHESTRATOR_LIB_DIR/common.sh"
  assert_failure
}

@test "should_run_post_cycle_companions allows only clean cycles by default" {
  run should_run_post_cycle_companions 0
  assert_success

  run should_run_post_cycle_companions 1
  assert_failure

  run should_run_post_cycle_companions 75
  assert_failure
}

@test "should_run_post_cycle_companions allows override for failed cycles" {
  run env SKIP_COMPANIONS_ON_RUN_FAILURE=0 bash -c 'source "$1"; should_run_post_cycle_companions 1' _ "$ORCHESTRATOR_LIB_DIR/common.sh"
  assert_success
}

@test "read_progress_summary reads canonical object-shaped progress.json" {
  local progress_file
  progress_file="$BATS_TEST_TMPDIR/progress.json"
  cat > "$progress_file" <<'JSON'
{
  "phases": {
    "1": {
      "subphases": {
        "1.A": {
          "items": [
            {"name": "done", "status": "complete"},
            {"name": "active", "status": "in_progress"},
            {"name": "next", "status": "planned"}
          ]
        }
      }
    },
    "2": {
      "subphases": {
        "2.A": {
          "items": [
            {"name": "later", "status": "planned"}
          ]
        }
      }
    }
  }
}
JSON

  run env PROGRESS_JSON="$progress_file" bash -c 'source "$1"; read_progress_summary' _ "$ORCHESTRATOR_LIB_DIR/common.sh"
  assert_success
  assert_output "1 1 2 4"
}

@test "read_progress_summary splits counts with orchestrator IFS" {
  local progress_file
  progress_file="$BATS_TEST_TMPDIR/progress.json"
  cat > "$progress_file" <<'JSON'
{
  "phases": {
    "1": {
      "subphases": {
        "1.A": {
          "items": [
            {"name": "done", "status": "complete"},
            {"name": "active", "status": "in_progress"},
            {"name": "next", "status": "planned"}
          ]
        }
      }
    }
  }
}
JSON

  run env PROGRESS_JSON="$progress_file" bash -c 'IFS=$'"'"'\n\t'"'"'; source "$1"; read_progress_summary' _ "$ORCHESTRATOR_LIB_DIR/common.sh"
  assert_success
  assert_output "1 1 1 3"
}

@test "read_progress_summary falls back to REPO_ROOT canonical path" {
  local repo_root progress_rel progress_file
  repo_root="$BATS_TEST_TMPDIR/repo"
  progress_rel="docs/content/building-gormes/architecture_plan/progress.json"
  progress_file="$repo_root/$progress_rel"
  mkdir -p "$(dirname "$progress_file")"
  cat > "$progress_file" <<'JSON'
{
  "phases": {
    "1": {
      "subphases": {
        "1.A": {
          "items": [
            {"name": "done", "status": "complete"},
            {"name": "next", "status": "planned"}
          ]
        }
      }
    }
  }
}
JSON

  run env -u PROGRESS_JSON REPO_ROOT="$repo_root" PROGRESS_JSON_REL="$progress_rel" \
    bash -c 'source "$1"; read_progress_summary' _ "$ORCHESTRATOR_LIB_DIR/common.sh"
  assert_success
  assert_output "1 0 1 2"
}

@test "provider_quota_exhausted detects codex usage limit final message" {
  local final_file
  final_file="$BATS_TEST_TMPDIR/quota.final.md"
  printf "You've hit your limit resets 8:20am (America/Monterrey)\n" > "$final_file"

  run provider_quota_exhausted "$final_file" "" ""
  assert_success
}

@test "provider_quota_message returns the matched quota line" {
  local final_file
  final_file="$BATS_TEST_TMPDIR/quota-message.final.md"
  printf "You've hit your limit resets 8:20am (America/Monterrey)\n" > "$final_file"

  run provider_quota_message "$final_file" "" ""
  assert_success
  assert_output --partial "You've hit your limit"
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
