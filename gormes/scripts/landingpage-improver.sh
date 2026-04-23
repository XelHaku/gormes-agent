#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"

LP_ROOT="${LP_ROOT:-$REPO_ROOT/.codex/landingpage-improver}"
LOG_DIR="$LP_ROOT/logs"
STATE_FILE="$LP_ROOT/landingpage_state.json"
REPORT_FILE="$LP_ROOT/latest_landingpage_report.md"
RAW_REPORT_FILE="$LP_ROOT/latest_landingpage_report.raw.md"
CONTEXT_FILE="$LP_ROOT/context.json"
PROMPT_FILE="$LP_ROOT/latest_prompt.txt"
LOCK_DIR="$LP_ROOT/run.lock"
LOCK_PID_FILE="$LOCK_DIR/pid"
LOCK_STARTED_FILE="$LOCK_DIR/started_at"
LOCK_COMMAND_FILE="$LOCK_DIR/command"

SITE_ROOT="$REPO_ROOT/www.gormes.ai"
SITE_CONTENT_GO="$SITE_ROOT/internal/site/content.go"
SITE_TEMPLATES_DIR="$SITE_ROOT/internal/site/templates"
SITE_STATIC_DIR="$SITE_ROOT/internal/site/static"
SITE_DATA_DIR="$SITE_ROOT/internal/site/data"
PROGRESS_JSON="$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
SITE_PROGRESS_JSON="$SITE_DATA_DIR/progress.json"
BENCHMARKS_JSON="$SITE_DATA_DIR/benchmarks.json"

RUN_AT_UTC="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
RUN_STAMP="$(date -u +"%Y%m%dT%H%M%SZ")"

CODEXU_JSONL="$LOG_DIR/$RUN_STAMP.codexu.jsonl"
CODEXU_STDERR="$LOG_DIR/$RUN_STAMP.codexu.stderr"
VALIDATION_LOG="$LOG_DIR/$RUN_STAMP.validation.log"

# Verbosity and commit mode settings
VERBOSE="${VERBOSE:-0}"
AUTO_COMMIT="${AUTO_COMMIT:-0}"
AUTO_PUSH="${AUTO_PUSH:-0}"
MAIN_BRANCH="${MAIN_BRANCH:-main}"
REMOTE_NAME="${REMOTE_NAME:-origin}"

mkdir -p "$LP_ROOT" "$LOG_DIR"

# ANSI color codes
if [[ -t 1 ]] || [[ "${FORCE_COLOR:-0}" == "1" ]]; then
  COLOR_RED='\033[0;31m'
  COLOR_YELLOW='\033[1;33m'
  COLOR_GREEN='\033[0;32m'
  COLOR_BLUE='\033[0;34m'
  COLOR_DIM='\033[2m'
  COLOR_RESET='\033[0m'
else
  COLOR_RED=''
  COLOR_YELLOW=''
  COLOR_GREEN=''
  COLOR_BLUE=''
  COLOR_DIM=''
  COLOR_RESET=''
fi

log() {
  printf '[landingpage-improver] %s\n' "$*"
}

log_kv() {
  local label="$1"
  local value="$2"
  log "$(printf '%-22s %s' "$label:" "$value")"
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

fail() {
  printf '[landingpage-improver] ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage:
  landingpage-improver.sh [run]
  landingpage-improver.sh status
  landingpage-improver.sh show-report
  landingpage-improver.sh doctor
  landingpage-improver.sh --help

Commands:
  run          Run one Codex landing-page improvement pass.
  status       Show latest landing page improver state.
  show-report  Print the latest landing page report.
  doctor       Validate required commands/paths.

Environment:
  REPO_ROOT    Default: $REPO_ROOT
  LP_ROOT      Default: $LP_ROOT
  VERBOSE      Set to 1 for detailed progress logging
  AUTO_COMMIT  Set to 1 to auto-commit changes after each stage
  AUTO_PUSH    Set to 1 to auto-push commits to remote
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
        $1 != self && $0 ~ /landingpage-improver[.]sh/ {
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
    fail "active landingpage-improver run owns $LOCK_DIR
PID: $lock_pid
Started: ${lock_started:-unknown}
Command: ${lock_command:-unknown}"
  fi

  if [[ -z "$lock_pid" ]]; then
    legacy_owner="$(find_legacy_lock_owner || true)"
    if [[ -n "$legacy_owner" ]]; then
      fail "active legacy landingpage-improver run owns $LOCK_DIR
Process: $legacy_owner
This run started before lock owner metadata existed; wait for it to finish."
    fi

    fail "landingpage-improver lock has no owner metadata: $LOCK_DIR
No active landingpage-improver process was detected. Remove the stale lock with: rmdir '$LOCK_DIR'"
  fi

  log "Removing stale landingpage-improver lock: $LOCK_DIR (PID: $lock_pid, Started: ${lock_started:-unknown})"
  remove_stale_lock
  mkdir "$LOCK_DIR" 2>/dev/null || fail "another landingpage-improver run claimed the lock: $LOCK_DIR"
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

json_array_from_lines() {
  jq -Rn '[inputs | select(length > 0)]'
}

collect_templates_json() {
  if [[ -d "$SITE_TEMPLATES_DIR" ]]; then
    find "$SITE_TEMPLATES_DIR" -type f -name '*.tmpl' | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
  else
    printf '[]\n'
  fi
}

collect_static_assets_json() {
  if [[ -d "$SITE_STATIC_DIR" ]]; then
    find "$SITE_STATIC_DIR" -type f \( -name '*.css' -o -name '*.js' -o -name '*.png' -o -name '*.svg' \) | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
  else
    printf '[]\n'
  fi
}

collect_site_data_json() {
  if [[ -d "$SITE_DATA_DIR" ]]; then
    find "$SITE_DATA_DIR" -type f -name '*.json' | sort | sed "s#^$REPO_ROOT/##" | json_array_from_lines
  else
    printf '[]\n'
  fi
}

write_context_bundle() {
  local templates_json static_json data_json

  templates_json="$(collect_templates_json)"
  static_json="$(collect_static_assets_json)"
  data_json="$(collect_site_data_json)"

  jq -n \
    --arg run_at_utc "$RUN_AT_UTC" \
    --arg repo_root "$REPO_ROOT" \
    --arg site_root "$SITE_ROOT" \
    --arg content_go "$SITE_CONTENT_GO" \
    --arg progress_json "$PROGRESS_JSON" \
    --arg site_progress_json "$SITE_PROGRESS_JSON" \
    --arg benchmarks_json "$BENCHMARKS_JSON" \
    --argjson templates "$templates_json" \
    --argjson static_assets "$static_json" \
    --argjson data_files "$data_json" \
    '{
      run_at_utc: $run_at_utc,
      repo_root: $repo_root,
      site_root: $site_root,
      content_go: $content_go,
      progress_json: $progress_json,
      site_progress_json: $site_progress_json,
      benchmarks_json: $benchmarks_json,
      templates: $templates,
      static_assets: $static_assets,
      data_files: $data_files
    }' > "$CONTEXT_FILE"
}

write_prompt_file() {
  cat > "$PROMPT_FILE" <<EOF
You are the Gormes Landing Page Improver.

Mission:
Improve the www.gormes.ai landing page to accurately reflect the current state of the Gormes project, ensure copy is compelling and accurate, and keep all data files in sync with the canonical progress.

Repository root:
- $REPO_ROOT

Site root:
- $SITE_ROOT

Files of record:
- Landing page content: $SITE_CONTENT_GO
- HTML templates: $SITE_TEMPLATES_DIR
- Static assets (CSS): $SITE_STATIC_DIR
- Progress data: $SITE_PROGRESS_JSON (should match $PROGRESS_JSON)
- Benchmarks data: $BENCHMARKS_JSON
- Context bundle: $CONTEXT_FILE

Rules:
- Landing page improvements only. Do not implement runtime features.
- Keep the zero-JavaScript, server-rendered approach.
- Ensure claims match actual implementation (binary size, features, etc).
- Sync progress.json when roadmap status changes.
- Keep edits focused on messaging, clarity, and accuracy.

Required tasks:
1) Review current landing page copy in content.go against actual implementation.
2) Check that features, roadmap, and CTAs are accurate and compelling.
3) Sync progress data if it differs from canonical progress.json.
4) Improve messaging clarity where needed (headlines, feature descriptions, CTAs).
5) Run validation commands:
   - go test ./internal/site/... -count=1
   - go test ./... -count=1 (from www.gormes.ai directory)
   - make build (if Makefile exists)

Required final report sections (exact headings):
1) Scope and baseline
2) Copy/content issues found
3) Landing page updates applied
4) Data sync status
5) Validation evidence
6) Risks / follow-ups
EOF
}

verify_final_report() {
  local file="$1"
  [[ -f "$file" ]] || return 1

  local number title pattern
  # The report format uses various header styles:
  #   ## 1) **Scope and baseline**  (markdown h2 with parenthesis)
  #   ## 1) Scope and baseline       (markdown h2 without bold)
  #   1) Scope and baseline          (plain format)
  # Pattern must handle all these variations with case-insensitive matching
  while IFS='|' read -r number title; do
    # Build pattern that matches:
    # - Optional ## at start of line
    # - Number with ) or .
    # - Optional space
    # - Optional ** around the title
    # - The title text (case insensitive)
    # - Optional closing **
    # - Optional trailing whitespace
    pattern="^[[:space:]]*(##[[:space:]]*)?${number}[).][[:space:]]+(\\*\\*)?${title}(\\*\\*)?[[:space:]]*$"
    grep -Ei "$pattern" "$file" > /dev/null || return 1
  done <<'EOF'
1|Scope and baseline
2|Copy/content issues found
3|Landing page updates applied
4|Data sync status
5|Validation evidence
6|Risks / follow-ups
EOF
}

extract_session_id() {
  local jsonl_file="$1"
  [[ -f "$jsonl_file" ]] || return 0
  jq -r 'select(.type=="thread.started") | (.thread_id // .session_id // empty)' "$jsonl_file" | head -n1
}

run_codexu_landingpage_pass() {
  log "Codex stdout JSONL: $CODEXU_JSONL"
  log "Codex stderr: $CODEXU_STDERR"
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
    [[ -f "$RAW_REPORT_FILE" ]] && printf '[landingpage-improver] Raw report saved at %s\n' "$RAW_REPORT_FILE" >&2
    fail "landing page final report did not match required format"
  fi
}

run_validation() {
  : > "$VALIDATION_LOG"

  log "Validation log: $VALIDATION_LOG"
  log "Validation command: go test ./internal/site/... -count=1"
  (
    cd "$SITE_ROOT"
    go test ./internal/site/... -count=1
    go test ./... -count=1
  ) >>"$VALIDATION_LOG" 2>&1 || {
    cat "$VALIDATION_LOG" >&2
    fail "validation failed"
  }
}

write_report() {
  cat > "$REPORT_FILE" <<EOF
# Landing Page Improver Run

- Run UTC: $RUN_AT_UTC
- Repo root: $REPO_ROOT
- Site root: $SITE_ROOT
- Context bundle: $CONTEXT_FILE
- Prompt file: $PROMPT_FILE
- Validation log: $VALIDATION_LOG

$(cat "$RAW_REPORT_FILE")
EOF
}

write_state_file() {
  local session_id
  session_id="$(extract_session_id "$CODEXU_JSONL" || true)"

  jq -n \
    --arg last_run_utc "$RUN_AT_UTC" \
    --arg repo_root "$REPO_ROOT" \
    --arg site_root "$SITE_ROOT" \
    --arg report_path "$REPORT_FILE" \
    --arg raw_report_path "$RAW_REPORT_FILE" \
    --arg context_path "$CONTEXT_FILE" \
    --arg prompt_path "$PROMPT_FILE" \
    --arg validation_log_path "$VALIDATION_LOG" \
    --arg codexu_jsonl_path "$CODEXU_JSONL" \
    --arg codexu_stderr_path "$CODEXU_STDERR" \
    --arg session_id "$session_id" \
    '{
      last_run_utc: $last_run_utc,
      repo_root: $repo_root,
      site_root: $site_root,
      report_path: $report_path,
      raw_report_path: $raw_report_path,
      context_path: $context_path,
      prompt_path: $prompt_path,
      validation_log_path: $validation_log_path,
      codexu_jsonl_path: $codexu_jsonl_path,
      codexu_stderr_path: $codexu_stderr_path,
      session_id: $session_id
    }' > "$STATE_FILE"
}

cmd_status() {
  require_cmd jq
  require_file "$STATE_FILE"

  printf 'Last run UTC: %s\n' "$(jq -r '.last_run_utc // "unknown"' "$STATE_FILE")"
  printf 'Report: %s\n' "$(jq -r '.report_path // "unknown"' "$STATE_FILE")"
  printf 'Validation log: %s\n' "$(jq -r '.validation_log_path // "unknown"' "$STATE_FILE")"
  printf 'Session ID: %s\n' "$(jq -r '.session_id // ""' "$STATE_FILE")"
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
  require_dir "$SITE_ROOT"
  require_file "$SITE_CONTENT_GO"
  require_file "$PROGRESS_JSON"
  log "doctor: ok"
}

cmd_run() {
  claim_lock "$@"

  log "Starting landing page improver run"
  log_kv "Run UTC" "$RUN_AT_UTC"
  log_kv "Repo root" "$REPO_ROOT"
  log_kv "Site root" "$SITE_ROOT"
  log_kv "LP root" "$LP_ROOT"
  log_kv "Lock" "$LOCK_DIR"
  log_kv "Report" "$REPORT_FILE"
  log_kv "State" "$STATE_FILE"

  log "Step 1/7: checking prerequisites"
  require_cmd jq
  require_cmd git
  require_cmd codexu
  require_cmd go
  require_dir "$REPO_ROOT"
  require_dir "$SITE_ROOT"
  require_file "$SITE_CONTENT_GO"
  require_file "$PROGRESS_JSON"

  log "Step 2/7: collecting landing page context"
  write_context_bundle
  log "Context bundle: $CONTEXT_FILE"

  log "Step 3/7: writing Codex prompt"
  write_prompt_file
  log "Prompt file: $PROMPT_FILE"

  log "Step 4/7: running Codex landing page pass"
  run_codexu_landingpage_pass

  local session_id
  session_id="$(extract_session_id "$CODEXU_JSONL" || true)"
  if [[ -n "$session_id" ]]; then
    log "Codex session: $session_id"
  else
    log "Codex session: unavailable"
  fi

  log "Step 5/7: validating landing page artifacts"
  run_validation

  log "Step 6/7: writing final report"
  write_report

  log "Step 7/7: writing state"
  write_state_file

  log "Landing page report: $REPORT_FILE"
  log "Landing page state: $STATE_FILE"
  log "Complete."
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
