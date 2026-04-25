#!/usr/bin/env bash
# Wrapper for the architecture-planner-loop one-shot. Used by the systemd timer
# installed via `architecture-planner-loop service install`. Keeps the planner
# in a known directory and forwards any extra args to the Go command.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

: "${BACKEND:=codexu}"
: "${MODE:=safe}"
export BACKEND MODE

exec go run ./cmd/architecture-planner-loop run "$@"
