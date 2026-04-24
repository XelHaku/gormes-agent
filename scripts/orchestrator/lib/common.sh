#!/usr/bin/env bash
# Common logging, path, and small utility helpers for the orchestrator.
# Sourced by gormes-auto-codexu-orchestrator.sh and its tests.
# Depends on: $VERBOSE (reads; default 0 if unset).

# Verbose logging functions
log_info() {
  if [[ "$VERBOSE" == "1" ]]; then
    echo "[INFO] $(date '+%H:%M:%S') $*" >&2
  fi
}

log_debug() {
  if [[ "$VERBOSE" == "1" ]]; then
    echo "[DEBUG] $(date '+%H:%M:%S') $*" >&2
  fi
}

log_warn() {
  echo "[WARN] $(date '+%H:%M:%S') $*" >&2
}

log_error() {
  echo "[ERROR] $(date '+%H:%M:%S') $*" >&2
}

# Progress indicator
show_progress() {
  local current=$1
  local total=$2
  local label="${3:-Progress}"
  local width=50
  local percentage=$((current * 100 / total))
  local filled=$((width * current / total))
  local empty=$((width - filled))

  printf "\r%s [%s%s] %3d%% (%d/%d)" \
    "$label" \
    "$(printf '%*s' "$filled" '' | tr ' ' '=')" \
    "$(printf '%*s' "$empty" '' | tr ' ' ' ')" \
    "$percentage" \
    "$current" \
    "$total"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: missing required command: $1" >&2
    exit 1
  }
}

safe_path_token() {
  printf '%s' "$1" | sed -E 's#[^A-Za-z0-9._-]+#-#g; s#^-+##; s#-+$##'
}

available_mem_mb() {
  free -m | awk '/^Mem:/ { print $7 }'
}

classify_worker_failure() {
  local rc="$1"
  if [[ "$rc" == "124" ]]; then
    printf 'timeout\n'
  elif [[ "$rc" == "137" ]]; then
    printf 'killed\n'
  elif [[ "$rc" == "1" ]]; then
    printf 'contract_or_test_failure\n'
  else
    printf 'worker_error\n'
  fi
}

# Check if process is alive and not a zombie.
proc_alive() {
  local pid="$1"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  [[ -d "/proc/$pid" ]] && ! grep -q 'Z)' "/proc/$pid/stat" 2>/dev/null
}

process_tree_pids() {
  local root="$1"
  [[ "$root" =~ ^[0-9]+$ ]] || return 0

  local child
  if command -v pgrep >/dev/null 2>&1; then
    while IFS= read -r child; do
      [[ -n "$child" ]] || continue
      process_tree_pids "$child"
    done < <(pgrep -P "$root" 2>/dev/null || true)
  fi

  printf '%s\n' "$root"
}

find_stale_orchestrator_pids() {
  ps -eo pid=,args= \
    | awk -v self="$$" '
      $1 != self && $0 ~ /[b]ash .*gormes-auto-codexu-orchestrator\.sh/ {
        print $1
      }
    ' \
    | head -10
}

abort_worker_pids() {
  local reason="${1:-worker failure}"
  shift || true

  local pid tree_pid
  local -a tree=()
  local grace="${FAIL_FAST_ABORT_GRACE_SECONDS:-2}"
  [[ "$grace" =~ ^[0-9]+$ ]] || grace=2

  for pid in "$@"; do
    if proc_alive "$pid"; then
      tree=()
      while IFS= read -r tree_pid; do
        [[ -n "$tree_pid" ]] && tree+=("$tree_pid")
      done < <(process_tree_pids "$pid")

      if (( ${#tree[@]} > 0 )); then
        kill -TERM "${tree[@]}" 2>/dev/null || true
      fi
      log_warn "Aborted worker pid $pid after $reason"
    fi
  done

  local deadline=$((SECONDS + grace))
  while (( SECONDS < deadline )); do
    local any_alive=0
    for pid in "$@"; do
      if proc_alive "$pid"; then
        any_alive=1
        break
      fi
    done
    (( any_alive == 0 )) && return 0
    sleep 0.1
  done

  for pid in "$@"; do
    if proc_alive "$pid"; then
      tree=()
      while IFS= read -r tree_pid; do
        [[ -n "$tree_pid" ]] && tree+=("$tree_pid")
      done < <(process_tree_pids "$pid")
      if (( ${#tree[@]} > 0 )); then
        kill -KILL "${tree[@]}" 2>/dev/null || true
      fi
      log_warn "Force-killed worker pid $pid after $reason"
    fi
  done
}

should_pause_after_cycle() {
  local cycle_rc="$1"
  [[ "${PAUSE_ON_RUN_FAILURE:-1}" == "1" ]] || return 1
  [[ "$cycle_rc" != "0" && "$cycle_rc" != "75" ]]
}

should_run_post_cycle_companions() {
  local cycle_rc="$1"
  [[ "${SKIP_COMPANIONS_ON_RUN_FAILURE:-1}" == "1" ]] || return 0
  [[ "$cycle_rc" == "0" ]]
}

worker_status_outcome() {
  local line="$1"
  if [[ "$line" =~ ^worker\[[0-9]+\]:[[:space:]]+(success|soft-success) ]]; then
    printf 'success\n'
  elif [[ "$line" =~ ^worker\[[0-9]+\]:[[:space:]]+quota-exhausted ]]; then
    printf 'quota\n'
  elif [[ "$line" =~ ^worker\[[0-9]+\]:[[:space:]]+timeout ]]; then
    printf 'timeout\n'
  elif [[ "$line" =~ ^worker\[[0-9]+\]:[[:space:]]+aborted-fail-fast ]]; then
    printf 'aborted\n'
  elif [[ "$line" =~ ^worker\[[0-9]+\]:[[:space:]]+failed ]]; then
    printf 'failed\n'
  else
    printf 'other\n'
  fi
}

provider_quota_message() {
  local file
  for file in "$@"; do
    [[ -n "$file" && -f "$file" ]] || continue
    grep -Eaim1 \
      "You've hit your limit|usage limit|rate limit|quota exceeded|too many requests|HTTP 429|429 Too Many Requests" \
      "$file" && return 0
  done
  return 1
}

provider_quota_exhausted() {
  provider_quota_message "$@" >/dev/null
}

read_progress_summary() {
  local summary complete in_progress planned total
  local progress_json="${PROGRESS_JSON:-}"

  if [[ ! -f "$progress_json" && -n "${REPO_ROOT:-}" && -n "${PROGRESS_JSON_REL:-}" ]]; then
    progress_json="$REPO_ROOT/$PROGRESS_JSON_REL"
  fi

  if [[ -f "$progress_json" ]]; then
    summary="$(jq -r '
      try (
        [
          (.phases // {})
          | to_entries[]
          | (.value.subphases // .value.sub_phases // {})
          | to_entries[]
          | (.value.items // [])[]
          | ((.status // "unknown") | tostring | ascii_downcase)
        ] as $statuses
        | [
            ($statuses | map(select(. == "complete")) | length),
            ($statuses | map(select(. == "in_progress")) | length),
            ($statuses | map(select(. == "planned")) | length)
          ] as $counts
        | "\($counts[0]) \($counts[1]) \($counts[2]) \($counts[0] + $counts[1] + $counts[2])"
      ) catch "0 0 0 0"
    ' "$progress_json" 2>/dev/null || true)"

    IFS=' ' read -r complete in_progress planned total <<< "${summary:-0 0 0 0}"

    if [[ "${total:-0}" == "0" ]]; then
      summary="$(jq -r '
        try (
          [ .phases[]?.subphases[]? | (.items // [])[] | ((.status // "unknown") | tostring | ascii_downcase) ] as $statuses
          | [
              ($statuses | map(select(. == "complete")) | length),
              ($statuses | map(select(. == "in_progress")) | length),
              ($statuses | map(select(. == "planned")) | length)
            ] as $counts
          | "\($counts[0]) \($counts[1]) \($counts[2]) \($counts[0] + $counts[1] + $counts[2])"
        ) catch "0 0 0 0"
      ' "$progress_json" 2>/dev/null || true)"
      IFS=' ' read -r complete in_progress planned total <<< "${summary:-0 0 0 0}"
    fi

    [[ "$complete" =~ ^[0-9]+$ ]] || complete=0
    [[ "$in_progress" =~ ^[0-9]+$ ]] || in_progress=0
    [[ "$planned" =~ ^[0-9]+$ ]] || planned=0
    [[ "$total" =~ ^[0-9]+$ ]] || total=0
  else
    complete=0
    in_progress=0
    planned=0
    total=0
  fi

  printf '%d %d %d %d\n' "$complete" "$in_progress" "$planned" "$total"
}
