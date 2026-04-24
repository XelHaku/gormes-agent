#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
  source_lib refill

  TMP_WS="$(mktmp_workspace)"
  export STATE_DIR="$TMP_WS/state"
  mkdir -p "$STATE_DIR"
  export REFILL_STATE_FILE="$STATE_DIR/refill-state.json"
}

@test "should_skip_refill: streak=0 always runs" {
  run should_skip_refill 0 0 1000 30
  assert_failure
  assert_output "run 0"
}

@test "should_skip_refill: streak=5 within cooldown skips" {
  # cool = 5 * 30 = 150s; elapsed = 10s -> skip, 140s remaining
  run should_skip_refill 5 990 1000 30
  assert_success
  assert_output "skip 140"
}

@test "should_skip_refill: streak=5 past cooldown runs" {
  # cool = 150s; elapsed = 200s -> run
  run should_skip_refill 5 800 1000 30
  assert_failure
  assert_output "run 0"
}

@test "should_skip_refill: cooldown caps at 3600s" {
  # streak=500, loop_sleep=30 -> raw cool = 15000, capped to 3600
  # elapsed=100 -> skip, remaining = 3500
  run should_skip_refill 500 900 1000 30
  assert_success
  assert_output "skip 3500"
}

@test "refill state round-trips" {
  write_refill_state 7 12345
  run read_refill_state
  assert_success
  assert_output $'7\t12345'
}

@test "read_refill_state: missing file returns zeros" {
  rm -f "$REFILL_STATE_FILE"
  run read_refill_state
  assert_success
  assert_output $'0\t0'
}
