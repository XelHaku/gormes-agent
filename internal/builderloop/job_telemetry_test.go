package builderloop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLedgerEvent_JobTelemetryFieldsRoundTrip(t *testing.T) {
	eventTime := time.Date(2026, 4, 26, 4, 0, 0, 0, time.UTC)
	event := LedgerEvent{
		TS:          eventTime,
		RunID:       "run-123",
		Event:       "job_finished",
		Worker:      2,
		Task:        "4/4.A/Azure Foundry",
		Status:      "failed",
		JobID:       "run-123/post-verify/1/2",
		JobKind:     "post_verify_command",
		Attempt:     1,
		Command:     "go test ./...",
		Dir:         "/repo",
		StartedAt:   eventTime.Format(time.RFC3339Nano),
		DurationMS:  1234,
		ExitError:   "exit status 1",
		StdoutTail:  "stdout tail",
		StderrTail:  "stderr tail",
		StdoutBytes: 4096,
		StderrBytes: 128,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got LedgerEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.JobID != event.JobID ||
		got.JobKind != event.JobKind ||
		got.Attempt != event.Attempt ||
		got.Command != event.Command ||
		got.Dir != event.Dir ||
		got.StartedAt != event.StartedAt ||
		got.DurationMS != event.DurationMS ||
		got.ExitError != event.ExitError ||
		got.StdoutTail != event.StdoutTail ||
		got.StderrTail != event.StderrTail ||
		got.StdoutBytes != event.StdoutBytes ||
		got.StderrBytes != event.StderrBytes {
		t.Fatalf("job telemetry round trip mismatch:\n got: %+v\nwant: %+v", got, event)
	}
}

func TestRunLoggedJobEmitsCompactSuccessEvents(t *testing.T) {
	runRoot := t.TempDir()
	cfg := Config{RunRoot: runRoot}
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		return Result{
			Stdout: strings.Repeat("success output\n", 100),
			Stderr: "warning that should not be copied on success",
		}
	})

	result := runLoggedJob(context.Background(), cfg, runner, "run-1", jobSpec{
		ID:      "run-1/post-verify/1/1",
		Kind:    "post_verify_command",
		Attempt: 1,
		Command: "go test ./...",
		Dir:     "/repo",
	}, Command{Name: "sh", Args: []string{"-lc", "go test ./..."}, Dir: "/repo"})
	if result.Err != nil {
		t.Fatalf("runLoggedJob() error = %v", result.Err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2: %+v", len(events), events)
	}
	if events[0].Event != "job_started" || events[0].Status != "started" {
		t.Fatalf("start event = %+v, want job_started/started", events[0])
	}
	finished := events[1]
	if finished.Event != "job_finished" || finished.Status != "ok" {
		t.Fatalf("finish event = %+v, want job_finished/ok", finished)
	}
	if finished.JobID != "run-1/post-verify/1/1" || finished.JobKind != "post_verify_command" {
		t.Fatalf("finish event missing job identity: %+v", finished)
	}
	if finished.DurationMS <= 0 {
		t.Fatalf("DurationMS = %d, want positive duration evidence", finished.DurationMS)
	}
	if finished.StdoutBytes == 0 || finished.StderrBytes == 0 {
		t.Fatalf("byte counts missing: stdout=%d stderr=%d", finished.StdoutBytes, finished.StderrBytes)
	}
	if finished.StdoutTail != "" || finished.StderrTail != "" {
		t.Fatalf("success event copied output tails: %+v", finished)
	}
}

func TestRunLoggedJobFailureIncludesBoundedRedactedEvidence(t *testing.T) {
	runRoot := t.TempDir()
	cfg := Config{RunRoot: runRoot}
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		return Result{
			Stdout: strings.Repeat("line\n", 300) + "OPENAI_API_KEY=sk-secret\n",
			Stderr: strings.Repeat("stderr\n", 300) + "Authorization: Bearer top-secret\nfinal failure\n",
			Err:    errors.New("exit status 1"),
		}
	})

	result := runLoggedJob(context.Background(), cfg, runner, "run-1", jobSpec{
		ID:      "run-1/worker/1",
		Kind:    "worker_backend",
		Command: "codexu exec <prompt>",
		Dir:     "/repo",
	}, Command{Name: "codexu", Args: []string{"exec", strings.Repeat("prompt ", 500)}, Dir: "/repo"})
	if result.Err == nil {
		t.Fatal("runLoggedJob() error = nil, want backend error returned")
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	finished := events[len(events)-1]
	if finished.Status != "failed" || finished.ExitError != "exit status 1" {
		t.Fatalf("finish event = %+v, want failed exit evidence", finished)
	}
	if strings.Contains(finished.StdoutTail, "sk-secret") || strings.Contains(finished.StderrTail, "top-secret") {
		t.Fatalf("secret leaked in output tails: stdout=%q stderr=%q", finished.StdoutTail, finished.StderrTail)
	}
	if !strings.Contains(finished.StdoutTail, "[REDACTED]") || !strings.Contains(finished.StderrTail, "[REDACTED]") {
		t.Fatalf("redaction marker missing: stdout=%q stderr=%q", finished.StdoutTail, finished.StderrTail)
	}
	if len(finished.StdoutTail) > maxJobOutputTailBytes || len(finished.StderrTail) > maxJobOutputTailBytes {
		t.Fatalf("tails not bounded: stdout=%d stderr=%d", len(finished.StdoutTail), len(finished.StderrTail))
	}
	if strings.Contains(finished.Command, "prompt prompt prompt") {
		t.Fatalf("command field contains prompt body: %q", finished.Command)
	}
}

func TestRunLoggedJobFailureKeepsFailureMarkerWhenTailIsNoisy(t *testing.T) {
	runRoot := t.TempDir()
	cfg := Config{RunRoot: runRoot}
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		return Result{
			Stdout: "--- FAIL: TestImportant (0.01s)\n    important_test.go:42: real failure\n" + strings.Repeat("ok  \tgithub.com/example/package\t0.001s\n", 200),
			Err:    errors.New("exit status 1"),
		}
	})

	result := runLoggedJob(context.Background(), cfg, runner, "run-1", jobSpec{
		ID:      "run-1/post-verify/1/1",
		Kind:    "post_verify_command",
		Command: "go test ./...",
		Dir:     "/repo",
	}, Command{Name: "sh", Args: []string{"-lc", "go test ./..."}, Dir: "/repo"})
	if result.Err == nil {
		t.Fatal("runLoggedJob() error = nil, want command failure")
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	finished := events[len(events)-1]
	if !strings.Contains(finished.StdoutTail, "--- FAIL: TestImportant") || !strings.Contains(finished.StdoutTail, "important_test.go:42") {
		t.Fatalf("stdout evidence missing failure marker:\n%s", finished.StdoutTail)
	}
	if len(finished.StdoutTail) > maxJobOutputTailBytes {
		t.Fatalf("stdout evidence too long: %d", len(finished.StdoutTail))
	}
}

func TestRunLoggedJobTelemetryFailureDoesNotFailJob(t *testing.T) {
	dir := t.TempDir()
	runRootFile := filepath.Join(dir, "runroot-file")
	if err := os.WriteFile(runRootFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{RunRoot: runRootFile}
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		return Result{Stdout: "ok"}
	})

	result := runLoggedJob(context.Background(), cfg, runner, "run-1", jobSpec{
		ID:      "run-1/post-verify/1/1",
		Kind:    "post_verify_command",
		Command: "true",
		Dir:     "/repo",
	}, Command{Name: "sh", Args: []string{"-lc", "true"}, Dir: "/repo"})
	if result.Err != nil {
		t.Fatalf("runLoggedJob() error = %v, want underlying command result despite telemetry failure", result.Err)
	}
}
