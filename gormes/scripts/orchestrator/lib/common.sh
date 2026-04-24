#!/usr/bin/env bash
# Common logging, path, and small utility helpers for the orchestrator.
# Sourced by gormes-auto-codexu-orchestrator.sh and its tests.
# Depends on: $VERBOSE (reads; default 0 if unset).

# Verbose logging functions
log_info() {
  if [[ "$VERBOSE" == "1" ]]; then
    echo "[INFO] $(date '+%H:%M:%S') $*" >&2
  fi
}

log_debug() {
  if [[ "$VERBOSE" == "1" ]]; then
    echo "[DEBUG] $(date '+%H:%M:%S') $*" >&2
  fi
}

log_warn() {
  echo "[WARN] $(date '+%H:%M:%S') $*" >&2
}

log_error() {
  echo "[ERROR] $(date '+%H:%M:%S') $*" >&2
}

# Progress indicator
show_progress() {
  local current=$1
  local total=$2
  local label="${3:-Progress}"
  local width=50
  local percentage=$((current * 100 / total))
  local filled=$((width * current / total))
  local empty=$((width - filled))

  printf "\r%s [%s%s] %3d%% (%d/%d)" \
    "$label" \
    "$(printf '%*s' "$filled" '' | tr ' ' '=')" \
    "$(printf '%*s' "$empty" '' | tr ' ' ' ')" \
    "$percentage" \
    "$current" \
    "$total"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ERROR: missing required command: $1" >&2
    exit 1
  }
}

safe_path_token() {
  printf '%s' "$1" | sed -E 's#[^A-Za-z0-9._-]+#-#g; s#^-+##; s#-+$##'
}

available_mem_mb() {
  free -m | awk '/^Mem:/ { print $7 }'
}

classify_worker_failure() {
  local rc="$1"
  if [[ "$rc" == "124" ]]; then
    printf 'timeout\n'
  elif [[ "$rc" == "137" ]]; then
    printf 'killed\n'
  elif [[ "$rc" == "1" ]]; then
    printf 'contract_or_test_failure\n'
  else
    printf 'worker_error\n'
  fi
}
