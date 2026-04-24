package learning

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

type fakeLLM struct {
	lastPrompt string
	response   DistillResponse
	err        error
	calls      int
}

func (f *fakeLLM) Distill(_ context.Context, prompt string) (DistillResponse, error) {
	f.calls++
	f.lastPrompt = prompt
	if f.err != nil {
		return DistillResponse{}, f.err
	}
	return f.response, nil
}

func TestExtractorSkipsSignalsBelowThreshold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning", "candidates.jsonl")
	llm := &fakeLLM{response: DistillResponse{Name: "noop", Description: "x", Body: "y"}}
	extractor := NewExtractor(llm, path)

	cand, extracted, err := extractor.Extract(context.Background(), Source{
		Signal: Signal{WorthLearning: false, Score: 0, Threshold: 2},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil", err)
	}
	if extracted {
		t.Fatal("extracted = true, want false for below-threshold signal")
	}
	if cand.Name != "" {
		t.Fatalf("Candidate = %#v, want zero value", cand)
	}
	if llm.calls != 0 {
		t.Fatalf("llm.calls = %d, want 0 (extractor must not consult LLM for skipped turns)", llm.calls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("candidate log created for skipped signal, stat err = %v", err)
	}
}

func TestExtractorDistillsWorthLearningTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning", "candidates.jsonl")
	llm := &fakeLLM{
		response: DistillResponse{
			Name:        "debug-restart-loop",
			Description: "Diagnose and patch a kernel restart loop using tracing + focused tests.",
			Body:        "## Steps\n1. Trace the failure.\n2. Read the restart path.\n3. Run the targeted tests until green.",
		},
	}
	extractor := NewExtractor(llm, path)

	fixed := time.Date(2026, 4, 23, 20, 30, 0, 0, time.UTC)
	extractor.SetClock(func() time.Time { return fixed })

	src := Source{
		Signal: Signal{
			WorthLearning: true,
			Score:         3,
			Threshold:     2,
			Reasons:       []string{"tool_calls", "multi_tool_calls", "duration"},
			ToolNames:     []string{"read_file", "run_tests"},
		},
		SessionID:        "sess-debug-restart",
		UserMessage:      "Trace the failure and fix the restart path before rerunning the tests.",
		AssistantMessage: "Traced the failure, patched the restart guard, and verified the fix with the targeted tests.",
		ToolEvents: []ToolEvent{
			{Name: "read_file", Arguments: `{"path":"internal/kernel/kernel.go"}`, Result: "// file contents"},
			{Name: "run_tests", Arguments: `{"package":"./internal/kernel"}`, Result: "ok ./internal/kernel"},
		},
	}

	cand, extracted, err := extractor.Extract(context.Background(), src)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !extracted {
		t.Fatal("extracted = false, want true for worth-learning signal")
	}

	if cand.Name != "debug-restart-loop" {
		t.Fatalf("Candidate.Name = %q, want %q", cand.Name, "debug-restart-loop")
	}
	if cand.Description == "" {
		t.Fatal("Candidate.Description empty, want distilled description")
	}
	if !strings.Contains(cand.Body, "Trace the failure") {
		t.Fatalf("Candidate.Body = %q, want it to retain distilled steps", cand.Body)
	}
	if cand.SessionID != "sess-debug-restart" {
		t.Fatalf("Candidate.SessionID = %q, want %q", cand.SessionID, "sess-debug-restart")
	}
	if !cand.DistilledAt.Equal(fixed) {
		t.Fatalf("Candidate.DistilledAt = %v, want %v", cand.DistilledAt, fixed)
	}
	if want := []string{"read_file", "run_tests"}; !stringSliceEqual(cand.ToolNames, want) {
		t.Fatalf("Candidate.ToolNames = %#v, want %#v", cand.ToolNames, want)
	}
	if cand.Score != 3 || cand.Threshold != 2 {
		t.Fatalf("Candidate score/threshold = %d/%d, want 3/2", cand.Score, cand.Threshold)
	}

	if llm.calls != 1 {
		t.Fatalf("llm.calls = %d, want 1", llm.calls)
	}
	// The prompt should carry the trace deterministically so operators can audit it.
	for _, must := range []string{
		"sess-debug-restart",
		"Trace the failure and fix the restart path",
		"Traced the failure, patched the restart guard",
		"read_file",
		"run_tests",
	} {
		if !strings.Contains(llm.lastPrompt, must) {
			t.Fatalf("prompt missing %q\nprompt=%s", must, llm.lastPrompt)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}
	var persisted Candidate
	if err := json.Unmarshal([]byte(lines[0]), &persisted); err != nil {
		t.Fatalf("json.Unmarshal(candidate line): %v", err)
	}
	if persisted.Name != "debug-restart-loop" {
		t.Fatalf("persisted.Name = %q, want %q", persisted.Name, "debug-restart-loop")
	}
	if persisted.SessionID != "sess-debug-restart" {
		t.Fatalf("persisted.SessionID = %q, want %q", persisted.SessionID, "sess-debug-restart")
	}
	if !persisted.DistilledAt.Equal(fixed) {
		t.Fatalf("persisted.DistilledAt = %v, want %v", persisted.DistilledAt, fixed)
	}
}

func TestExtractorRejectsIncompleteLLMResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidates.jsonl")
	llm := &fakeLLM{response: DistillResponse{Name: "only-name"}} // missing description + body
	extractor := NewExtractor(llm, path)

	cand, extracted, err := extractor.Extract(context.Background(), Source{
		Signal:      Signal{WorthLearning: true, Score: 3, Threshold: 2},
		SessionID:   "sess-bad",
		UserMessage: "hi",
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want non-nil for incomplete LLM response")
	}
	if extracted {
		t.Fatal("extracted = true, want false when validation fails")
	}
	if cand.Name != "" {
		t.Fatalf("Candidate = %#v, want zero value when validation fails", cand)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("candidate log created despite validation failure, stat err = %v", statErr)
	}
}

func TestExtractorPropagatesLLMError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidates.jsonl")
	boom := errors.New("llm offline")
	llm := &fakeLLM{err: boom}
	extractor := NewExtractor(llm, path)

	_, extracted, err := extractor.Extract(context.Background(), Source{
		Signal:      Signal{WorthLearning: true, Score: 3, Threshold: 2},
		SessionID:   "sess-offline",
		UserMessage: "hi",
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Extract() err = %v, want wrapped %v", err, boom)
	}
	if extracted {
		t.Fatal("extracted = true, want false when LLM fails")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("candidate log created despite LLM error, stat err = %v", statErr)
	}
}

func TestExtractorRequiresLLM(t *testing.T) {
	extractor := NewExtractor(nil, filepath.Join(t.TempDir(), "x.jsonl"))
	_, _, err := extractor.Extract(context.Background(), Source{
		Signal: Signal{WorthLearning: true, Score: 3, Threshold: 2},
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want error when LLM seam is nil")
	}
}

func TestExtractorNoopWhenNil(t *testing.T) {
	var extractor *Extractor
	cand, extracted, err := extractor.Extract(context.Background(), Source{
		Signal: Signal{WorthLearning: true, Score: 3, Threshold: 2},
	})
	if err != nil {
		t.Fatalf("nil extractor returned err = %v, want nil", err)
	}
	if extracted || cand.Name != "" {
		t.Fatalf("nil extractor produced candidate %+v extracted=%v, want zero", cand, extracted)
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
