#!/usr/bin/env bash
# Install + enable the gormes-orchestrator-audit timer (runs every 20 min).
# FORCE=1 overwrites existing unit files.
# AUTO_START=0 skips the enable --now step (default: 1).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SVC_TEMPLATE="$SCRIPT_DIR/systemd/gormes-orchestrator-audit.service.in"
TIMER_TEMPLATE="$SCRIPT_DIR/systemd/gormes-orchestrator-audit.timer.in"
AUDIT_PATH="$SCRIPT_DIR/audit.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

for f in "$SVC_TEMPLATE" "$TIMER_TEMPLATE" "$AUDIT_PATH"; do
  [[ -f "$f" ]] || { echo "ERROR: missing $f" >&2; exit 1; }
done
[[ -x "$AUDIT_PATH" ]] || chmod +x "$AUDIT_PATH"
command -v systemctl >/dev/null || { echo "ERROR: systemctl not found" >&2; exit 1; }

UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
SVC_FILE="$UNIT_DIR/gormes-orchestrator-audit.service"
TIMER_FILE="$UNIT_DIR/gormes-orchestrator-audit.timer"
mkdir -p "$UNIT_DIR"

install_file() {
  local template="$1" target="$2"
  if [[ -f "$target" && "${FORCE:-0}" != "1" ]]; then
    echo "Unit already exists: $target (set FORCE=1 to overwrite)"
    return 0
  fi
  sed -e "s|@AUDIT_PATH@|$AUDIT_PATH|g" \
      -e "s|@REPO_ROOT@|$REPO_ROOT|g" \
      "$template" > "$target"
  echo "Installed: $target"
}

install_file "$SVC_TEMPLATE" "$SVC_FILE"
install_file "$TIMER_TEMPLATE" "$TIMER_FILE"
systemctl --user daemon-reload

if [[ "${AUTO_START:-1}" == "1" ]]; then
  systemctl --user enable --now gormes-orchestrator-audit.timer
  echo
  echo "Timer status:"
  systemctl --user list-timers gormes-orchestrator-audit.timer --no-pager || true
  echo
  echo "Running an initial audit now..."
  systemctl --user start gormes-orchestrator-audit.service
  sleep 1
  echo "--- latest audit output ---"
  journalctl --user -u gormes-orchestrator-audit --no-pager -n 40 | tail -30
else
  echo "Timer installed but not enabled (AUTO_START=0)."
  echo "Enable with: systemctl --user enable --now gormes-orchestrator-audit.timer"
fi
