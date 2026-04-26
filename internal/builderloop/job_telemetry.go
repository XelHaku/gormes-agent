package builderloop

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

const maxJobOutputTailBytes = 1200

var secretTelemetryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[^\s]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)\s*=\s*)[^\s]+`),
}

var failureEvidenceMarkers = []string{
	"--- FAIL:",
	"\nFAIL\t",
	"\nFAIL ",
	"\nFAIL\n",
	"panic:",
	"undefined:",
	"Error:",
	"error:",
}

type jobSpec struct {
	ID            string
	Kind          string
	Attempt       int
	Command       string
	Dir           string
	Worker        int
	Task          string
	Branch        string
	FailureStatus string
}

func runLoggedJob(ctx context.Context, cfg Config, runner Runner, runID string, spec jobSpec, command Command) Result {
	start := time.Now().UTC()
	spec = normalizeJobSpec(spec, command)
	appendJobStarted(cfg, runID, spec, start)

	result := runner.Run(ctx, command)
	appendJobFinished(cfg, runID, spec, start, result)
	return result
}

func runLoggedOperation(cfg Config, runID string, spec jobSpec, fn func() Result) Result {
	start := time.Now().UTC()
	spec = normalizeJobSpec(spec, Command{})
	appendJobStarted(cfg, runID, spec, start)
	result := fn()
	appendJobFinished(cfg, runID, spec, start, result)
	return result
}

func appendJobStarted(cfg Config, runID string, spec jobSpec, start time.Time) {
	_ = appendRunLedgerEvent(cfg, LedgerEvent{
		TS:        start,
		RunID:     runID,
		Event:     "job_started",
		Worker:    spec.Worker,
		Task:      spec.Task,
		Branch:    spec.Branch,
		Status:    "started",
		JobID:     spec.ID,
		JobKind:   spec.Kind,
		Attempt:   spec.Attempt,
		Command:   spec.Command,
		Dir:       spec.Dir,
		StartedAt: start.Format(time.RFC3339Nano),
	})
}

func appendJobFinished(cfg Config, runID string, spec jobSpec, start time.Time, result Result) {
	finished := time.Now().UTC()
	durationMS := finished.Sub(start).Milliseconds()
	if durationMS <= 0 {
		durationMS = 1
	}
	event := LedgerEvent{
		TS:          finished,
		RunID:       runID,
		Event:       "job_finished",
		Worker:      spec.Worker,
		Task:        spec.Task,
		Branch:      spec.Branch,
		Status:      jobStatus(result.Err, spec.FailureStatus),
		JobID:       spec.ID,
		JobKind:     spec.Kind,
		Attempt:     spec.Attempt,
		Command:     spec.Command,
		Dir:         spec.Dir,
		StartedAt:   start.Format(time.RFC3339Nano),
		DurationMS:  durationMS,
		StdoutBytes: len(result.Stdout),
		StderrBytes: len(result.Stderr),
	}
	if result.Err != nil {
		event.ExitError = result.Err.Error()
		event.StdoutTail = boundedRedactedTail(result.Stdout)
		event.StderrTail = boundedRedactedTail(result.Stderr)
	}
	_ = appendRunLedgerEvent(cfg, event)
}

func normalizeJobSpec(spec jobSpec, command Command) jobSpec {
	if spec.Command == "" {
		spec.Command = command.Name
		if len(command.Args) > 0 {
			spec.Command += " " + strings.Join(command.Args, " ")
		}
	}
	spec.Command = sanitizeTelemetryCommand(spec.Command)
	if spec.Dir == "" {
		spec.Dir = command.Dir
	}
	return spec
}

func sanitizeTelemetryCommand(command string) string {
	command = redactTelemetry(command)
	const maxCommandBytes = 240
	if len(command) <= maxCommandBytes {
		return command
	}
	return strings.TrimSpace(command[:maxCommandBytes]) + " ... [truncated]"
}

func jobStatus(err error, failureStatus string) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case failureStatus != "":
		return failureStatus
	default:
		return "failed"
	}
}

func tailString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func boundedRedactedTail(value string) string {
	return tailString(failureEvidenceWindow(redactTelemetry(value), maxJobOutputTailBytes), maxJobOutputTailBytes)
}

func redactTelemetry(value string) string {
	out := value
	for _, pattern := range secretTelemetryPatterns {
		out = pattern.ReplaceAllString(out, "${1}[REDACTED]")
	}
	return out
}

func failureEvidenceWindow(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	idx := firstFailureEvidenceMarker(value)
	if idx < 0 {
		return tailString(value, limit)
	}

	start := idx - 200
	if start < 0 {
		start = 0
	}
	prefix := ""
	if start > 0 {
		prefix = "... [head elided]\n"
	}
	suffix := "\n... [tail elided]"
	budget := limit - len(prefix) - len(suffix)
	if budget < 1 {
		budget = limit - len(prefix)
		suffix = ""
	}
	end := start + budget
	if end > len(value) {
		end = len(value)
		suffix = ""
	}
	return tailString(prefix+value[start:end]+suffix, limit)
}

func firstFailureEvidenceMarker(value string) int {
	first := -1
	for _, marker := range failureEvidenceMarkers {
		idx := strings.Index(value, marker)
		if idx < 0 {
			continue
		}
		if first < 0 || idx < first {
			first = idx
		}
	}
	return first
}
