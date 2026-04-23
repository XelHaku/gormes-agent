package main

import "testing"

func TestNewRootCommand_IncludesACPSubcommand(t *testing.T) {
	root := newRootCommand()
	if _, _, err := root.Find([]string{"acp"}); err != nil {
		t.Fatalf("root.Find(acp): %v", err)
	}
}
