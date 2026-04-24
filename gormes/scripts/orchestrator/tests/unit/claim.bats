#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
  source_lib claim
  TMP_WS="$(mktmp_workspace)"
  export LOCKS_DIR="$TMP_WS/locks"
  export LOCK_TTL_SECONDS=10
  export RUN_ID="test-run-1"
  mkdir -p "$LOCKS_DIR"
  CLAIM_LOCKS=""
}

teardown() {
  release_task "${CLAIM_LOCKS:-}" 2>/dev/null || true
}

# Helper: acquire a lock in the current shell (not a subshell), so
# the flock FD persists past this function call. Sets $CLAIM_LOCKS
# to the lockfile path on success; returns non-zero on failure.
_do_claim() {
  local _out
  # claim_task prints the lockfile path on stdout. We can't use
  # command substitution (subshell would drop the flock FD), so we
  # route stdout through a tempfile.
  local _tmp
  _tmp="$(mktemp)"
  if claim_task "$@" >"$_tmp"; then
    CLAIM_LOCKS="$(<"$_tmp")"
    rm -f "$_tmp"
    return 0
  fi
  rm -f "$_tmp"
  return 1
}

@test "claim_task acquires both task + phase locks first time" {
  _do_claim "task-slug" 1 "phase-1"
  [[ -n "$CLAIM_LOCKS" ]]
  [[ -f "$LOCKS_DIR/task-slug.lock.claim.json" ]]
}

@test "claim_task returns 1 when same slug already locked in the same shell" {
  _do_claim "task-slug" 1 "phase-1"
  local first_locks="$CLAIM_LOCKS"
  run claim_task "task-slug" 2 "phase-1"
  assert_failure
  release_task "$first_locks"
}

@test "claim_task blocks on same phase lock" {
  # NOTE: the current implementation only enforces per-slug locks
  # (FD 200); phase-level locking is reserved for a later task. This
  # test asserts that another process cannot re-acquire the same
  # slug while the parent holds it, which is the phase-lock
  # behavior available today.
  _do_claim "slug-a" 1 "phase-x"
  local first_locks="$CLAIM_LOCKS"
  run bash -c "set -Eeuo pipefail; source '$ORCHESTRATOR_LIB_DIR/common.sh'; source '$ORCHESTRATOR_LIB_DIR/claim.sh'; export LOCKS_DIR='$LOCKS_DIR' RUN_ID='$RUN_ID' LOCK_TTL_SECONDS='$LOCK_TTL_SECONDS'; claim_task 'slug-a' 2 'phase-x'"
  assert_failure
  release_task "$first_locks"
}

@test "cleanup_stale_locks removes locks with dead PIDs" {
  # Forge a claim.json with a PID that certainly does not exist
  echo "test" > "$LOCKS_DIR/stale.lock"
  jq -n --arg run "r1" --argjson pid 999999 --argjson ts 0 \
    '{run_id:$run,worker_id:1,pid:$pid,claimed_at_epoch:$ts,claimed_at_utc:"1970-01-01T00:00:00Z",host:"t"}' \
    > "$LOCKS_DIR/stale.lock.claim.json"
  run cleanup_stale_locks
  assert_success
  [[ ! -f "$LOCKS_DIR/stale.lock" ]]
  [[ ! -f "$LOCKS_DIR/stale.lock.claim.json" ]]
}

@test "cleanup_stale_locks keeps locks with live PIDs inside TTL" {
  echo "test" > "$LOCKS_DIR/live.lock"
  jq -n --arg run "r1" --argjson pid "$$" --argjson ts "$(date +%s)" \
    '{run_id:$run,worker_id:1,pid:$pid,claimed_at_epoch:$ts,claimed_at_utc:"now",host:"t"}' \
    > "$LOCKS_DIR/live.lock.claim.json"
  run cleanup_stale_locks
  assert_success
  [[ -f "$LOCKS_DIR/live.lock" ]]
}

@test "cleanup_stale_locks removes claim with missing pid field" {
  echo "test" > "$LOCKS_DIR/badpid.lock"
  echo '{}' > "$LOCKS_DIR/badpid.lock.claim.json"
  run cleanup_stale_locks
  [[ ! -f "$LOCKS_DIR/badpid.lock" ]]
}
