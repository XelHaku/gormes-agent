#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  TMP_WS="$(mktmp_workspace)"
  FAKE_BIN="$TMP_WS/bin"
  mkdir -p "$FAKE_BIN"

  cat > "$FAKE_BIN/systemctl" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${FAKE_SYSTEMCTL_LOG:?}"
exit 0
SH
  chmod +x "$FAKE_BIN/systemctl"

  export PATH="$FAKE_BIN:$PATH"
  export FAKE_SYSTEMCTL_LOG="$TMP_WS/systemctl.log"
  export XDG_CONFIG_HOME="$TMP_WS/config"
}

@test "install-audit writes flattened repo root into systemd service" {
  local expected_repo_root service_file
  expected_repo_root="$(cd "$ORCHESTRATOR_SCRIPTS_DIR/.." && pwd)"
  service_file="$XDG_CONFIG_HOME/systemd/user/gormes-orchestrator-audit.service"

  run env FORCE=1 AUTO_START=0 "$ORCHESTRATOR_SCRIPTS_DIR/orchestrator/install-audit.sh"
  assert_success
  [ -f "$service_file" ]

  run grep -F "Environment=REPO_ROOT=$expected_repo_root" "$service_file"
  assert_success
}
