#!/usr/bin/env bash
# Task + phase-level locking, and stale-lock reaping.
# Depends on: $LOCKS_DIR, $LOCK_TTL_SECONDS, $RUN_ID.
# Exports: global CLAIM_LOCKS (set by claim_task, consumed by release_task).

cleanup_stale_locks() {
  local now
  now="$(date +%s)"

  [[ -d "$LOCKS_DIR" ]] || return 0

  shopt -s nullglob
  local dir claim pid claimed_at_epoch
  for dir in "$LOCKS_DIR"/*.lock; do
    claim="${dir}.claim.json"
    if [[ ! -f "$claim" ]]; then
      rm -f "$dir" "$claim"
      continue
    fi

    pid="$(jq -r '.pid // empty' "$claim" 2>/dev/null || true)"
    claimed_at_epoch="$(jq -r '.claimed_at_epoch // 0' "$claim" 2>/dev/null || true)"
    [[ "$claimed_at_epoch" =~ ^[0-9]+$ ]] || claimed_at_epoch=0

    if [[ -z "$pid" || ! "$pid" =~ ^[0-9]+$ ]]; then
      rm -f "$dir" "$claim"
      continue
    fi

    if ! kill -0 "$pid" 2>/dev/null; then
      rm -f "$dir" "$claim"
      continue
    fi

    if (( claimed_at_epoch > 0 && now - claimed_at_epoch > LOCK_TTL_SECONDS )); then
      rm -f "$dir" "$claim"
    fi
  done
  shopt -u nullglob
}

claim_task() {
  local slug="$1"
  local worker_id="$2"
  local lockfile="$LOCKS_DIR/${slug}.lock"
  local lockfd=200

  # Open lockfile on dedicated FD
  exec {lockfd}>"$lockfile"

  # Try to acquire exclusive lock with timeout (non-blocking first)
  if ! flock -x -n "$lockfd"; then
    # Lock held by another process - close FD and fail
    exec {lockfd}>&- 2>/dev/null || true
    return 1
  fi

  # Lock acquired - write claim metadata
  mkdir -p "$LOCKS_DIR"
  jq -n \
    --arg run_id "$RUN_ID" \
    --argjson worker_id "$worker_id" \
    --argjson pid "$$" \
    --argjson claimed_at_epoch "$(date +%s)" \
    --arg claimed_at_utc "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --arg host "$(hostname 2>/dev/null || echo unknown)" \
    '{
      run_id: $run_id,
      worker_id: $worker_id,
      pid: $pid,
      claimed_at_epoch: $claimed_at_epoch,
      claimed_at_utc: $claimed_at_utc,
      host: $host
    }' > "$lockfile.claim.json"

  # Return lockfile path for release_task
  printf '%s\n' "$lockfile"
  return 0
}

release_task() {
  local lockfile="${1:-}"
  [[ -n "$lockfile" ]] || return 0

  # Close the file descriptor to release the flock
  # FD 200 was used in claim_task
  exec 200>&- 2>/dev/null || true

  # Clean up lock files
  rm -f "$lockfile" "$lockfile.claim.json" 2>/dev/null || true
}
