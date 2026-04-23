package tui

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

// TestSubmitRoutesToSubmitter: typing text and pressing Enter invokes the
// Submitter callback with the exact text typed. Proves Update -> tea.Cmd
// -> Submitter path works end-to-end.
func TestSubmitRoutesToSubmitter(t *testing.T) {
	var submitted atomic.Value
	submitted.Store("")

	frames := make(chan kernel.RenderFrame, 4)
	// Seed an initial idle frame so the Model knows we're Idle.
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Model: "hermes-agent", Seq: 1}

	m := NewModel(
		frames,
		func(text string) { submitted.Store(text) },
		func() {},
	)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	tm.Type("hello")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	// Allow the submit Cmd to run.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if submitted.Load().(string) == "hello" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := submitted.Load().(string); got != "hello" {
		t.Errorf("submitted text = %q, want %q", got, "hello")
	}

	// Feed a terminal frame then send Ctrl+D to exit cleanly.
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Model: "hermes-agent", Seq: 2}
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestCtrlCDuringInFlightCallsCancel: with inFlight=true (via Enter), Ctrl+C
// should invoke the cancel callback, not quit the program. We deliberately
// avoid pre-queueing an idle frame so the initial waitFrame never returns
// an Idle frame that would race with the Enter-driven inFlight flip.
func TestCtrlCDuringInFlightCallsCancel(t *testing.T) {
	var cancelled atomic.Bool

	// Unbuffered-friendly: keep the channel empty so waitFrame blocks until we
	// deliberately push a PhaseConnecting frame after inFlight has flipped.
	frames := make(chan kernel.RenderFrame, 4)

	m := NewModel(
		frames,
		func(text string) {}, // submit no-op for this test
		func() { cancelled.Store(true) },
	)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Submit something to set inFlight = true.
	tm.Type("q")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	// Push a Connecting frame to keep inFlight from ever flipping to false.
	frames <- kernel.RenderFrame{Phase: kernel.PhaseConnecting, Seq: 1}

	// Give the program a moment to process the above so the ordering is stable
	// before we send Ctrl+C.
	time.Sleep(50 * time.Millisecond)

	// Now send Ctrl+C; should invoke cancel, not quit.
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cancelled.Load() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cancelled.Load() {
		t.Error("Ctrl+C during in-flight turn did not invoke cancel callback")
	}

	// Clean shutdown: send Ctrl+D now that we're no longer inFlight-gated.
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 2}
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestViewRendersAssistantContent: after a frame with History contents, the
// rendered output includes the assistant text. Proves frameMsg -> m.frame ->
// View() rendering path.
func TestViewRendersAssistantContent(t *testing.T) {
	frames := make(chan kernel.RenderFrame, 4)
	// Pad the history with leading messages so the distinctive assistant text
	// lands below the top few rows that the standard renderer may drop when
	// View's total line count exceeds the reported terminal height.
	history := []hermes.Message{
		{Role: "user", Content: "warmup-1"},
		{Role: "user", Content: "warmup-2"},
		{Role: "user", Content: "warmup-3"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello-world-here"},
	}
	frames <- kernel.RenderFrame{
		Phase:   kernel.PhaseIdle,
		Model:   "hermes-agent",
		Seq:     1,
		History: history,
	}

	m := NewModel(frames, func(string) {}, func() {})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("hello-world-here"))
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestResizeDoesNotPanic: send a sequence of WindowSizeMsgs including
// absurdly small ones; View must return a banner string, never panic.
func TestResizeDoesNotPanic(t *testing.T) {
	frames := make(chan kernel.RenderFrame, 4)
	frames <- kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 1}

	m := NewModel(frames, func(string) {}, func() {})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	for _, w := range []int{200, 80, 50, 10, 2, 200} {
		tm.Send(tea.WindowSizeMsg{Width: w, Height: 24})
	}
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestRenderSidebarIncludesSelfMonitoringCounters(t *testing.T) {
	out := renderSidebar(kernel.RenderFrame{
		Telemetry: telemetry.Snapshot{
			Model:              "gormes-agent",
			TokensPerSec:       12.5,
			LatencyMsLast:      44,
			TokensInTotal:      21,
			TokensOutTotal:     34,
			TurnsTotal:         3,
			TurnsCompleted:     2,
			TurnsFailed:        1,
			ToolCallsTotal:     5,
			ToolCallsFailed:    2,
			ToolCallsCancelled: 1,
			LastTurnStatus:     telemetry.TurnStatusFailed,
		},
	}, 28)

	for _, want := range []string{
		"turns: 3",
		"ok/f/c: 2/1/0",
		"tools: 5",
		"fail/c: 2/1",
		"last: failed",
	} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("sidebar = %q, want substring %q", out, want)
		}
	}
}
