#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=${REPO_ROOT:-$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)}
SERVICE=${GORMES_ORCHESTRATOR_SERVICE:-gormes-orchestrator.service}

log() {
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

cd "$REPO_ROOT"

LOCK_BASE=${XDG_RUNTIME_DIR:-/tmp}
mkdir -p "$LOCK_BASE"
LOCK_DIR="$LOCK_BASE/gormes-orchestrator-watchdog.lock"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  log "watchdog already running; skipping overlapping tick"
  exit 0
fi
cleanup() {
  rmdir "$LOCK_DIR" 2>/dev/null || true
}
trap cleanup EXIT
trap 'cleanup; exit 129' HUP
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM

checkpoint_dirty() {
  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    return 0
  fi

  dirty=$(git status --porcelain 2>/dev/null || true)
  if [ -z "$dirty" ]; then
    return 0
  fi

  ts=$(date -u '+%Y%m%dT%H%M%SZ')
  log "dirty worktree detected; committing watchdog checkpoint"
  if ! git add -A; then
    log "git add failed during watchdog checkpoint"
    return 0
  fi

  last_subject=$(git log -1 --pretty=%s 2>/dev/null || true)
  case "$last_subject" in
    "builder-loop: watchdog checkpoint "*|"builder-loop: checkpoint dirty worktree "*)
      if ! git -c user.name="Gormes Builder Loop" -c user.email="builder-loop@gormes.local" -c commit.gpgsign=false commit --amend --no-edit; then
        log "git commit failed or had nothing to commit during watchdog checkpoint"
      fi
      return 0
      ;;
  esac

  if ! git -c user.name="Gormes Builder Loop" -c user.email="builder-loop@gormes.local" -c commit.gpgsign=false commit -m "builder-loop: watchdog checkpoint $ts"; then
    log "git commit failed or had nothing to commit during watchdog checkpoint"
  fi
}

restart_service() {
  reason=$1
  log "$reason; restarting $SERVICE"
  if ! systemctl --user reset-failed "$SERVICE"; then
    log "reset-failed failed for $SERVICE"
  fi
  if ! systemctl --user restart "$SERVICE"; then
    log "restart failed for $SERVICE"
  fi
}

run_lock_path() {
  if [ -n "${PLANNER_RUN_LOCK:-}" ]; then
    printf '%s\n' "$PLANNER_RUN_LOCK"
    return 0
  fi
  if [ -f "$REPO_ROOT/.codex/planner-loop/run.lock" ]; then
    printf '%s\n' "$REPO_ROOT/.codex/planner-loop/run.lock"
    return 0
  fi
  if [ -f "$REPO_ROOT/.codex/architecture-planner/run.lock" ]; then
    printf '%s\n' "$REPO_ROOT/.codex/architecture-planner/run.lock"
    return 0
  fi
  printf '%s\n' "$REPO_ROOT/.codex/planner-loop/run.lock"
}

check_dead_run_lock() {
  lock_path=$(run_lock_path)
  if [ ! -f "$lock_path" ]; then
    return 0
  fi
  pid=$(awk -F= '$1 == "pid" {print $2; exit}' "$lock_path" 2>/dev/null || true)
  case "$pid" in
    ""|*[!0-9]*)
      return 0
      ;;
  esac
  if kill -0 "$pid" 2>/dev/null; then
    log "run lock pid $pid live"
    return 0
  fi
  restart_service "run lock pid $pid is not live ($lock_path)"
}

run_repair_check() {
  label=$1
  shift
  if "$@"; then
    log "$label ok"
    return 0
  fi
  restart_service "$label failed"
  return 0
}

checkpoint_dirty

if systemctl --user is-active --quiet "$SERVICE"; then
  log "$SERVICE active"
else
  restart_service "$SERVICE inactive"
fi

check_dead_run_lock

run_repair_check "builder-loop doctor" go run ./cmd/builder-loop doctor
run_repair_check "planner-loop doctor" go run ./cmd/planner-loop doctor

if go run ./cmd/builder-loop audit; then
  log "builder-loop audit ok"
else
  log "builder-loop audit failed"
fi

checkpoint_dirty
