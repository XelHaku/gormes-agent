package cron

import (
	"context"
	"strings"
	"testing"
)

func TestCronContextFromInjectsMostRecentCompletedOutputBeforePrompt(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKernel("target response", 0)
	e, _, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	source := NewJob("source", "@hourly", "Collect source data.")
	target := NewJob("target", "@daily", "Summarize the source data.")
	target.ContextFrom = []string{source.ID}
	if err := e.cfg.JobStore.Create(source); err != nil {
		t.Fatalf("create source job: %v", err)
	}
	if err := e.cfg.JobStore.Create(target); err != nil {
		t.Fatalf("create target job: %v", err)
	}
	recordContextRun(t, e.cfg.RunStore, source.ID, 100, "old source output")
	recordContextRun(t, e.cfg.RunStore, source.ID, 200, "fresh source output")

	e.Run(ctx, target)

	prompt := submittedCronPrompt(t, fk)
	if !strings.HasPrefix(prompt, CronHeartbeatPrefix) {
		t.Fatalf("submitted prompt missing cron heartbeat prefix: %q", prompt)
	}
	if !strings.Contains(prompt, "## Output from job '"+source.ID+"'") {
		t.Fatalf("submitted prompt = %q, want context header for source job", prompt)
	}
	if !strings.Contains(prompt, "fresh source output") {
		t.Fatalf("submitted prompt = %q, want most recent source output", prompt)
	}
	if strings.Contains(prompt, "old source output") {
		t.Fatalf("submitted prompt = %q, should not include older source output", prompt)
	}
	contextAt := strings.Index(prompt, "fresh source output")
	basePromptAt := strings.Index(prompt, "Summarize the source data.")
	if contextAt < 0 || basePromptAt < 0 || contextAt > basePromptAt {
		t.Fatalf("submitted prompt context index=%d base index=%d, want context before base prompt: %q", contextAt, basePromptAt, prompt)
	}
}

func TestCronContextFromTruncatesEachSourceAt8000Characters(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKernel("target response", 0)
	e, _, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	sourceA := NewJob("source-a", "@hourly", "Collect A.")
	sourceB := NewJob("source-b", "@hourly", "Collect B.")
	target := NewJob("target", "@daily", "Compare source outputs.")
	target.ContextFrom = []string{sourceA.ID, sourceB.ID}
	for _, job := range []Job{sourceA, sourceB, target} {
		if err := e.cfg.JobStore.Create(job); err != nil {
			t.Fatalf("create job %s: %v", job.Name, err)
		}
	}
	recordContextRun(t, e.cfg.RunStore, sourceA.ID, 100, strings.Repeat("a", 8050))
	recordContextRun(t, e.cfg.RunStore, sourceB.ID, 100, strings.Repeat("b", 8050))

	e.Run(ctx, target)

	prompt := submittedCronPrompt(t, fk)
	if strings.Contains(prompt, strings.Repeat("a", 8050)) || strings.Contains(prompt, strings.Repeat("b", 8050)) {
		t.Fatalf("submitted prompt contains unbounded context output")
	}
	if got := strings.Count(prompt, "[... output truncated ...]"); got != 2 {
		t.Fatalf("truncation markers = %d, want one marker per source in prompt: %q", got, prompt)
	}
	if !strings.Contains(prompt, strings.Repeat("a", 8000)) || !strings.Contains(prompt, strings.Repeat("b", 8000)) {
		t.Fatalf("submitted prompt missing bounded source content")
	}
}

func TestCronContextFromSkipsMissingInvalidAndUnreadableSources(t *testing.T) {
	ctx := context.Background()
	fk := newFakeKernel("target response", 0)
	e, deliveries, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	source := NewJob("source", "@hourly", "Collect source data.")
	target := NewJob("target", "@daily", "Use only available context.")
	target.ContextFrom = []string{"../../../etc/passwd", source.ID}
	if err := e.cfg.JobStore.Create(source); err != nil {
		t.Fatalf("create source job: %v", err)
	}
	if err := e.cfg.JobStore.Create(target); err != nil {
		t.Fatalf("create target job: %v", err)
	}

	e.Run(ctx, target)

	prompt := submittedCronPrompt(t, fk)
	if strings.Contains(prompt, "etc/passwd") || strings.Contains(prompt, "## Output from job") {
		t.Fatalf("submitted prompt included skipped context source: %q", prompt)
	}
	if !strings.Contains(prompt, "Use only available context.") {
		t.Fatalf("submitted prompt lost base prompt: %q", prompt)
	}

	recordContextRun(t, e.cfg.RunStore, source.ID, 300, "stored output that cannot be read")
	if err := e.cfg.RunStore.db.Close(); err != nil {
		t.Fatalf("close run store db: %v", err)
	}
	fk.mu.Lock()
	fk.events = nil
	fk.mu.Unlock()

	e.Run(ctx, target)

	prompt = submittedCronPrompt(t, fk)
	if strings.Contains(prompt, "stored output that cannot be read") || strings.Contains(prompt, "## Output from job") {
		t.Fatalf("submitted prompt included unreadable source output: %q", prompt)
	}
	got := deliveries.Load().([]string)
	if len(got) != 2 {
		t.Fatalf("deliveries = %d, want executor to preserve run behavior while skipping context", len(got))
	}
}

func recordContextRun(t *testing.T, rs *RunStore, jobID string, startedAt int64, output string) {
	t.Helper()
	if err := rs.RecordRun(context.Background(), Run{
		JobID:         jobID,
		StartedAt:     startedAt,
		FinishedAt:    startedAt + 1,
		PromptHash:    "context",
		Status:        "success",
		Delivered:     true,
		OutputPreview: output,
	}); err != nil {
		t.Fatalf("record context run: %v", err)
	}
}

func submittedCronPrompt(t *testing.T, fk *fakeKernel) string {
	t.Helper()
	fk.mu.Lock()
	defer fk.mu.Unlock()
	if len(fk.events) != 1 {
		t.Fatalf("submitted events = %d, want 1", len(fk.events))
	}
	return fk.events[0].Text
}
