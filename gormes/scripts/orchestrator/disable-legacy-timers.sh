#!/usr/bin/env bash
# Disable legacy planner systemd timers and cron entries so the orchestrator's
# companion seam is the sole scheduler for planner / doc-improver / landing-page.
# Safe to re-run; no-ops on things that are already gone.
set -Eeuo pipefail

command -v systemctl >/dev/null || { echo "ERROR: systemctl not available" >&2; exit 1; }

LEGACY_TIMERS=(
  "gormes-architecture-planner-tasks-manager.timer"
  "gormes-architectureplanneragent.timer"
)

for unit in "${LEGACY_TIMERS[@]}"; do
  if systemctl --user list-unit-files --no-legend 2>/dev/null | awk '{print $1}' | grep -Fxq "$unit"; then
    systemctl --user disable --now "$unit" 2>/dev/null || true
    echo "Disabled systemd user timer: $unit"
  else
    echo "Not installed (OK): $unit"
  fi
done

# Cron: remove any line that runs one of the companion scripts
CRON_PATTERNS=(
  'gormes-architecture-planner-tasks-manager\.sh'
  'documentation-improver\.sh'
  'landingpage-improver\.sh'
)

if command -v crontab >/dev/null 2>&1 && crontab -l >/dev/null 2>&1; then
  current="$(crontab -l 2>/dev/null || true)"
  filtered="$current"
  removed=0
  for pat in "${CRON_PATTERNS[@]}"; do
    before="$filtered"
    filtered="$(printf '%s\n' "$before" | grep -Ev "$pat" || true)"
    if [[ "$before" != "$filtered" ]]; then
      removed=$((removed + 1))
    fi
  done
  if (( removed > 0 )); then
    printf '%s\n' "$filtered" | crontab -
    echo "Removed $removed cron line(s) referencing companion scripts"
  else
    echo "No matching cron entries (OK)"
  fi
else
  echo "No user crontab (OK)"
fi

echo
echo "Legacy timers disabled. The orchestrator's companion seam is now the sole scheduler."
echo "To re-enable the planner timer later: bash scripts/gormes-architecture-planner-tasks-manager.sh install-schedule"
