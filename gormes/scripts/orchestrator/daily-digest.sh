#!/usr/bin/env bash
# Summary of last 24h orchestrator activity.
# Usage: daily-digest.sh [--output FILE]
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${REPO_ROOT:=$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || (cd "$SCRIPT_DIR/../../.." && pwd))}"
: "${RUN_ROOT:=$REPO_ROOT/gormes/.codex/orchestrator}"
: "${RUNS_LEDGER:=$RUN_ROOT/state/runs.jsonl}"
: "${AUDIT_DIR:=$HOME/.cache/gormes-orchestrator-audit}"
: "${REPO_SLUG:=TrebuchetDynamics/gormes-agent}"

output_file=""
while (( $# > 0 )); do
  case "$1" in
    --output) output_file="$2"; shift 2 ;;
    -h|--help) sed -n '2,4p' "${BASH_SOURCE[0]}" | sed 's/^# //'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

since_ts="$(date -u -d '-24 hours' +%Y-%m-%dT%H:%M:%SZ)"

# Pull events from ledger
stats="$(jq -rs --arg since "$since_ts" '
  [.[] | select(.ts > $since)] as $ev |
  {
    runs: ($ev | map(select(.event == "run_started")) | length),
    claimed: ($ev | map(select(.event == "worker_claimed")) | length),
    success: ($ev | map(select(.event == "worker_success")) | length),
    failed_by_status: ($ev | map(select(.event == "worker_failed")) | group_by(.status) | map({(.[0].status): length}) | add // {}),
    promoted: ($ev | map(select(.event == "worker_promoted")) | length),
    pr_opened: ($ev | map(select(.event == "worker_pr_opened")) | length),
    pr_fallback: ($ev | map(select(.event == "worker_pr_fallback")) | length)
  }
' "$RUNS_LEDGER" 2>/dev/null || echo '{}')"

# Top poisoned tasks
poisoned="$(jq -rs --arg since "$since_ts" '
  [.[] | select(.ts > $since and (.event == "worker_failed" or .event == "worker_promotion_failed"))
    | (.detail | split("@")[0])]
  | group_by(.)
  | map({slug: .[0], count: length})
  | sort_by(-.count) | .[0:3]
' "$RUNS_LEDGER" 2>/dev/null || echo '[]')"

# Cost from CSV (if present)
cost_summary=""
if [[ -f "$AUDIT_DIR/report.csv" ]]; then
  cost_summary="$(awk -F, 'NR>1 && $1>"'"$since_ts"'" { tokens+=$12; dollars+=$13 } END { printf "tokens=%d dollars≈%.2f", tokens, dollars }' "$AUDIT_DIR/report.csv" 2>/dev/null || true)"
fi

# Recent PRs via gh (if auth'd)
pr_block=""
if command -v gh >/dev/null && gh auth status >/dev/null 2>&1; then
  pr_block="$(gh pr list --repo "$REPO_SLUG" --label autoloop-bot --limit 20 --json number,title,state,url --jq '.[] | "- #\(.number) [\(.state)] \(.title) — \(.url)"' 2>/dev/null || true)"
fi

emit_output() {
  cat <<EOF
# gormes-orchestrator daily digest

Window: $since_ts → $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Activity
EOF
  jq -r '"runs=\(.runs) claimed=\(.claimed) success=\(.success) promoted=\(.promoted) pr_opened=\(.pr_opened) pr_fallback=\(.pr_fallback)"' <<<"$stats"
  echo ""
  echo "## Failures by status"
  jq -r '.failed_by_status | to_entries[] | "- \(.key): \(.value)"' <<<"$stats"
  echo ""
  echo "## Top poisoned tasks"
  jq -r '.[] | "- \(.slug): \(.count) failures"' <<<"$poisoned"
  echo ""
  echo "## Cost"
  echo "$cost_summary"
  echo ""
  if [[ -n "$pr_block" ]]; then
    echo "## PRs from autoloop (last 20)"
    echo "$pr_block"
  fi
}

if [[ -n "$output_file" ]]; then
  emit_output > "$output_file"
  echo "Written to $output_file"
else
  emit_output
fi
