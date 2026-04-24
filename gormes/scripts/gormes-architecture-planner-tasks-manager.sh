#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'
shopt -s inherit_errexit 2>/dev/null || true

# === STEP TIMING ===
# Track elapsed time per step for performance monitoring
STEP_TIMERS=()
STEP_START_TIME=""

start_step_timer() {
  STEP_START_TIME=$(date +%s)
}

get_step_elapsed() {
  local start="$1"
  local end
  end=$(date +%s)
  echo $((end - start))
}

format_duration() {
  local seconds=$1
  if [[ $seconds -lt 60 ]]; then
    echo "${seconds}s"
  elif [[ $seconds -lt 3600 ]]; then
    echo "$((seconds / 60))m $((seconds % 60))s"
  else
    echo "$((seconds / 3600))h $(((seconds % 3600) / 60))m"
  fi
}

log_step_duration() {
  local step_name="$1"
  local start="$2"
  local end
  end=$(date +%s)
  local elapsed=$((end - start))
  log_info "Step '$step_name' completed in $(format_duration $elapsed)"
}

# === SIGNAL HANDLING ===
# Graceful shutdown support
SHUTDOWN_REQUESTED=false
CLEANUP_HOOKS=()

request_shutdown() {
  SHUTDOWN_REQUESTED=true
  log_warn "Shutdown requested - finishing gracefully..."
}

cleanup() {
  local exit_code=$?
  # Prevent re-entry
  trap - EXIT INT TERM HUP

  log_info "Shutdown complete (exit code: $exit_code)"

  # Run cleanup hooks in reverse order (LIFO)
  while [[ ${#CLEANUP_HOOKS[@]} -gt 0 ]]; do
    local hook="${CLEANUP_HOOKS[-1]}"
    unset 'CLEANUP_HOOKS[-1]'
    log_debug "Running cleanup: $hook"
    eval "$hook" 2>/dev/null || true
  done

  exit "$exit_code"
}

# Register signal handlers
trap cleanup EXIT INT TERM HUP
trap request_shutdown INT TERM

# === ERROR HANDLER ===
err_trap() {
  local exit_code=$?
  local line=${BASH_LINENO[0]}
  local cmd="${BASH_COMMAND}"
  echo "ERROR at line $line: exit $exit_code: $cmd" >&2
  log_json "ERROR" "Script error at line $line: exit $exit_code: $cmd"
  local i=0
  while caller "$i" >&2; do ((i++)); done
}
trap err_trap ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"

PLANNER_ROOT="${PLANNER_ROOT:-$REPO_ROOT/.codex/planner}"
LOG_DIR="$PLANNER_ROOT/logs"
STATE_FILE="$PLANNER_ROOT/planner_state.json"
REPORT_FILE="$PLANNER_ROOT/latest_planner_report.md"
RAW_REPORT_FILE="$PLANNER_ROOT/latest_planner_report.raw.md"
CONTEXT_FILE="$PLANNER_ROOT/context.json"
PROMPT_FILE="$PLANNER_ROOT/latest_prompt.txt"
TASKS_MD_FILE="$PLANNER_ROOT/architecture-planner-tasks.md"
LOCK_DIR="$PLANNER_ROOT/run.lock"
LOCK_PID_FILE="$LOCK_DIR/pid"
LOCK_STARTED_FILE="$LOCK_DIR/started_at"
LOCK_COMMAND_FILE="$LOCK_DIR/command"

PROGRESS_JSON="$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
ARCH_PLAN_DIR="$REPO_ROOT/docs/content/building-gormes/architecture_plan"
ARCH_PLAN_JSON="$ARCH_PLAN_DIR/architecture_plan.json"
CORE_SYSTEMS_DIR="$REPO_ROOT/docs/content/building-gormes/core-systems"
RUN_AT_UTC="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
RUN_STAMP="$(date -u +"%Y%m%dT%H%M%SZ")"

PLANNER_TIMER_NAME="${PLANNER_TIMER_NAME:-gormes-architecture-planner-tasks-manager}"
PLANNER_INTERVAL="${PLANNER_INTERVAL:-4h}"
PLANNER_BOOT_DELAY="${PLANNER_BOOT_DELAY:-5m}"
PLANNER_INSTALL_SCHEDULE="${PLANNER_INSTALL_SCHEDULE:-1}"

# Verbosity and commit mode settings
VERBOSE="${VERBOSE:-0}"
AUTO_COMMIT="${AUTO_COMMIT:-0}"
AUTO_PUSH="${AUTO_PUSH:-0}"
MAIN_BRANCH="${MAIN_BRANCH:-main}"
REMOTE_NAME="${REMOTE_NAME:-origin}"
COMMIT_INTERVAL="${COMMIT_INTERVAL:-1}"
JSON_LOG="${JSON_LOG:-}"

mkdir -p "$PLANNER_ROOT" "$LOG_DIR"

CODEXU_JSONL="$LOG_DIR/$RUN_STAMP.codexu.jsonl"
CODEXU_STDERR="$LOG_DIR/$RUN_STAMP.codexu.stderr"
VALIDATION_LOG="$LOG_DIR/$RUN_STAMP.validation.log"

UPSTREAM_HERMES_DIR=""
UPSTREAM_COMMIT=""
UPSTREAM_BRANCH=""
LOCAL_GIT_ROOT=""
LOCAL_COMMIT=""
LOCAL_BRANCH=""
ARCH_PLAN_JSON_PRESENT="false"
SCHEDULE_METHOD="none"

# ANSI color codes for colored output
if [[ -t 1 ]] || [[ "${FORCE_COLOR:-0}" == "1" ]]; then
  COLOR_RED='\033[0;31m'
  COLOR_YELLOW='\033[1;33m'
  COLOR_GREEN='\033[0;32m'
  COLOR_BLUE='\033[0;34m'
  COLOR_DIM='\033[2m'
  COLOR_BOLD='\033[1m'
  COLOR_RESET='\033[0m'
else
  COLOR_RED=''
  COLOR_YELLOW=''
  COLOR_GREEN=''
  COLOR_BLUE=''
  COLOR_DIM=''
  COLOR_BOLD=''
  COLOR_RESET=''
fi

log() {
  printf '[architecture-planner-tasks-manager] %s\n' "$*"
}

log_debug() {
  if [[ "$VERBOSE" == "1" ]]; then
    printf '%b[DEBUG]%b %s %s\n' "$COLOR_DIM" "$COLOR_RESET" "$(date '+%H:%M:%S')" "$*" >&2
  fi
}

log_info() {
  if [[ "$VERBOSE" == "1" ]]; then
    printf '%b[INFO]%b  %s %s\n' "$COLOR_BLUE" "$COLOR_RESET" "$(date '+%H:%M:%S')" "$*"
  fi
}

log_warn() {
  printf '%b[WARN]%b  %s %s\n' "$COLOR_YELLOW" "$COLOR_RESET" "$(date '+%H:%M:%S')" "$*" >&2
}

log_error() {
  printf '%b[ERROR]%b %s %s\n' "$COLOR_RED" "$COLOR_RESET" "$(date '+%H:%M:%S')" "$*" >&2
}

# Structured JSON logging
log_json() {
  local level="$1"
  shift
  if [[ -n "$JSON_LOG" ]]; then
    jq -nc \
      --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg level "$level" \
      --arg msg "$*" \
      --arg script "architecture-planner-tasks-manager" \
      '{ts:$ts,level:$level,msg:$msg,script:$script}' >> "$JSON_LOG"
  fi
}

# Progress bar display
show_progress() {
  local current=$1
  local total=$2
  local label="${3:-Progress}"
  local width=40
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

progress() {
  local step="$1"
  shift
  printf '[architecture-planner-tasks-manager] %s/5 %s\n' "$step" "$*"
  log_info "Progress step $step: $*"
}

fail() {
  printf '[architecture-planner-tasks-manager] ERROR: %s\n' "$*" >&2
  log_error "$*"
  exit 1
}

# Git pre-flight checks before commits
git_preflight_check() {
  local repo="${1:-$REPO_ROOT}"

  log_debug "Running git pre-flight checks for $repo"

  if ! git -C "$repo" rev-parse --git-dir > /dev/null 2>&1; then
    log_error "Not a git repository: $repo"
    return 1
  fi

  local current_branch
  current_branch=$(git -C "$repo" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

  if [[ "$current_branch" != "$MAIN_BRANCH" ]]; then
    log_warn "Currently on branch '$current_branch', not '$MAIN_BRANCH'"
  fi

  if git -C "$repo" rev-parse --verify MERGE_HEAD > /dev/null 2>&1; then
    log_error "Merge in progress in $repo - cannot commit"
    return 1
  fi

  if git -C "$repo" rev-parse --verify REBASE_HEAD > /dev/null 2>&1; then
    log_error "Rebase in progress in $repo - cannot commit"
    return 1
  fi

  log_debug "Git pre-flight checks passed"
  return 0
}

# Incremental commit functionality
commit_changes() {
  local message="${1:-Planner auto-commit}"
  local repo="${2:-$REPO_ROOT}"

  if [[ "$AUTO_COMMIT" != "1" ]]; then
    log_debug "AUTO_COMMIT disabled, skipping commit"
    return 0
  fi

  log_info "Checking for changes to commit in $repo"

  if ! git_preflight_check "$repo"; then
    log_warn "Git pre-flight check failed, skipping commit"
    return 1
  fi

  if ! git -C "$repo" diff --quiet HEAD || ! git -C "$repo" diff --cached --quiet; then
    log_info "Committing changes: $message"
    git -C "$repo" add -A
    if git -C "$repo" commit -m "$message" -m "Auto-generated by architecture-planner-tasks-manager" 2>/dev/null; then
      local commit_hash
      commit_hash=$(git -C "$repo" rev-parse --short HEAD)
      log_info "Created commit $commit_hash"
      log_json "INFO" "Committed changes: $message ($commit_hash)"

      if [[ "$AUTO_PUSH" == "1" ]]; then
        push_changes "$repo"
      fi

      return 0
    else
      log_warn "No changes to commit or commit failed"
      return 1
    fi
  else
    log_debug "No changes to commit"
    return 0
  fi
}

push_changes() {
  local repo="${1:-$REPO_ROOT}"

  if [[ "$AUTO_PUSH" != "1" ]]; then
    return 0
  fi

  log_info "Pushing to $REMOTE_NAME/$MAIN_BRANCH"
  if git -C "$repo" push "$REMOTE_NAME" "$MAIN_BRANCH"; then
    log_info "Successfully pushed to remote"
    log_json "INFO" "Pushed to $REMOTE_NAME/$MAIN_BRANCH"
  else
    log_error "Failed to push to remote"
    return 1
  fi
}

# Smart commit based on operation type
smart_commit() {
  local operation="$1"
  local details="${2:-}"
  local commit_msg="planner: $operation"

  if [[ -n "$details" ]]; then
    commit_msg="$commit_msg - $details"
  fi

  # Add timestamp for frequent commits
  commit_msg="$commit_msg [$(date '+%H:%M:%S')]"

  commit_changes "$commit_msg"
}

usage() {
  cat <<EOF
Usage:
  gormes-architecture-planner-tasks-manager.sh [run]
  gormes-architecture-planner-tasks-manager.sh status
  gormes-architecture-planner-tasks-manager.sh show-report
  gormes-architecture-planner-tasks-manager.sh doctor
  gormes-architecture-planner-tasks-manager.sh install-schedule
  gormes-architecture-planner-tasks-manager.sh --help

Commands:
  run              Audit upstream Hermes, refresh planner context/tasks/report, validate, and schedule.
  status           Print the latest planner state summary.
  show-report      Print the latest planner report.
  doctor           Validate local dependencies and required planning files.
  install-schedule Install or refresh the periodic systemd/cron schedule only.

Environment:
  REPO_ROOT                 Default: $REPO_ROOT
  PLANNER_ROOT              Default: $PLANNER_ROOT
  PLANNER_INSTALL_SCHEDULE  1 installs/refreshes schedule during run; 0 disables it.
  PLANNER_TIMER_NAME        Default: $PLANNER_TIMER_NAME
  PLANNER_INTERVAL          Default: $PLANNER_INTERVAL
  PLANNER_BOOT_DELAY        Default: $PLANNER_BOOT_DELAY
  UPSTREAM_HERMES_DIR       Optional explicit upstream Hermes checkout.

  # New verbosity and commit options
  VERBOSE                   Set to 1 for detailed progress logging
  AUTO_COMMIT               Set to 1 to auto-commit changes after each stage
  AUTO_PUSH                 Set to 1 to auto-push commits to remote
  MAIN_BRANCH               Target branch for commits (default: main)
  REMOTE_NAME               Git remote name (default: origin)
  JSON_LOG                  Path to JSON log file for structured logging

Examples:
  VERBOSE=1 ./gormes-architecture-planner-tasks-manager.sh
  AUTO_COMMIT=1 AUTO_PUSH=1 ./gormes-architecture-planner-tasks-manager.sh
  VERBOSE=1 JSON_LOG=/tmp/planner.jsonl ./gormes-architecture-planner-tasks-manager.sh
EOF
}

release_lock() {
  if [[ -d "$LOCK_DIR" ]]; then
    rm -f "$LOCK_PID_FILE" "$LOCK_STARTED_FILE" "$LOCK_COMMAND_FILE"
    rmdir "$LOCK_DIR" 2>/dev/null || true
  fi
}

write_lock_metadata() {
  printf '%s\n' "$$" > "$LOCK_PID_FILE"
  printf '%s\n' "$RUN_AT_UTC" > "$LOCK_STARTED_FILE"
  printf '%q ' "$0" "$@" > "$LOCK_COMMAND_FILE"
  printf '\n' >> "$LOCK_COMMAND_FILE"
}

read_lock_file() {
  local file="$1"
  if [[ -f "$file" ]]; then
    head -n1 "$file"
  fi
}

process_is_running() {
  local pid="$1"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null
}

remove_stale_lock() {
  rm -f "$LOCK_PID_FILE" "$LOCK_STARTED_FILE" "$LOCK_COMMAND_FILE"
  rmdir "$LOCK_DIR" 2>/dev/null || fail "stale lock could not be removed safely: $LOCK_DIR"
}

find_legacy_lock_owner() {
  ps -eo pid=,etime=,stat=,args= 2>/dev/null \
    | awk -v self="$$" '
        $1 != self && $0 ~ /gormes-architecture-planner-tasks-manager[.]sh/ {
          print
          exit
        }
      '
}

claim_lock() {
  local lock_pid lock_started lock_command legacy_owner

  if mkdir "$LOCK_DIR" 2>/dev/null; then
    write_lock_metadata "$@"
    trap release_lock EXIT
    return 0
  fi

  lock_pid="$(read_lock_file "$LOCK_PID_FILE" || true)"
  lock_started="$(read_lock_file "$LOCK_STARTED_FILE" || true)"
  lock_command="$(read_lock_file "$LOCK_COMMAND_FILE" || true)"

  if process_is_running "$lock_pid"; then
    fail "active planner run owns $LOCK_DIR
PID: $lock_pid
Started: ${lock_started:-unknown}
Command: ${lock_command:-unknown}"
  fi

  if [[ -z "$lock_pid" ]]; then
    legacy_owner="$(find_legacy_lock_owner || true)"
    if [[ -n "$legacy_owner" ]]; then
      fail "active legacy planner run owns $LOCK_DIR
Process: $legacy_owner
This run started before lock owner metadata existed; wait for it to finish."
    fi

    fail "planner lock has no owner metadata: $LOCK_DIR
No active planner process was detected. Remove the stale lock with: rmdir '$LOCK_DIR'"
  fi

  log "Removing stale planner lock: $LOCK_DIR (PID: $lock_pid, Started: ${lock_started:-unknown})"
  remove_stale_lock
  mkdir "$LOCK_DIR" 2>/dev/null || fail "another planner run claimed the lock: $LOCK_DIR"
  write_lock_metadata "$@"
  trap release_lock EXIT
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

require_file() {
  [[ -f "$1" ]] || fail "required file not found: $1"
}

require_dir() {
  [[ -d "$1" ]] || fail "required directory not found: $1"
}

git_field() {
  local repo="$1"
  local field="$2"
  case "$field" in
    commit)
      git -C "$repo" rev-parse HEAD
      ;;
    branch)
      git -C "$repo" rev-parse --abbrev-ref HEAD
      ;;
    root)
      git -C "$repo" rev-parse --show-toplevel
      ;;
    *)
      return 1
      ;;
  esac
}

is_upstream_hermes_root() {
  local candidate="$1"
  [[ -d "$candidate/.git" ]] \
    && [[ -f "$candidate/run_agent.py" ]] \
    && [[ -d "$candidate/gateway" ]] \
    && [[ -d "$candidate/tools" ]]
}

detect_upstream_hermes_dir() {
  if [[ -n "${UPSTREAM_HERMES_DIR:-}" ]]; then
    is_upstream_hermes_root "$UPSTREAM_HERMES_DIR" || fail "UPSTREAM_HERMES_DIR is not a Hermes root: $UPSTREAM_HERMES_DIR"
    printf '%s\n' "$UPSTREAM_HERMES_DIR"
    return 0
  fi

  local candidate
  candidate="$(cd "$REPO_ROOT/.." && pwd)"
  if is_upstream_hermes_root "$candidate"; then
    printf '%s\n' "$candidate"
    return 0
  fi

  candidate="$REPO_ROOT"
  while [[ "$candidate" != "/" ]]; do
    if is_upstream_hermes_root "$candidate"; then
      printf '%s\n' "$candidate"
      return 0
    fi
    candidate="$(dirname "$candidate")"
  done

  fail "unable to auto-detect upstream Hermes root from $REPO_ROOT"
}

json_array_from_lines() {
  jq -Rn '[inputs | select(length > 0)]'
}

collect_architecture_docs_json() {
  find "$ARCH_PLAN_DIR" -maxdepth 1 -type f -name '*.md' | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
}

collect_core_docs_json() {
  if [[ -d "$CORE_SYSTEMS_DIR" ]]; then
    find "$CORE_SYSTEMS_DIR" -maxdepth 1 -type f -name '*.md' | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
  else
    printf '[]\n'
  fi
}

collect_upstream_features_json() {
  local upstream="$1"
  {
    if [[ -d "$upstream/gateway/platforms" ]]; then echo "gateway/platforms"; fi
    if [[ -f "$upstream/gateway/session.py" ]]; then echo "gateway/session.py"; fi
    if [[ -d "$upstream/tools" ]]; then echo "tools/"; fi
    if [[ -d "$upstream/cron" ]]; then echo "cron/"; fi
    if [[ -d "$upstream/hermes_cli" ]]; then echo "hermes_cli/"; fi
    if [[ -d "$upstream/tests/e2e" ]]; then echo "tests/e2e"; fi
    if [[ -d "$upstream/skills" ]]; then echo "skills/"; fi
  } | json_array_from_lines
}

collect_recent_changed_files_json() {
  local repo="$1"
  if git -C "$repo" rev-parse HEAD~1 >/dev/null 2>&1; then
    git -C "$repo" diff --name-only HEAD~1..HEAD | head -n 50 | json_array_from_lines
  else
    printf '[]\n'
  fi
}

collect_gormes_packages_json() {
  find "$REPO_ROOT/internal" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
}

collect_gormes_commands_json() {
  find "$REPO_ROOT/cmd" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
}

collect_progress_items_json() {
  jq -c '
    [
      (.phases // {}) | to_entries[] as $phase
      | (($phase.value.subphases // {}) | to_entries[]) as $subphase
      | ($subphase.value.items // [])[] as $item
      | {
          phase_id: $phase.key,
          phase_name: ($phase.value.name // $phase.key),
          subphase_id: $subphase.key,
          subphase_name: ($subphase.value.name // $subphase.key),
          item_name: ($item.name // $item.item_name // $item.title // ""),
          status: ($item.status // "unknown"),
          note: ($item.note // "")
        }
    ]
  ' "$PROGRESS_JSON"
}

collect_progress_summary_json() {
  jq -c '
    def items:
      [
        (.phases // {}) | to_entries[] as $phase
        | (($phase.value.subphases // {}) | to_entries[]) as $subphase
        | ($subphase.value.items // [])[] as $item
        | {
            status: ($item.status // "unknown"),
            note: ($item.note // "")
          }
      ];
    def subphases:
      [
        (.phases // {}) | to_entries[] as $phase
        | (($phase.value.subphases // {}) | to_entries[]) as $subphase
        | {
            id: $subphase.key,
            priority: ($subphase.value.priority // "")
          }
      ];
    {
      phase_count: ((.phases // {}) | length),
      subphase_count: (subphases | length),
      item_count: (items | length),
      planned_items: (items | map(select(.status == "planned")) | length),
      in_progress_items: (items | map(select(.status == "in_progress")) | length),
      complete_items: (items | map(select(.status == "complete")) | length),
      items_missing_notes: (items | map(select(.note == "")) | length),
      subphases_missing_priority: (subphases | map(select(.priority == "")) | length)
    }
  ' "$PROGRESS_JSON"
}

read_previous_state_json() {
  if [[ -f "$STATE_FILE" ]]; then
    cat "$STATE_FILE"
  else
    printf '{}\n'
  fi
}

write_context_bundle() {
  local upstream_features_json recent_changed_json arch_docs_json core_docs_json packages_json commands_json items_json summary_json prev_state_json

  upstream_features_json="$(collect_upstream_features_json "$UPSTREAM_HERMES_DIR")"
  recent_changed_json="$(collect_recent_changed_files_json "$UPSTREAM_HERMES_DIR")"
  arch_docs_json="$(collect_architecture_docs_json)"
  core_docs_json="$(collect_core_docs_json)"
  packages_json="$(collect_gormes_packages_json)"
  commands_json="$(collect_gormes_commands_json)"
  items_json="$(collect_progress_items_json)"
  summary_json="$(collect_progress_summary_json)"
  prev_state_json="$(read_previous_state_json)"

  jq -n \
    --arg run_at_utc "$RUN_AT_UTC" \
    --arg gormes_repo_root "$REPO_ROOT" \
    --arg progress_json "$PROGRESS_JSON" \
    --arg architecture_plan_json "$ARCH_PLAN_JSON" \
    --arg upstream_hermes_dir "$UPSTREAM_HERMES_DIR" \
    --arg upstream_commit "$UPSTREAM_COMMIT" \
    --arg upstream_branch "$UPSTREAM_BRANCH" \
    --arg local_git_root "$LOCAL_GIT_ROOT" \
    --arg local_commit "$LOCAL_COMMIT" \
    --arg local_branch "$LOCAL_BRANCH" \
    --argjson architecture_plan_json_present "$ARCH_PLAN_JSON_PRESENT" \
    --argjson upstream_feature_surfaces "$upstream_features_json" \
    --argjson upstream_recent_changed_files "$recent_changed_json" \
    --argjson architecture_docs "$arch_docs_json" \
    --argjson core_system_docs "$core_docs_json" \
    --argjson gormes_internal_packages "$packages_json" \
    --argjson gormes_commands "$commands_json" \
    --argjson progress_items "$items_json" \
    --argjson progress_summary "$summary_json" \
    --argjson previous_state "$prev_state_json" \
    '{
      run_at_utc: $run_at_utc,
      gormes_repo_root: $gormes_repo_root,
      progress_json: $progress_json,
      architecture_plan_json: $architecture_plan_json,
      architecture_plan_json_present: $architecture_plan_json_present,
      architecture_docs: $architecture_docs,
      core_system_docs: $core_system_docs,
      upstream_hermes_dir: $upstream_hermes_dir,
      upstream_commit: $upstream_commit,
      upstream_branch: $upstream_branch,
      upstream_feature_surfaces: $upstream_feature_surfaces,
      upstream_recent_changed_files: $upstream_recent_changed_files,
      local_git_root: $local_git_root,
      local_commit: $local_commit,
      local_branch: $local_branch,
      gormes_internal_packages: $gormes_internal_packages,
      gormes_commands: $gormes_commands,
      progress_summary: $progress_summary,
      progress_items: $progress_items,
      previous_state: $previous_state
    }' > "$CONTEXT_FILE"
}

write_prompt_file() {
  cat > "$PROMPT_FILE" <<EOF
You are the Gormes Architecture Planner Agent.

Mission:
Audit upstream Hermes against Gormes, detect drift or stale assumptions in planning artifacts, and update planning/docs/progress so execution agents receive smaller, more accurate, TDD-first slices.

Automatic-run constraints:
- Planning/docs/progress work only. Do not implement runtime feature code.
- Be conservative with truth-changing edits.
- Do not mark implementation complete without concrete repository evidence.
- Do not delete tasks unless they are clearly obsolete and unreferenced.
- Prefer smaller, dependency-aware tasks with explicit TDD sequencing.
- If \`$ARCH_PLAN_JSON\` is missing, treat \`$PROGRESS_JSON\` plus the architecture markdown under \`$ARCH_PLAN_DIR\` as the active planning source of truth.
- Use internal naming GONCHO when memory subsystem naming appears, while preserving Honcho-compatible external interfaces.

Repository roots:
- Upstream Hermes monorepo: $UPSTREAM_HERMES_DIR
- Gormes target repo: $REPO_ROOT

Files of record:
- Progress ledger: $PROGRESS_JSON
- Architecture narrative directory: $ARCH_PLAN_DIR
- Core systems docs: $CORE_SYSTEMS_DIR
- Context bundle generated by the coordinator: $CONTEXT_FILE

Operator scope hints (inherited from the orchestrator env):
- PHASE_FLOOR=${PHASE_FLOOR:-unset} — when set, prioritize planning work
  for phases <= this floor. Higher-phase tasks may still be kept but should
  be described as "deferred" so execution agents skip them.
- PHASE_PRIORITY_BOOST=${PHASE_PRIORITY_BOOST:-unset} — subphase IDs that
  should be decomposed into the most granular TDD-ready slices first.
- PHASE_SKIP_SUBPHASES=${PHASE_SKIP_SUBPHASES:-unset} — subphases the
  orchestrator is intentionally NOT working on right now. Do not create
  new tasks inside these subphases; do not remove existing ones either —
  they stay as "deferred" so the shape is preserved when the user resumes
  them.

Required work:
1. Inspect upstream Hermes surfaces and current Gormes code/docs/progress.
2. Detect missing tasks, stale task status, stale assumptions, weak task granularity, and missing dependencies.
3. Update planning artifacts conservatively so execution agents can work from small, TDD-ready slices.
   When PHASE_FLOOR is set, focus decomposition effort on subphases inside
   that floor; leave higher-phase items at their current granularity.
4. Keep documentation synchronized when progress or roadmap wording changes.
5. Run validation after any doc/progress edits:
   - go run ./cmd/progress-gen -write
   - go run ./cmd/progress-gen -validate
   - go test ./internal/progress -count=1
   - go test ./docs -count=1

Required final report format:
1) Scope scanned
2) Upstream Hermes delta summary
3) Our repo status summary
4) Plan quality problems
5) Proposed changes
6) Actual changes written
7) Recommended next execution tasks
8) Risks / ambiguities
EOF
}

verify_final_report() {
  local file="$1"
  [[ -f "$file" ]] || return 1

  local number title pattern
  # The report format uses various header styles:
  #   **1. Scope Scanned**  (bold around whole line with period)
  #   1) **Scope scanned**   (number with parenthesis, bold around title only)
  #   1. Scope scanned       (plain format without bold)
  # Pattern must handle all these variations
  while IFS='|' read -r number title; do
    # Build pattern that matches:
    # - Optional ** at start of line
    # - Number with . or )
    # - Optional space
    # - Optional ** around the title
    # - The title text
    # - Optional closing **
    # - Optional trailing whitespace
    pattern="^[[:space:]]*(\\*\\*)?${number}[.)][[:space:]]+(\\*\\*)?${title}(\\*\\*)?[[:space:]]*$"
    # Use case-insensitive matching (-i) since report titles may have capital letters
    grep -Ei "$pattern" "$file" > /dev/null || return 1
  done <<'EOF'
1|Scope scanned
2|Upstream Hermes delta summary
3|Our repo status summary
4|Plan quality problems
5|Proposed changes
6|Actual changes written
7|Recommended next execution tasks
8|Risks / ambiguities
EOF
}

write_tasks_markdown() {
  jq -r --arg generated "$RUN_AT_UTC" '
    def rows:
      [
        (.phases // {}) | to_entries[] as $phase
        | (($phase.value.subphases // {}) | to_entries[]) as $subphase
        | ($subphase.value.items // [])[] as $item
        | {
            phase_name: ($phase.value.name // ("Phase " + $phase.key)),
            subphase_id: $subphase.key,
            subphase_name: ($subphase.value.name // $subphase.key),
            item_name: ($item.name // $item.item_name // $item.title // ""),
            status: (($item.status // "unknown") | tostring | ascii_downcase),
            note: ($item.note // "")
          }
        | select(.item_name != "")
      ];
    def count_status($status): rows | map(select(.status == $status)) | length;
    [
      "# Gormes Architecture Planner Tasks",
      "",
      "Generated UTC: \($generated)",
      "",
      "## Summary",
      "",
      "- Complete: \(count_status("complete"))",
      "- In progress: \(count_status("in_progress"))",
      "- Planned: \(count_status("planned"))",
      "",
      "## Open Tasks",
      ""
    ],
    (
      rows
      | map(select(.status != "complete"))
      | if length == 0 then
          ["No open tasks."]
        else
          map("- [\(if .status == "in_progress" then "~" else " " end)] \(.phase_name) / \(.subphase_id): \(.item_name)")
        end
    )
    | .[]
  ' "$PROGRESS_JSON" > "$TASKS_MD_FILE"
}

log_context_summary() {
  local phase_count subphase_count item_count planned in_progress complete missing_notes arch_doc_count core_doc_count

  phase_count="$(jq -r '.progress_summary.phase_count // 0' "$CONTEXT_FILE")"
  subphase_count="$(jq -r '.progress_summary.subphase_count // 0' "$CONTEXT_FILE")"
  item_count="$(jq -r '.progress_summary.item_count // 0' "$CONTEXT_FILE")"
  planned="$(jq -r '.progress_summary.planned_items // 0' "$CONTEXT_FILE")"
  in_progress="$(jq -r '.progress_summary.in_progress_items // 0' "$CONTEXT_FILE")"
  complete="$(jq -r '.progress_summary.complete_items // 0' "$CONTEXT_FILE")"
  missing_notes="$(jq -r '.progress_summary.items_missing_notes // 0' "$CONTEXT_FILE")"
  arch_doc_count="$(jq -r '(.architecture_docs // []) | length' "$CONTEXT_FILE")"
  core_doc_count="$(jq -r '(.core_system_docs // []) | length' "$CONTEXT_FILE")"

  log "Repo root: $REPO_ROOT"
  log "Local git root: $LOCAL_GIT_ROOT"
  log "Local branch: $LOCAL_BRANCH"
  log "Local commit: $LOCAL_COMMIT"
  log "Upstream Hermes: $UPSTREAM_HERMES_DIR"
  log "Upstream branch: $UPSTREAM_BRANCH"
  log "Upstream commit: $UPSTREAM_COMMIT"
  log "Progress items: phases=$phase_count subphases=$subphase_count total=$item_count planned=$planned in_progress=$in_progress complete=$complete missing_notes=$missing_notes"
  log "Architecture docs: $arch_doc_count; core system docs: $core_doc_count"
  log "Task Markdown: $TASKS_MD_FILE"
}

report_validation_error() {
  local file="$1"
  if [[ -f "$file" ]]; then
    printf '[architecture-planner-tasks-manager] Raw report saved at %s\n' "$file" >&2
  fi
}

extract_session_id() {
  local jsonl_file="$1"
  [[ -f "$jsonl_file" ]] || return 0
  jq -r 'select(.type=="thread.started") | (.thread_id // .session_id // empty)' "$jsonl_file" | head -n1
}

run_codexu_planner() {
  write_prompt_file

  (
    cd "$REPO_ROOT"
    codexu exec --json \
      -c approval_policy=never \
      -c sandbox_mode=danger-full-access \
      --output-last-message "$RAW_REPORT_FILE" \
      "$(cat "$PROMPT_FILE")" \
      >"$CODEXU_JSONL" 2>"$CODEXU_STDERR"
  )

  if ! verify_final_report "$RAW_REPORT_FILE"; then
    report_validation_error "$RAW_REPORT_FILE"
    fail "planner final report did not match the required format"
  fi
}

run_validation() {
  : > "$VALIDATION_LOG"

  (
    cd "$REPO_ROOT"
    go run ./cmd/progress-gen -write
    go run ./cmd/progress-gen -validate
    go test ./internal/progress -count=1
    go test ./docs -count=1
  ) >>"$VALIDATION_LOG" 2>&1 || {
    cat "$VALIDATION_LOG" >&2
    fail "validation failed"
  }
}

install_systemd_timer() {
  local unit_dir service_file timer_file script_path
  script_path="$SCRIPT_DIR/gormes-architecture-planner-tasks-manager.sh"
  unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  service_file="$unit_dir/$PLANNER_TIMER_NAME.service"
  timer_file="$unit_dir/$PLANNER_TIMER_NAME.timer"

  mkdir -p "$unit_dir"

  cat > "$service_file" <<EOF
[Unit]
Description=Gormes architecture planner tasks manager

[Service]
Type=oneshot
WorkingDirectory=$REPO_ROOT
Environment=REPO_ROOT=$REPO_ROOT
Environment=UPSTREAM_HERMES_DIR=$UPSTREAM_HERMES_DIR
ExecStart=/usr/bin/env bash $script_path
EOF

  cat > "$timer_file" <<EOF
[Unit]
Description=Run the Gormes architecture planner tasks manager periodically

[Timer]
OnBootSec=$PLANNER_BOOT_DELAY
OnUnitActiveSec=$PLANNER_INTERVAL
Persistent=true
Unit=$PLANNER_TIMER_NAME.service

[Install]
WantedBy=timers.target
EOF

  systemctl --user daemon-reload
  systemctl --user enable --now "$PLANNER_TIMER_NAME.timer"
  SCHEDULE_METHOD="systemd"
}

install_crontab_entry() {
  local script_path cron_expr existing cron_file
  script_path="$SCRIPT_DIR/gormes-architecture-planner-tasks-manager.sh"
  cron_expr="0 */4 * * * cd $REPO_ROOT && /usr/bin/env bash $script_path >> $LOG_DIR/cron.log 2>&1"
  cron_file="$(mktemp)"

  if crontab -l >/dev/null 2>&1; then
    crontab -l > "$cron_file"
  else
    : > "$cron_file"
  fi

  if ! grep -Fq "$script_path" "$cron_file"; then
    printf '%s\n' "$cron_expr" >> "$cron_file"
    crontab "$cron_file"
  fi
  rm -f "$cron_file"
  SCHEDULE_METHOD="cron"
}

install_periodic_schedule() {
  if command -v systemctl >/dev/null 2>&1; then
    if install_systemd_timer 2>>"$CODEXU_STDERR"; then
      return 0
    fi
  fi

  if command -v crontab >/dev/null 2>&1; then
    if install_crontab_entry 2>>"$CODEXU_STDERR"; then
      return 0
    fi
  fi

  SCHEDULE_METHOD="none"
}

write_report() {
  cat > "$REPORT_FILE" <<EOF
# Architecture Planner Run

- Run UTC: $RUN_AT_UTC
- Gormes repo: $REPO_ROOT
- Upstream Hermes: $UPSTREAM_HERMES_DIR
- Local git root: $LOCAL_GIT_ROOT
- Context bundle: $CONTEXT_FILE
- Task Markdown: $TASKS_MD_FILE
- Prompt file: $PROMPT_FILE
- Validation log: $VALIDATION_LOG
- Periodic schedule: $SCHEDULE_METHOD

$(cat "$RAW_REPORT_FILE")
EOF
}

write_state_file() {
  local session_id item_count
  session_id="$(extract_session_id "$CODEXU_JSONL" || true)"
  item_count="$(jq -r '.progress_summary.item_count // 0' "$CONTEXT_FILE")"

  jq -n \
    --arg last_run_utc "$RUN_AT_UTC" \
    --arg gormes_repo_root "$REPO_ROOT" \
    --arg upstream_hermes_dir "$UPSTREAM_HERMES_DIR" \
    --arg upstream_commit "$UPSTREAM_COMMIT" \
    --arg upstream_branch "$UPSTREAM_BRANCH" \
    --arg local_git_root "$LOCAL_GIT_ROOT" \
    --arg local_commit "$LOCAL_COMMIT" \
    --arg local_branch "$LOCAL_BRANCH" \
    --arg report_path "$REPORT_FILE" \
    --arg raw_report_path "$RAW_REPORT_FILE" \
    --arg context_path "$CONTEXT_FILE" \
    --arg tasks_md_path "$TASKS_MD_FILE" \
    --arg prompt_path "$PROMPT_FILE" \
    --arg validation_log_path "$VALIDATION_LOG" \
    --arg codexu_jsonl_path "$CODEXU_JSONL" \
    --arg codexu_stderr_path "$CODEXU_STDERR" \
    --arg session_id "$session_id" \
    --arg schedule_method "$SCHEDULE_METHOD" \
    --arg item_count "$item_count" \
    --argjson architecture_plan_json_present "$ARCH_PLAN_JSON_PRESENT" \
    '{
      last_run_utc: $last_run_utc,
      gormes_repo_root: $gormes_repo_root,
      upstream_hermes_dir: $upstream_hermes_dir,
      upstream_commit: $upstream_commit,
      upstream_branch: $upstream_branch,
      local_git_root: $local_git_root,
      local_commit: $local_commit,
      local_branch: $local_branch,
      report_path: $report_path,
      raw_report_path: $raw_report_path,
      context_path: $context_path,
      tasks_md_path: $tasks_md_path,
      prompt_path: $prompt_path,
      validation_log_path: $validation_log_path,
      codexu_jsonl_path: $codexu_jsonl_path,
      codexu_stderr_path: $codexu_stderr_path,
      session_id: $session_id,
      schedule_method: $schedule_method,
      scanned_progress_item_count: ($item_count | tonumber),
      architecture_plan_json_present: $architecture_plan_json_present
    }' > "$STATE_FILE"
}

cmd_status() {
  require_cmd jq
  require_file "$STATE_FILE"

  printf 'Last run UTC: %s\n' "$(jq -r '.last_run_utc // "unknown"' "$STATE_FILE")"
  printf 'Upstream Hermes: %s\n' "$(jq -r '.upstream_hermes_dir // "unknown"' "$STATE_FILE")"
  printf 'Upstream branch: %s\n' "$(jq -r '.upstream_branch // "unknown"' "$STATE_FILE")"
  printf 'Upstream commit: %s\n' "$(jq -r '.upstream_commit // "unknown"' "$STATE_FILE")"
  printf 'Local git root: %s\n' "$(jq -r '.local_git_root // "unknown"' "$STATE_FILE")"
  printf 'Local branch: %s\n' "$(jq -r '.local_branch // "unknown"' "$STATE_FILE")"
  printf 'Local commit: %s\n' "$(jq -r '.local_commit // "unknown"' "$STATE_FILE")"
  printf 'Report: %s\n' "$(jq -r '.report_path // "unknown"' "$STATE_FILE")"
  printf 'Raw report: %s\n' "$(jq -r '.raw_report_path // "unknown"' "$STATE_FILE")"
  printf 'Context: %s\n' "$(jq -r '.context_path // "unknown"' "$STATE_FILE")"
  printf 'Tasks: %s\n' "$(jq -r '.tasks_md_path // "unknown"' "$STATE_FILE")"
  printf 'Validation log: %s\n' "$(jq -r '.validation_log_path // "unknown"' "$STATE_FILE")"
  printf 'Schedule: %s\n' "$(jq -r '.schedule_method // "unknown"' "$STATE_FILE")"
  printf 'Scanned items: %s\n' "$(jq -r '.scanned_progress_item_count // "unknown"' "$STATE_FILE")"
}

cmd_show_report() {
  require_file "$REPORT_FILE"
  cat "$REPORT_FILE"
}

cmd_doctor() {
  require_cmd jq
  require_cmd git
  require_cmd codexu
  require_cmd go
  require_dir "$REPO_ROOT"
  require_dir "$ARCH_PLAN_DIR"
  require_file "$PROGRESS_JSON"
  detect_upstream_hermes_dir >/dev/null
  log "doctor: ok"
}

cmd_install_schedule_only() {
  require_cmd git
  require_dir "$REPO_ROOT"
  UPSTREAM_HERMES_DIR="$(detect_upstream_hermes_dir)"
  install_periodic_schedule
  log "Periodic schedule: $SCHEDULE_METHOD"
}

cmd_run() {
  claim_lock "$@"

  local total_run_start
  total_run_start=$(date +%s)

  log_info "Starting architecture planner run"
  log_info "Configuration: VERBOSE=$VERBOSE, AUTO_COMMIT=$AUTO_COMMIT, AUTO_PUSH=$AUTO_PUSH"

  progress 1 "preflight"
  local step_start
  step_start=$(date +%s)
  log_info "Step 1/5: Preflight checks"
  require_cmd jq
  require_cmd git
  require_cmd codexu
  require_cmd go
  require_dir "$REPO_ROOT"
  require_dir "$ARCH_PLAN_DIR"
  require_file "$PROGRESS_JSON"
  [[ "$PLANNER_INSTALL_SCHEDULE" == "0" || "$PLANNER_INSTALL_SCHEDULE" == "1" ]] || fail "PLANNER_INSTALL_SCHEDULE must be 0 or 1"

  if [[ -f "$ARCH_PLAN_JSON" ]]; then
    ARCH_PLAN_JSON_PRESENT="true"
    log_info "Found architecture plan JSON"
  fi

  UPSTREAM_HERMES_DIR="$(detect_upstream_hermes_dir)"
  UPSTREAM_COMMIT="$(git_field "$UPSTREAM_HERMES_DIR" commit)"
  UPSTREAM_BRANCH="$(git_field "$UPSTREAM_HERMES_DIR" branch)"
  LOCAL_GIT_ROOT="$(git_field "$REPO_ROOT" root)"
  LOCAL_COMMIT="$(git_field "$REPO_ROOT" commit)"
  LOCAL_BRANCH="$(git_field "$REPO_ROOT" branch)"

  log_info "Upstream: $UPSTREAM_HERMES_DIR ($UPSTREAM_BRANCH @ $UPSTREAM_COMMIT)"
  log_info "Local: $LOCAL_GIT_ROOT ($LOCAL_BRANCH @ $LOCAL_COMMIT)"
  log_step_duration "preflight" "$step_start"

  progress 2 "context"
  step_start=$(date +%s)
  log_info "Step 2/5: Building context bundle"
  write_context_bundle
  smart_commit "context: bundle" "generated context bundle"
  write_tasks_markdown
  smart_commit "context: tasks" "updated tasks markdown"
  log_context_summary
  smart_commit "context: complete" "context generation complete"
  log_step_duration "context" "$step_start"

  progress 3 "planning"
  step_start=$(date +%s)
  log_info "Step 3/5: Running codexu planner"
  run_codexu_planner
  smart_commit "planner: codexu" "planner run complete"
  log_step_duration "planning" "$step_start"

  progress 4 "validation"
  step_start=$(date +%s)
  log_info "Step 4/5: Running validation"
  run_validation
  log "Validation log: $VALIDATION_LOG"
  log_info "Validation completed successfully"
  smart_commit "validation: passed" "validation passed"
  log_step_duration "validation" "$step_start"

  progress 5 "schedule"
  step_start=$(date +%s)
  log_info "Step 5/5: Installing schedule"
  if [[ "$PLANNER_INSTALL_SCHEDULE" == "1" ]]; then
    install_periodic_schedule
  else
    SCHEDULE_METHOD="disabled"
  fi
  log "Schedule method: $SCHEDULE_METHOD"
  write_report
  write_state_file
  smart_commit "planner: complete" "planner run completed"
  log_step_duration "schedule" "$step_start"

  local total_run_elapsed=$(( $(date +%s) - total_run_start ))

  # Final summary
  echo
  echo "═══════════════════════════════════════════════════════════════"
  echo "              PLANNER RUN COMPLETED SUCCESSFULLY"
  echo "═══════════════════════════════════════════════════════════════"
  echo "  Report:      $REPORT_FILE"
  echo "  Tasks:       $TASKS_MD_FILE"
  echo "  State:       $STATE_FILE"
  echo "  Schedule:    $SCHEDULE_METHOD"
  echo "  Total time:  $(format_duration $total_run_elapsed)"
  echo "  Auto-commit: $([[ "$AUTO_COMMIT" == "1" ]] && echo "${COLOR_GREEN}enabled ✓${COLOR_RESET}" || echo "disabled")"
  echo "  Auto-push:   $([[ "$AUTO_PUSH" == "1" ]] && echo "${COLOR_GREEN}enabled ✓${COLOR_RESET}" || echo "disabled")"
  echo "═══════════════════════════════════════════════════════════════"

  log_info "Planner run completed successfully in $(format_duration $total_run_elapsed)"
}

main() {
  local command="${1:-run}"
  case "$command" in
    ""|run)
      shift || true
      cmd_run "$@"
      ;;
    status)
      shift || true
      cmd_status "$@"
      ;;
    show-report)
      shift || true
      cmd_show_report "$@"
      ;;
    doctor)
      shift || true
      cmd_doctor "$@"
      ;;
    install-schedule)
      shift || true
      cmd_install_schedule_only "$@"
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      usage >&2
      fail "unknown command: $command"
      ;;
  esac
}

main "$@"
