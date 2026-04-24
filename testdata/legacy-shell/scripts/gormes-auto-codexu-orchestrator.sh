#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'
shopt -s inherit_errexit 2>/dev/null || true

ORCHESTRATOR_LIB_DIR="${ORCHESTRATOR_LIB_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/orchestrator/lib}"
# shellcheck source=/dev/null
for _lib in common backend candidates report failures claim worktree promote companions refill; do
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
BACKEND="${BACKEND:-codexu}"
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
QUOTA_BACKOFF_SECONDS="${QUOTA_BACKOFF_SECONDS:-600}"
ORCHESTRATOR_ONCE="${ORCHESTRATOR_ONCE:-0}"
AUTO_PROMOTE_SUCCESS="${AUTO_PROMOTE_SUCCESS:-1}"
ALLOW_SOFT_SUCCESS_NONZERO="${ALLOW_SOFT_SUCCESS_NONZERO:-1}"
FAIL_FAST_ON_WORKER_FAILURE="${FAIL_FAST_ON_WORKER_FAILURE:-1}"
PAUSE_ON_RUN_FAILURE="${PAUSE_ON_RUN_FAILURE:-1}"
SKIP_COMPANIONS_ON_RUN_FAILURE="${SKIP_COMPANIONS_ON_RUN_FAILURE:-1}"
ALLOW_DIRTY_WORKER_WORKTREES="${ALLOW_DIRTY_WORKER_WORKTREES:-0}"
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

# Companion scheduling defaults (planner / doc-improver / landingpage).
DISABLE_COMPANIONS="${DISABLE_COMPANIONS:-0}"
COMPANION_ON_IDLE="${COMPANION_ON_IDLE:-1}"
COMPANION_TIMEOUT_SECONDS="${COMPANION_TIMEOUT_SECONDS:-600}"
PLANNER_EVERY_N_CYCLES="${PLANNER_EVERY_N_CYCLES:-4}"
DOC_IMPROVER_EVERY_N_CYCLES="${DOC_IMPROVER_EVERY_N_CYCLES:-6}"
LANDINGPAGE_EVERY_N_HOURS="${LANDINGPAGE_EVERY_N_HOURS:-24}"
PLANNER_ROOT="${PLANNER_ROOT:-$REPO_ROOT/.codex/planner}"
CANDIDATE_LOW_WATERMARK="${CANDIDATE_LOW_WATERMARK:-5}"

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
  $0 --codexu              # use codexu backend (default)
  $0 --claudeu             # use claudeu (Claude Code) backend via PATH shim
  $0 --opencode            # use opencode backend
  $0 status [run_id]       # show run/worker status
  $0 salvage [run_id]      # list worker worktrees/state for manual recovery
  $0 tail [run_id] [n]     # tail orchestrator logs (default n=80)
  $0 abort [run_id]        # terminate active worker pids for run
  $0 cleanup               # cleanup stale locks and enforce worktree cap
  $0 promote-commit <run_id> <worker_id> [target_branch]
  $0 verify-gh-auth [repo_slug]   # check gh CLI auth + repo view access

Env:
  REPO_ROOT                  Default: $REPO_ROOT
  MAX_AGENTS                 Default: 4 (hard-capped by MAX_AGENTS_HARD_CAP)
  MAX_AGENTS_HARD_CAP        Default: 8
  MODE                       safe | unattended | full
  BACKEND                    codexu (default) | claudeu | opencode
                             Equivalent CLI flags: --codexu --claudeu --opencode
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
  QUOTA_BACKOFF_SECONDS      Sleep between probe cycles after provider quota exhaustion (default: 600)
  ORCHESTRATOR_ONCE          Set to 1 to run a single batch and exit
  AUTO_PROMOTE_SUCCESS       Set to 1 to promote successful workers before next cycle
  ALLOW_SOFT_SUCCESS_NONZERO Set to 1 to treat non-zero codex exits as success if report+commit pass validation
  FAIL_FAST_ON_WORKER_FAILURE Set to 1 to terminate remaining workers after first worker failure (default: 1)
  PAUSE_ON_RUN_FAILURE       Set to 1 to stop forever mode after non-quota failures (default: 1)
  SKIP_COMPANIONS_ON_RUN_FAILURE Set to 1 to run companions only after clean cycles (default: 1)
  ALLOW_DIRTY_WORKER_WORKTREES Set to 1 to bypass retained dirty worker-worktree launch guard (default: 0)
  PROMOTION_MODE             "pr" (default) opens a PR per successful worker and falls back
                             to cherry-pick on any gh/push failure. "cherry-pick" skips the
                             PR flow entirely.
  PR_REPO_SLUG               owner/name for gh pr create (default: TrebuchetDynamics/gormes-agent)
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

  # Companion scheduling (planner / doc-improver / landingpage between cycles)
  DISABLE_COMPANIONS         Set to 1 to fully disable all companion runs (default: 0)
  COMPANION_ON_IDLE          1 gates companions to idle/post-promotion cycles; 0 runs every cycle (default: 1)
  COMPANION_TIMEOUT_SECONDS  Wall-clock timeout per companion invocation (default: 600)
  PLANNER_EVERY_N_CYCLES     Run planner companion every N cycles (default: 4)
  DOC_IMPROVER_EVERY_N_CYCLES Run doc-improver every N cycles with promotion (default: 6)
  LANDINGPAGE_EVERY_N_HOURS  Run landing-page companion every N hours (default: 24)
  COMPANION_PLANNER_CMD      Override path for planner companion (default: scripts/gormes-architecture-planner-tasks-manager.sh)
  COMPANION_DOC_IMPROVER_CMD Override path for doc-improver companion (default: scripts/documentation-improver.sh)
  COMPANION_LANDINGPAGE_CMD  Override path for landingpage companion (default: scripts/landingpage-improver.sh)
  CANDIDATE_LOW_WATERMARK    When write_candidates_file yields fewer than this many unfinished
                             tasks, run_once fires the planner companion synchronously to
                             refill the pool. Default: 5
  PHASE_FLOOR                Optional positive integer. When set, only candidates whose
                             numeric phase_id <= PHASE_FLOOR are considered. Lets the
                             operator prioritize early phases (e.g. PHASE_FLOOR=4 works
                             on phases 1-4 first, skipping 5-6). Default: unset (no filter).

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
    stale_pids="$(find_stale_orchestrator_pids)"
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
  CMD_ARGS=()
  COMMAND_MODE=""

  while (( $# > 0 )); do
    case "$1" in
      --codexu)   BACKEND=codexu;   shift ;;
      --claudeu)  BACKEND=claudeu;  shift ;;
      --opencode) BACKEND=opencode; shift ;;
      --resume)
        [[ -n "${2:-}" ]] || { echo "ERROR: --resume requires run_id" >&2; exit 1; }
        RESUME_RUN_ID="$2"
        RUN_ID="$RESUME_RUN_ID"
        WORKTREES_DIR="${RUN_ROOT}/worktrees/${RUN_ID}"
        CANDIDATES_FILE="$STATE_DIR/candidates.$RUN_ID.json"
        RUN_PIDS_DIR="$STATE_DIR/pids/$RUN_ID"
        RUN_WORKER_STATE_DIR="$STATE_DIR/workers/$RUN_ID"
        COMMAND_MODE="resume"
        shift 2
        ;;
      status|salvage|tail|abort|cleanup|promote-commit|verify-gh-auth)
        COMMAND_MODE="$1"
        shift
        # Remaining positional args belong to the subcommand.
        while (( $# > 0 )); do
          CMD_ARGS+=("$1")
          shift
        done
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      "" )
        shift
        ;;
      *)
        echo "ERROR: unknown command '$1'" >&2
        usage
        exit 1
        ;;
    esac
  done

  if [[ -z "$COMMAND_MODE" ]]; then
    COMMAND_MODE="run"
  fi
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
  require_cmd "$BACKEND"
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
  if [[ "$FAIL_FAST_ON_WORKER_FAILURE" != "0" && "$FAIL_FAST_ON_WORKER_FAILURE" != "1" ]]; then
    echo "ERROR: FAIL_FAST_ON_WORKER_FAILURE must be 0 or 1" >&2
    exit 1
  fi
  if [[ "$PAUSE_ON_RUN_FAILURE" != "0" && "$PAUSE_ON_RUN_FAILURE" != "1" ]]; then
    echo "ERROR: PAUSE_ON_RUN_FAILURE must be 0 or 1" >&2
    exit 1
  fi
  if [[ "$SKIP_COMPANIONS_ON_RUN_FAILURE" != "0" && "$SKIP_COMPANIONS_ON_RUN_FAILURE" != "1" ]]; then
    echo "ERROR: SKIP_COMPANIONS_ON_RUN_FAILURE must be 0 or 1" >&2
    exit 1
  fi
  if [[ "$ALLOW_DIRTY_WORKER_WORKTREES" != "0" && "$ALLOW_DIRTY_WORKER_WORKTREES" != "1" ]]; then
    echo "ERROR: ALLOW_DIRTY_WORKER_WORKTREES must be 0 or 1" >&2
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
      done < <(build_backend_cmd)
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
      local verify_failed=0
      local report_validation_failed=0
      local quota_exhausted=0
      local quota_message=""

      log_info "Worker $worker_id: codexu exited with code $rc"

      if [[ "$rc" != "0" ]] && quota_message="$(provider_quota_message "$final_file" "$stderr_file" "$jsonl_file")"; then
        quota_exhausted=1
      fi

      if [[ "$rc" == "0" ]] && ! wait_for_valid_final_report "$worker_id" "$final_file" "$stderr_file" "$jsonl_file"; then
        rc=1
        report_validation_failed=1
        echo "$rc" > "$run_base.exitcode"
      fi
      if [[ "$rc" == "0" ]] && ! verify_worker_commit "$worker_id" "$final_file"; then
        rc=1
        verify_failed=1
        echo "$rc" > "$run_base.exitcode"
      fi

      if (( quota_exhausted == 0 )) && [[ "$rc" != "0" ]] && try_soft_success_nonzero "$worker_id" "$rc" "$final_file" "$stderr_file" "$jsonl_file"; then
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
        failure_record_reset "$slug"
      elif (( quota_exhausted == 1 )); then
        echo "worker[$worker_id]: quota-exhausted -> $slug" | tee "$LOGS_DIR/worker_${worker_id}.status"
        save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'failed' --arg slug "$slug" --arg reason 'quota_exhausted' --arg rc "$rc" --arg message "$quota_message" '{run_id:$run_id,status:$status,slug:$slug,reason:$reason,rc:($rc|tonumber),message:$message}')"
        log_event "worker_failed" "$worker_id" "$slug" "quota_exhausted"
        local quota_errors_json
        quota_errors_json="$(jq -nc --arg message "$quota_message" '[$message]')"
        failure_record_write "$slug" "$rc" "quota_exhausted" "$stderr_file" "$quota_errors_json"
      elif [[ "$rc" == "124" ]]; then
        echo "worker[$worker_id]: timeout(${WORKER_TIMEOUT_SECONDS}s) -> $slug" | tee "$LOGS_DIR/worker_${worker_id}.status"
        save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'failed' --arg slug "$slug" --arg reason 'timeout' '{run_id:$run_id,status:$status,slug:$slug,reason:$reason}')"
        log_event "worker_failed" "$worker_id" "$slug" "timeout"
        local timeout_final_errors_raw timeout_final_errors_json
        timeout_final_errors_raw="$(collect_final_report_issues "$final_file" 2>/dev/null || true)"
        timeout_final_errors_json="$(printf '%s' "$timeout_final_errors_raw" | jq -Rnc '[inputs | select(length > 0)]' 2>/dev/null || true)"
        [[ -z "$timeout_final_errors_json" ]] && timeout_final_errors_json='[]'
        failure_record_write "$slug" "$rc" "timeout" "$stderr_file" "$timeout_final_errors_json"
      else
        local failure_reason
        failure_reason="$(classify_worker_failure "$rc")"
        # Override the rc-based bucket with granular taxonomy when we
        # know the specific verify/report failure that tripped rc=1.
        if (( verify_failed == 1 )) && [[ -n "${LAST_VERIFY_REASON:-}" ]]; then
          failure_reason="$LAST_VERIFY_REASON"
        elif (( report_validation_failed == 1 )); then
          failure_reason="report_validation_failed"
        fi
        echo "worker[$worker_id]: failed($rc) -> $slug" | tee "$LOGS_DIR/worker_${worker_id}.status"
        save_worker_state "$worker_id" "$(jq -nc --arg run_id "$RUN_ID" --arg status 'failed' --arg slug "$slug" --arg reason "$failure_reason" --arg rc "$rc" '{run_id:$run_id,status:$status,slug:$slug,reason:$reason,rc:($rc|tonumber)}')"
        log_event "worker_failed" "$worker_id" "$slug" "$failure_reason"
        local final_errors_raw final_errors_json
        final_errors_raw="$(collect_final_report_issues "$final_file" 2>/dev/null || true)"
        final_errors_json="$(printf '%s' "$final_errors_raw" | jq -Rnc '[inputs | select(length > 0)]' 2>/dev/null || true)"
        [[ -z "$final_errors_json" ]] && final_errors_json='[]'
        failure_record_write "$slug" "$rc" "$failure_reason" "$stderr_file" "$final_errors_json"
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
    IFS=' ' read -r progress_complete progress_in_progress progress_planned progress_total <<< "$(read_progress_summary)"

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

cmd_salvage() {
  local target_run=""
  target_run="$(resolve_target_run_id "${1:-}")"
  worker_salvage_report "$target_run"
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

maybe_refill_candidates() {
  local watermark="${CANDIDATE_LOW_WATERMARK:-5}"
  [[ "$watermark" =~ ^[0-9]+$ ]] || watermark=5
  local before; before="$(candidate_count)"
  if (( before >= watermark )); then
    return 0
  fi
  if [[ "${DISABLE_COMPANIONS:-0}" == "1" ]]; then
    return 0
  fi

  local state streak last_ts now decision
  state="$(read_refill_state)"
  streak="${state%%$'\t'*}"
  last_ts="${state##*$'\t'}"
  now="$(date +%s)"
  decision="$(should_skip_refill "$streak" "$last_ts" "$now" "${LOOP_SLEEP_SECONDS:-30}" || true)"
  if [[ "$decision" == skip* ]]; then
    local remaining="${decision#skip }"
    echo "Candidate refill skipped by backoff (streak=$streak, ${remaining}s left)"
    log_event "candidate_refill_skipped_backoff" null \
      "before=$before streak=$streak remaining_s=$remaining" "skipped"
    return 0
  fi

  echo "Candidate pool low ($before < $watermark); running planner companion synchronously to refill"
  log_event "candidate_refill_triggered" null "before=$before watermark=$watermark streak=$streak" "triggered"
  run_companion planner --sync || true
  # Re-read progress.json (planner may have edited it) and regenerate candidates
  write_candidates_file
  local after; after="$(candidate_count)"

  if (( after > before )); then
    streak=0
  else
    streak=$(( streak + 1 ))
    if (( streak > 100 )); then streak=100; fi
  fi
  write_refill_state "$streak" "$now"

  log_event "candidate_refilled" null "before=$before after=$after streak=$streak" "refilled"
  echo "Candidate pool refill: $before -> $after (streak=$streak)"
}

run_once() {
  validate

  # Recreate run-scoped paths in case --resume changed RUN_ID.
  mkdir -p "$RUN_PIDS_DIR" "$RUN_WORKER_STATE_DIR" "$WORKTREES_DIR" "$STATE_DIR" "$LOGS_DIR" "$PROMPTS_DIR" "$LOCKS_DIR"

  if ! refuse_dirty_worker_worktrees; then
    log_event "dirty_worker_worktrees_blocked" null "run=$RUN_ID" "blocked"
    return 1
  fi
  preflight_resource_safety
  cleanup_stale_locks
  write_candidates_file
  maybe_refill_candidates
  enforce_worktree_dir_cap
  log_event "run_started" null "mode=$MODE workers=$MAX_AGENTS" "started"

  local total workers
  total="$(candidate_count)"
  if (( total == 0 )); then
    echo "No unfinished tasks in $PROGRESS_JSON_REL"
    log_event "run_completed" null "no unfinished tasks" "empty"
    return 0
  fi

  local progress_complete progress_in_progress progress_planned progress_total
  IFS=' ' read -r progress_complete progress_in_progress progress_planned progress_total <<< "$(read_progress_summary)"
  [[ "$progress_total" =~ ^[0-9]+$ ]] || progress_total=0
  if (( progress_total == 0 )); then
    echo "ERROR: progress summary returned 0 total from $PROGRESS_JSON while candidate pool has $total tasks." >&2
    echo "Refusing to launch workers until the canonical progress parser is healthy." >&2
    log_event "progress_summary_failed" null "progress_json=$PROGRESS_JSON candidates=$total" "failed"
    return 1
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
  local fail_fast_triggered=0
  while (( ${#remaining[@]} > 0 )); do
    local new_remaining=()
    local found_done=false
    local worker_failed_this_pass=false

    for pid in "${remaining[@]}"; do
      if ! proc_alive "$pid"; then
        # Process has exited - reap it
        if ! wait "$pid"; then
          rc=1
          worker_failed_this_pass=true
        fi
        found_done=true
      else
        new_remaining+=("$pid")
      fi
    done

    remaining=("${new_remaining[@]}")

    if $worker_failed_this_pass && [[ "$FAIL_FAST_ON_WORKER_FAILURE" == "1" ]] && (( fail_fast_triggered == 0 )) && (( ${#remaining[@]} > 0 )); then
      fail_fast_triggered=1
      echo "Fail-fast: worker failure detected; aborting ${#remaining[@]} still-running worker(s) to avoid wasted tokens." >&2
      log_event "run_fail_fast_abort" null "aborting=${#remaining[@]}" "aborting"

      local abort_pid abort_worker_id
      for abort_pid in "${remaining[@]}"; do
        abort_worker_id=""
        for (( i = 0; i < ${#pids[@]}; i++ )); do
          if [[ "${pids[$i]}" == "$abort_pid" ]]; then
            abort_worker_id="$((i + 1))"
            break
          fi
        done

        if [[ -n "$abort_worker_id" ]]; then
          echo "worker[$abort_worker_id]: aborted-fail-fast -> prior worker failure" | tee "$LOGS_DIR/worker_${abort_worker_id}.status"
          save_worker_state "$abort_worker_id" "$(jq -nc \
            --arg run_id "$RUN_ID" \
            --arg status "aborted" \
            --arg reason "fail_fast_worker_failure" \
            '{run_id:$run_id,status:$status,reason:$reason}')"
        fi
      done

      abort_worker_pids "worker failure in run $RUN_ID" "${remaining[@]}"
      # The workers above are now explicitly recorded as aborted and their
      # process trees have been terminated. Do not wait indefinitely for
      # nested model/timeout processes; continue to summary and promotion so
      # already-successful workers are harvested before the failed cycle
      # pauses.
      remaining=()
    fi

    if ! $found_done && (( ${#remaining[@]} > 0 )); then
      # No process exited this iteration - sleep briefly
      sleep 0.5
    fi
  done

  kill "$heartbeat_pid" 2>/dev/null || true
  wait "$heartbeat_pid" 2>/dev/null || true

  echo
  echo "Worker summary:"
  local success_count=0 failed_count=0 timeout_count=0 quota_count=0 aborted_count=0 other_count=0
  for (( i = 1; i <= workers; i++ )); do
    if [[ -f "$LOGS_DIR/worker_${i}.status" ]]; then
      local status_line outcome
      status_line=$(cat "$LOGS_DIR/worker_${i}.status")
      echo "$status_line"

      # Count outcomes. Use $((...)) assignment instead of ((var++)) because
      # the post-increment form returns the old value, which is a "failure"
      # exit (0) under set -e when the counter is still zero.
      outcome="$(worker_status_outcome "$status_line")"
      case "$outcome" in
        success) success_count=$((success_count + 1)) ;;
        quota) quota_count=$((quota_count + 1)) ;;
        timeout) timeout_count=$((timeout_count + 1)) ;;
        aborted) aborted_count=$((aborted_count + 1)) ;;
        failed) failed_count=$((failed_count + 1)) ;;
        *) other_count=$((other_count + 1)) ;;
      esac
    fi
  done

  log_info "Worker outcomes - Success: $success_count, Failed: $failed_count, Timeout: $timeout_count, Quota: $quota_count, Aborted: $aborted_count, Other: $other_count"

  if ! promote_successful_workers "$workers"; then
    rc=1
  fi

  if (( quota_count > 0 && success_count == 0 )); then
    rc=75
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
  echo "  Quota limited:    $quota_count"
  echo "  Aborted:          $aborted_count"
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
    log_event "run_completed" null "workers=${workers} success=${success_count} failed=${failed_count} aborted=${aborted_count}" "success"
  elif [[ "$rc" == "75" ]]; then
    log_error "Run paused by provider quota exhaustion"
    log_event "run_completed" null "workers=${workers} success=${success_count} failed=${failed_count} quota=${quota_count} aborted=${aborted_count}" "quota_exhausted"
  else
    log_error "Run completed with failures"
    log_event "run_completed" null "workers=${workers} success=${success_count} failed=${failed_count} aborted=${aborted_count}" "failure"
  fi

  enforce_worktree_dir_cap
  return "$rc"
}

emit_startup_env_banner() {
  # List of env vars to dump at startup. Grouped by concern for readability.
  local -a keys=(
    MODE MAX_AGENTS BACKEND
    DISABLE_COMPANIONS COMPANION_ON_IDLE COMPANION_TIMEOUT_SECONDS
    MAX_RETRIES CANDIDATE_LOW_WATERMARK
    ORCHESTRATOR_ONCE AUTO_PROMOTE_SUCCESS ALLOW_SOFT_SUCCESS_NONZERO
    FAIL_FAST_ON_WORKER_FAILURE PAUSE_ON_RUN_FAILURE SKIP_COMPANIONS_ON_RUN_FAILURE
    LOOP_SLEEP_SECONDS QUOTA_BACKOFF_SECONDS WORKER_TIMEOUT_SECONDS
    COMPANION_PLANNER_CMD COMPANION_DOC_IMPROVER_CMD COMPANION_LANDINGPAGE_CMD
  )

  # Human-readable lines to stderr (visible via journalctl).
  local -a group1=(MODE MAX_AGENTS BACKEND)
  local -a group2=(DISABLE_COMPANIONS COMPANION_ON_IDLE COMPANION_TIMEOUT_SECONDS)
  local -a group3=(MAX_RETRIES CANDIDATE_LOW_WATERMARK)
  local -a group4=(ORCHESTRATOR_ONCE AUTO_PROMOTE_SUCCESS ALLOW_SOFT_SUCCESS_NONZERO FAIL_FAST_ON_WORKER_FAILURE PAUSE_ON_RUN_FAILURE SKIP_COMPANIONS_ON_RUN_FAILURE)
  local -a group5=(LOOP_SLEEP_SECONDS QUOTA_BACKOFF_SECONDS WORKER_TIMEOUT_SECONDS)
  local -a group6=(COMPANION_PLANNER_CMD COMPANION_DOC_IMPROVER_CMD COMPANION_LANDINGPAGE_CMD)

  _emit_env_group() {
    local line="[startup env]"
    local k v
    for k in "$@"; do
      v="${!k-}"
      line+=" $k=$v"
    done
    printf '%s\n' "$line" >&2
  }
  _emit_env_group "${group1[@]}"
  _emit_env_group "${group2[@]}"
  _emit_env_group "${group3[@]}"
  _emit_env_group "${group4[@]}"
  _emit_env_group "${group5[@]}"
  _emit_env_group "${group6[@]}"
  unset -f _emit_env_group

  # Structured ledger event so audit.sh / downstream tooling can read it.
  local k v detail_json
  detail_json='{}'
  for k in "${keys[@]}"; do
    v="${!k-}"
    detail_json="$(jq -c --arg k "$k" --arg v "$v" '. + {($k): $v}' <<<"$detail_json")"
  done
  mkdir -p "$STATE_DIR"
  jq -nc \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg run_id "$RUN_ID" \
    --arg event "startup_env" \
    --argjson detail "$detail_json" \
    --arg status "emitted" \
    '{ts:$ts, run_id:$run_id, event:$event, worker_id:null, detail:$detail, status:$status}' \
    >> "$RUNS_LEDGER"
}

main() {
  parse_cli_args "$@"

  if [[ "$COMMAND_MODE" == "status" ]]; then
    cmd_status "${CMD_ARGS[0]:-}"
    return 0
  elif [[ "$COMMAND_MODE" == "salvage" ]]; then
    cmd_salvage "${CMD_ARGS[0]:-}"
    return 0
  elif [[ "$COMMAND_MODE" == "tail" ]]; then
    cmd_tail "${CMD_ARGS[0]:-}" "${CMD_ARGS[1]:-80}"
    return 0
  elif [[ "$COMMAND_MODE" == "abort" ]]; then
    cmd_abort "${CMD_ARGS[0]:-}"
    return 0
  elif [[ "$COMMAND_MODE" == "cleanup" ]]; then
    validate
    cmd_cleanup
    return 0
  elif [[ "$COMMAND_MODE" == "promote-commit" ]]; then
    [[ -n "${CMD_ARGS[0]:-}" && -n "${CMD_ARGS[1]:-}" ]] || { echo "Usage: $0 promote-commit <run_id> <worker_id> [target_branch]" >&2; return 1; }
    validate
    cmd_promote_commit "${CMD_ARGS[0]}" "${CMD_ARGS[1]}" "${CMD_ARGS[2]:-}"
    return 0
  elif [[ "$COMMAND_MODE" == "verify-gh-auth" ]]; then
    orchestrator_verify_gh_auth "${CMD_ARGS[0]:-$PR_REPO_SLUG}"
    return $?
  fi

  claim_run_lock
  load_extra_args
  emit_startup_env_banner
  setup_integration_root

  if [[ "$ORCHESTRATOR_ONCE" == "1" || "$COMMAND_MODE" == "resume" ]]; then
    local once_rc=0
    if run_once; then
      once_rc=0
    else
      once_rc="$?"
    fi
    if should_run_post_cycle_companions "$once_rc"; then
      maybe_run_companions 1 "${PROMOTED_LAST_CYCLE:-0}"
    else
      echo "Skipping companions after run exit $once_rc to avoid wasted tokens."
      log_event "companions_skipped_unclean_cycle" null "cycle=1 rc=$once_rc" "skipped"
    fi
    return "$once_rc"
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

    if should_run_post_cycle_companions "$cycle_rc"; then
      maybe_run_companions "$cycle" "${PROMOTED_LAST_CYCLE:-0}"
    else
      echo "Skipping companions after run exit $cycle_rc to avoid wasted tokens."
      log_event "companions_skipped_unclean_cycle" null "cycle=$cycle rc=$cycle_rc" "skipped"
    fi

    if [[ "${EXHAUSTION_TRIGGERED:-0}" == "1" ]]; then
      EXHAUSTION_TRIGGERED=0
      echo "Loop cycle $cycle completed with exit $cycle_rc; exhausted → skipping sleep."
      continue
    fi

    if [[ "$cycle_rc" == "75" ]]; then
      echo
      echo "Loop cycle $cycle hit provider quota; sleeping ${QUOTA_BACKOFF_SECONDS}s before next probe."
      sleep "$QUOTA_BACKOFF_SECONDS"
      continue
    fi

    if should_pause_after_cycle "$cycle_rc"; then
      echo
      echo "Loop cycle $cycle completed with exit $cycle_rc; pausing forever loop to avoid wasted tokens."
      log_event "loop_paused" null "cycle=$cycle rc=$cycle_rc" "paused"
      return 0
    fi

    echo
    echo "Loop cycle $cycle completed with exit $cycle_rc; sleeping ${LOOP_SLEEP_SECONDS}s before next run."
    sleep "$LOOP_SLEEP_SECONDS"
  done
}

if [[ "${GORMES_ORCHESTRATOR_SOURCE_ONLY:-0}" != "1" ]]; then
  main "$@"
fi
