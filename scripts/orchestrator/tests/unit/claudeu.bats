#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
}

@test "claudeu ignores codex ephemeral flag and preserves positional prompt" {
  local ws fakebin final log
  ws="$(mktmp_workspace)"
  fakebin="$ws/bin"
  mkdir -p "$fakebin"
  final="$ws/final.md"
  log="$ws/claude-stdin.log"

  cat > "$fakebin/claude" <<'SH'
#!/usr/bin/env bash
stdin="$(cat)"
printf '%s' "$stdin" > "${FAKE_CLAUDE_STDIN_LOG:?}"
printf '{"type":"result","subtype":"success","result":"claude final","usage":{"input_tokens":1,"output_tokens":1}}\n'
exit 0
SH
  chmod +x "$fakebin/claude"

  run env \
    PATH="$fakebin:/usr/local/bin:/usr/bin:/bin" \
    HOME="$ws/home" \
    CLAUDEU_LOG_DIR="$ws/claudeu-cache" \
    FAKE_CLAUDE_STDIN_LOG="$log" \
    "$ORCHESTRATOR_SCRIPTS_DIR/orchestrator/claudeu" \
    exec --json --ephemeral -c approval_policy=never --sandbox workspace-write \
    --output-last-message "$final" \
    "prompt body"

  assert_success
  grep -Fxq 'prompt body' "$log"
  grep -Fxq 'claude final' "$final"
}

@test "claudeu falls back to real codexu on live usage-limit reset message" {
  local ws fakebin final log
  ws="$(mktmp_workspace)"
  fakebin="$ws/bin"
  mkdir -p "$fakebin"
  final="$ws/final.md"
  log="$ws/codexu.log"

  cat > "$fakebin/claude" <<'SH'
#!/usr/bin/env bash
cat >/dev/null
printf "You've hit your limit · resets 8:20am (America/Monterrey)\n"
exit 1
SH
  chmod +x "$fakebin/claude"

  cat > "$fakebin/codexu" <<'SH'
#!/usr/bin/env bash
log="${FAKE_CODEXU_FALLBACK_LOG:?}"
final=""
while (( $# > 0 )); do
  case "$1" in
    --output-last-message)
      final="${2:-}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf 'fallback-codexu-called\n' >> "$log"
[[ -n "$final" ]] && printf 'codexu fallback final\n' > "$final"
printf '{"type":"thread.started","thread_id":"codexu-fallback"}\n'
exit 0
SH
  chmod +x "$fakebin/codexu"

  run env \
    PATH="$fakebin:/usr/local/bin:/usr/bin:/bin" \
    HOME="$ws/home" \
    CLAUDEU_LOG_DIR="$ws/claudeu-cache" \
    FAKE_CODEXU_FALLBACK_LOG="$log" \
    "$ORCHESTRATOR_SCRIPTS_DIR/orchestrator/claudeu" \
    exec --json -c approval_policy=never --sandbox workspace-write \
    --output-last-message "$final" \
    "prompt body"

  assert_success
  assert_output --partial 'thread_id":"codexu-fallback'
  grep -Fq 'fallback-codexu-called' "$log"
  grep -Fq 'codexu fallback final' "$final"
}

@test "claudeu fallback repairs PATH when codex is only installed under nvm" {
  local ws fakebin nvm_bin final log
  ws="$(mktmp_workspace)"
  fakebin="$ws/bin"
  nvm_bin="$ws/home/.nvm/versions/node/v22.21.1/bin"
  mkdir -p "$fakebin" "$nvm_bin"
  final="$ws/final.md"
  log="$ws/codex.log"

  cat > "$fakebin/claude" <<'SH'
#!/usr/bin/env bash
cat >/dev/null
printf "You've hit your limit · resets 8:20am (America/Monterrey)\n"
exit 1
SH
  chmod +x "$fakebin/claude"

  cat > "$fakebin/codexu" <<'SH'
#!/usr/bin/env bash
exec codex "$@"
SH
  chmod +x "$fakebin/codexu"

  cat > "$nvm_bin/codex" <<'SH'
#!/usr/bin/env bash
log="${FAKE_CODEX_BIN_LOG:?}"
final=""
while (( $# > 0 )); do
  case "$1" in
    --output-last-message)
      final="${2:-}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf 'nvm-codex-called\n' >> "$log"
[[ -n "$final" ]] && printf 'nvm codex final\n' > "$final"
printf '{"type":"thread.started","thread_id":"nvm-codex"}\n'
exit 0
SH
  chmod +x "$nvm_bin/codex"

  run env \
    PATH="$fakebin:/usr/local/bin:/usr/bin:/bin" \
    HOME="$ws/home" \
    CLAUDEU_LOG_DIR="$ws/claudeu-cache" \
    FAKE_CODEX_BIN_LOG="$log" \
    "$ORCHESTRATOR_SCRIPTS_DIR/orchestrator/claudeu" \
    exec --json -c approval_policy=never --sandbox workspace-write \
    --output-last-message "$final" \
    "prompt body"

  assert_success
  assert_output --partial 'thread_id":"nvm-codex'
  grep -Fq 'nvm-codex-called' "$log"
  grep -Fq 'nvm codex final' "$final"
}
