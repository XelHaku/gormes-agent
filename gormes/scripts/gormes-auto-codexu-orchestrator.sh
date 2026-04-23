#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'
shopt -s inherit_errexit 2>/dev/null || true

ORCHESTRATOR_LIB_DIR="${ORCHESTRATOR_LIB_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/orchestrator/lib}"
# shellcheck source=/dev/null
for _lib in common candidates report claim worktree promote; do
  source "$ORCHESTRATOR_LIB_DIR/${_lib}.sh"
done
unset _lib

# Error handler with stack trace
err_trap() {
  local exit_code=$?
  local line=${BASH_LINENO[0]}
  local cmd="${BASH_COMMAND}"
  echo "ERROR at line $line: exit $exit_code: $cmd" >&2
  # Print function stack
  local i=0
  while caller "$i" >&2; do ((i++)); done
}
trap err_trap ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"
ORIGINAL_REPO_ROOT="$REPO_ROOT"
PROGRESS_JSON_REL="docs/content/building-gormes/architecture_plan/progress.json"
PROGRESS_JSON="$REPO_ROOT/$PROGRESS_JSON_REL"

MAX_AGENTS="${MAX_AGENTS:-4}"
MAX_AGENTS_HARD_CAP="${MAX_AGENTS_HARD_CAP:-8}"
MODE="${MODE:-safe}"
RUN_ROOT="${RUN_ROOT:-$REPO_ROOT/.codex/orchestrator}"
RUN_ID_SEED="${RUN_ID:-}"
WORKTREES_DIR_SEED="${WORKTREES_DIR:-}"
RUN_ID="${RUN_ID_SEED:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"
LOCKS_DIR="$RUN_ROOT/locks"
LOGS_DIR="$RUN_ROOT/logs"
HEARTBEAT_JSON_LOG="${HEARTBEAT_JSON_LOG:-$LOGS_DIR/heartbeat.$RUN_ID.jsonl}"
PROMPTS_DIR="$RUN_ROOT/prompts"
STATE_DIR="$RUN_ROOT/state"
WORKTREES_DIR="${WORKTREES_DIR_SEED:-$RUN_ROOT/worktrees/$RUN_ID}"
CANDIDATES_FILE="$STATE_DIR/candidates.$RUN_ID.json"
RUN_LOCK_DIR="$RUN_ROOT/run.lock"

LOCK_TTL_SECONDS="${LOCK_TTL_SECONDS:-21600}"
WORKER_TIMEOUT_SECONDS="${WORKER_TIMEOUT_SECONDS:-7200}"
WORKER_TIMEOUT_GRACE_SECONDS="${WORKER_TIMEOUT_GRACE_SECONDS:-30}"
FINAL_REPORT_GRACE_SECONDS="${FINAL_REPORT_GRACE_SECONDS:-3}"
KEEP_WORKTREES="${KEEP_WORKTREES:-1}"

# Host-safety guards to reduce freeze risk during parallel Codex execution.
MIN_AVAILABLE_MEM_MB="${MIN_AVAILABLE_MEM_MB:-8192}"
MIN_MEM_PER_WORKER_MB="${MIN_MEM_PER_WORKER_MB:-4096}"
MAX_EXISTING_CHROMIUM="${MAX_EXISTING_CHROMIUM:-2}"
FORCE_RUN_UNDER_PRESSURE="${FORCE_RUN_UNDER_PRESSURE:-0}"

EXTRA_CODEX_ARGS="${EXTRA_CODEX_ARGS:-}"
EXTRA_CODEX_ARGS_FILE="${EXTRA_CODEX_ARGS_FILE:-}"

HEARTBEAT_SECONDS="${HEARTBEAT_SECONDS:-20}"
LOOP_SLEEP_SECONDS="${LOOP_SLEEP_SECONDS:-30}"
ORCHESTRATOR_ONCE="${ORCHESTRATOR_ONCE:-0}"
AUTO_PROMOTE_SUCCESS="${AUTO_PROMOTE_SUCCESS:-1}"
ALLOW_SOFT_SUCCESS_NONZERO="${ALLOW_SOFT_SUCCESS_NONZERO:-1}"
INTEGRATION_BRANCH="${INTEGRATION_BRANCH:-codexu/autoloop}"
INTEGRATION_WORKTREE="${INTEGRATION_WORKTREE:-}"
MAX_RUN_WORKTREE_DIRS="${MAX_RUN_WORKTREE_DIRS:-4}"
ACTIVE_FIRST="${ACTIVE_FIRST:-1}"
RUNS_LEDGER="$STATE_DIR/runs.jsonl"

# Verbosity and commit mode settings
VERBOSE="${VERBOSE:-0}"
COMMIT_TO_MAIN="${COMMIT_TO_MAIN:-0}"
AUTO_PUSH="${AUTO_PUSH:-0}"
MAIN_BRANCH="${MAIN_BRANCH:-main}"
REMOTE_NAME="${REMOTE_NAME:-origin}"
PROGRESS_INTERVAL="${PROGRESS_INTERVAL:-10}"
PINNED_RUNS_FILE="$STATE_DIR/pinned-runs.txt"
RUN_PIDS_DIR="$STATE_DIR/pids/$RUN_ID"
RUN_WORKER_STATE_DIR="$STATE_DIR/workers/$RUN_ID"
RESUME_RUN_ID=""
COMMAND_MODE="run"

GIT_ROOT=""
REPO_SUBDIR=""
BASE_COMMIT=""

declare -a EXTRA_CODEX_CMD_ARGS=()

mkdir -p "$LOCKS_DIR" "$LOGS_DIR" "$PROMPTS_DIR" "$STATE_DIR" "$WORKTREES_DIR"
mkdir -p "$RUN_PIDS_DIR" "$RUN_WORKER_STATE_DIR"
[[ -f "$PINNED_RUNS_FILE" ]] || : > "$PINNED_RUNS_FILE"

usage() {
  cat <<EOF
Usage:
  $0                       # run orchestrator
  $0 --resume <run_id>     # resume unfinished workers from a prior run
  $0 status [run_id]       # show run/worker status
  $0 tail [run_id] [n]     # tail orchestrator logs (default n=80)
  $0 abort [run_id]        # terminate active worker pids for run
  $0 cleanup               # cleanup stale locks and enforce worktree cap
  $0 promote-commit <run_id> <worker_id> [target_branch]

Env:
  REPO_ROOT                  Default: $REPO_ROOT
  MAX_AGENTS                 Default: 4 (hard-capped by MAX_AGENTS_HARD_CAP)
  MAX_AGENTS_HARD_CAP        Default: 8
  MODE                       safe | unattended | full
  RUN_ROOT                   Default: $RUN_ROOT
  WORKTREES_DIR              Default: $WORKTREES_DIR
  WORKER_TIMEOUT_SECONDS     Default: $WORKER_TIMEOUT_SECONDS
  FINAL_REPORT_GRACE_SECONDS Default: $FINAL_REPORT_GRACE_SECONDS
  LOCK_TTL_SECONDS           Default: $LOCK_TTL_SECONDS
  KEEP_WORKTREES             Default: $KEEP_WORKTREES (1 keeps per-worker worktrees)
  EXTRA_CODEX_ARGS_FILE      One extra codexu arg per line
  MIN_AVAILABLE_MEM_MB       Minimum available RAM required to start
  MIN_MEM_PER_WORKER_MB      RAM budget per worker used for auto-throttling
  MAX_EXISTING_CHROMIUM      Abort if existing chromium/chrome process count exceeds this
  FORCE_RUN_UNDER_PRESSURE   Set to 1 to bypass safety aborts (not recommended)
  HEARTBEAT_SECONDS          Status heartbeat interval while workers run
  LOOP_SLEEP_SECONDS         Sleep between forever-loop cycles (default: 30)
  ORCHESTRATOR_ONCE          Set to 1 to run a single batch and exit
  AUTO_PROMOTE_SUCCESS       Set to 1 to promote successful workers before next cycle
  ALLOW_SOFT_SUCCESS_NONZERO Set to 1 to treat non-zero codex exits as success if report+commit pass validation
  INTEGRATION_BRANCH         Branch that accumulates promoted worker commits
  INTEGRATION_WORKTREE       Optional managed worktree for INTEGRATION_BRANCH
  MAX_RUN_WORKTREE_DIRS      Max kept run-level worktree dirs under worktrees/ (default: 4)
  ACTIVE_FIRST               1 sorts in_progress before planned when selecting tasks
  
  # New verbosity and commit options
  VERBOSE                    Set to 1 for detailed progress logging
  COMMIT_TO_MAIN             Set to 1 to commit directly to main branch (no worker branches)
  AUTO_PUSH                  Set to 1 to automatically push commits to remote
  MAIN_BRANCH                Target branch for direct commits (default: main)
  REMOTE_NAME                Git remote name for push (default: origin)
  PROGRESS_INTERVAL          Seconds between progress updates (default: 10)

Notes:
  - Default run mode loops forever. Use ORCHESTRATOR_ONCE=1 for previous one-shot behavior.
  - Successful worker commits are promoted onto INTEGRATION_BRANCH by default,
    and later loop cycles select tasks from that branch so work does not repeat.
  - 'safe' and 'unattended' are both fully automatic: approval_policy=never with
    workspace-write sandboxing.
  - 'full' is fully automatic with danger-full-access sandboxing.
  - EXTRA_CODEX_ARGS is intentionally unsupported; use EXTRA_CODEX_ARGS_FILE so
    argument boundaries stay unambiguous.
  - COMMIT_TO_MAIN=1 bypasses worker branches and commits directly to main.
    Use with caution - enables rapid iteration but loses isolation.

Examples:
  MAX_AGENTS=4 MODE=safe $0
  printf '%s\n' '-c' 'model_reasoning_effort="high"' > /tmp/codexu.args
  MAX_AGENTS=2 EXTRA_CODEX_ARGS_FILE=/tmp/codexu.args $0
  VERBOSE=1 COMMIT_TO_MAIN=1 AUTO_PUSH=1 $0  # Verbose, direct to main, auto-push
EOF
}

release_run_lock() {
  [[ -d "$RUN_LOCK_DIR" ]] && rmdir "$RUN_LOCK_DIR" 2>/dev/null || true
}

claim_run_lock() {
  if ! mkdir "$RUN_LOCK_DIR" 2>/dev/null; then
    echo "WARNING: stale lock found at $RUN_LOCK_DIR" >&2
    echo "Checking for stale processes..." >&2
    local stale_pids
    stale_pids=$(ps aux | grep "[gormes-auto-codexu-orchestrator] bash" | awk '{print $2}' | grep -v "^$$\$" | head -10)
    if [[ -n "$stale_pids" ]]; then
      echo "Auto-killing stale processes: $stale_pids" >&2
      echo "$stale_pids" | xargs -r kill -9 2>/dev/null || true
      sleep 1
    fi
    rmdir "$RUN_LOCK_DIR" 2>/dev/null || true
    if ! mkdir "$RUN_LOCK_DIR" 2>/dev/null; then
      echo "ERROR: could not acquire lock after cleanup" >&2
      exit 1
    fi
    echo "Stale processes killed, lock acquired" >&2
  fi
  trap release_run_lock EXIT
}

refresh_repo_paths() {
  PROGRESS_JSON="$REPO_ROOT/$PROGRESS_JSON_REL"
}

fresh_run_id() {
  local cycle="$1"
  local stamp
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"

  if [[ -n "$RUN_ID_SEED" ]]; then
    printf '%s-%03d\n' "$RUN_ID_SEED" "$cycle"
  else
    printf '%s-%s-%03d\n' "$stamp" "$$" "$cycle"
  fi
}

reset_run_scope() {
  local cycle="$1"

  RUN_ID="$(fresh_run_id "$cycle")"
  if [[ -n "$WORKTREES_DIR_SEED" ]]; then
    WORKTREES_DIR="${WORKTREES_DIR_SEED%/}/$RUN_ID"
  else
    WORKTREES_DIR="$RUN_ROOT/worktrees/$RUN_ID"
  fi
  CANDIDATES_FILE="$STATE_DIR/candidates.$RUN_ID.json"
  RUN_PIDS_DIR="$STATE_DIR/pids/$RUN_ID"
  RUN_WORKER_STATE_DIR="$STATE_DIR/workers/$RUN_ID"
}

run_worker_state_file() {
  local worker_id="$1"
  printf '%s/worker_%s.json' "$RUN_WORKER_STATE_DIR" "$worker_id"
}

log_event() {
  local event="$1"
  local worker_id="${2:-null}"
  local detail="${3:-}"
  local status="${4:-}"

  mkdir -p "$STATE_DIR"
  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg run_id "$RUN_ID" \
    --arg event "$event" \
    --arg worker_id "$worker_id" \
    --arg detail "$detail" \
    --arg status "$status" \
    '{
      ts: $ts,
      run_id: $run_id,
      event: $event,
      worker_id: (if $worker_id == "null" then null else ($worker_id|tonumber) end),
      detail: $detail,
      status: $status
    }' >> "$RUNS_LEDGER"
}

save_worker_state() {
  local worker_id="$1"
  local state_json="$2"
  local path
  path="$(run_worker_state_file "$worker_id")"
  mkdir -p "$RUN_WORKER_STATE_DIR"
  printf '%s\n' "$state_json" > "$path"
}

load_worker_state() {
  local worker_id="$1"
  local path
  path="$(run_worker_state_file "$worker_id")"
  [[ -f "$path" ]] || return 1
  cat "$path"
}

parse_cli_args() {
  local cmd="${1:-}"
  case "$cmd" in
    "" ) COMMAND_MODE="run" ;;
    --resume)
      [[ -n "${2:-}" ]] || { echo "ERROR: --resume requires run_id" >&2; exit 1; }
      RESUME_RUN_ID="$2"
      RUN_ID="$RESUME_RUN_ID"
      WORKTREES_DIR="${RUN_ROOT}/worktrees/${RUN_ID}"
      CANDIDATES_FILE="$STATE_DIR/candidates.$RUN_ID.json"
      RUN_PIDS_DIR="$STATE_DIR/pids/$RUN_ID"
      RUN_WORKER_STATE_DIR="$STATE_DIR/workers/$RUN_ID"
      COMMAND_MODE="resume"
      shift 2 || true
      ;;
    status|tail|abort|cleanup|promote-commit)
      COMMAND_MODE="$cmd"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown command '$cmd'" >&2
      usage
      exit 1
      ;;
  esac
}

load_extra_args() {
  if [[ -n "$EXTRA_CODEX_ARGS" ]]; then
    echo "ERROR: EXTRA_CODEX_ARGS is unsafe; use EXTRA_CODEX_ARGS_FILE with one argument per line" >&2
    exit 1
  fi

  [[ -n "$EXTRA_CODEX_ARGS_FILE" ]] || return 0
  [[ -f "$EXTRA_CODEX_ARGS_FILE" ]] || {
    echo "ERROR: EXTRA_CODEX_ARGS_FILE not found: $EXTRA_CODEX_ARGS_FILE" >&2
    exit 1
  }

  local line=""
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" ]] && continue
    EXTRA_CODEX_CMD_ARGS+=("$line")
  done < "$EXTRA_CODEX_ARGS_FILE"
}

validate() {
  require_cmd jq
  require_cmd git
  require_cmd timeout
  require_cmd codexu
  require_cmd free

  [[ -d "$REPO_ROOT" ]] || { echo "ERROR: repo root not found: $REPO_ROOT" >&2; exit 1; }
  [[ -f "$PROGRESS_JSON" ]] || { echo "ERROR: progress file not found: $PROGRESS_JSON" >&2; exit 1; }

  GIT_ROOT="$(git -C "$REPO_ROOT" rev-parse --show-toplevel)"
  BASE_COMMIT="$(git -C "$GIT_ROOT" rev-parse HEAD)"
  REPO_SUBDIR="."
  if [[ "$REPO_ROOT" != "$GIT_ROOT" ]]; then
    REPO_SUBDIR="${REPO_ROOT#"$GIT_ROOT"/}"
  fi

  if ! [[ "$MAX_AGENTS" =~ ^[0-9]+$ ]]; then
    echo "ERROR: MAX_AGENTS must be an integer" >&2
    exit 1
  fi
  if ! [[ "$MAX_AGENTS_HARD_CAP" =~ ^[0-9]+$ ]] || (( MAX_AGENTS_HARD_CAP < 1 )); then
    echo "ERROR: MAX_AGENTS_HARD_CAP must be a positive integer" >&2
    exit 1
  fi
  if (( MAX_AGENTS < 1 )); then
    echo "ERROR: MAX_AGENTS must be >= 1" >&2
    exit 1
  fi
  if (( MAX_AGENTS > MAX_AGENTS_HARD_CAP )); then
    MAX_AGENTS="$MAX_AGENTS_HARD_CAP"
  fi
  if ! [[ "$WORKER_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] || (( WORKER_TIMEOUT_SECONDS < 1 )); then
    echo "ERROR: WORKER_TIMEOUT_SECONDS must be a positive integer" >&2
    exit 1
  fi
  if ! [[ "$FINAL_REPORT_GRACE_SECONDS" =~ ^[0-9]+$ ]]; then
    echo "ERROR: FINAL_REPORT_GRACE_SECONDS must be a non-negative integer" >&2
    exit 1
  fi
  if ! [[ "$LOCK_TTL_SECONDS" =~ ^[0-9]+$ ]] || (( LOCK_TTL_SECONDS < 1 )); then
    echo "ERROR: LOCK_TTL_SECONDS must be a positive integer" >&2
    exit 1
  fi
  if ! [[ "$MIN_AVAILABLE_MEM_MB" =~ ^[0-9]+$ ]] || (( MIN_AVAILABLE_MEM_MB < 1 )); then
    echo "ERROR: MIN_AVAILABLE_MEM_MB must be a positive integer" >&2
    exit 1
  fi
  if ! [[ "$MIN_MEM_PER_WORKER_MB" =~ ^[0-9]+$ ]] || (( MIN_MEM_PER_WORKER_MB < 1 )); then
    echo "ERROR: MIN_MEM_PER_WORKER_MB must be a positive integer" >&2
    exit 1
  fi
  if ! [[ "$MAX_EXISTING_CHROMIUM" =~ ^[0-9]+$ ]]; then
    echo "ERROR: MAX_EXISTING_CHROMIUM must be a non-negative integer" >&2
    exit 1
  fi
  if ! [[ "$HEARTBEAT_SECONDS" =~ ^[0-9]+$ ]] || (( HEARTBEAT_SECONDS < 1 )); then
    echo "ERROR: HEARTBEAT_SECONDS must be a positive integer" >&2
    exit 1
  fi
  if ! [[ "$LOOP_SLEEP_SECONDS" =~ ^[0-9]+$ ]] || (( LOOP_SLEEP_SECONDS < 1 )); then
    echo "ERROR: LOOP_SLEEP_SECONDS must be a positive integer" >&2
    exit 1
  fi
  if [[ "$ORCHESTRATOR_ONCE" != "0" && "$ORCHESTRATOR_ONCE" != "1" ]]; then
    echo "ERROR: ORCHESTRATOR_ONCE must be 0 or 1" >&2
    exit 1
  fi
  if [[ "$AUTO_PROMOTE_SUCCESS" != "0" && "$AUTO_PROMOTE_SUCCESS" != "1" ]]; then
    echo "ERROR: AUTO_PROMOTE_SUCCESS must be 0 or 1" >&2
    exit 1
  fi
  if [[ "$ALLOW_SOFT_SUCCESS_NONZERO" != "0" && "$ALLOW_SOFT_SUCCESS_NONZERO" != "1" ]]; then
    echo "ERROR: ALLOW_SOFT_SUCCESS_NONZERO must be 0 or 1" >&2
    exit 1
  fi
  if promotion_enabled && [[ -z "$INTEGRATION_BRANCH" ]]; then
    echo "ERROR: INTEGRATION_BRANCH must not be empty when AUTO_PROMOTE_SUCCESS=1" >&2
    exit 1
  fi
  if ! [[ "$MAX_RUN_WORKTREE_DIRS" =~ ^[0-9]+$ ]] || (( MAX_RUN_WORKTREE_DIRS < 1 )); then
    echo "ERROR: MAX_RUN_WORKTREE_DIRS must be a positive integer" >&2
    exit 1
  fi
}

preflight_resource_safety() {
  local avail chromium_count
  avail="$(available_mem_mb)"
  [[ "$avail" =~ ^[0-9]+$ ]] || {
    echo "ERROR: unable to parse available memory" >&2
    exit 1
  }

  chromium_count="$(ps -eo comm= | grep -Ec '^chromium$|^chrome$' || true)"
  [[ "$chromium_count" =~ ^[0-9]+$ ]] || chromium_count=0

  if (( avail < MIN_AVAILABLE_MEM_MB )) && [[ "$FORCE_RUN_UNDER_PRESSURE" != "1" ]]; then
    echo "ERROR: available memory ${avail}MB is below MIN_AVAILABLE_MEM_MB=${MIN_AVAILABLE_MEM_MB}MB" >&2
    echo "Set FORCE_RUN_UNDER_PRESSURE=1 to bypass (not recommended)." >&2
    exit 1
  fi

  if (( chromium_count > MAX_EXISTING_CHROMIUM )) && [[ "$FORCE_RUN_UNDER_PRESSURE" != "1" ]]; then
    echo "ERROR: detected ${chromium_count} chromium/chrome processes (> ${MAX_EXISTING_CHROMIUM})." >&2
    echo "Close browser-heavy workloads or set FORCE_RUN_UNDER_PRESSURE=1 to bypass." >&2
    exit 1
  fi
}

cap_workers_by_memory() {
  local requested="$1"
  local avail max_by_mem

  avail="$(available_mem_mb)"
  max_by_mem=$(( avail / MIN_MEM_PER_WORKER_MB ))
  (( max_by_mem < 1 )) && max_by_mem=1

  if (( requested > max_by_mem )); then
    echo "Safety throttle: reducing workers from ${requested} to ${max_by_mem} based on available memory ${avail}MB and MIN_MEM_PER_WORKER_MB=${MIN_MEM_PER_WORKER_MB}MB"
    requested="$max_by_mem"
  fi

  printf '%s\n' "$requested"
}

build_codex_cmd() {
  case "$MODE" in
    safe|unattended)
      printf '%s\0' codexu exec --json \
        -c approval_policy=never \
        --sandbox workspace-write
      ;;
    full)
      printf '%s\0' codexu exec --json \
        -c approval_policy=never \
        --sandbox danger-full-access
      ;;
    *)
      echo "ERROR: invalid MODE=$MODE" >&2
      exit 1
      ;;
  esac
}

# Commit directly to main branch (when COMMIT_TO_MAIN=1)
commit_to_main_directly() {
  local worktree_root="$1"
  local worker_id="$2"
  local commit_msg="${3:-Worker $worker_id auto-commit}"

  log_info "Committing directly to $MAIN_BRANCH from worker $worker_id"

  # Ensure we're on main and it's up to date
  git -C "$worktree_root" fetch "$REMOTE_NAME" "$MAIN_BRANCH" 2>/dev/null || true
  git -C "$worktree_root" checkout "$MAIN_BRANCH" 2>/dev/null || git -C "$worktree_root" checkout -b "$MAIN_BRANCH"

  # Pull latest changes (with merge)
  if ! git -C "$worktree_root" pull "$REMOTE_NAME" "$MAIN_BRANCH" --no-rebase 2>/dev/null; then
    log_warn "Could not pull latest $MAIN_BRANCH, continuing with local version"
  fi

  # Stage and commit all changes
  git -C "$worktree_root" add -A
  if git -C "$worktree_root" diff --cached --quiet; then
    log_info "No changes to commit for worker $worker_id"
    return 1
  fi

  # Create the commit
  if git -C "$worktree_root" commit -m "$commit_msg" -m "Auto-generated by gormes-auto-codexu-orchestrator"; then
    local commit_hash
    commit_hash=$(git -C "$worktree_root" rev-parse HEAD)
    log_info "Created commit $commit_hash on $MAIN_BRANCH"

    # Push if AUTO_PUSH is enabled
    if [[ "$AUTO_PUSH" == "1" ]]; then
      log_info "Pushing commit to $REMOTE_NAME/$MAIN_BRANCH"
      if git -C "$worktree_root" push "$REMOTE_NAME" "$MAIN_BRANCH"; then
        log_info "Successfully pushed to $REMOTE_NAME/$MAIN_BRANCH"
      else
        log_error "Failed to push to $REMOTE_NAME/$MAIN_BRANCH"
        return 1
      fi
    fi

    printf '%s\n' "$commit_hash"
    return 0
  fi

  return 1
}

try_soft_success_nonzero() {
  local worker_id="$1"
  local rc="$2"
  local final_file="$3"
  local stderr_file="$4"
  local jsonl_file="$5"

  [[ "$ALLOW_SOFT_SUCCESS_NONZERO" == "1" ]] || return 1
  [[ "$rc" != "124" && "$rc" != "137" ]] || return 1

  wait_for_valid_final_report "$worker_id" "$final_file" "$stderr_file" "$jsonl_file" || return 1
  verify_worker_commit "$worker_id" "$final_file" || return 1
  return 0
}

latest_worker_log_prefix() {
  local run_id="$1"
  local worker_id="$2"
  find "$LOGS_DIR" -type f -name "*__worker${worker_id}__*.meta.json" -printf '%f\n' \
    | grep -F "$run_id" \
    | sed 's/\.meta\.json$//' \
    | sort \
    | tail -n1
}

run_worker() {
  local worker_id="$1"
  local total idx pivots candidate phase_id subphase_id item_name slug trail=""
  local claim_dir=""

  # Signal handler for graceful shutdown with process group cleanup
  cleanup_worker() {
    local sig="${1:-TERM}"
    [[ -n "$claim_dir" ]] && release_task "$claim_dir"
    # Kill entire process group if we're the leader
    kill -"$sig" -$$ 2>/dev/null || true
    maybe_remove_worker_worktree "$worker_id"
  }

  trap 'cleanup_worker INT' INT
  trap 'cleanup_worker TERM' TERM
  trap 'cleanup_worker EXIT' EXIT

  total="$(candidate_count)"
  if (( total == 0 )); then
    echo "worker[$worker_id]: no unfinished tasks" | tee "$LOGS_DIR/worker_${worker_id}.status"
    save_worker_state "$worker_id" "$(jq -nc --arg status 'no_task' --arg run_id "$RUN_ID" '{run_id:$run_id,status:$status}')"
    log_event "worker_no_task" "$worker_id" "no unfinished tasks" "no_task"
    return 0
  fi

  idx=$((worker_id - 1))
  pivots=0

  while (( pivots < total )); do
    local normalized_idx=$(( idx % total ))
    candidate="$(candidate_at "$normalized_idx")"

    phase_id="$(jq -r '.phase_id' <<<"$candidate")"
    subphase_id="$(jq -r '.subphase_id' <<<"$candidate")"
    item_name="$(jq -r '.item_name' <<<"$candidate")"
    slug="$(task_slug "$phase_id" "$subphase_id" "$item_name")"

    [[ -n "$trail" ]] && trail+=", "
    trail+="$normalized_idx:$phase_id/$subphase_id/$item_name"

    log_info "Worker $worker_id: attempting to claim task $slug"

    if claim_dir="$(claim_task "$slug" "$worker_id")"; then
      local stamp run_base prompt_file meta_file jsonl_file stderr_file final_file
      local worktree_root worker_dir branch rc session_id head_commit original_rc soft_success
      stamp="$(date -u +%Y%m%dT%H%M%SZ)"
      run_base="$LOGS_DIR/${slug}__worker${worker_id}__${stamp}"
      prompt_file="$PROMPTS_DIR/${slug}__worker${worker_id}__${stamp}.prompt.txt"
      meta_file="$run_base.meta.json"
      jsonl_file="$run_base.jsonl"
      stderr_file="$run_base.stderr"
      final_file="$run_base.final.md"
      worktree_root="$(worker_worktree_root "$worker_id")"
      worker_dir="$(worker_repo_root "$worker_id")"
      branch="$(worker_branch_name "$worker_id")"

      create_worker_worktree "$worker_id"

      save_worker_state "$worker_id" "$(jq -nc \
        --arg run_id "$RUN_ID" \
        --arg status "claimed" \
        --arg phase_id "$phase_id" \
        --arg subphase_id "$subphase_id" \
        --arg item_name "$item_name" \
        --arg trail "$trail" \
        --arg slug "$slug" \
        '{run_id:$run_id,status:$status,phase_id:$phase_id,subphase_id:$subphase_id,item_name:$item_name,trail:$trail,slug:$slug}')"
      log_event "worker_claimed" "$worker_id" "$phase_id/$subphase_id/$item_name" "claimed"

      jq -n \
        --arg repo_root "$worker_dir" \
        --arg progress_json "$PROGRESS_JSON_REL" \
        --argjson selected_task "$candidate" \
        --arg trail "$trail" \
        --arg worker_id "$worker_id" \
        --arg worktree_root "$worktree_root" \
        --arg branch "$branch" \
        --arg base_commit "$BASE_COMMIT" \
        '{
          repo_root: $repo_root,
          progress_json: $progress_json,
          worker_id: ($worker_id | tonumber),
          selected_task: $selected_task,
          deterministic_index_trail: $trail,
          worktree_root: $worktree_root,
          branch: $branch,
          base_commit: $base_commit,
          started_at_utc: (now | todate)
        }' > "$meta_file"

      build_prompt "$worker_id" "$candidate" "$trail" "$prompt_file"

      local -a cmd=()
      while IFS= read -r -d '' part; do
        cmd+=("$part")
      done < <(build_codex_cmd)
      cmd+=("${EXTRA_CODEX_CMD_ARGS[@]}")

      echo "worker[$worker_id]: claimed $phase_id / $subphase_id / $item_name"
      echo "worker[$worker_id]: worktree -> $worktree_root"

      (
        # Forward signals to child process group for proper cleanup
        trap 'kill -TERM -$$' INT TERM
        cd "$worker_dir"
        exec </dev/null
        set +e
        timeout \
          --signal=TERM \
          --kill-after="${WORKER_TIMEOUT_GRACE_SECONDS}s" \
          "${WORKER_TIMEOUT_SECONDS}s" \
          "${cmd[@]}" \
          --output-last-message "$final_file" \
          "$(cat "$prompt_file")" \
          >"$jsonl_file" 2>"$stderr_file"
        rc=$?
        set -e
        echo "$rc" > "$run_base.exitcode"
      )

      rc="$(cat "$run_base.exitcode")"
      original_rc="$rc"
      soft_success=0

      log_info "Worker $worker_id: codexu exited with code $rc"

      if [[ "$rc" == "0" ]] && ! wait_for_valid_final_report "$worker_id" "$final_file" "$stderr_file" "$jsonl_file"; then
        rc=1
        echo "$rc" > "$run_base.exitcode"
      fi
      if [[ "$rc" == "0" ]] && ! verify_worker_commit "$worker_id" "$final_file"; then
        rc=1
        echo "$rc" > "$run_base.exitcode"
      fi

      if [[ "$rc" != "0" ]] && try_soft_success_nonzero "$worker_id" "$rc" "$final_file" "$stderr_file" "$jsonl_file"; then
        soft_success=1
        rc=0
        echo "$rc" > "$run_base.exitcode"
        echo "$original_rc" > "$run_base.original_exitcode"
      fi

      session_id="$(extract_session_id "$jsonl_file" || true)"
      [[ -n "$session_id" ]] && echo "$session_id" > "$run_base.session_id"
      head_commit="$(git -C "$worktree_root" rev-parse HEAD 2>/dev/null || true)"
      [[ -n "$head_commit" ]] && echo "$head_commit" > "$run_base.head"

      if [[ "$rc" == "0" ]]; then
        # Handle COMMIT_TO_MAIN mode - commit directly to main instead of keeping worker branch
        if [[ "$COMMIT_TO_MAIN" == "1" ]]; then
          log_info "Worker $worker_id: COMMIT_TO_MAIN enabled, committing directly to $MAIN_BRANCH"
          local main_commit
          if main_commit=$(commit_to_main_directly "$worktree_root" "$worker_id" "$slug"); then
            head_commit="$main_commit"
            echo "$head_commit" > "$run_base.head"
            log_info "Worker $worker_id: Successfully committed to $MAIN_BRANCH: $head_commit"
          else
            log_warn "Worker $worker_id: Failed to commit to $MAIN_BRANCH, keeping worker branch"
          fi
        fi

        if [[ "$soft_success" == "1" ]]; then
          echo "worker[$worker_id]: soft-success(nonzero=$original_rc) -> $slug ($head_commit)" | tee "$LOGS_DIR/worker_${worker_id}.status"
          save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'success' --arg slug "$slug" --arg commit "$head_commit" --arg original_rc "$original_rc" --arg mode 'soft_success_nonzero' '{run_id:$run_id,status:$status,slug:$slug,commit:$commit,original_rc:($original_rc|tonumber),mode:$mode}')"
          log_event "worker_success" "$worker_id" "$slug@$head_commit" "soft_success_nonzero"
        else
          echo "worker[$worker_id]: success -> $slug ($head_commit)" | tee "$LOGS_DIR/worker_${worker_id}.status"
          save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'success' --arg slug "$slug" --arg commit "$head_commit" '{run_id:$run_id,status:$status,slug:$slug,commit:$commit}')"
          log_event "worker_success" "$worker_id" "$slug@$head_commit" "success"
        fi
      elif [[ "$rc" == "124" ]]; then
        echo "worker[$worker_id]: timeout(${WORKER_TIMEOUT_SECONDS}s) -> $slug" | tee "$LOGS_DIR/worker_${worker_id}.status"
        save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'failed' --arg slug "$slug" --arg reason 'timeout' '{run_id:$run_id,status:$status,slug:$slug,reason:$reason}')"
        log_event "worker_failed" "$worker_id" "$slug" "timeout"
      else
        local reason
        reason="$(classify_worker_failure "$rc")"
        echo "worker[$worker_id]: failed($rc) -> $slug" | tee "$LOGS_DIR/worker_${worker_id}.status"
        save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'failed' --arg slug "$slug" --arg reason "$reason" --arg rc "$rc" '{run_id:$run_id,status:$status,slug:$slug,reason:$reason,rc:($rc|tonumber)}')"
        log_event "worker_failed" "$worker_id" "$slug" "$reason"
      fi

      return "$rc"
    fi

    idx=$((idx + 4))
    pivots=$((pivots + 1))
  done

  echo "worker[$worker_id]: no claimable task in +4 lane" | tee "$LOGS_DIR/worker_${worker_id}.status"
  save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'no_claim' '{run_id:$run_id,status:$status}')"
  log_event "worker_no_claim" "$worker_id" "no claimable task" "no_claim"
  return 0
}

run_worker_resume() {
  local worker_id="$1"
  local state_json phase_id subphase_id item_name status
  state_json="$(load_worker_state "$worker_id" 2>/dev/null || true)"
  [[ -n "$state_json" ]] || return 1

  status="$(jq -r '.status // ""' <<<"$state_json")"
  if [[ "$status" == "success" ]]; then
    echo "worker[$worker_id]: already successful in run $RUN_ID"
    return 0
  fi

  phase_id="$(jq -r '.phase_id // ""' <<<"$state_json")"
  subphase_id="$(jq -r '.subphase_id // ""' <<<"$state_json")"
  item_name="$(jq -r '.item_name // ""' <<<"$state_json")"
  if [[ -z "$phase_id" || -z "$subphase_id" || -z "$item_name" ]]; then
    return 1
  fi

  # resume falls back to normal selector; determinism restored by existing locks + active-first order.
  run_worker "$worker_id"
}

# Check if process is alive and not zombie using /proc
proc_alive() {
  local pid=$1
  [[ -d "/proc/$pid" ]] && ! grep -q 'Z)' "/proc/$pid/stat" 2>/dev/null
}

# Get memory pressure from PSI (Linux 4.20+)
get_memory_psi() {
  if [[ -f /proc/pressure/memory ]]; then
    awk '{print $4}' /proc/pressure/memory | cut -d= -f2 | head -n1
  fi
}

# Emit JSON heartbeat for machine parsing
emit_heartbeat_json() {
  local -n pids_ref=$1
  local run_id=$2
  local cycle=$3

  local workers_json="["
  local first=true
  local i pid state pid_num state_escaped

  for (( i=0; i<${#pids_ref[@]}; i++ )); do
    pid="${pids_ref[$i]}"
    if proc_alive "$pid"; then
      state="alive"
    elif [[ -f "$LOGS_DIR/worker_$((i+1)).status" ]]; then
      state="$(tr -d '\n' < "$LOGS_DIR/worker_$((i+1)).status")"
    else
      state="exited"
    fi

    if [[ "$pid" =~ ^[0-9]+$ ]]; then
      pid_num="$pid"
    else
      pid_num=0
    fi

    state_escaped="${state//\\/\\\\}"
    state_escaped="${state_escaped//\"/\\\"}"

    $first || workers_json+="," 
    first=false
    workers_json+="{\"worker\":$((i+1)),\"state\":\"$state_escaped\",\"pid\":$pid_num}"
  done
  workers_json+="]"

  local progress_complete="${5:-0}"
  local progress_total="${6:-0}"
  local progress_pct=0

  [[ "$progress_complete" =~ ^[0-9]+$ ]] || progress_complete=0
  [[ "$progress_total" =~ ^[0-9]+$ ]] || progress_total=0

  if (( progress_total > 0 )); then
    progress_pct=$((progress_complete * 100 / progress_total))
  fi

  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg run_id "$run_id" \
    --arg cycle "$cycle" \
    --arg alive "$4" \
    --arg workers "$workers_json" \
    --arg progress_complete "$progress_complete" \
    --arg progress_total "$progress_total" \
    --arg progress_pct "$progress_pct" \
    '{
      ts:$ts,
      run_id:$run_id,
      cycle:($cycle|tonumber? // 0),
      alive:($alive|tonumber? // 0),
      workers:($workers|fromjson? // []),
      progress:{
        complete:($progress_complete|tonumber? // 0),
        total:($progress_total|tonumber? // 0),
        percent:($progress_pct|tonumber? // 0)
      }
    }'
}

# Read progress from progress.json using a single resilient jq query
read_progress_summary() {
  local summary complete in_progress planned total

  if [[ -f "$PROGRESS_JSON" ]]; then
    summary="$(jq -r '
      try (
        [ .phases[]?.subphases[]? | (.items // [])[] | .status ] as $statuses
        | [
            ($statuses | map(select(. == "complete")) | length),
            ($statuses | map(select(. == "in_progress")) | length),
            ($statuses | map(select(. == "planned")) | length)
          ] as $counts
        | "\($counts[0]) \($counts[1]) \($counts[2]) \($counts[0] + $counts[1] + $counts[2])"
      ) catch "0 0 0 0"
    ' "$PROGRESS_JSON" 2>/dev/null || true)"

    read -r complete in_progress planned total <<< "${summary:-0 0 0 0}"

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

# Get current item a worker is working on
get_worker_task() {
  local worker_num=$1
  local worker_file="$STATE_DIR/workers/$RUN_ID/worker_${worker_num}.json"

  if [[ -f "$worker_file" ]]; then
    local item_name phase_id subphase_id
    item_name=$(jq -r '.item_name // ""' "$worker_file" 2>/dev/null)
    phase_id=$(jq -r '.phase_id // ""' "$worker_file" 2>/dev/null)
    subphase_id=$(jq -r '.subphase_id // ""' "$worker_file" 2>/dev/null)

    if [[ -n "$item_name" && "$item_name" != "null" ]]; then
      echo "${phase_id}/${subphase_id}: ${item_name}"
    else
      echo "idle"
    fi
  else
    echo "idle"
  fi
}

heartbeat_loop() {
  local -a pids=("$@")
  local cycle=${HEARTBEAT_CYCLE:-0}

  while true; do
    local alive=0
    local status_line=""
    local i pid

    for (( i=0; i<${#pids[@]}; i++ )); do
      pid="${pids[$i]}"
      if proc_alive "$pid"; then
        alive=$((alive + 1))
        local worker_task
        worker_task=$(get_worker_task $((i+1)))
        status_line+=" w$((i+1))=${worker_task}"
      else
        if [[ -f "$LOGS_DIR/worker_$((i+1)).status" ]]; then
          status_line+=" w$((i+1))=$(tr -d '\n' < "$LOGS_DIR/worker_$((i+1)).status")"
        else
          status_line+=" w$((i+1))=done"
        fi
      fi
    done

    if (( alive == 0 )); then
      return 0
    fi

    local progress_complete progress_in_progress progress_planned progress_total
    read -r progress_complete progress_in_progress progress_planned progress_total <<< "$(read_progress_summary)"

    [[ "$progress_complete" =~ ^[0-9]+$ ]] || progress_complete=0
    [[ "$progress_in_progress" =~ ^[0-9]+$ ]] || progress_in_progress=0
    [[ "$progress_planned" =~ ^[0-9]+$ ]] || progress_planned=0
    [[ "$progress_total" =~ ^[0-9]+$ ]] || progress_total=0

    local progress_pct=0
    if (( progress_total > 0 )); then
      progress_pct=$((progress_complete * 100 / progress_total))
    fi

    local timestamp
    timestamp=$(date -u +%H:%M:%S)

    printf '%s [progress] %3d%% (%d/%d) | alive=%d |%s\n' \
      "$timestamp" \
      "$progress_pct" \
      "$progress_complete" \
      "$progress_total" \
      "$alive" \
      "$status_line"

    if (( (cycle % 6) == 0 )) && [[ -f "$PROGRESS_JSON" ]]; then
      printf '           phases: %d complete | %d in-progress | %d planned\n' \
        "$progress_complete" \
        "$progress_in_progress" \
        "$progress_planned" >&2
    fi

    if [[ -n "${HEARTBEAT_JSON_LOG:-}" ]]; then
      emit_heartbeat_json pids "$RUN_ID" "$cycle" "$alive" "$progress_complete" "$progress_total" >> "$HEARTBEAT_JSON_LOG"
    fi

    sleep "$HEARTBEAT_SECONDS"
    cycle=$((cycle + 1))
  done
}

latest_ledger_run_id() {
  [[ -f "$RUNS_LEDGER" ]] || return 0
  jq -r '.run_id // empty' "$RUNS_LEDGER" 2>/dev/null | tail -n 1
}

resolve_target_run_id() {
  local requested_run="${1:-}"
  local latest_run=""

  if [[ -n "$requested_run" ]]; then
    printf '%s\n' "$requested_run"
    return 0
  fi

  latest_run="$(latest_ledger_run_id)"
  if [[ -n "$latest_run" ]]; then
    printf '%s\n' "$latest_run"
    return 0
  fi

  printf '%s\n' "$RUN_ID"
}

cmd_status() {
  local target_run=""
  target_run="$(resolve_target_run_id "${1:-}")"
  echo "Run: $target_run"
  if [[ -f "$RUNS_LEDGER" ]]; then
    jq -c --arg run_id "$target_run" 'select(.run_id == $run_id)' "$RUNS_LEDGER" | tail -n 20
  else
    echo "No ledger found at $RUNS_LEDGER"
  fi
}

cmd_tail() {
  local target_run=""
  target_run="$(resolve_target_run_id "${1:-}")"
  local n="${2:-80}"
  find "$LOGS_DIR" -type f -name "*${target_run}*" | sort | tail -n 1 | while read -r f; do
    echo "Tailing: $f"
    tail -n "$n" "$f"
  done
}

cmd_abort() {
  local target_run="${1:-$RUN_ID}"
  local pid_dir="$STATE_DIR/pids/$target_run"
  if [[ ! -d "$pid_dir" ]]; then
    echo "No pid dir for run $target_run"
    return 0
  fi
  local p
  for p in "$pid_dir"/*.pid; do
    [[ -f "$p" ]] || continue
    local pid
    pid="$(cat "$p")"
    if [[ "$pid" =~ ^[0-9]+$ ]]; then
      kill "$pid" 2>/dev/null || true
      echo "aborted pid $pid"
    fi
  done
}

cmd_cleanup() {
  cleanup_stale_locks
  enforce_worktree_dir_cap
  echo "cleanup complete"
}

run_once() {
  validate

  # Recreate run-scoped paths in case --resume changed RUN_ID.
  mkdir -p "$RUN_PIDS_DIR" "$RUN_WORKER_STATE_DIR" "$WORKTREES_DIR" "$STATE_DIR" "$LOGS_DIR" "$PROMPTS_DIR" "$LOCKS_DIR"

  preflight_resource_safety
  cleanup_stale_locks
  write_candidates_file
  enforce_worktree_dir_cap
  log_event "run_started" null "mode=$MODE workers=$MAX_AGENTS" "started"

  local total workers
  total="$(candidate_count)"
  if (( total == 0 )); then
    echo "No unfinished tasks in $PROGRESS_JSON_REL"
    log_event "run_completed" null "no unfinished tasks" "empty"
    return 0
  fi

  workers="$MAX_AGENTS"
  if (( total < workers )); then
    workers="$total"
  fi
  workers="$(cap_workers_by_memory "$workers")"

  echo "Repo:             $REPO_ROOT"
  echo "Git root:         $GIT_ROOT"
  echo "Base commit:      $BASE_COMMIT"
  echo "Run ID:           $RUN_ID"
  echo "Progress file:    $PROGRESS_JSON_REL"
  echo "Unfinished tasks: $total"
  echo "Launching workers: $workers"
  echo "Mode:             $MODE"
  echo "Safety floor:     min-available-mem=${MIN_AVAILABLE_MEM_MB}MB, per-worker=${MIN_MEM_PER_WORKER_MB}MB"
  echo "Verbose:          $VERBOSE"
  echo "Commit to main:   $COMMIT_TO_MAIN"
  echo "Auto push:        $AUTO_PUSH"
  [[ "$COMMIT_TO_MAIN" == "1" ]] && echo "Main branch:      $MAIN_BRANCH"
  [[ "$AUTO_PUSH" == "1" ]] && echo "Remote:           $REMOTE_NAME"
  echo

  log_info "Starting orchestration run"
  log_info "Total unfinished tasks: $total"
  log_info "Workers allocated: $workers"

  local pids=()
  local i
  for (( i = 1; i <= workers; i++ )); do
    if [[ "$COMMAND_MODE" == "resume" ]] && load_worker_state "$i" >/dev/null 2>&1; then
      run_worker_resume "$i" &
    else
      run_worker "$i" &
    fi
    pids+=("$!")
    echo "${pids[$((i-1))]}" > "$RUN_PIDS_DIR/worker_${i}.pid"
  done

  heartbeat_loop "${pids[@]}" &
  local heartbeat_pid=$!

  local rc=0
  local remaining=()
  local pid

  # Validate PIDs before waiting
  for pid in "${pids[@]}"; do
    if ! proc_alive "$pid"; then
      echo "Warning: Worker PID $pid is not alive at startup" >&2
    fi
    remaining+=("$pid")
  done

  # Wait for workers using wait -n for faster reaping (Bash 4.3+)
  while (( ${#remaining[@]} > 0 )); do
    local new_remaining=()
    local found_done=false

    for pid in "${remaining[@]}"; do
      if ! proc_alive "$pid"; then
        # Process has exited - reap it
        if ! wait "$pid"; then
          rc=1
        fi
        found_done=true
      else
        new_remaining+=("$pid")
      fi
    done

    remaining=("${new_remaining[@]}")

    if ! $found_done && (( ${#remaining[@]} > 0 )); then
      # No process exited this iteration - sleep briefly
      sleep 0.5
    fi
  done

  kill "$heartbeat_pid" 2>/dev/null || true
  wait "$heartbeat_pid" 2>/dev/null || true

  echo
  echo "Worker summary:"
  local success_count=0 failed_count=0 timeout_count=0 other_count=0
  for (( i = 1; i <= workers; i++ )); do
    if [[ -f "$LOGS_DIR/worker_${i}.status" ]]; then
      local status_line
      status_line=$(cat "$LOGS_DIR/worker_${i}.status")
      echo "$status_line"

      # Count outcomes. Use $((...)) assignment instead of ((var++)) because
      # the post-increment form returns the old value, which is a "failure"
      # exit (0) under set -e when the counter is still zero.
      if echo "$status_line" | grep -q "success"; then
        success_count=$((success_count + 1))
      elif echo "$status_line" | grep -q "timeout"; then
        timeout_count=$((timeout_count + 1))
      elif echo "$status_line" | grep -q "failed"; then
        failed_count=$((failed_count + 1))
      else
        other_count=$((other_count + 1))
      fi
    fi
  done

  log_info "Worker outcomes - Success: $success_count, Failed: $failed_count, Timeout: $timeout_count, Other: $other_count"

  if ! promote_successful_workers "$workers"; then
    rc=1
  fi

  echo
  echo "═══════════════════════════════════════════════════════════════"
  echo "                    RUN SUMMARY REPORT"
  echo "═══════════════════════════════════════════════════════════════"
  echo "  Run ID:           $RUN_ID"
  echo "  Workers:          $workers"
  echo "  Successful:       $success_count"
  echo "  Failed:           $failed_count"
  echo "  Timeout:          $timeout_count"
  echo "  Other:            $other_count"
  echo "  Overall status:   $([[ "$rc" == "0" ]] && echo "SUCCESS ✓" || echo "FAILURE ✗")"
  echo
  echo "  Artifacts:"
  echo "    Logs:      $LOGS_DIR"
  echo "    Prompts:   $PROMPTS_DIR"
  echo "    Locks:     $LOCKS_DIR"
  echo "    State:     $STATE_DIR"
  echo "    Worktrees: $WORKTREES_DIR"
  if promotion_enabled; then
    echo "    Integration branch: $INTEGRATION_BRANCH"
    echo "    Integration tree:   $GIT_ROOT"
  fi
  if [[ "$COMMIT_TO_MAIN" == "1" ]]; then
    echo "    Main branch commits: enabled"
    [[ "$AUTO_PUSH" == "1" ]] && echo "    Auto-push: enabled to $REMOTE_NAME/$MAIN_BRANCH"
  fi
  echo "═══════════════════════════════════════════════════════════════"

  if [[ "$rc" == "0" ]]; then
    log_info "Run completed successfully"
    log_event "run_completed" null "workers=${workers} success=${success_count} failed=${failed_count}" "success"
  else
    log_error "Run completed with failures"
    log_event "run_completed" null "workers=${workers} success=${success_count} failed=${failed_count}" "failure"
  fi

  enforce_worktree_dir_cap
  return "$rc"
}

main() {
  parse_cli_args "${1:-}" "${2:-}"

  if [[ "$COMMAND_MODE" == "status" ]]; then
    cmd_status "${2:-}"
    return 0
  elif [[ "$COMMAND_MODE" == "tail" ]]; then
    cmd_tail "${2:-}" "${3:-80}"
    return 0
  elif [[ "$COMMAND_MODE" == "abort" ]]; then
    cmd_abort "${2:-}"
    return 0
  elif [[ "$COMMAND_MODE" == "cleanup" ]]; then
    validate
    cmd_cleanup
    return 0
  elif [[ "$COMMAND_MODE" == "promote-commit" ]]; then
    [[ -n "${2:-}" && -n "${3:-}" ]] || { echo "Usage: $0 promote-commit <run_id> <worker_id> [target_branch]" >&2; return 1; }
    validate
    cmd_promote_commit "$2" "$3" "${4:-}"
    return 0
  fi

  claim_run_lock
  load_extra_args
  setup_integration_root

  if [[ "$ORCHESTRATOR_ONCE" == "1" || "$COMMAND_MODE" == "resume" ]]; then
    run_once
    return "$?"
  fi

  local cycle=0
  local cycle_rc=0
  echo "Forever loop enabled. Set ORCHESTRATOR_ONCE=1 to run a single batch."
  if promotion_enabled; then
    echo "Auto-promotion enabled: successful workers advance $INTEGRATION_BRANCH."
    echo "Coordinator repo: $REPO_ROOT"
  fi

  while true; do
    cycle=$((cycle + 1))
    reset_run_scope "$cycle"
    echo
    echo "Loop cycle:       $cycle"
    echo "Loop run ID:      $RUN_ID"

    if run_once; then
      cycle_rc=0
    else
      cycle_rc="$?"
    fi

    echo
    echo "Loop cycle $cycle completed with exit $cycle_rc; sleeping ${LOOP_SLEEP_SECONDS}s before next run."
    sleep "$LOOP_SLEEP_SECONDS"
  done
}

main "$@"
