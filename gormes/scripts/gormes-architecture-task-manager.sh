#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"

PLANNER_ROOT="${PLANNER_ROOT:-$REPO_ROOT/.codex/planner}"
LOG_DIR="$PLANNER_ROOT/logs"
STATE_FILE="$PLANNER_ROOT/planner_state.json"
REPORT_FILE="$PLANNER_ROOT/latest_planner_report.md"
RAW_REPORT_FILE="$PLANNER_ROOT/latest_planner_report.raw.md"
CONTEXT_FILE="$PLANNER_ROOT/context.json"
PROMPT_FILE="$PLANNER_ROOT/latest_prompt.txt"
TASKS_MD_FILE="$PLANNER_ROOT/tasks.md"
LOCK_DIR="$PLANNER_ROOT/run.lock"

PROGRESS_JSON="$REPO_ROOT/docs/content/building-gormes/architecture_plan/progress.json"
ARCH_PLAN_DIR="$REPO_ROOT/docs/content/building-gormes/architecture_plan"
ARCH_PLAN_JSON="$ARCH_PLAN_DIR/architecture_plan.json"
CORE_SYSTEMS_DIR="$REPO_ROOT/docs/content/building-gormes/core-systems"
RUN_AT_UTC="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
RUN_STAMP="$(date -u +"%Y%m%dT%H%M%SZ")"

PLANNER_TIMER_NAME="${PLANNER_TIMER_NAME:-gormes-architecture-task-manager}"
PLANNER_INTERVAL="${PLANNER_INTERVAL:-4h}"
PLANNER_BOOT_DELAY="${PLANNER_BOOT_DELAY:-5m}"
PLANNER_INSTALL_SCHEDULE="${PLANNER_INSTALL_SCHEDULE:-1}"

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

log() {
  printf '[architecture-task-manager] %s\n' "$*"
}

progress() {
  local step="$1"
  shift
  printf '[architecture-task-manager] %s/5 %s\n' "$step" "$*"
}

fail() {
  printf '[architecture-task-manager] ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<EOF
Usage:
  gormes-architecture-task-manager.sh [run]
  gormes-architecture-task-manager.sh status
  gormes-architecture-task-manager.sh show-report
  gormes-architecture-task-manager.sh doctor
  gormes-architecture-task-manager.sh install-schedule
  gormes-architecture-task-manager.sh --help

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
EOF
}

release_lock() {
  [[ -d "$LOCK_DIR" ]] && rmdir "$LOCK_DIR" 2>/dev/null || true
}

claim_lock() {
  if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    fail "another planner run is already in progress: $LOCK_DIR"
  fi
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

Required work:
1. Inspect upstream Hermes surfaces and current Gormes code/docs/progress.
2. Detect missing tasks, stale task status, stale assumptions, weak task granularity, and missing dependencies.
3. Update planning artifacts conservatively so execution agents can work from small, TDD-ready slices.
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
  while IFS='|' read -r number title; do
    pattern="^[[:space:]]*(#+[[:space:]]*)?${number}[.)][[:space:]]*(\\*\\*)?${title}(\\*\\*)?[[:space:]]*$"
    grep -Eiq "$pattern" "$file" || return 1
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
      "# Gormes Architecture Tasks",
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
    printf '[architecture-task-manager] Raw report saved at %s\n' "$file" >&2
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
  script_path="$SCRIPT_DIR/gormes-architecture-task-manager.sh"
  unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  service_file="$unit_dir/$PLANNER_TIMER_NAME.service"
  timer_file="$unit_dir/$PLANNER_TIMER_NAME.timer"

  mkdir -p "$unit_dir"

  cat > "$service_file" <<EOF
[Unit]
Description=Gormes architecture task manager

[Service]
Type=oneshot
WorkingDirectory=$REPO_ROOT
Environment=REPO_ROOT=$REPO_ROOT
Environment=UPSTREAM_HERMES_DIR=$UPSTREAM_HERMES_DIR
ExecStart=/usr/bin/env bash $script_path
EOF

  cat > "$timer_file" <<EOF
[Unit]
Description=Run the Gormes architecture task manager periodically

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
  script_path="$SCRIPT_DIR/gormes-architecture-task-manager.sh"
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
  claim_lock

  progress 1 "preflight"
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
  fi

  UPSTREAM_HERMES_DIR="$(detect_upstream_hermes_dir)"
  UPSTREAM_COMMIT="$(git_field "$UPSTREAM_HERMES_DIR" commit)"
  UPSTREAM_BRANCH="$(git_field "$UPSTREAM_HERMES_DIR" branch)"
  LOCAL_GIT_ROOT="$(git_field "$REPO_ROOT" root)"
  LOCAL_COMMIT="$(git_field "$REPO_ROOT" commit)"
  LOCAL_BRANCH="$(git_field "$REPO_ROOT" branch)"

  progress 2 "context"
  write_context_bundle
  write_tasks_markdown
  log_context_summary

  progress 3 "planning"
  run_codexu_planner

  progress 4 "validation"
  run_validation
  log "Validation log: $VALIDATION_LOG"

  progress 5 "schedule"
  if [[ "$PLANNER_INSTALL_SCHEDULE" == "1" ]]; then
    install_periodic_schedule
  else
    SCHEDULE_METHOD="disabled"
  fi
  log "Schedule method: $SCHEDULE_METHOD"
  write_report
  write_state_file

  log "Planner report: $REPORT_FILE"
  log "Task Markdown: $TASKS_MD_FILE"
  log "Planner state: $STATE_FILE"
  log "Periodic schedule: $SCHEDULE_METHOD"
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
