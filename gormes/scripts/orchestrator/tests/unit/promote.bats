#!/usr/bin/env bats

load '../lib/test_env'

make_fixture_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" -c user.email=t@t -c user.name=T commit -q --allow-empty -m init
}

write_worker_state() {
  local id="$1" slug="$2" commit="$3" status="$4"
  local dir="$RUN_WORKER_STATE_DIR"
  mkdir -p "$dir"
  jq -n --arg run "$RUN_ID" --arg s "$status" --arg slug "$slug" --arg c "$commit" \
    '{run_id:$run,status:$s,slug:$slug,commit:$c}' > "$dir/worker_${id}.json"
}

setup() {
  load_helpers
  source_lib common
  source_lib promote
  TMP_WS="$(mktmp_workspace)"
  export GIT_ROOT="$TMP_WS/int"
  export INTEGRATION_BRANCH="codexu/autoloop"
  export AUTO_PROMOTE_SUCCESS=1
  export RUN_ID="prom-1"
  export RUN_WORKER_STATE_DIR="$TMP_WS/workers/$RUN_ID"
  export STATE_DIR="$TMP_WS/state"
  export RUNS_LEDGER="$STATE_DIR/runs.jsonl"
  export AUTO_PUSH=0
  # Default these unit tests to the cherry-pick path so the historical
  # fixtures still exercise the same code paths. PR-mode tests opt in.
  export PROMOTION_MODE="cherry-pick"
  mkdir -p "$STATE_DIR"
  make_fixture_repo "$GIT_ROOT"
  git -C "$GIT_ROOT" checkout -q -b "$INTEGRATION_BRANCH"
  # Re-source load_worker_state + log_event — they live in entry script until those extractions;
  # for promote.bats we define lightweight stubs if absent.
  type load_worker_state >/dev/null 2>&1 || load_worker_state() { cat "$RUN_WORKER_STATE_DIR/worker_$1.json" 2>/dev/null; }
  # Emit events into the per-test RUNS_LEDGER so PR-mode tests can grep them.
  log_event() {
    local event="$1" worker_id="${2:-null}" detail="${3:-}" status="${4:-}"
    mkdir -p "$STATE_DIR"
    jq -nc \
      --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg run_id "${RUN_ID:-}" \
      --arg event "$event" \
      --arg worker_id "$worker_id" \
      --arg detail "$detail" \
      --arg status "$status" \
      '{ts:$ts, run_id:$run_id, event:$event,
        worker_id:(if $worker_id == "null" then null else ($worker_id|tonumber) end),
        detail:$detail, status:$status}' >> "$RUNS_LEDGER"
  }
  export -f log_event
}

@test "promote_successful_workers skips when feature disabled" {
  export AUTO_PROMOTE_SUCCESS=0
  run promote_successful_workers 2
  assert_success
}

@test "promote_successful_workers cherry-picks one success" {
  # Build a branch that modifies a file, record its commit, then reset integration
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"
  run promote_successful_workers 1
  assert_success
  local head
  head="$(git -C "$GIT_ROOT" log --format=%s -n1 "$INTEGRATION_BRANCH")"
  assert_equal "$head" "add-a"
}

@test "promote_successful_workers auto-resolves cherry-pick conflict with -Xtheirs" {
  # Worker 1 commits a→"one"; integration then commits a→"two"; with -Xtheirs the
  # worker's version wins on conflicting hunks and the cherry-pick succeeds.
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo one > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m feat-a )
  local worker_commit
  worker_commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  ( cd "$GIT_ROOT" && echo two > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m int-a )
  write_worker_state 1 "foo__bar" "$worker_commit" "success"
  run promote_successful_workers 1
  assert_success
  [[ ! -f "$GIT_ROOT/.git/CHERRY_PICK_HEAD" ]]
  # Worker's version ("one") wins on the conflicting hunk.
  assert_equal "$(cat "$GIT_ROOT/a")" "one"
  # Integration branch advanced: init, int-a, feat-a (cherry-picked).
  local count
  count="$(git -C "$GIT_ROOT" rev-list --count "$INTEGRATION_BRANCH")"
  assert_equal "$count" "3"
}

@test "cherry-pick auto-resolves progress.json overlap with Xtheirs" {
  # Both worker branches edit the same line of progress.fixture.json to
  # different values — the canonical failure mode. With -Xtheirs both
  # cherry-picks succeed and integration grows by two commits.
  ( cd "$GIT_ROOT" && echo '{"item":"pending"}' > progress.fixture.json && git -c user.email=t@t -c user.name=T add progress.fixture.json && git -c user.email=t@t -c user.name=T commit -q -m base )

  # Worker 1: progress.fixture.json → {"item":"worker1_done"}
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b w1 )
  ( cd "$GIT_ROOT" && echo '{"item":"worker1_done"}' > progress.fixture.json && git -c user.email=t@t -c user.name=T add progress.fixture.json && git -c user.email=t@t -c user.name=T commit -q -m w1-flip )
  local w1_commit
  w1_commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"

  # Worker 2: branched from the same base, writes {"item":"worker2_done"}
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b w2 )
  ( cd "$GIT_ROOT" && echo '{"item":"worker2_done"}' > progress.fixture.json && git -c user.email=t@t -c user.name=T add progress.fixture.json && git -c user.email=t@t -c user.name=T commit -q -m w2-flip )
  local w2_commit
  w2_commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"

  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  local base_count
  base_count="$(git -C "$GIT_ROOT" rev-list --count "$INTEGRATION_BRANCH")"

  write_worker_state 1 "w__one" "$w1_commit" "success"
  write_worker_state 2 "w__two" "$w2_commit" "success"
  # Call directly (not via `run`) so the exported PROMOTED_LAST_CYCLE
  # survives into the parent shell for inspection.
  promote_successful_workers 2
  [[ ! -f "$GIT_ROOT/.git/CHERRY_PICK_HEAD" ]]
  assert_equal "$PROMOTED_LAST_CYCLE" "2"

  # Integration branch grew by exactly 2 commits.
  local after_count
  after_count="$(git -C "$GIT_ROOT" rev-list --count "$INTEGRATION_BRANCH")"
  assert_equal "$after_count" "$((base_count + 2))"
}

@test "promote_successful_workers exports PROMOTED_LAST_CYCLE" {
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"
  promote_successful_workers 1
  assert_equal "$PROMOTED_LAST_CYCLE" "1"
}

# --- PR-mode tests --------------------------------------------------------

# Build a tiny fake `git` shim that records pushes as no-ops so PR-mode tests
# can exercise the push path without a real remote. Because the fixture's
# `git` calls run through PATH, we shim with the same name earlier on PATH.
install_fake_git_push_passthrough() {
  local shim_dir="$TMP_WS/bin"
  mkdir -p "$shim_dir"
  local real_git
  real_git="$(command -v git)"
  cat > "$shim_dir/git" <<EOF
#!/usr/bin/env bash
# Fake git: treat any 'push' subcommand as a no-op, delegate everything else
# to the real git.
argv=("\$@")
for ((i=0; i<\${#argv[@]}; i++)); do
  arg="\${argv[i]}"
  if [[ "\$arg" == "push" ]]; then
    exit 0
  fi
  if [[ "\$arg" != "-C" && "\$arg" != "-c" && "\$arg" != -* ]]; then
    break
  fi
  # Some flags take a value — skip it.
  if [[ "\$arg" == "-C" || "\$arg" == "-c" ]]; then
    ((i++))
  fi
done
exec "$real_git" "\$@"
EOF
  chmod +x "$shim_dir/git"
  export PATH="$shim_dir:$PATH"
}

install_fake_gh_on_path() {
  local shim_dir="$TMP_WS/bin"
  mkdir -p "$shim_dir"
  cp "$FIXTURES_DIR/bin/fake-gh" "$shim_dir/gh"
  chmod +x "$shim_dir/gh"
  export PATH="$shim_dir:$PATH"
}

@test "PROMOTION_MODE=pr emits worker_pr_opened on happy path" {
  # Build a worker commit the same way the cherry-pick test does.
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"

  export PROMOTION_MODE="pr"
  export PR_REPO_SLUG="x/y"
  install_fake_git_push_passthrough
  install_fake_gh_on_path
  export FAKE_GH_AUTH=ok
  export FAKE_GH_PR_URL="https://github.com/x/y/pull/42"
  # Make worker_branch_name exist so PR titles/branches compose cleanly.
  worker_branch_name() { printf 'codexu/%s/worker%s' "${RUN_ID:-fake}" "$1"; }
  worker_worktree_root() { printf '%s' "$GIT_ROOT"; }
  export -f worker_branch_name worker_worktree_root

  promote_successful_workers 1
  assert_equal "$PROMOTED_LAST_CYCLE" "1"

  run jq -r 'select(.event=="worker_pr_opened") | .detail' "$RUNS_LEDGER"
  assert_success
  assert_output --partial "foo__bar@https://github.com/x/y/pull/42"

  # No cherry-pick should have happened — integration HEAD is unchanged.
  local head_subj
  head_subj="$(git -C "$GIT_ROOT" log --format=%s -n1 "$INTEGRATION_BRANCH")"
  assert_equal "$head_subj" "init"
}

@test "PROMOTION_MODE=pr falls back to cherry-pick when gh auth fails" {
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"

  export PROMOTION_MODE="pr"
  install_fake_gh_on_path
  export FAKE_GH_AUTH=fail

  promote_successful_workers 1
  assert_equal "$PROMOTED_LAST_CYCLE" "1"

  # Pre-flight catches auth failure, so the per-worker fallback event is NOT
  # emitted — we log pr_mode_unavailable once for the cycle instead.
  run jq -r 'select(.event=="pr_mode_unavailable") | .detail' "$RUNS_LEDGER"
  assert_success
  [[ -n "$output" ]]

  # Integration HEAD advanced — cherry-pick ran.
  local head_subj
  head_subj="$(git -C "$GIT_ROOT" log --format=%s -n1 "$INTEGRATION_BRANCH")"
  assert_equal "$head_subj" "add-a"
}

@test "PROMOTION_MODE=pr emits worker_pr_fallback when pr create fails mid-cycle" {
  ( cd "$GIT_ROOT" && git -c user.email=t@t -c user.name=T checkout -q -b feat )
  ( cd "$GIT_ROOT" && echo a > a && git -c user.email=t@t -c user.name=T add a && git -c user.email=t@t -c user.name=T commit -q -m add-a )
  local commit
  commit="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  ( cd "$GIT_ROOT" && git checkout -q "$INTEGRATION_BRANCH" )
  write_worker_state 1 "foo__bar" "$commit" "success"

  export PROMOTION_MODE="pr"
  install_fake_git_push_passthrough
  install_fake_gh_on_path
  export FAKE_GH_AUTH=ok
  export FAKE_GH_PR_CREATE=fail
  worker_branch_name() { printf 'codexu/%s/worker%s' "${RUN_ID:-fake}" "$1"; }
  worker_worktree_root() { printf '%s' "$GIT_ROOT"; }
  export -f worker_branch_name worker_worktree_root

  promote_successful_workers 1
  assert_equal "$PROMOTED_LAST_CYCLE" "1"

  run jq -r 'select(.event=="worker_pr_fallback") | .detail' "$RUNS_LEDGER"
  assert_success
  assert_output --partial "foo__bar:pr_create_failed"

  # Fallback means cherry-pick landed — HEAD advanced.
  local head_subj
  head_subj="$(git -C "$GIT_ROOT" log --format=%s -n1 "$INTEGRATION_BRANCH")"
  assert_equal "$head_subj" "add-a"
}

@test "orchestrator_verify_gh_auth prints PASS when gh is healthy" {
  install_fake_gh_on_path
  export FAKE_GH_AUTH=ok
  run orchestrator_verify_gh_auth "x/y"
  assert_success
  assert_output --partial "PASS"
}

@test "orchestrator_verify_gh_auth prints FAIL when auth is broken" {
  install_fake_gh_on_path
  export FAKE_GH_AUTH=fail
  run orchestrator_verify_gh_auth "x/y"
  assert_failure
  assert_output --partial "FAIL"
}
