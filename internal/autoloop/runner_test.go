package autoloop

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFakeRunnerCapturesCommand(t *testing.T) {
	want := Command{
		Name: "codex",
		Args: []string{"exec", "task"},
		Dir:  "/tmp/repo",
		Env:  []string{"A=B"},
	}
	wantErr := errors.New("boom")
	runner := &FakeRunner{
		Results: []Result{
			{Stdout: "ok", Stderr: "warn", Err: wantErr},
		},
	}

	got := runner.Run(context.Background(), want)

	if !reflect.DeepEqual(got, (Result{Stdout: "ok", Stderr: "warn", Err: wantErr})) {
		t.Fatalf("Run() = %#v, want queued result", got)
	}

	if !reflect.DeepEqual(runner.Commands, []Command{want}) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, []Command{want})
	}

	if len(runner.Results) != 0 {
		t.Fatalf("Results length = %d, want 0", len(runner.Results))
	}
}
