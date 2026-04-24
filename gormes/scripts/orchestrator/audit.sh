#!/usr/bin/env bash
# Periodic effectiveness audit for gormes-orchestrator.
# Reads the runs.jsonl ledger, tracks deltas vs. a cursor file, and emits:
#   - human-readable summary on stdout (captured by journald)
#   - structured ndjson append at $AUDIT_DIR/report.ndjson
# Designed to run under a systemd --user timer every N minutes.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Resolve the repo root from the script's own location
# (gormes/scripts/orchestrator/audit.sh -> up 3 -> repo root).
# Prefer `git rev-parse --show-toplevel` when available for correctness
# inside git worktrees; fall back to the path walk.
: "${REPO_ROOT:=$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || (cd "$SCRIPT_DIR/../../.." && pwd))}"
: "${RUN_ROOT:=$REPO_ROOT/gormes/.codex/orchestrator}"
: "${RUNS_LEDGER:=$RUN_ROOT/state/runs.jsonl}"
: "${COMPANIONS_DIR:=$RUN_ROOT/companions}"
: "${LOGS_DIR:=$RUN_ROOT/logs}"
: "${INTEGRATION_WT:=$RUN_ROOT/integration/codexu-autoloop}"
: "${AUDIT_DIR:=$HOME/.cache/gormes-orchestrator-audit}"
: "${CURSOR_FILE:=$AUDIT_DIR/cursor}"
: "${REPORT_FILE:=$AUDIT_DIR/report.ndjson}"
: "${CSV_FILE:=$AUDIT_DIR/report.csv}"
: "${SERVICE:=gormes-orchestrator.service}"
: "${LOOKBACK_SECONDS:=1200}"

mkdir -p "$AUDIT_DIR"

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
now_epoch() { date +%s; }

read_cursor() {
  if [[ -f "$CURSOR_FILE" ]]; then
    cat "$CURSOR_FILE"
  else
    # First run: consider events in the last LOOKBACK_SECONDS.
    date -u -d "-${LOOKBACK_SECONDS} seconds" +%Y-%m-%dT%H:%M:%SZ
  fi
}

write_cursor() {
  printf '%s\n' "$1" > "$CURSOR_FILE"
}

service_block() {
  # Returns JSON with service active, uptime_seconds, restart_count, recent_restart.
  local active state_ts uptime_s restart_count
  active="$(systemctl --user is-active "$SERVICE" 2>/dev/null || echo unknown)"
  state_ts="$(systemctl --user show -p ActiveEnterTimestamp --value "$SERVICE" 2>/dev/null || echo '')"
  if [[ -n "$state_ts" ]]; then
    uptime_s=$(( $(now_epoch) - $(date -d "$state_ts" +%s 2>/dev/null || echo "$(now_epoch)") ))
  else
    uptime_s=0
  fi
  restart_count="$(systemctl --user show -p NRestarts --value "$SERVICE" 2>/dev/null || echo 0)"
  [[ "$restart_count" =~ ^[0-9]+$ ]] || restart_count=0
  jq -nc \
    --arg active "$active" \
    --argjson uptime_s "$uptime_s" \
    --argjson restart_count "$restart_count" \
    '{active:$active, uptime_seconds:$uptime_s, nrestarts:$restart_count}'
}

ledger_summary() {
  local cursor="$1"
  if [[ ! -f "$RUNS_LEDGER" ]]; then
    printf '{"events_since_cursor":0}\n'
    return
  fi
  jq -nc --slurpfile log "$RUNS_LEDGER" --arg cursor "$cursor" '
    ($log // []) as $events |
    ($events | map(select(.ts > $cursor))) as $new |
    {
      events_since_cursor: ($new | length),
      run_started: ($new | map(select(.event == "run_started")) | length),
      run_completed: ($new | map(select(.event == "run_completed")) | length),
      worker_claimed: ($new | map(select(.event == "worker_claimed")) | length),
      worker_success: ($new | map(select(.event == "worker_success")) | length),
      worker_failed: ($new | map(select(.event == "worker_failed")) | length),
      worker_promoted: ($new | map(select(.event == "worker_promoted")) | length),
      worker_promotion_failed: ($new | map(select(.event == "worker_promotion_failed")) | length),
      fail_status_breakdown: ($new
        | map(select(.event == "worker_failed"))
        | group_by(.status)
        | map({(.[0].status // "unknown"): length})
        | add // {}),
      last_event_ts: ($events[-1].ts // null),
      last_event: ($events[-1].event // null),
      last_event_detail: ($events[-1].detail // null)
    }
  '
}

integration_head() {
  local head="unknown" subj=""
  if [[ -d "$INTEGRATION_WT" ]]; then
    head="$(git -C "$INTEGRATION_WT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
    subj="$(git -C "$INTEGRATION_WT" log -1 --format=%s 2>/dev/null || echo '')"
  fi
  jq -nc --arg head "$head" --arg subj "$subj" '{short:$head, subject:$subj}'
}

companions_block() {
  local out="{}"
  if [[ -d "$COMPANIONS_DIR" ]]; then
    local tmp="$(mktemp)"
    trap "rm -f $tmp" RETURN
    printf '%s' "{}" > "$tmp"
    local f name rc ts
    for f in "$COMPANIONS_DIR"/*.last.json; do
      [[ -f "$f" ]] || continue
      name="$(basename "$f" .last.json)"
      jq --arg name "$name" --slurpfile src "$f" \
        '.[$name] = $src[0]' \
        "$tmp" > "$tmp.new" && mv "$tmp.new" "$tmp"
    done
    out="$(cat "$tmp")"
  fi
  printf '%s\n' "$out"
}

cursor="$(read_cursor)"
ts="$(now_utc)"
svc="$(service_block)"
summary="$(ledger_summary "$cursor")"
integ="$(integration_head)"
comps="$(companions_block)"

# Advance cursor to the latest event's ts (or now if no new events)
new_cursor="$(jq -r '.last_event_ts // empty' <<<"$summary")"
[[ -z "$new_cursor" || "$new_cursor" == "null" ]] && new_cursor="$ts"
write_cursor "$new_cursor"

# Build combined report line
line="$(jq -nc \
  --arg ts "$ts" \
  --arg cursor_from "$cursor" \
  --arg cursor_to "$new_cursor" \
  --argjson service "$svc" \
  --argjson summary "$summary" \
  --argjson integration "$integ" \
  --argjson companions "$comps" \
  '{ts:$ts, cursor_from:$cursor_from, cursor_to:$cursor_to, service:$service, summary:$summary, integration:$integration, companions:$companions}')"

printf '%s\n' "$line" >> "$REPORT_FILE"

# CSV append for easy trending (header written once if file missing).
csv_header="ts,active,uptime_s,nrestarts,claimed,success,failed,promoted,cherry_pick_failed,productivity_pct,integration_head_short"
if [[ ! -f "$CSV_FILE" ]]; then
  printf '%s\n' "$csv_header" > "$CSV_FILE"
fi
csv_row="$(jq -r '
  [
    .ts,
    .service.active,
    (.service.uptime_seconds|tostring),
    (.service.nrestarts|tostring),
    (.summary.worker_claimed|tostring),
    (.summary.worker_success|tostring),
    (.summary.worker_failed|tostring),
    (.summary.worker_promoted|tostring),
    (.summary.worker_promotion_failed|tostring),
    (if (.summary.worker_claimed // 0) > 0
      then ((.summary.worker_promoted * 100) / .summary.worker_claimed | floor | tostring)
      else "0" end),
    .integration.short
  ] | @csv
' <<<"$line")"
printf '%s\n' "$csv_row" >> "$CSV_FILE"

# Human-readable summary for journald
active="$(jq -r '.service.active' <<<"$line")"
uptime="$(jq -r '.service.uptime_seconds' <<<"$line")"
nrest="$(jq -r '.service.nrestarts' <<<"$line")"
ev="$(jq -r '.summary.events_since_cursor' <<<"$line")"
claimed="$(jq -r '.summary.worker_claimed' <<<"$line")"
success="$(jq -r '.summary.worker_success' <<<"$line")"
failed="$(jq -r '.summary.worker_failed' <<<"$line")"
promoted="$(jq -r '.summary.worker_promoted' <<<"$line")"
cpf="$(jq -r '.summary.worker_promotion_failed' <<<"$line")"
runs="$(jq -r '.summary.run_started' <<<"$line")"
fails="$(jq -r '.summary.fail_status_breakdown | to_entries | map("\(.key)=\(.value)") | join(",")' <<<"$line")"
head="$(jq -r '.integration.short' <<<"$line")"
subj="$(jq -r '.integration.subject' <<<"$line")"
last_ev="$(jq -r '.summary.last_event // "none"' <<<"$line")"
last_ts="$(jq -r '.summary.last_event_ts // "none"' <<<"$line")"

# Compute human productivity ratio
if (( claimed > 0 )); then
  rate=$(( promoted * 100 / claimed ))
else
  rate=0
fi

cat <<SUMMARY
gormes-orchestrator-audit @ $ts
  service:      $active  uptime=${uptime}s  nrestarts=$nrest
  window:       $cursor -> $new_cursor
  events:       total=$ev  run_started=$runs
  workers:      claimed=$claimed  success=$success  failed=$failed  promoted=$promoted  cherry_pick_failed=$cpf
  productivity: ${rate}% of claims landed this window
  fails by status: ${fails:-none}
  integration:  $head  "$subj"
  last ledger:  $last_ev @ $last_ts
SUMMARY

# Companions compact line
companion_tail_failing_logs() {
  # Args: comps_json
  # Side effect: prints "--- last 10 lines of <log> ---" + tail for any
  # companion whose rc is non-empty and not 0. Looks up logs via ls -t under
  # $LOGS_DIR/companion_<name>.*.log.
  local comps_json="$1"
  local name rc latest_log
  while IFS='|' read -r name rc; do
    [[ -n "$rc" && "$rc" != "0" ]] || continue
    latest_log="$(ls -t "${LOGS_DIR}/companion_${name}."*.log 2>/dev/null | head -n1 || true)"
    [[ -n "$latest_log" && -f "$latest_log" ]] || continue
    printf '      --- last 10 lines of %s ---\n' "$(basename "$latest_log")"
    tail -n 10 "$latest_log" | sed 's/^/      /'
  done < <(jq -r 'to_entries[] | "\(.key)|\(.value.rc // "")"' <<<"$comps_json")
}

if [[ "$comps" != "{}" ]]; then
  printf '  companions:\n'
  jq -r 'to_entries[] | "    \(.key): rc=\(.value.rc // "?") ts=\(.value.ts_utc // "?") cycle=\(.value.cycle // "?")"' <<<"$comps"
  companion_tail_failing_logs "$comps"
fi

# Alert-worthy conditions (prefix with WARN/ERROR so grep -E 'WARN|ERROR' finds them)
if [[ "$active" != "active" ]]; then
  echo "ERROR: service is $active"
fi
if (( nrest > 0 )) && (( uptime < 300 )); then
  echo "WARN: service recently restarted (nrestarts=$nrest, uptime=${uptime}s)"
fi
if (( claimed > 0 )) && (( promoted == 0 )) && (( failed == claimed )); then
  echo "WARN: entire audit window produced zero promotions"
fi
