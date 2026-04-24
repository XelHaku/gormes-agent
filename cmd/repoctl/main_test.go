package main

import "testing"

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"nope"}); err == nil {
		t.Fatal("run returned nil for unknown command")
	}
}
