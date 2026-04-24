#!/usr/bin/env bash
# Candidate-pool refill backoff helpers.
# Sourced by gormes-auto-codexu-orchestrator.sh and its tests.
# Depends on: $STATE_DIR (for default state path), jq on PATH.

refill_state_file() {
  printf '%s\n' "${REFILL_STATE_FILE:-$STATE_DIR/refill-state.json}"
}

read_refill_state() {
  # Emits "streak<TAB>last_refill_ts" to stdout. Missing/invalid => "0\t0".
  local f; f="$(refill_state_file)"
  if [[ -f "$f" ]]; then
    jq -r '"\(.streak // 0)\t\(.last_refill_ts // 0)"' "$f" 2>/dev/null || printf '0\t0\n'
  else
    printf '0\t0\n'
  fi
}

write_refill_state() {
  local streak="$1" last_ts="$2"
  local f; f="$(refill_state_file)"
  mkdir -p "$(dirname "$f")"
  jq -nc --argjson streak "$streak" --argjson last_refill_ts "$last_ts" \
    '{streak:$streak, last_refill_ts:$last_refill_ts}' > "$f"
}

# should_skip_refill <streak> <last_refill_ts> <now_epoch> <loop_sleep_seconds>
# stdout: "skip <remaining_seconds>" or "run 0"
# return 0 when refill should be skipped, 1 when it should run.
# Cooldown formula: min(streak * loop_sleep, 3600).
should_skip_refill() {
  local streak="$1" last_ts="$2" now="$3" loop_sleep="$4"
  [[ "$streak" =~ ^[0-9]+$ ]] || streak=0
  [[ "$last_ts" =~ ^[0-9]+$ ]] || last_ts=0
  [[ "$now" =~ ^[0-9]+$ ]] || now=0
  [[ "$loop_sleep" =~ ^[0-9]+$ ]] || loop_sleep=30
  if (( streak <= 0 )); then
    printf 'run 0\n'
    return 1
  fi
  local cool=$(( streak * loop_sleep ))
  if (( cool > 3600 )); then cool=3600; fi
  local elapsed=$(( now - last_ts ))
  if (( elapsed < cool )); then
    printf 'skip %d\n' $(( cool - elapsed ))
    return 0
  fi
  printf 'run 0\n'
  return 1
}
