#!/usr/bin/env bats
# Task 7: when write_candidates_file yields fewer than CANDIDATE_LOW_WATERMARK
# unfinished tasks, run_once fires a synchronous planner companion via
# run_companion --sync, then re-runs write_candidates_file so the main loop
# continues with a refilled pool.

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"

  # Fakes (codexu + refill planner) first on PATH.
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=success
  export FAKE_CODEXU_LOG="$TMP_WS/fake-codexu.log"
  export FAKE_COMPANION_MARKER_DIR="$TMP_WS/markers"
  mkdir -p "$FAKE_COMPANION_MARKER_DIR"

  # Repo fixture: only 2 non-complete items (below default watermark=5).
  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"
  cp "$FIXTURES_DIR/progress.low-pool.json" "$REPO_ROOT/progress.json"
  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.low-pool.json" \
     "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  (
    cd "$REPO_ROOT"
    git -c user.email=t@t -c user.name=T add -A
    git -c user.email=t@t -c user.name=T commit -q -m init
  )

  export RUN_ROOT="$TMP_WS/run"
  export MAX_AGENTS=1
  export MODE=safe
  export ORCHESTRATOR_ONCE=1
  export HEARTBEAT_SECONDS=1
  export FINAL_REPORT_GRACE_SECONDS=1
  export WORKER_TIMEOUT_SECONDS=60
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export MAX_EXISTING_CHROMIUM=9999
  export FORCE_RUN_UNDER_PRESSURE=1
  export AUTO_PROMOTE_SUCCESS=1
  export INTEGRATION_BRANCH="codexu/test-refill"
  export KEEP_WORKTREES=0

  # Default watermark=5, below which refill fires. We want the refill pipeline
  # triggered, not the idle-gated periodic-companion pipeline.
  export CANDIDATE_LOW_WATERMARK=5
  export DISABLE_COMPANIONS=0
  # Suppress the periodic companions so only the sync refill fires the planner.
  export COMPANION_ON_IDLE=1
  export PLANNER_EVERY_N_CYCLES=9999
  export DOC_IMPROVER_EVERY_N_CYCLES=9999
  export LANDINGPAGE_EVERY_N_HOURS=9999
  export COMPANION_TIMEOUT_SECONDS=30
  export COMPANION_PLANNER_CMD="$FIXTURES_DIR/bin/fake-planner-refill"
  export PLANNER_ROOT="$TMP_WS/planner"
  mkdir -p "$PLANNER_ROOT"
}

@test "run_once fires synchronous planner refill when pool below watermark" {
  run "$ENTRY_SCRIPT"
  # We don't strictly require success — the fake-codexu worker might still run
  # and succeed, but the thing we are asserting is the refill event.

  [ -f "$RUN_ROOT/state/runs.jsonl" ]

  # The refill trigger event should record before=2 and the default watermark.
  run grep 'candidate_refill_triggered' "$RUN_ROOT/state/runs.jsonl"
  assert_success
  [[ "$output" == *"before=2"* ]] || { echo "$output" >&2; false; }
  [[ "$output" == *"watermark=5"* ]] || { echo "$output" >&2; false; }

  # After the sync planner ran, candidate_refilled should show after > before.
  run grep 'candidate_refilled' "$RUN_ROOT/state/runs.jsonl"
  assert_success
  local refilled_line="$output"
  local before after
  before="$(printf '%s\n' "$refilled_line" | jq -r '.detail' | sed -E 's/.*before=([0-9]+).*/\1/')"
  after="$(printf '%s\n' "$refilled_line" | jq -r '.detail' | sed -E 's/.*after=([0-9]+).*/\1/')"
  [[ "$before" == "2" ]] || { echo "before=$before" >&2; false; }
  (( after > before )) || { echo "after=$after not > before=$before" >&2; false; }

  # Confirm the fake-planner-refill actually ran.
  [ -f "$FAKE_COMPANION_MARKER_DIR/planner-refill.marker" ]

  # And the regenerated candidates.json should reflect the refilled pool
  # (strictly more than the original 2 items).
  local candidates_file
  candidates_file="$(ls "$RUN_ROOT"/state/candidates.*.json 2>/dev/null | head -n1)"
  [ -n "$candidates_file" ]
  run jq 'length' "$candidates_file"
  assert_success
  (( output > 2 )) || { echo "candidates length=$output (expected > 2)" >&2; false; }
}
