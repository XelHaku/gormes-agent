#!/usr/bin/env bash
# Backend (agent CLI) adapter. BACKEND env var selects which agent drives
# workers. The orchestrator's contract is unchanged — workers still get the
# same prompt; each backend translates argv and output conventions.
#
# Supported: codexu (default), claudeu, opencode.
# Depends on: $BACKEND (optional; default codexu), $MODE.

build_backend_cmd() {
  local backend="${BACKEND:-codexu}"
  case "$backend" in
    codexu)
      case "$MODE" in
        safe|unattended)
          printf '%s\0' codexu exec --json \
            -m gpt-5.5 \
            -c approval_policy=never \
            --sandbox workspace-write
          ;;
        full)
          printf '%s\0' codexu exec --json \
            -m gpt-5.5 \
            -c approval_policy=never \
            --sandbox danger-full-access
          ;;
        *)
          echo "ERROR: invalid MODE=$MODE" >&2
          return 1
          ;;
      esac
      ;;
    claudeu)
      # claudeu shim already translates the argv; it accepts the same flag
      # shape so we emit the exact same argv as codexu. The shim on PATH
      # does the translation to `claude --print`.
      case "$MODE" in
        safe|unattended)
          printf '%s\0' claudeu exec --json \
            -c approval_policy=never \
            --sandbox workspace-write
          ;;
        full)
          printf '%s\0' claudeu exec --json \
            -c approval_policy=never \
            --sandbox danger-full-access
          ;;
        *)
          echo "ERROR: invalid MODE=$MODE" >&2
          return 1
          ;;
      esac
      ;;
    opencode)
      # opencode's non-interactive invocation. Command shape approximate;
      # update when real opencode CLI surface is tested.
      printf '%s\0' opencode run --no-interactive
      ;;
    *)
      echo "ERROR: unknown BACKEND=$backend (supported: codexu|claudeu|opencode)" >&2
      return 1
      ;;
  esac
}

# Legacy alias — kept one release so entry script still calls build_codex_cmd
# without breaking. Remove in Oil Release 2.
build_codex_cmd() {
  build_backend_cmd "$@"
}
