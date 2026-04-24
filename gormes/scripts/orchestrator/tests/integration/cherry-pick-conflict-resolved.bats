#!/usr/bin/env bats
# Pins the -Xtheirs conflict-resolution behavior introduced to fix the 79%
# promotion-failure mode measured in the 24h audit. Two workers edit the
# same line of progress.fixture.json from the same BASE_COMMIT; both produce
# valid commits; sequential cherry-pick promotion with -Xtheirs lands BOTH.
# The second worker's version wins on the overlapping hunk, which is
# semantically correct since each worker owns its own progress entries.

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"

  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=conflict
  export FAKE_CODEXU_LOG="$TMP_WS/fake-codexu.log"

  export REPO_ROOT="$TMP_WS/repo"
  git init -q -b main "$REPO_ROOT"

  # progress.fixture.json must be tracked at BASE_COMMIT so each worker's
  # worktree starts from the same line-level content. The conflict mode in
  # fake-codexu overwrites this file with a PID-tagged payload, so the two
  # parallel workers end up with different commits that overlap on the same
  # hunk; -Xtheirs resolves the overlap in favour of the incoming worker.
  echo '{}' > "$REPO_ROOT/progress.fixture.json"

  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.fixture.json" \
     "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  (
    cd "$REPO_ROOT"
    git -c user.email=t@t -c user.name=T add -A
    git -c user.email=t@t -c user.name=T commit -q -m init
  )

  export RUN_ROOT="$TMP_WS/run"
  export MAX_AGENTS=2
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
  # Different branch from happy-path.bats so runs don't stomp each other.
  export INTEGRATION_BRANCH="codexu/test-autoloop-conflict"
  export KEEP_WORKTREES=0
}

@test "two conflicting workers both promote via -Xtheirs, integration clean" {
  run "$ENTRY_SCRIPT"
  # With -Xtheirs both cherry-picks land cleanly; we care about the ledger +
  # git state, not the exit code.

  [ -f "$RUN_ROOT/state/runs.jsonl" ]

  # Both workers should have produced a successful final report (worker_success).
  local success_count
  success_count="$(grep -c 'worker_success' "$RUN_ROOT/state/runs.jsonl" || true)"
  assert_equal "$success_count" "2"

  # Sequential promotion: both cherry-picks land.
  grep -q 'worker_promoted' "$RUN_ROOT/state/runs.jsonl"

  # Exactly two promoted rows (one per worker).
  local promoted_count
  promoted_count="$(grep -c 'worker_promoted.*promoted' "$RUN_ROOT/state/runs.jsonl" || true)"
  assert_equal "$promoted_count" "2"

  # No cherry_pick_failed events — -Xtheirs resolved the overlap.
  local failed_count
  failed_count="$(grep -c 'cherry_pick_failed' "$RUN_ROOT/state/runs.jsonl" || true)"
  assert_equal "$failed_count" "0"

  # No lingering CHERRY_PICK_HEAD in the integration worktree.
  # The integration worktree lives under $RUN_ROOT/integration/... and its
  # per-worktree git dir is at $REPO_ROOT/.git/worktrees/<safe_branch>/.
  run bash -c "find '$REPO_ROOT/.git/worktrees' -name CHERRY_PICK_HEAD 2>/dev/null"
  assert_success
  assert_output ""

  # Integration branch advanced by exactly two commits beyond init.
  run git -C "$REPO_ROOT" log --oneline "$INTEGRATION_BRANCH"
  assert_success
  [ "$(echo "$output" | wc -l)" -eq 3 ]
}
