#!/usr/bin/env bats

load '../lib/test_env'

make_fixture_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" -c user.email=t@t -c user.name=T commit -q --allow-empty -m init
}

setup() {
  load_helpers
  source_lib common
  source_lib report
  source_lib worktree
  TMP_WS="$(mktmp_workspace)"
  export GIT_ROOT="$TMP_WS/repo"
  export WORKTREES_DIR="$TMP_WS/wt"
  export REPO_SUBDIR="."
  export RUN_ID="wrt-run-1"
  export PROGRESS_JSON_REL="progress.json"
  make_fixture_repo "$GIT_ROOT"
  export BASE_COMMIT="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  mkdir -p "$WORKTREES_DIR"
}

@test "worker_branch_name format" {
  run worker_branch_name 3
  assert_output "codexu/wrt-run-1/worker3"
}

@test "worker_worktree_root format" {
  run worker_worktree_root 2
  assert_output "$WORKTREES_DIR/worker2"
}

@test "create_worker_worktree checks out base commit on new branch" {
  run create_worker_worktree 1
  assert_success
  [[ -d "$WORKTREES_DIR/worker1" ]]
  local head
  head="$(git -C "$WORKTREES_DIR/worker1" rev-parse HEAD)"
  assert_equal "$head" "$BASE_COMMIT"
  local branch
  branch="$(git -C "$WORKTREES_DIR/worker1" rev-parse --abbrev-ref HEAD)"
  assert_equal "$branch" "codexu/wrt-run-1/worker1"
}

@test "verify_worker_commit rejects unchanged HEAD" {
  create_worker_worktree 1
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$BASE_COMMIT" > "$report"
  run verify_worker_commit 1 "$report"
  assert_failure
  assert_output --partial "HEAD did not advance"
}

@test "verify_worker_commit exports LAST_VERIFY_REASON=no_commit_made on unchanged HEAD" {
  create_worker_worktree 1
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$BASE_COMMIT" > "$report"
  # Call directly (not via `run`) so LAST_VERIFY_REASON survives into
  # the parent shell for inspection.
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "no_commit_made"
}

@test "verify_worker_commit rejects multiple commits" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo b > b && git -c user.email=t@t -c user.name=T add b && git -c user.email=t@t -c user.name=T commit -q -m b )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  run verify_worker_commit 1 "$report"
  assert_failure
  assert_output --partial "commit count"
}

@test "verify_worker_commit exports LAST_VERIFY_REASON=wrong_commit_count on multiple commits" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo b > b && git -c user.email=t@t -c user.name=T add b && git -c user.email=t@t -c user.name=T commit -q -m b )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "wrong_commit_count"
}

@test "verify_worker_commit rejects dirty worktree" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo stray > stray )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  run verify_worker_commit 1 "$report"
  assert_failure
  assert_output --partial "not clean"
}

@test "verify_worker_commit exports LAST_VERIFY_REASON=worktree_dirty on dirty tree" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo stray > stray )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "worktree_dirty"
}

@test "verify_worker_commit exports LAST_VERIFY_REASON=report_commit_mismatch on bad commit hash" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  local report="$TMP_WS/f.md"
  # Report a wrong commit hash that doesn't prefix-match HEAD.
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "report_commit_mismatch"
}

@test "verify_worker_commit exports LAST_VERIFY_REASON=branch_mismatch on wrong branch in report" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/wrong-branch\nCommit: %s\n' "$head" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "branch_mismatch"
}

@test "verify_worker_commit exports LAST_VERIFY_REASON=scope_violation when change escapes subdir" {
  # Rebuild a fixture with a non-trivial REPO_SUBDIR so scope is enforceable.
  export REPO_SUBDIR="sub"
  mkdir -p "$GIT_ROOT/sub"
  ( cd "$GIT_ROOT" && echo keep > sub/keep && git -c user.email=t@t -c user.name=T add sub/keep && git -c user.email=t@t -c user.name=T commit -q -m seed )
  export BASE_COMMIT="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  # Change a file OUTSIDE the subdir scope.
  ( cd "$wt" && echo outside > outside.txt && git -c user.email=t@t -c user.name=T add outside.txt && git -c user.email=t@t -c user.name=T commit -q -m outside )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "scope_violation"
}

@test "verify_worker_commit accepts single valid commit" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  run verify_worker_commit 1 "$report"
  assert_success
}

@test "verify_worker_commit accepts multi-commit work when ALLOW_MULTI_COMMIT=1" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo b > b && git -c user.email=t@t -c user.name=T add b && git -c user.email=t@t -c user.name=T commit -q -m b )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  ALLOW_MULTI_COMMIT=1 run verify_worker_commit 1 "$report"
  assert_success
}

@test "verify_worker_commit still rejects multi-commit without ALLOW_MULTI_COMMIT" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo b > b && git -c user.email=t@t -c user.name=T add b && git -c user.email=t@t -c user.name=T commit -q -m b )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "wrong_commit_count"
}

@test "verify_worker_commit accepts untracked-only dirty tree when TOLERATE_WORKTREE_UNTRACKED=1" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo stray > stray )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  TOLERATE_WORKTREE_UNTRACKED=1 run verify_worker_commit 1 "$report"
  assert_success
}

@test "verify_worker_commit still rejects untracked-only dirty tree without TOLERATE_WORKTREE_UNTRACKED" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo stray > stray )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  ! verify_worker_commit 1 "$report" 2>/dev/null
  assert_equal "$LAST_VERIFY_REASON" "worktree_dirty"
}

@test "verify_worker_commit report section 10 AllowMultiCommit overrides env default" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo b > b && git -c user.email=t@t -c user.name=T add b && git -c user.email=t@t -c user.name=T commit -q -m b )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  {
    printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head"
    printf '\n### 10) Runtime flags\nAllowMultiCommit: true\n'
  } > "$report"
  # Explicitly ensure the env default is rigid (0) to prove the report flag wins.
  ALLOW_MULTI_COMMIT=0 run verify_worker_commit 1 "$report"
  assert_success
}

@test "verify_worker_commit report section 10 TolerateWorktreeUntracked overrides env default" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  ( cd "$wt" && echo stray > stray )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  {
    printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head"
    printf '\n10) Runtime flags\nTolerateWorktreeUntracked: true\n'
  } > "$report"
  TOLERATE_WORKTREE_UNTRACKED=0 run verify_worker_commit 1 "$report"
  assert_success
}

@test "verify_worker_commit clears LAST_VERIFY_REASON on success" {
  create_worker_worktree 1
  local wt="$WORKTREES_DIR/worker1"
  ( cd "$wt" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m a )
  local head
  head="$(git -C "$wt" rev-parse HEAD)"
  local report="$TMP_WS/f.md"
  printf 'Branch: codexu/wrt-run-1/worker1\nCommit: %s\n' "$head" > "$report"
  # Seed with a stale reason that would leak if the function didn't reset it.
  export LAST_VERIFY_REASON="stale_value"
  verify_worker_commit 1 "$report"
  assert_equal "$LAST_VERIFY_REASON" ""
}
