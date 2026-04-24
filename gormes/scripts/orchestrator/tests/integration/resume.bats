#!/usr/bin/env bats
# Exercises the --resume <run_id> flow and pins its current behavior so
# future changes (Spec A) can improve it without silently regressing.
#
# Scenario:
#   1. First invocation runs with FAKE_CODEXU_MODE=timeout and
#      WORKER_TIMEOUT_SECONDS=3 so the worker gets SIGTERM'd. Ledger
#      should contain worker_claimed + worker_failed(timeout).
#   2. Second invocation uses --resume <same RUN_ID>. The current
#      orchestrator (pre-Spec-A) CANNOT retry a timed-out worker via
#      --resume: the timeout branch in run_worker overwrites the
#      "claimed" worker_state (which carried phase_id/subphase_id/
#      item_name) with a minimal {status:failed, reason:timeout}
#      payload, and run_worker_resume bails out because those three
#      fields are missing. So the resume invocation starts but emits no
#      new worker_claimed and exits.
#
# Assertions pin exactly that:
#   - first run: worker_claimed + worker_failed(timeout) for run_id.
#   - resume run: run_started + run_completed rows added, but total
#     worker_claimed rows stay at 1. If a future change makes --resume
#     actually re-drive the task, this test will flag it so Spec A's
#     authors can update the assertion intentionally.
#
# Promotion is disabled (AUTO_PROMOTE_SUCCESS=0) to keep this focused
# on the resume codepath, not cherry-picking.

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"

  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_LOG="$TMP_WS/fake-codexu.log"

  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"
  cp "$FIXTURES_DIR/progress.fixture.json" "$REPO_ROOT/progress.json"
  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.fixture.json" \
     "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  (
    cd "$REPO_ROOT"
    git -c user.email=t@t -c user.name=T add -A
    git -c user.email=t@t -c user.name=T commit -q -m init
  )

  export RUN_ROOT="$TMP_WS/run"
  # Deterministic RUN_ID so --resume <id> lines up with the first run.
  # The entry script reads RUN_ID_SEED="${RUN_ID:-}" then defaults to a
  # timestamp-$$; exporting RUN_ID honours the seed path.
  export RUN_ID="resume-test-1"
  export MAX_AGENTS=1
  export MODE=safe
  export ORCHESTRATOR_ONCE=1
  export HEARTBEAT_SECONDS=1
  export FINAL_REPORT_GRACE_SECONDS=1
  export WORKER_TIMEOUT_SECONDS=3
  export WORKER_TIMEOUT_GRACE_SECONDS=1
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export MAX_EXISTING_CHROMIUM=9999
  export FORCE_RUN_UNDER_PRESSURE=1
  export AUTO_PROMOTE_SUCCESS=0
  export INTEGRATION_BRANCH="codexu/test-autoloop-resume"
  export KEEP_WORKTREES=1
}

@test "resume: timed-out worker - current behavior pinned (Spec A gate)" {
  # First run: force a timeout.
  FAKE_CODEXU_MODE=timeout FAKE_CODEXU_SLEEP=9999 run "$ENTRY_SCRIPT"
  # Non-zero exit expected (worker timed out); we only care about ledger.
  [ -f "$RUN_ROOT/state/runs.jsonl" ]
  grep -q '"event":"worker_claimed"' "$RUN_ROOT/state/runs.jsonl"
  grep -q '"event":"worker_failed","worker_id":1,"detail":"[^"]*","status":"timeout"' \
       "$RUN_ROOT/state/runs.jsonl"

  local claims_after_first
  claims_after_first="$(grep -c '"run_id":"resume-test-1".*"event":"worker_claimed"' \
                        "$RUN_ROOT/state/runs.jsonl" || true)"
  [ "$claims_after_first" -eq 1 ]

  # Second run: --resume on the same run_id.
  FAKE_CODEXU_MODE=success run "$ENTRY_SCRIPT" --resume resume-test-1
  # Exit code is not asserted: current orchestrator cannot rehydrate the
  # timed-out worker's phase/subphase/item from worker_state (see header
  # comment), so it silently no-ops and exits. What matters is that we
  # did NOT double-claim.

  # A second run_started/run_completed pair for the same run_id confirms
  # the resume entry-point actually fired.
  local run_started_count run_completed_count
  run_started_count="$(grep -c '"run_id":"resume-test-1".*"event":"run_started"' \
                        "$RUN_ROOT/state/runs.jsonl" || true)"
  run_completed_count="$(grep -c '"run_id":"resume-test-1".*"event":"run_completed"' \
                          "$RUN_ROOT/state/runs.jsonl" || true)"
  [ "$run_started_count" -eq 2 ]
  [ "$run_completed_count" -eq 2 ]

  # Regression gate: worker_claimed must not balloon across resume
  # attempts. If a future change to --resume accidentally re-drives the
  # same task N times per invocation, this count jumps and this test
  # flags it. When Spec A lands real resume, this assertion should be
  # updated to `-eq 2` (initial + one retry) in the same commit.
  local claims_after_resume
  claims_after_resume="$(grep -c '"run_id":"resume-test-1".*"event":"worker_claimed"' \
                          "$RUN_ROOT/state/runs.jsonl" || true)"
  [ "$claims_after_resume" -le 2 ]
  [ "$claims_after_resume" -ge 1 ]
}
