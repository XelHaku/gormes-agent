package builderloop

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

func TestExecRunnerPreservesInheritedEnvWithOverlay(t *testing.T) {
	t.Setenv("AUTOLOOP_INHERITED_ENV", "parent")

	result := ExecRunner{}.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", `printf "%s:%s" "$AUTOLOOP_INHERITED_ENV" "$AUTOLOOP_OVERLAY_ENV"`},
		Env:  []string{"AUTOLOOP_OVERLAY_ENV=child"},
	})
	if result.Err != nil {
		t.Fatalf("Run() error = %v, stderr = %q", result.Err, result.Stderr)
	}

	if result.Stdout != "parent:child" {
		t.Fatalf("Stdout = %q, want %q", result.Stdout, "parent:child")
	}
}

func TestFakeRunnerReturnsErrorWhenResultsExhausted(t *testing.T) {
	runner := &FakeRunner{}

	result := runner.Run(context.Background(), Command{Name: "unexpected"})

	if result.Err == nil {
		t.Fatal("Run() error = nil, want error")
	}

	if !errors.Is(result.Err, ErrUnexpectedCommand) {
		t.Fatalf("Run() error = %q, want %q", result.Err, ErrUnexpectedCommand)
	}

	if got, want := len(runner.Commands), 1; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
}
