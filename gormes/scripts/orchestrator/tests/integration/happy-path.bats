#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"

  # Put our fake-codexu (symlinked as "codexu") first on PATH.
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_MODE=success
  export FAKE_CODEXU_LOG="$TMP_WS/fake-codexu.log"

  # Minimal Go-repo-shape fixture: a git repo with progress.json at the
  # canonical orchestrator path plus a root copy.
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
  export INTEGRATION_BRANCH="codexu/test-autoloop"
  export KEEP_WORKTREES=0
}

@test "one worker succeeds, promotes to integration branch" {
  run "$ENTRY_SCRIPT"
  assert_success

  # Integration branch got at least one new commit beyond init.
  run git -C "$REPO_ROOT" log --oneline "$INTEGRATION_BRANCH"
  assert_success
  [ "$(echo "$output" | wc -l)" -ge 2 ]

  # Ledger recorded worker_success AND worker_promoted.
  [ -f "$RUN_ROOT/state/runs.jsonl" ]
  grep -q 'worker_success' "$RUN_ROOT/state/runs.jsonl"
  grep -q 'worker_promoted' "$RUN_ROOT/state/runs.jsonl"
}
