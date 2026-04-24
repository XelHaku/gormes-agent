#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib common
  source_lib companions

  TMP_WS="$(mktmp_workspace)"
  export RUN_ROOT="$TMP_WS/run"
  export STATE_DIR="$RUN_ROOT/state"
  export LOGS_DIR="$RUN_ROOT/logs"
  mkdir -p "$STATE_DIR" "$LOGS_DIR"

  export PLANNER_EVERY_N_CYCLES=4
  export DOC_IMPROVER_EVERY_N_CYCLES=6
  export LANDINGPAGE_EVERY_N_HOURS=24
  export PLANNER_ROOT="$TMP_WS/planner"
  mkdir -p "$PLANNER_ROOT"

  export CANDIDATES_FILE="$TMP_WS/cands.json"
  echo '[]' > "$CANDIDATES_FILE"

  # Ensure companion_state_dir is computed
  export ORCH_COMPANION_STATE_DIR="$(companion_state_dir)"
  mkdir -p "$ORCH_COMPANION_STATE_DIR"
}

write_companion_state() {
  local name="$1" ts="$2" cycle="$3"
  jq -n --arg ts "$ts" --argjson cycle "$cycle" --argjson rc 0 \
    '{ts_epoch:($ts|tonumber),cycle:$cycle,rc:$rc}' \
    > "$ORCH_COMPANION_STATE_DIR/${name}.last.json"
}

@test "companion_cycles_since returns large N when never run" {
  run companion_cycles_since planner 10
  assert_success
  (( output >= 10 ))
}

@test "companion_cycles_since returns diff since last run" {
  write_companion_state planner "$(date +%s)" 5
  run companion_cycles_since planner 9
  assert_output "4"
}

@test "should_run_planner fires on exhaustion (unclaimed<10%)" {
  # 10 candidates total, 0 unclaimed
  echo '[]' > "$CANDIDATES_FILE"
  write_companion_state planner "$(date +%s)" 0
  export _TOTAL_PROGRESS_ITEMS=10
  run should_run_planner 1
  assert_success
}

@test "should_run_planner fires on cycle interval" {
  echo '[]' > "$CANDIDATES_FILE"
  write_companion_state planner "$(date +%s)" 0
  export _TOTAL_PROGRESS_ITEMS=100
  echo '[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20]' > "$CANDIDATES_FILE"
  run should_run_planner 4
  assert_success
}

@test "should_run_planner skips if external systemd ran recently" {
  cp "$FIXTURES_DIR/planner_state.fixture.json" "$PLANNER_ROOT/planner_state.json"
  # Set the fixture's last_run to just now
  jq --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '.last_run_utc = $now' "$PLANNER_ROOT/planner_state.json" \
    > "$PLANNER_ROOT/planner_state.json.tmp" && mv "$PLANNER_ROOT/planner_state.json.tmp" "$PLANNER_ROOT/planner_state.json"
  write_companion_state planner 0 0
  run should_run_planner 99
  assert_failure
}

@test "should_run_doc_improver skips when no promotions last cycle" {
  write_companion_state doc_improver 0 0
  export PROMOTED_LAST_CYCLE=0
  run should_run_doc_improver 10
  assert_failure
}

@test "should_run_doc_improver fires when interval reached + promotion happened" {
  write_companion_state doc_improver 0 0
  export PROMOTED_LAST_CYCLE=1
  run should_run_doc_improver 10
  assert_success
}

@test "should_run_landingpage fires after 24h" {
  # 25 hours ago
  write_companion_state landingpage "$(( $(date +%s) - 25 * 3600 ))" 0
  run should_run_landingpage
  assert_success
}

@test "should_run_landingpage skips within 24h" {
  write_companion_state landingpage "$(( $(date +%s) - 3600 ))" 0
  run should_run_landingpage
  assert_failure
}

@test "companion_already_running reports true for live pid, false otherwise" {
  # No pid file => not running.
  run companion_already_running planner
  assert_failure

  # Pid file pointing at a long-running fake => running.
  sleep 30 &
  local live_pid=$!
  echo "$live_pid" > "$ORCH_COMPANION_STATE_DIR/planner.pid"
  run companion_already_running planner
  assert_success

  # Kill and test that dead pid => not running.
  kill "$live_pid" 2>/dev/null || true
  wait "$live_pid" 2>/dev/null || true
  run companion_already_running planner
  assert_failure

  # Garbage pid file => not running.
  echo "not-a-pid" > "$ORCH_COMPANION_STATE_DIR/planner.pid"
  run companion_already_running planner
  assert_failure

  rm -f "$ORCH_COMPANION_STATE_DIR/planner.pid"
}

@test "companion_reap_stale drops dead pid files, keeps live ones" {
  # Live pid should survive.
  sleep 30 &
  local live_pid=$!
  echo "$live_pid" > "$ORCH_COMPANION_STATE_DIR/planner.pid"

  # Dead pid (a very unlikely PID; force-fabricate).
  echo "2147483646" > "$ORCH_COMPANION_STATE_DIR/doc_improver.pid"
  # Garbage pid.
  echo "junk" > "$ORCH_COMPANION_STATE_DIR/landingpage.pid"

  run companion_reap_stale
  assert_success

  [ -f "$ORCH_COMPANION_STATE_DIR/planner.pid" ]
  [ ! -f "$ORCH_COMPANION_STATE_DIR/doc_improver.pid" ]
  [ ! -f "$ORCH_COMPANION_STATE_DIR/landingpage.pid" ]

  kill "$live_pid" 2>/dev/null || true
  wait "$live_pid" 2>/dev/null || true
  rm -f "$ORCH_COMPANION_STATE_DIR/planner.pid"
}

@test "run_companion returns <1s even if companion takes 5s (async)" {
  export GIT_ROOT="$TMP_WS"
  local fake_dir="$TMP_WS/bin"
  mkdir -p "$fake_dir"
  cat > "$fake_dir/slow-planner" <<'EOF'
#!/usr/bin/env bash
sleep 5
EOF
  chmod +x "$fake_dir/slow-planner"
  export COMPANION_PLANNER_CMD="$fake_dir/slow-planner"
  export COMPANION_TIMEOUT_SECONDS=30

  local before after delta
  before="$(date +%s)"
  run run_companion planner
  after="$(date +%s)"
  assert_success
  delta=$(( after - before ))
  (( delta < 3 )) || { echo "run_companion took ${delta}s (expected <3)" >&2; false; }

  local pid_file="$ORCH_COMPANION_STATE_DIR/planner.pid"
  [ -f "$pid_file" ]
  local bg_pid
  bg_pid="$(cat "$pid_file")"
  [[ "$bg_pid" =~ ^[0-9]+$ ]]

  # Clean up: kill the whole session so timeout + bash -c + sleep all exit.
  if kill -0 "$bg_pid" 2>/dev/null; then
    kill -TERM -"$bg_pid" 2>/dev/null || kill -TERM "$bg_pid" 2>/dev/null || true
    sleep 0.2
    kill -KILL -"$bg_pid" 2>/dev/null || kill -KILL "$bg_pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
}

@test "run_companion --sync runs foreground and returns rc" {
  export GIT_ROOT="$TMP_WS"
  local fake_dir="$TMP_WS/bin"
  mkdir -p "$fake_dir"
  cat > "$fake_dir/quick-planner" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "$fake_dir/quick-planner"
  export COMPANION_PLANNER_CMD="$fake_dir/quick-planner"
  export COMPANION_TIMEOUT_SECONDS=30

  run run_companion planner --sync
  assert_success

  # No pid file should linger for sync mode.
  [ ! -f "$ORCH_COMPANION_STATE_DIR/planner.pid" ]
  [ -f "$ORCH_COMPANION_STATE_DIR/planner.last.json" ]
  run jq -r '.sync' "$ORCH_COMPANION_STATE_DIR/planner.last.json"
  assert_output "true"
  run jq -r '.rc' "$ORCH_COMPANION_STATE_DIR/planner.last.json"
  assert_output "0"
}
