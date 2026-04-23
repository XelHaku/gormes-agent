#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'
shopt -s inherit_errexit 2>/dev/null || true

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

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: missing required command: $1" >&2
    exit 1
  }
}

promotion_enabled() {
  [[ "$AUTO_PROMOTE_SUCCESS" == "1" ]]
}

safe_path_token() {
  printf '%s' "$1" | sed -E 's#[^A-Za-z0-9._-]+#-#g; s#^-+##; s#-+$##'
}

refresh_repo_paths() {
  PROGRESS_JSON="$REPO_ROOT/$PROGRESS_JSON_REL"
}

branch_worktree_path() {
  local git_root="$1"
  local branch="$2"

  git -C "$git_root" worktree list --porcelain \
    | awk -v branch_ref="refs/heads/${branch}" '
        /^worktree / { path = substr($0, 10) }
        /^branch / {
          if (!found && substr($0, 8) == branch_ref) {
            print path
            found = 1
          }
        }
      '
}

setup_integration_root() {
  promotion_enabled || return 0

  require_cmd git

  local source_git_root source_subdir safe_branch existing_worktree
  source_git_root="$(git -C "$ORIGINAL_REPO_ROOT" rev-parse --show-toplevel)"
  source_subdir="."
  if [[ "$ORIGINAL_REPO_ROOT" != "$source_git_root" ]]; then
    source_subdir="${ORIGINAL_REPO_ROOT#"$source_git_root"/}"
  fi

  if ! git -C "$source_git_root" show-ref --verify --quiet "refs/heads/$INTEGRATION_BRANCH"; then
    git -C "$source_git_root" branch "$INTEGRATION_BRANCH" HEAD
  fi

  existing_worktree="$(branch_worktree_path "$source_git_root" "$INTEGRATION_BRANCH")"
  if [[ -n "$existing_worktree" ]]; then
    INTEGRATION_WORKTREE="$existing_worktree"
  else
    if [[ -z "$INTEGRATION_WORKTREE" ]]; then
      safe_branch="$(safe_path_token "$INTEGRATION_BRANCH")"
      INTEGRATION_WORKTREE="$RUN_ROOT/integration/$safe_branch"
    fi
    mkdir -p "$(dirname "$INTEGRATION_WORKTREE")"
    git -C "$source_git_root" worktree add "$INTEGRATION_WORKTREE" "$INTEGRATION_BRANCH" >/dev/null
  fi

  if [[ -n "$(git -C "$INTEGRATION_WORKTREE" status --short)" ]]; then
    echo "ERROR: integration worktree is dirty: $INTEGRATION_WORKTREE" >&2
    echo "Resolve or remove it before running the forever orchestrator." >&2
    exit 1
  fi

  git -C "$INTEGRATION_WORKTREE" reset --hard "$INTEGRATION_BRANCH" >/dev/null

  if [[ "$source_subdir" == "." ]]; then
    REPO_ROOT="$INTEGRATION_WORKTREE"
  else
    REPO_ROOT="$INTEGRATION_WORKTREE/$source_subdir"
  fi
  refresh_repo_paths
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

available_mem_mb() {
  free -m | awk '/^Mem:/ { print $7 }'
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

normalize_candidates() {
  jq -c --arg active_first "$ACTIVE_FIRST" '
    def status_rank(s):
      if ($active_first == "1") then
        if (s == "in_progress") then 0
        elif (s == "planned") then 1
        else 2 end
      else 0 end;

    [
      (.phases // {})
      | to_entries[]
      | .key as $phase_id
      | (.value.subphases // .value.sub_phases // {})
      | to_entries[]
      | .key as $subphase_id
      | (.value.items // [])[]
      | {
          phase_id: $phase_id,
          subphase_id: $subphase_id,
          item_name: (.item_name // .name // .title // .id),
          status: ((.status // "unknown") | tostring | ascii_downcase)
        }
      | select(.item_name != null and .item_name != "")
      | select(.status != "complete")
      | . + {status_rank: status_rank(.status)}
    ]
    | unique_by([.phase_id, .subphase_id, .item_name])
    | sort_by([.status_rank, .phase_id, .subphase_id, .item_name])
    | map(del(.status_rank))
  ' "$PROGRESS_JSON"
}

write_candidates_file() {
  normalize_candidates > "$CANDIDATES_FILE"
}

candidate_count() {
  jq 'length' "$CANDIDATES_FILE"
}

candidate_at() {
  local idx="$1"
  jq -c ".[$idx]" "$CANDIDATES_FILE"
}

task_slug() {
  local phase_id="$1"
  local subphase_id="$2"
  local item_name="$3"

  printf '%s__%s__%s' "$phase_id" "$subphase_id" "$item_name" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//; s/--+/-/g'
}

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

worker_branch_name() {
  local worker_id="$1"
  printf 'codexu/%s/worker%d' "$RUN_ID" "$worker_id"
}

worker_worktree_root() {
  local worker_id="$1"
  printf '%s/worker%d' "$WORKTREES_DIR" "$worker_id"
}

worker_repo_root() {
  local worker_id="$1"
  local worktree_root
  worktree_root="$(worker_worktree_root "$worker_id")"
  if [[ "$REPO_SUBDIR" == "." ]]; then
    printf '%s\n' "$worktree_root"
  else
    printf '%s/%s\n' "$worktree_root" "$REPO_SUBDIR"
  fi
}

create_worker_worktree() {
  local worker_id="$1"
  local worktree_root branch
  worktree_root="$(worker_worktree_root "$worker_id")"
  branch="$(worker_branch_name "$worker_id")"

  mkdir -p "$(dirname "$worktree_root")"
  git -C "$GIT_ROOT" worktree add -b "$branch" "$worktree_root" "$BASE_COMMIT" >/dev/null 2>&1
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

# Push integration branch to remote
push_integration_branch() {
  if [[ "$AUTO_PUSH" != "1" ]]; then
    return 0
  fi

  log_info "Pushing $INTEGRATION_BRANCH to $REMOTE_NAME"

  if ! git -C "$GIT_ROOT" push "$REMOTE_NAME" "$INTEGRATION_BRANCH"; then
    log_error "Failed to push $INTEGRATION_BRANCH to $REMOTE_NAME"
    return 1
  fi

  log_info "Successfully pushed $INTEGRATION_BRANCH"
  return 0
}

maybe_remove_worker_worktree() {
  local worker_id="$1"
  local worktree_root
  worktree_root="$(worker_worktree_root "$worker_id")"

  if [[ "$KEEP_WORKTREES" == "0" && -d "$worktree_root" ]]; then
    git -C "$GIT_ROOT" worktree remove --force "$worktree_root" >/dev/null 2>&1 || true
  fi
}

enforce_worktree_dir_cap() {
  local keep="$MAX_RUN_WORKTREE_DIRS"
  local dirs=()
  local d

  while IFS= read -r d; do
    [[ -n "$d" ]] && dirs+=("$d")
  done < <(find "$RUN_ROOT/worktrees" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\n' 2>/dev/null | sort -nr | awk '{print $2}')

  local idx=0
  for d in "${dirs[@]}"; do
    idx=$((idx + 1))
    if (( idx <= keep )); then
      continue
    fi
    if [[ "$(basename "$d")" == "$RUN_ID" ]]; then
      continue
    fi
    if grep -Fxq "$(basename "$d")" "$PINNED_RUNS_FILE" 2>/dev/null; then
      continue
    fi
    git -C "$GIT_ROOT" worktree remove --force "$d" >/dev/null 2>&1 || true
    rm -rf "$d" 2>/dev/null || true
  done
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

build_prompt() {
  local worker_id="$1"
  local selected_json="$2"
  local trail="$3"
  local prompt_file="$4"

  local phase_id subphase_id item_name status branch worker_dir
  phase_id="$(jq -r '.phase_id' <<<"$selected_json")"
  subphase_id="$(jq -r '.subphase_id' <<<"$selected_json")"
  item_name="$(jq -r '.item_name' <<<"$selected_json")"
  status="$(jq -r '.status' <<<"$selected_json")"
  branch="$(worker_branch_name "$worker_id")"
  worker_dir="$(worker_repo_root "$worker_id")"

  cat > "$prompt_file" <<EOF
Repository root:
  $worker_dir

Mission:
Pick exactly ONE unfinished phase task and complete it with strict Test-Driven Development (TDD), while documenting progress and related docs before and after implementation.

Coordinator-selected worker lane:
- WORKER_ID: $worker_id
- Deterministic index trail: $trail

Selected task:
- Phase/Subphase/Item: $phase_id / $subphase_id / $item_name
- Current status: $status

Isolated execution context:
- Git worktree: $worker_dir
- Git branch: $branch
- Base commit: $BASE_COMMIT

Selection contract:
- This task was preselected by the coordinator from:
  $PROGRESS_JSON_REL
- Do not switch to another task in this run.
- Keep all changes inside the current repository root.
- Create exactly one new commit on $branch.
- Do not amend, squash, or rewrite history.
- Leave the worktree clean after your commit.
- If blocked/conflict/not-viable, provide the exact command + exact error and stop without creating extra commits.

==================================================
1) DOCUMENTATION GATE (MANDATORY: BEFORE + AFTER)
==================================================
Before writing tests:
- Record current status of selected item in progress.json
- Identify all related docs/progress files likely impacted
- Add a short implementation intent note in your report

After implementation/tests:
- Update progress/docs that reflect completed behavior
- Regenerate/validate progress artifacts when relevant
- Include exact file paths + summaries of doc/progress edits
- Do not claim completion if docs/progress are stale

==================================================
2) TDD PROTOCOL (MANDATORY)
==================================================
A) RED
- Write failing test(s) first.
- Run targeted tests and capture the failing command, exit code, and a short failure snippet.

B) GREEN
- Implement minimal code to satisfy tests.
- Re-run the targeted test command and capture the passing command, exit code, and a short success snippet.

C) REFACTOR
- Improve structure/naming without changing behavior.
- Re-run targeted tests and capture the command, exit code, and a short success snippet.

D) REGRESSION
- Run broader tests for touched packages.
- If progress/docs changed, also run:
  - go run ./cmd/progress-gen -write
  - go run ./cmd/progress-gen -validate
  - related progress assertions/tests
- Capture the regression command, exit code, and a short success snippet.

==================================================
3) REQUIRED FINAL REPORT FORMAT
==================================================
1) Selected task
Task: <phase / subphase / item>

2) Pre-doc baseline
Files:
- <path>

3) RED proof
Command: <exact command>
Exit: <non-zero integer>
Snippet: <short failure output>

4) GREEN proof
Command: <exact command>
Exit: 0
Snippet: <short passing output>

5) REFACTOR proof
Command: <exact command>
Exit: 0
Snippet: <short passing output>

6) Regression proof
Command: <exact command>
Exit: 0
Snippet: <short passing output>

7) Post-doc closeout
Files:
- <path>

8) Commit
Branch: $branch
Commit: <git hash>
Files:
- <path>
EOF
}

collect_final_report_issues() {
  local final_file="$1"
  local missing=0

  [[ -f "$final_file" ]] || {
    echo "Missing final report: $final_file"
    return 1
  }

  local -a section_titles=(
    "Selected task"
    "Pre-doc baseline"
    "RED proof"
    "GREEN proof"
    "REFACTOR proof"
    "Regression proof"
    "Post-doc closeout"
    "Commit"
  )
  local i section_title section_pattern
  for (( i = 0; i < ${#section_titles[@]}; i++ )); do
    section_title="${section_titles[$i]}"
    section_pattern="^[[:space:]]*$((i + 1))[).][[:space:]]*${section_title}[[:space:]]*$"
    if ! grep -Eq "$section_pattern" "$final_file"; then
      echo "Missing section '$((i + 1))) ${section_title}' in $final_file"
      missing=1
    fi
  done

  local command_count exit_count zero_count
  command_count="$(grep -Ec '^[[:space:]]*Command: .+' "$final_file" || true)"
  exit_count="$(grep -Ec '^[[:space:]]*Exit: ' "$final_file" || true)"
  zero_count="$(grep -Ec '^[[:space:]]*Exit:[[:space:]]*`?0`?[[:space:]]*$' "$final_file" || true)"

  if (( command_count < 4 )); then
    echo "Final report missing command evidence in $final_file"
    missing=1
  fi
  if (( exit_count < 4 )); then
    echo "Final report missing exit-code evidence in $final_file"
    missing=1
  fi
  if ! grep -Eq '^[[:space:]]*Exit:[[:space:]]*`?[1-9][0-9]*`?[[:space:]]*$' "$final_file"; then
    echo "Final report missing non-zero RED exit code in $final_file"
    missing=1
  fi
  if (( zero_count < 3 )); then
    echo "Final report missing GREEN/REFACTOR/REGRESSION zero exits in $final_file"
    missing=1
  fi
  if ! grep -Eq '^[[:space:]]*Branch:[[:space:]]*`?.+`?[[:space:]]*$' "$final_file"; then
    echo "Final report missing Branch field in $final_file"
    missing=1
  fi
  if ! grep -Eq '^[[:space:]]*Commit:[[:space:]]*`?[0-9a-f]{7,40}`?[[:space:]]*$' "$final_file"; then
    echo "Final report missing Commit hash in $final_file"
    missing=1
  fi

  return "$missing"
}

verify_final_report() {
  local final_file="$1"
  local issues=""

  issues="$(collect_final_report_issues "$final_file")"
  if [[ $? -eq 0 ]]; then
    return 0
  fi

  [[ -n "$issues" ]] && printf '%s\n' "$issues" >&2
  return 1
}

print_final_report_diagnostics() {
  local worker_id="$1"
  local final_file="$2"
  local stderr_file="$3"
  local jsonl_file="$4"

  echo "worker[$worker_id]: final report validation failed after ${FINAL_REPORT_GRACE_SECONDS}s" >&2
  echo "worker[$worker_id]: artifacts final=$final_file stderr=$stderr_file jsonl=$jsonl_file" >&2

  if [[ -f "$final_file" ]]; then
    echo "worker[$worker_id]: tail(final report)" >&2
    tail -n 20 "$final_file" >&2 || true
  else
    echo "worker[$worker_id]: final report file not found" >&2
  fi

  if [[ -s "$stderr_file" ]]; then
    echo "worker[$worker_id]: tail(stderr)" >&2
    tail -n 20 "$stderr_file" >&2 || true
  fi

  if [[ -s "$jsonl_file" ]]; then
    echo "worker[$worker_id]: tail(jsonl)" >&2
    tail -n 20 "$jsonl_file" >&2 || true
  fi
}

wait_for_valid_final_report() {
  local worker_id="$1"
  local final_file="$2"
  local stderr_file="$3"
  local jsonl_file="$4"
  local attempts attempt issues rc

  attempts=$(( FINAL_REPORT_GRACE_SECONDS * 10 + 1 ))
  (( attempts < 1 )) && attempts=1

  for (( attempt = 1; attempt <= attempts; attempt++ )); do
    issues="$(collect_final_report_issues "$final_file")"
    rc=$?
    if [[ "$rc" == "0" ]]; then
      return 0
    fi
    if (( attempt < attempts )); then
      sleep 0.1
    fi
  done

  [[ -n "$issues" ]] && printf '%s\n' "$issues" >&2
  print_final_report_diagnostics "$worker_id" "$final_file" "$stderr_file" "$jsonl_file"
  return 1
}

extract_report_field() {
  local label="$1"
  local final_file="$2"
  local value

  value="$(
    awk -v label="$label" '
      $0 ~ "^[[:space:]]*" label ":" {
        sub("^[[:space:]]*" label ":[[:space:]]*", "", $0)
        sub("[[:space:]]*$", "", $0)
        print
        exit
      }
    ' "$final_file"
  )"
  value="${value#\`}"
  value="${value%\`}"
  printf '%s\n' "$value"
}

extract_report_commit() {
  local final_file="$1"
  extract_report_field "Commit" "$final_file"
}

extract_report_branch() {
  local final_file="$1"
  extract_report_field "Branch" "$final_file"
}

extract_session_id() {
  local jsonl_file="$1"
  [[ -f "$jsonl_file" ]] || return 0
  jq -r 'select(.type=="thread.started") | (.thread_id // .session_id // empty)' "$jsonl_file" | head -n1
}

verify_worker_commit() {
  local worker_id="$1"
  local final_file="$2"
  local worktree_root branch head_commit report_commit report_branch commit_count status_output changed_files file

  worktree_root="$(worker_worktree_root "$worker_id")"
  branch="$(worker_branch_name "$worker_id")"
  head_commit="$(git -C "$worktree_root" rev-parse HEAD)"
  if [[ "$head_commit" == "$BASE_COMMIT" ]]; then
    echo "worker[$worker_id]: HEAD did not advance beyond $BASE_COMMIT" >&2
    return 1
  fi

  commit_count="$(git -C "$worktree_root" rev-list --count "${BASE_COMMIT}..HEAD")"
  if [[ "$commit_count" != "1" ]]; then
    echo "worker[$worker_id]: commit count = $commit_count, want exactly 1" >&2
    return 1
  fi

  status_output="$(git -C "$worktree_root" status --short)"
  if [[ -n "$status_output" ]]; then
    echo "worker[$worker_id]: worktree not clean after run" >&2
    printf '%s\n' "$status_output" >&2
    return 1
  fi

  report_commit="$(extract_report_commit "$final_file")"
  if [[ -z "$report_commit" || "$head_commit" != "$report_commit"* ]]; then
    echo "worker[$worker_id]: report commit does not match HEAD ($report_commit vs $head_commit)" >&2
    return 1
  fi

  report_branch="$(extract_report_branch "$final_file")"
  if [[ "$report_branch" != "$branch" ]]; then
    echo "worker[$worker_id]: report branch does not match expected branch ($report_branch vs $branch)" >&2
    return 1
  fi

  changed_files="$(git -C "$worktree_root" diff --name-only "${BASE_COMMIT}..HEAD")"
  if [[ -z "$changed_files" ]]; then
    echo "worker[$worker_id]: commit contains no file changes" >&2
    return 1
  fi

  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    if [[ "$REPO_SUBDIR" != "." && "$file" != "$REPO_SUBDIR/"* ]]; then
      echo "worker[$worker_id]: changed file escaped allowed scope: $file" >&2
      return 1
    fi
  done <<< "$changed_files"

  return 0
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

cmd_promote_commit() {
  local target_run="$1"
  local worker_id="$2"
  local target_branch="${3:-$(git -C "$GIT_ROOT" rev-parse --abbrev-ref HEAD)}"
  local prefix commit
  prefix="$(latest_worker_log_prefix "$target_run" "$worker_id")"
  [[ -n "$prefix" ]] || { echo "No logs for run=$target_run worker=$worker_id" >&2; return 1; }
  commit="$(cat "$LOGS_DIR/${prefix}.head" 2>/dev/null || true)"
  [[ -n "$commit" ]] || { echo "No commit head found for $prefix" >&2; return 1; }

  git -C "$GIT_ROOT" checkout "$target_branch" >/dev/null
  git -C "$GIT_ROOT" cherry-pick "$commit"
  echo "promoted commit $commit onto $target_branch"
}

promote_successful_workers() {
  local workers="$1"
  promotion_enabled || return 0

  local rc=0 promoted=0 i state_json status commit slug

  if [[ -n "$(git -C "$GIT_ROOT" status --short)" ]]; then
    echo "ERROR: integration branch worktree is dirty before promotion: $GIT_ROOT" >&2
    return 1
  fi

  for (( i = 1; i <= workers; i++ )); do
    state_json="$(load_worker_state "$i" 2>/dev/null || true)"
    [[ -n "$state_json" ]] || continue

    status="$(jq -r '.status // ""' <<<"$state_json")"
    [[ "$status" == "success" ]] || continue

    commit="$(jq -r '.commit // ""' <<<"$state_json")"
    slug="$(jq -r '.slug // ""' <<<"$state_json")"
    if [[ -z "$commit" || "$commit" == "null" ]]; then
      echo "worker[$i]: success state missing commit; cannot promote" >&2
      log_event "worker_promotion_failed" "$i" "$slug" "missing_commit"
      rc=1
      continue
    fi

    if git -C "$GIT_ROOT" merge-base --is-ancestor "$commit" HEAD 2>/dev/null; then
      echo "worker[$i]: already promoted -> $slug ($commit)"
      log_event "worker_promoted" "$i" "$slug@$commit" "already_promoted"
      continue
    fi

    echo "worker[$i]: promoting -> $slug ($commit) onto $INTEGRATION_BRANCH"
    if git -C "$GIT_ROOT" cherry-pick "$commit" >/dev/null; then
      promoted=$((promoted + 1))
      log_event "worker_promoted" "$i" "$slug@$commit" "promoted"
    else
      git -C "$GIT_ROOT" cherry-pick --abort >/dev/null 2>&1 || true
      echo "worker[$i]: promotion failed -> $slug ($commit)" >&2
      log_event "worker_promotion_failed" "$i" "$slug@$commit" "cherry_pick_failed"
      rc=1
    fi
  done

  if (( promoted > 0 )); then
    echo "Promoted worker commits: $promoted"
    echo "Integration head: $(git -C "$GIT_ROOT" rev-parse --short HEAD)"

    # Push integration branch to remote if AUTO_PUSH is enabled
    if [[ "$AUTO_PUSH" == "1" ]]; then
      push_integration_branch || rc=1
    fi
  fi

  return "$rc"
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

      # Count outcomes
      if echo "$status_line" | grep -q "success"; then
        ((success_count++))
      elif echo "$status_line" | grep -q "timeout"; then
        ((timeout_count++))
      elif echo "$status_line" | grep -q "failed"; then
        ((failed_count++))
      else
        ((other_count++))
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
