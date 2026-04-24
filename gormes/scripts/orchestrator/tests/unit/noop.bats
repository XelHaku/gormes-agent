#!/usr/bin/env bats

load '../lib/test_env'

setup() {
  load_helpers
}

@test "harness runs" {
  run true
  assert_success
}
