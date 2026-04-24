#!/usr/bin/env bats
# Verifies maybe_run_companions fires after run_once in ORCHESTRATOR_ONCE=1
# mode, that COMPANION_ON_IDLE=0 forces invocation on every cycle, and that
# DISABLE_COMPANIONS=1 suppresses all three companion scripts.

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"

  # Fake codexu + fake companion scripts first on PATH.
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=success
  export FAKE_CODEXU_LOG="$TMP_WS/fake-codexu.log"
  export FAKE_COMPANION_MARKER_DIR="$TMP_WS/markers"
  mkdir -p "$FAKE_COMPANION_MARKER_DIR"

  # Minimal repo fixture — same shape as happy-path.bats.
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
  export INTEGRATION_BRANCH="codexu/test-companion"
  export KEEP_WORKTREES=0

  # Companion cadence: fire on first cycle, bypass idle gate.
  export PLANNER_EVERY_N_CYCLES=1
  export DOC_IMPROVER_EVERY_N_CYCLES=1
  export LANDINGPAGE_EVERY_N_HOURS=0
  export COMPANION_ON_IDLE=0
  export COMPANION_TIMEOUT_SECONDS=30
  export COMPANION_PLANNER_CMD="$FIXTURES_DIR/bin/fake-planner"
  export COMPANION_DOC_IMPROVER_CMD="$FIXTURES_DIR/bin/fake-doc-improver"
  export COMPANION_LANDINGPAGE_CMD="$FIXTURES_DIR/bin/fake-landingpage"
  export PLANNER_ROOT="$TMP_WS/planner"
  mkdir -p "$PLANNER_ROOT"
}

# Wait up to $1 seconds for marker file $2 to exist. Returns 0 if it appears,
# 1 if it times out. Companions now run async (setsid nohup), so the
# orchestrator exits before the marker exists on disk.
wait_for_marker() {
  local timeout="$1"
  local marker="$2"
  local waited=0
  while (( waited < timeout )); do
    [ -f "$marker" ] && return 0
    sleep 1
    waited=$(( waited + 1 ))
  done
  return 1
}

@test "orchestrator triggers companions after successful cycle" {
  run "$ENTRY_SCRIPT"
  # Planner fires when interval reached (every 1 cycle, no external state).
  wait_for_marker 10 "$FAKE_COMPANION_MARKER_DIR/planner.marker"
  [ -f "$FAKE_COMPANION_MARKER_DIR/planner.marker" ]
  # Landing-page fires because N_HOURS=0 → always.
  wait_for_marker 10 "$FAKE_COMPANION_MARKER_DIR/landingpage.marker"
  [ -f "$FAKE_COMPANION_MARKER_DIR/landingpage.marker" ]
  # Doc-improver only fires when a promotion happened this cycle.
  if [ -f "$RUN_ROOT/state/runs.jsonl" ] && grep -q 'worker_promoted' "$RUN_ROOT/state/runs.jsonl"; then
    wait_for_marker 10 "$FAKE_COMPANION_MARKER_DIR/doc_improver.marker"
    [ -f "$FAKE_COMPANION_MARKER_DIR/doc_improver.marker" ]
  fi
}

@test "DISABLE_COMPANIONS=1 blocks all companion runs" {
  export DISABLE_COMPANIONS=1
  run "$ENTRY_SCRIPT"
  # Give any (incorrectly-fired) async companion a chance to write its marker.
  sleep 2
  [ ! -f "$FAKE_COMPANION_MARKER_DIR/planner.marker" ]
  [ ! -f "$FAKE_COMPANION_MARKER_DIR/doc_improver.marker" ]
  [ ! -f "$FAKE_COMPANION_MARKER_DIR/landingpage.marker" ]
}
