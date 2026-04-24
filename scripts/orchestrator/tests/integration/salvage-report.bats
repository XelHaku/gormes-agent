#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  export REPO_ROOT="$TMP_WS/repo"
  export RUN_ROOT="$TMP_WS/run"
  export RUN_ID="salvage-cli-seed"
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
