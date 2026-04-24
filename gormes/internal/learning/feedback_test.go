package learning

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFeedbackStoreAppendsOutcomeAsJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning", "feedback.jsonl")
	store := NewFeedbackStore(path)

	err := store.RecordOutcome(context.Background(), Outcome{
		SkillName:  "careful-review",
		Success:    true,
		SessionID:  "sess-123",
		OccurredAt: time.Date(2026, 4, 23, 19, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}

	var persisted Outcome
	if err := json.Unmarshal([]byte(lines[0]), &persisted); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if persisted.SkillName != "careful-review" {
		t.Fatalf("SkillName = %q, want %q", persisted.SkillName, "careful-review")
	}
	if !persisted.Success {
		t.Fatal("Success = false, want true")
	}
	if persisted.SessionID != "sess-123" {
		t.Fatalf("SessionID = %q, want %q", persisted.SessionID, "sess-123")
	}
	if !persisted.OccurredAt.Equal(time.Date(2026, 4, 23, 19, 10, 0, 0, time.UTC)) {
		t.Fatalf("OccurredAt = %v, want 2026-04-23T19:10:00Z", persisted.OccurredAt)
	}
}

func TestFeedbackStoreSkipsEmptySkillName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.jsonl")
	store := NewFeedbackStore(path)

	if err := store.RecordOutcome(context.Background(), Outcome{SkillName: "   ", Success: true}); err != nil {
		t.Fatalf("RecordOutcome(empty): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("feedback file created for empty skill name, stat err = %v", err)
	}
}

func TestFeedbackStoreAggregatesEffectivenessWithLaplaceSmoothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.jsonl")
	store := NewFeedbackStore(path)
	ctx := context.Background()

	record := func(name string, success bool) {
		t.Helper()
		if err := store.RecordOutcome(ctx, Outcome{SkillName: name, Success: success}); err != nil {
			t.Fatalf("RecordOutcome(%s,%v): %v", name, success, err)
		}
	}

	record("careful-review", true)
	record("careful-review", true)
	record("careful-review", true)
	record("careful-review", false)
	record("tdd-guard", false)
	record("tdd-guard", false)

	scores, err := store.Scores(ctx)
	if err != nil {
		t.Fatalf("Scores(): %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("len(scores) = %d, want 2", len(scores))
	}

	// Sorted by Score descending, then name ascending.
	if scores[0].SkillName != "careful-review" {
		t.Fatalf("scores[0].SkillName = %q, want careful-review", scores[0].SkillName)
	}
	if scores[1].SkillName != "tdd-guard" {
		t.Fatalf("scores[1].SkillName = %q, want tdd-guard", scores[1].SkillName)
	}

	got := scores[0]
	if got.Uses != 4 || got.Successes != 3 || got.Failures != 1 {
		t.Fatalf("careful-review counts = {Uses:%d Successes:%d Failures:%d}, want {4 3 1}", got.Uses, got.Successes, got.Failures)
	}
	// Laplace smoothing: (3+1)/(4+2) = 4/6 = 0.6666...
	if math.Abs(got.Score-4.0/6.0) > 1e-9 {
		t.Fatalf("careful-review Score = %v, want %v", got.Score, 4.0/6.0)
	}

	got = scores[1]
	// Laplace smoothing: (0+1)/(2+2) = 1/4 = 0.25
	if math.Abs(got.Score-0.25) > 1e-9 {
		t.Fatalf("tdd-guard Score = %v, want 0.25", got.Score)
	}
}

func TestFeedbackStoreScoresEmptyWhenFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.jsonl")
	store := NewFeedbackStore(path)

	scores, err := store.Scores(context.Background())
	if err != nil {
		t.Fatalf("Scores() error = %v, want nil", err)
	}
	if len(scores) != 0 {
		t.Fatalf("len(scores) = %d, want 0 for missing file", len(scores))
	}
}

func TestFeedbackStoreWeightFallsBackToPriorForUnknownSkill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedback.jsonl")
	store := NewFeedbackStore(path)
	ctx := context.Background()

	if err := store.RecordOutcome(ctx, Outcome{SkillName: "alpha", Success: true}); err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}

	// Known skill: (1+1)/(1+2) = 2/3.
	if got := store.Weight(ctx, "alpha"); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Fatalf("Weight(alpha) = %v, want %v", got, 2.0/3.0)
	}
	// Unknown skill falls back to the neutral Laplace prior 1/2.
	if got := store.Weight(ctx, "unknown-skill"); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("Weight(unknown) = %v, want 0.5", got)
	}
	// Empty skill name also returns the prior — no panic, no file read required.
	if got := store.Weight(ctx, "   "); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("Weight(blank) = %v, want 0.5", got)
	}
}
