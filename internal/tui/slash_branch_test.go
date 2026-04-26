package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// recordingBranchFunc captures the BranchRequest it receives and returns
// the configured BranchResult or error. Used by the TUI /branch tests to
// prove the slash handler builds the right request and applies the result
// to the model without going through kernel.Submit.
type recordingBranchFunc struct {
	calls   int
	gotReq  BranchRequest
	gotCtx  context.Context
	result  BranchResult
	err     error
}

func (r *recordingBranchFunc) call(ctx context.Context, req BranchRequest) (BranchResult, error) {
	r.calls++
	r.gotReq = req
	r.gotCtx = ctx
	if r.err != nil {
		return BranchResult{}, r.err
	}
	return r.result, nil
}

// nopSubmitter records whether kernel.Submit was reached. The /branch
// handler MUST never let the slash text fall through to the kernel.
type nopSubmitter struct {
	calls int
}

func (s *nopSubmitter) submit(string) { s.calls++ }

func newBranchTestModel(t *testing.T, history []hermes.Message, frameSessionID string, fn SessionBranchFunc, sub Submitter) Model {
	t.Helper()
	frames := make(chan kernel.RenderFrame, 1)
	if sub == nil {
		sub = func(string) {}
	}
	m := NewModelWithOptions(frames, sub, func() {}, Options{
		MouseTracking: true,
		SessionBranch: fn,
	})
	m.frame.History = history
	m.frame.SessionID = frameSessionID
	return m
}

func TestSlashBranch_EmptyHistoryReturnsStatus(t *testing.T) {
	rec := &recordingBranchFunc{result: BranchResult{SessionID: "must-not-be-used"}}
	sub := &nopSubmitter{}
	m := newBranchTestModel(t, nil, "sess-parent", rec.call, sub.submit)

	res := branchSlashHandler("/branch", &m)

	if !res.Handled {
		t.Fatal("Handled = false, want true (slash MUST be consumed even when history is empty)")
	}
	if res.StatusMessage != "branch: no conversation" {
		t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, "branch: no conversation")
	}
	if rec.calls != 0 {
		t.Fatalf("SessionBranchFunc called %d times, want 0 (must short-circuit before calling fork)", rec.calls)
	}
	if sub.calls != 0 {
		t.Fatalf("Submit called %d times, want 0", sub.calls)
	}
	if m.SessionID() != "sess-parent" {
		t.Fatalf("SessionID = %q, want sess-parent (no fork happened)", m.SessionID())
	}
}

func TestSlashBranch_HappyPathSwitchesSessionIDAndDoesNotSubmit(t *testing.T) {
	history := []hermes.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ack"},
	}
	rec := &recordingBranchFunc{result: BranchResult{
		SessionID:        "sess-child",
		ParentSessionID:  "sess-parent",
		Title:            "",
		TranscriptCopied: 2,
	}}
	sub := &nopSubmitter{}
	m := newBranchTestModel(t, history, "sess-parent", rec.call, sub.submit)

	res := branchSlashHandler("/branch", &m)

	if !res.Handled {
		t.Fatal("Handled = false, want true")
	}
	if rec.calls != 1 {
		t.Fatalf("SessionBranchFunc called %d times, want 1", rec.calls)
	}
	if rec.gotReq.ParentSessionID != "sess-parent" {
		t.Fatalf("BranchRequest.ParentSessionID = %q, want sess-parent (current frame SessionID)", rec.gotReq.ParentSessionID)
	}
	if rec.gotReq.HistoryCount != 2 {
		t.Fatalf("BranchRequest.HistoryCount = %d, want 2", rec.gotReq.HistoryCount)
	}
	if rec.gotReq.Title != "" {
		t.Fatalf("BranchRequest.Title = %q, want empty for /branch with no name", rec.gotReq.Title)
	}
	if got := m.SessionID(); got != "sess-child" {
		t.Fatalf("model SessionID = %q, want sess-child after fork", got)
	}
	if sub.calls != 0 {
		t.Fatalf("kernel.Submit called %d times, want 0 (slash must never reach the kernel)", sub.calls)
	}
}

func TestSlashBranch_CustomNamePreserved(t *testing.T) {
	history := []hermes.Message{{Role: "user", Content: "hi"}}
	rec := &recordingBranchFunc{result: BranchResult{SessionID: "sess-child", ParentSessionID: "sess-parent"}}
	m := newBranchTestModel(t, history, "sess-parent", rec.call, nil)

	res := branchSlashHandler("/branch refactor path", &m)

	if !res.Handled {
		t.Fatal("Handled = false, want true")
	}
	if rec.gotReq.Title != "refactor path" {
		t.Fatalf("BranchRequest.Title = %q, want %q", rec.gotReq.Title, "refactor path")
	}
}

func TestSlashBranch_NoActiveSessionReturnsStatus(t *testing.T) {
	history := []hermes.Message{{Role: "user", Content: "hi"}}
	rec := &recordingBranchFunc{}
	m := newBranchTestModel(t, history, "", rec.call, nil)

	res := branchSlashHandler("/branch", &m)

	if !res.Handled {
		t.Fatal("Handled = false, want true")
	}
	if res.StatusMessage != "branch: no active session" {
		t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, "branch: no active session")
	}
	if rec.calls != 0 {
		t.Fatalf("SessionBranchFunc called %d times, want 0", rec.calls)
	}
}

func TestSlashBranch_StoreUnavailableWhenFuncMissing(t *testing.T) {
	history := []hermes.Message{{Role: "user", Content: "hi"}}
	m := newBranchTestModel(t, history, "sess-parent", nil, nil)

	res := branchSlashHandler("/branch", &m)

	if !res.Handled {
		t.Fatal("Handled = false, want true")
	}
	if res.StatusMessage != "branch: store unavailable" {
		t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, "branch: store unavailable")
	}
	if m.SessionID() != "sess-parent" {
		t.Fatalf("SessionID = %q, want sess-parent (fork must not switch when store missing)", m.SessionID())
	}
}

func TestSlashBranch_ForkErrorLeavesParentActive(t *testing.T) {
	history := []hermes.Message{{Role: "user", Content: "hi"}}
	rec := &recordingBranchFunc{err: errors.New("disk full")}
	m := newBranchTestModel(t, history, "sess-parent", rec.call, nil)

	res := branchSlashHandler("/branch", &m)

	if !res.Handled {
		t.Fatal("Handled = false, want true")
	}
	if res.StatusMessage != "branch: fork failed: disk full" {
		t.Fatalf("StatusMessage = %q, want %q", res.StatusMessage, "branch: fork failed: disk full")
	}
	if m.SessionID() != "sess-parent" {
		t.Fatalf("SessionID = %q, want sess-parent (fork failure must leave parent active)", m.SessionID())
	}
}

func TestSlashBranch_RegisteredOnDefaultRegistry(t *testing.T) {
	rec := &recordingBranchFunc{result: BranchResult{SessionID: "sess-child", ParentSessionID: "sess-parent"}}
	sub := &nopSubmitter{}
	frames := make(chan kernel.RenderFrame, 1)
	m := NewModelWithOptions(frames, sub.submit, func() {}, Options{
		MouseTracking: true,
		SessionBranch: rec.call,
	})
	m.frame.History = []hermes.Message{{Role: "user", Content: "hi"}}
	m.frame.SessionID = "sess-parent"

	res := m.slashRegistry.Dispatch("/branch", &m)
	if !res.Handled {
		t.Fatal("Default registry did not route /branch — slash must be registered out of the box")
	}
	if rec.calls != 1 {
		t.Fatalf("SessionBranchFunc called %d times via registry, want 1", rec.calls)
	}
}
