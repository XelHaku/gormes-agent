#!/usr/bin/env bash
# Companion-script periodic invocation helpers.
# Depends on: $RUN_ROOT, $STATE_DIR, $PLANNER_EVERY_N_CYCLES, $DOC_IMPROVER_EVERY_N_CYCLES,
#             $LANDINGPAGE_EVERY_N_HOURS, $PLANNER_ROOT, $LOOP_SLEEP_SECONDS,
#             $PROMOTED_LAST_CYCLE, $DISABLE_COMPANIONS, $COMPANION_ON_IDLE,
#             $COMPANION_TIMEOUT_SECONDS, $COMPANION_PLANNER_CMD,
#             $COMPANION_DOC_IMPROVER_CMD, $COMPANION_LANDINGPAGE_CMD.
# Reads the candidates file + optional $_TOTAL_PROGRESS_ITEMS override (tests only).

companion_state_dir() {
  printf '%s/companions\n' "$RUN_ROOT"
}

companion_last_ts() {
  local name="$1"
  local f
  f="$(companion_state_dir)/${name}.last.json"
  if [[ -f "$f" ]]; then
    jq -r '.ts_epoch // 0' "$f"
  else
    printf '0\n'
  fi
}

companion_last_cycle() {
  local name="$1"
  local f
  f="$(companion_state_dir)/${name}.last.json"
  if [[ -f "$f" ]]; then
    jq -r '.cycle // 0' "$f"
  else
    printf '0\n'
  fi
}

companion_cycles_since() {
  local name="$1"
  local current_cycle="$2"
  local last
  last="$(companion_last_cycle "$name")"
  printf '%d\n' $(( current_cycle - last ))
}

companion_already_running() {
  local name="$1"
  local pid_file
  pid_file="$(companion_state_dir)/${name}.pid"
  [[ -f "$pid_file" ]] || return 1
  local pid
  pid="$(cat "$pid_file" 2>/dev/null || true)"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null
}

companion_reap_stale() {
  local state_dir
  state_dir="$(companion_state_dir)"
  [[ -d "$state_dir" ]] || return 0
  local pid_file pid
  for pid_file in "$state_dir"/*.pid; do
    [[ -f "$pid_file" ]] || continue
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    if [[ -z "$pid" ]] || [[ ! "$pid" =~ ^[0-9]+$ ]] || ! kill -0 "$pid" 2>/dev/null; then
      rm -f "$pid_file"
    fi
  done
}

_candidates_remaining() {
  local cf="${CANDIDATES_FILE:-}"
  [[ -n "$cf" && -f "$cf" ]] || { printf '0\n'; return; }
  jq 'length' "$cf"
}

_planner_external_recent() {
  local state="$PLANNER_ROOT/planner_state.json"
  [[ -f "$state" ]] || return 1
  local ts
  ts="$(jq -r '.last_run_utc // empty' "$state")"
  [[ -n "$ts" ]] || return 1
  local epoch
  epoch="$(date -d "$ts" +%s 2>/dev/null || true)"
  [[ -n "$epoch" ]] || return 1
  local threshold=$(( PLANNER_EVERY_N_CYCLES * ${LOOP_SLEEP_SECONDS:-30} * 2 ))
  local now
  now="$(date +%s)"
  (( now - epoch < threshold ))
}

should_run_planner() {
  local cycle="$1"
  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 1
  companion_already_running planner && return 1
  _planner_external_recent && return 1
  local remaining total
  remaining="$(_candidates_remaining)"
  total="${_TOTAL_PROGRESS_ITEMS:-$remaining}"
  (( total > 0 )) || return 0
  # Exhaustion trigger: unclaimed < 10%
  if (( remaining * 10 < total )); then
    return 0
  fi
  local since
  since="$(companion_cycles_since planner "$cycle")"
  (( since >= PLANNER_EVERY_N_CYCLES ))
}

should_run_doc_improver() {
  local cycle="$1"
  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 1
  companion_already_running doc_improver && return 1
  local promoted="${PROMOTED_LAST_CYCLE:-0}"
  (( promoted >= 1 )) || return 1
  local since
  since="$(companion_cycles_since doc_improver "$cycle")"
  (( since >= DOC_IMPROVER_EVERY_N_CYCLES ))
}

should_run_landingpage() {
  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 1
  companion_already_running landingpage && return 1
  local last
  last="$(companion_last_ts landingpage)"
  local now
  now="$(date +%s)"
  local delta=$(( now - last ))
  (( delta >= LANDINGPAGE_EVERY_N_HOURS * 3600 ))
}

run_companion() {
  local name="$1"
  local sync_mode=0
  shift || true
  if [[ "${1:-}" == "--sync" ]]; then
    sync_mode=1
    shift
  fi

  local cmd_var_name
  case "$name" in
    planner)       cmd_var_name="COMPANION_PLANNER_CMD" ;;
    doc_improver)  cmd_var_name="COMPANION_DOC_IMPROVER_CMD" ;;
    landingpage)   cmd_var_name="COMPANION_LANDINGPAGE_CMD" ;;
    *) echo "run_companion: unknown companion '$name'" >&2; return 1 ;;
  esac
  local cmd="${!cmd_var_name:-}"
  if [[ -z "$cmd" ]]; then
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
    case "$name" in
      planner)       cmd="$script_dir/gormes-architecture-planner-tasks-manager.sh" ;;
      doc_improver)  cmd="$script_dir/documentation-improver.sh" ;;
      landingpage)   cmd="$script_dir/landingpage-improver.sh" ;;
    esac
  fi

  local state_dir
  state_dir="$(companion_state_dir)"
  mkdir -p "$state_dir"
  local pid_file="$state_dir/${name}.pid"
  local log_file="${LOGS_DIR}/companion_${name}.$(date -u +%Y%m%dT%H%M%SZ).log"

  # Skip if already running.
  if [[ -f "$pid_file" ]]; then
    local prev_pid
    prev_pid="$(cat "$pid_file" 2>/dev/null || true)"
    if [[ "$prev_pid" =~ ^[0-9]+$ ]] && kill -0 "$prev_pid" 2>/dev/null; then
      return 0
    fi
    rm -f "$pid_file"
  fi

  # Per-companion timeout, falling back to the shared default. Planner's
  # full upstream-Hermes scan + codex LLM call routinely exceeds 10 min;
  # doc-improver / landingpage are cheaper. Specialized knobs:
  #   COMPANION_PLANNER_TIMEOUT_SECONDS        (default 1800)
  #   COMPANION_DOC_IMPROVER_TIMEOUT_SECONDS   (default COMPANION_TIMEOUT_SECONDS)
  #   COMPANION_LANDINGPAGE_TIMEOUT_SECONDS    (default COMPANION_TIMEOUT_SECONDS)
  local timeout_s="${COMPANION_TIMEOUT_SECONDS:-600}"
  case "$name" in
    planner)      timeout_s="${COMPANION_PLANNER_TIMEOUT_SECONDS:-1800}" ;;
    doc_improver) timeout_s="${COMPANION_DOC_IMPROVER_TIMEOUT_SECONDS:-$timeout_s}" ;;
    landingpage)  timeout_s="${COMPANION_LANDINGPAGE_TIMEOUT_SECONDS:-$timeout_s}" ;;
  esac
  local ts_start
  ts_start="$(date +%s)"
  local ts_utc
  ts_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  if (( sync_mode == 1 )); then
    # Foreground execution (used by candidate refill, Task 7).
    local rc=0
    (
      cd "$GIT_ROOT"
      AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0 \
        timeout "$timeout_s" bash "$cmd" \
        >"$log_file" 2>&1
    ) || rc=$?
    jq -n \
      --arg name "$name" \
      --argjson ts_epoch "$ts_start" \
      --arg ts_utc "$ts_utc" \
      --argjson cycle "${ORCH_CURRENT_CYCLE:-0}" \
      --argjson rc "$rc" \
      --arg log_file "$log_file" \
      --argjson sync true \
      '{name:$name,ts_epoch:$ts_epoch,ts_utc:$ts_utc,cycle:$cycle,rc:$rc,log_file:$log_file,sync:$sync}' \
      > "$state_dir/${name}.last.json"
    type log_event >/dev/null 2>&1 && log_event "companion_${name}_completed" null "rc=$rc sync=1" "completed" || true
    return "$rc"
  fi

  # Detached launch: setsid + nohup so the child survives the orchestrator
  # and does not block the main forever-loop. The inner bash -c script
  # records its own last.json + cleans its pid file when done.
  local current_cycle="${ORCH_CURRENT_CYCLE:-0}"
  setsid nohup bash -c "
    cd '$GIT_ROOT' || exit 1
    export AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0
    export PHASE_FLOOR='${PHASE_FLOOR:-}' PHASE_PRIORITY_BOOST='${PHASE_PRIORITY_BOOST:-}'
    export PHASE_SKIP_SUBPHASES='${PHASE_SKIP_SUBPHASES:-}' MAX_RETRIES='${MAX_RETRIES:-}' BACKEND='${BACKEND:-}'
    timeout '$timeout_s' bash '$cmd' >'$log_file' 2>&1
    ec=\$?
    jq -n \
      --arg name '$name' \
      --argjson ts_epoch '$ts_start' \
      --arg ts_utc '$ts_utc' \
      --argjson cycle '$current_cycle' \
      --argjson rc \$ec \
      --arg log_file '$log_file' \
      --argjson sync false \
      '{name:\$name,ts_epoch:\$ts_epoch,ts_utc:\$ts_utc,cycle:\$cycle,rc:\$rc,log_file:\$log_file,sync:\$sync}' \
      > '$state_dir/${name}.last.json'
    rm -f '$pid_file'
  " </dev/null >/dev/null 2>&1 &
  local bg_pid=$!
  echo "$bg_pid" > "$pid_file"
  type log_event >/dev/null 2>&1 && log_event "companion_${name}_started" null "pid=$bg_pid async=1" "started" || true
  return 0
}

maybe_run_companions() {
  local cycle="$1"
  local promoted="${2:-0}"
  export ORCH_CURRENT_CYCLE="$cycle"
  export PROMOTED_LAST_CYCLE="$promoted"

  [[ "${DISABLE_COMPANIONS:-0}" == "1" ]] && return 0

  companion_reap_stale

  local exhausted=0
  local remaining
  remaining="$(_candidates_remaining)"
  local total="${_TOTAL_PROGRESS_ITEMS:-$remaining}"
  if (( total > 0 )) && (( remaining * 10 < total )); then
    exhausted=1
  fi

  if [[ "${COMPANION_ON_IDLE:-1}" == "1" && "$exhausted" == "0" && "$promoted" == "0" ]]; then
    return 0
  fi

  if should_run_planner "$cycle"; then
    run_companion planner || true
    export EXHAUSTION_TRIGGERED="$exhausted"
  fi
  if should_run_doc_improver "$cycle"; then
    run_companion doc_improver || true
  fi
  if should_run_landingpage; then
    run_companion landingpage || true
  fi
}
