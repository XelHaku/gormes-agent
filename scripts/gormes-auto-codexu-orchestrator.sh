#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
LEGACY_ORCHESTRATOR="$REPO_ROOT/testdata/legacy-shell/scripts/gormes-auto-codexu-orchestrator.sh"
export REPO_ROOT
cd "$REPO_ROOT"

case "${1:-}" in
  status|tail|abort|cleanup|promote-commit|verify-gh-auth|--resume)
    exec "$LEGACY_ORCHESTRATOR" "$@"
    ;;
  run|audit|digest|service)
    exec go run ./cmd/autoloop "$@"
    ;;
  *)
    exec go run ./cmd/autoloop run "$@"
    ;;
esac
