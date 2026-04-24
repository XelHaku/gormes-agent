#!/usr/bin/env bash
# Install the gormes-orchestrator systemd --user service from the template.
# Defaults: installs, enables, and starts. Override with AUTO_START=0.
# FORCE=1 overwrites an existing unit file.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE="$SCRIPT_DIR/systemd/gormes-orchestrator.service.in"
ORCHESTRATOR_PATH="$(cd "$SCRIPT_DIR/.." && pwd)/gormes-auto-codexu-orchestrator.sh"
WORKDIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

[[ -f "$TEMPLATE" ]] || { echo "ERROR: missing template $TEMPLATE" >&2; exit 1; }
[[ -f "$ORCHESTRATOR_PATH" ]] || { echo "ERROR: orchestrator not found at $ORCHESTRATOR_PATH" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "ERROR: systemctl not available" >&2; exit 1; }

UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
UNIT_FILE="$UNIT_DIR/gormes-orchestrator.service"
mkdir -p "$UNIT_DIR"

if [[ -f "$UNIT_FILE" && "${FORCE:-0}" != "1" ]]; then
  echo "Unit already exists: $UNIT_FILE"
  echo "Re-run with FORCE=1 to overwrite."
  exit 1
fi

sed -e "s|@ORCHESTRATOR_PATH@|$ORCHESTRATOR_PATH|g" \
    -e "s|@WORKDIR@|$WORKDIR|g" \
    "$TEMPLATE" > "$UNIT_FILE"

echo "Installed unit: $UNIT_FILE"
systemctl --user daemon-reload

# Warn if a non-systemd orchestrator is already running
if pgrep -af 'gormes-auto-codexu-orchestrator\.sh' | grep -vq 'systemd'; then
  echo
  echo "WARNING: an orchestrator process is already running outside systemd:"
  pgrep -af 'gormes-auto-codexu-orchestrator\.sh' || true
  echo "Stop it (Ctrl+C in its terminal, or pkill -TERM -f gormes-auto-codexu-orchestrator\\.sh)"
  echo "BEFORE enabling the service, or the two will race on the run.lock."
fi

if [[ "${AUTO_START:-1}" == "1" ]]; then
  echo
  echo "Enabling and starting gormes-orchestrator.service..."
  systemctl --user enable --now gormes-orchestrator.service
  echo "Status:"
  systemctl --user --no-pager --lines=0 status gormes-orchestrator.service || true
  echo
  echo "Tail logs with: journalctl --user -u gormes-orchestrator -f"
else
  echo
  echo "Unit installed but not started (AUTO_START=0)."
  echo "Start with:  systemctl --user enable --now gormes-orchestrator.service"
fi
