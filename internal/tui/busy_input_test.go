package tui

import (
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// fakeBusyEvaluator is a hand-rolled BusyInputEvaluator that the TUI tests use
// to exercise the busy-guard branch in Update without spinning a real
// long-running command on a background goroutine. The fields are written from
// tests before each Update call and read by the model during dispatch.
type fakeBusyEvaluator struct {
	rejected bool
	evidence string
}

func (f *fakeBusyEvaluator) EvaluateInput(string) BusyInputVerdict {
	return BusyInputVerdict{Rejected: f.rejected, Evidence: f.evidence}
}

// TestEnterRejectedWhileBusyDoesNotSubmit asserts the TUI consults the busy
// evaluator before submitting a turn. While busy, a plain-text Enter must be
// turned into a status message and MUST NOT call Submitter — otherwise an
// overlapping prompt would race the active long-running command.
func TestEnterRejectedWhileBusyDoesNotSubmit(t *testing.T) {
	var submitCount atomic.Int32
	frames := make(chan kernel.RenderFrame, 4)
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1}

	guard := &fakeBusyEvaluator{rejected: true, evidence: "Gormes is busy — Compressing context... — wait or send /stop"}
	m := NewModelWithOptions(
		frames,
		func(string) { submitCount.Add(1) },
		func() {},
		Options{MouseTracking: true, BusyGuard: guard},
	)

	m.editor.SetValue("hello there")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mUpdated, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}

	if got := submitCount.Load(); got != 0 {
		t.Errorf("Submitter called %d times while busy, want 0", got)
	}
	if mUpdated.editor.Value() != "" {
		t.Errorf("editor not reset after busy rejection, value = %q", mUpdated.editor.Value())
	}
	if !strings.Contains(strings.ToLower(mUpdated.statusMessage), "busy") {
		t.Errorf("statusMessage = %q, want to mention busy", mUpdated.statusMessage)
	}
	if !strings.Contains(mUpdated.statusMessage, "Compressing context") {
		t.Errorf("statusMessage = %q, want to include guard evidence", mUpdated.statusMessage)
	}
	if mUpdated.inFlight {
		t.Errorf("inFlight = true after rejected submit, want false")
	}
}

// TestEnterPlainTextProceedsWhenIdleEvaluator asserts that an evaluator
// reporting Rejected=false leaves the existing submit path intact — we cannot
// regress the normal-input case while adding the busy guard.
func TestEnterPlainTextProceedsWhenIdleEvaluator(t *testing.T) {
	var submitted atomic.Value
	submitted.Store("")
	frames := make(chan kernel.RenderFrame, 4)
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1}

	guard := &fakeBusyEvaluator{rejected: false}
	m := NewModelWithOptions(
		frames,
		func(text string) { submitted.Store(text) },
		func() {},
		Options{MouseTracking: true, BusyGuard: guard},
	)

	m.editor.SetValue("hello there")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mUpdated, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	if cmd == nil {
		t.Fatal("Update returned nil tea.Cmd, want a submit batch")
	}
	// Run the returned cmd to drive the submit goroutine.
	_ = cmd()
	if got := submitted.Load().(string); got != "hello there" {
		t.Errorf("Submitter received %q, want %q", got, "hello there")
	}
	if !mUpdated.inFlight {
		t.Errorf("inFlight = false after accepted submit, want true")
	}
	if mUpdated.editor.Value() != "" {
		t.Errorf("editor not reset after accepted submit, value = %q", mUpdated.editor.Value())
	}
}

// TestEnterAllowsBypassSlashWhileBusy asserts the busy guard does not block a
// bypass slash command (the evaluator returns Rejected=false for /stop). The
// slash dispatcher then handles it normally — the model must not call
// Submitter and must clear the editor.
func TestEnterAllowsBypassSlashWhileBusy(t *testing.T) {
	var submitCount atomic.Int32
	frames := make(chan kernel.RenderFrame, 4)
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1}

	// Evaluator simulates the real BusyCommandGuard's bypass behavior: it
	// returns Rejected=false for /stop even though the guard is active.
	guard := &slashAwareBusyEvaluator{busy: true, rejectEvidence: "Gormes is busy"}
	m := NewModelWithOptions(
		frames,
		func(string) { submitCount.Add(1) },
		func() {},
		Options{MouseTracking: true, BusyGuard: guard},
	)
	// Register a /stop handler that records dispatch.
	var dispatched atomic.Bool
	m.slashRegistry.Register("stop", func(input string, _ *Model) SlashResult {
		dispatched.Store(true)
		return SlashResult{Handled: true, StatusMessage: "stop dispatched"}
	})

	m.editor.SetValue("/stop")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mUpdated, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	if !dispatched.Load() {
		t.Error("/stop handler not dispatched while busy, want bypass to proceed")
	}
	if got := submitCount.Load(); got != 0 {
		t.Errorf("Submitter called %d times for /stop dispatch, want 0", got)
	}
	if mUpdated.editor.Value() != "" {
		t.Errorf("editor not reset after /stop, value = %q", mUpdated.editor.Value())
	}
}

// slashAwareBusyEvaluator mirrors the real BusyCommandGuard's slash-bypass
// branch: while busy, /stop is allowed through but plain text is rejected.
type slashAwareBusyEvaluator struct {
	busy           bool
	rejectEvidence string
}

func (s *slashAwareBusyEvaluator) EvaluateInput(input string) BusyInputVerdict {
	if !s.busy {
		return BusyInputVerdict{}
	}
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "/stop") || strings.HasPrefix(trimmed, "/help") {
		return BusyInputVerdict{}
	}
	return BusyInputVerdict{Rejected: true, Evidence: s.rejectEvidence}
}
