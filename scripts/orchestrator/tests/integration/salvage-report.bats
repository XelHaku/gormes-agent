#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export REPO_ROOT="$TMP_WS/repo"
  export RUN_ROOT="$TMP_WS/run"
  export RUN_ID="salvage-cli-seed"
  export PATH="$FIXTURES_DIR/bin:$PATH"
  export FAKE_CODEXU_LOG="$TMP_WS/fake-codexu.log"
  mkdir -p "$REPO_ROOT"
}

make_worker_repo() {
  local run_id="$1"
  local worker_id="$2"
  local dir="$RUN_ROOT/worktrees/$run_id/worker$worker_id"
  mkdir -p "$dir"
  git init -q -b main "$dir"
  (
    cd "$dir"
    echo base > base.txt
    git -c user.email=t@t -c user.name=T add base.txt
    git -c user.email=t@t -c user.name=T commit -q -m base
    git checkout -q -b "codexu/$run_id/worker$worker_id"
  )
  printf '%s\n' "$dir"
}

@test "salvage command reports worker worktrees without starting backend" {
  local run_id="salvage-cli-run"
  local wt
  wt="$(make_worker_repo "$run_id" 2)"
  mkdir -p "$RUN_ROOT/state/workers/$run_id"
  printf '{"status":"failed","reason":"report_invalid","slug":"some-task"}\n' \
    > "$RUN_ROOT/state/workers/$run_id/worker_2.json"
  echo local-change > "$wt/local.txt"

  run "$ENTRY_SCRIPT" salvage "$run_id"

  assert_success
  assert_output --partial "Run: $run_id"
  assert_output --partial "worker2 status=failed reason=report_invalid"
  assert_output --partial "dirty=1"
  assert_output --partial "inspect=$wt"
  assert_output --partial "?? local.txt"
}

@test "dirty retained worker worktree guard stops run before backend launch" {
  local dirty_run="prior-dirty-run"
  local wt
  wt="$(make_worker_repo "$dirty_run" 1)"
  echo local-change > "$wt/local.txt"

  git init -q -b main "$REPO_ROOT"
  mkdir -p "$REPO_ROOT/docs/content/building-gormes/architecture_plan"
  cp "$FIXTURES_DIR/progress.fixture.json" \
    "$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
  (
    cd "$REPO_ROOT"
    git -c user.email=t@t -c user.name=T add -A
    git -c user.email=t@t -c user.name=T commit -q -m init
  )

  export ORCHESTRATOR_ONCE=1
  export MAX_AGENTS=1
  export MIN_AVAILABLE_MEM_MB=1
  export MIN_MEM_PER_WORKER_MB=1
  export FORCE_RUN_UNDER_PRESSURE=1
  export AUTO_PROMOTE_SUCCESS=0

  run "$ENTRY_SCRIPT"

  assert_failure
  assert_output --partial "Refusing to launch workers: dirty retained worker worktrees found"
  assert_output --partial "scripts/gormes-auto-codexu-orchestrator.sh salvage $dirty_run"
  [ ! -f "$FAKE_CODEXU_LOG" ]
}
