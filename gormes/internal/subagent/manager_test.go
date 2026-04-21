package subagent

import (
	"context"
	"testing"
	"time"
)

func newStubManager(t *testing.T, depth int) (SubagentManager, context.Context, context.CancelFunc) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	mgr := NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     depth,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return StubRunner{} },
	})
	return mgr, parentCtx, cancel
}

func TestManagerSpawnHappyPath(t *testing.T) {
	mgr, parentCtx, cancel := newStubManager(t, 0)
	defer cancel()

	sa, err := mgr.Spawn(parentCtx, SubagentConfig{
		Goal:          "collect status",
		MaxIterations: 7,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sa == nil {
		t.Fatal("Spawn returned nil subagent")
	}
	if sa.Depth != 1 {
		t.Fatalf("Depth = %d, want 1", sa.Depth)
	}
	if sa.ParentID != "parent_test" {
		t.Fatalf("ParentID = %q, want %q", sa.ParentID, "parent_test")
	}

	select {
	case ev, ok := <-sa.Events():
		if !ok {
			t.Fatal("Events closed before first event")
		}
		if ev.Type != EventStarted {
			t.Fatalf("first event type = %q, want %q", ev.Type, EventStarted)
		}
		if ev.Message != "collect status" {
			t.Fatalf("first event message = %q, want %q", ev.Message, "collect status")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for started event")
	}

	select {
	case ev, ok := <-sa.Events():
		if !ok {
			t.Fatal("Events closed before completed event")
		}
		if ev.Type != EventCompleted {
			t.Fatalf("second event type = %q, want %q", ev.Type, EventCompleted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for completed event")
	}

	select {
	case _, ok := <-sa.Events():
		if ok {
			t.Fatal("Events still open after runner completion")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for events channel close")
	}

	res, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if res == nil {
		t.Fatal("WaitForResult returned nil result")
	}
	if res.Status != StatusCompleted {
		t.Fatalf("result status = %q, want %q", res.Status, StatusCompleted)
	}
	if res.Summary != "collect status" {
		t.Fatalf("result summary = %q, want %q", res.Summary, "collect status")
	}
	if res.ExitReason != "stub_runner_no_llm_yet" {
		t.Fatalf("result exit reason = %q, want %q", res.ExitReason, "stub_runner_no_llm_yet")
	}
	if res.ID == "" {
		t.Fatal("result ID not set")
	}
	if res.Iterations != 0 {
		t.Fatalf("result iterations = %d, want 0 from StubRunner", res.Iterations)
	}
}

func TestManagerSpawnAppliesIterationDefault(t *testing.T) {
	mgr, parentCtx, cancel := newStubManager(t, 0)
	defer cancel()

	sa, err := mgr.Spawn(parentCtx, SubagentConfig{Goal: "default iterations"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if sa.cfg.MaxIterations != DefaultMaxIterations {
		t.Fatalf("cfg.MaxIterations = %d, want %d", sa.cfg.MaxIterations, DefaultMaxIterations)
	}

	res, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if res == nil {
		t.Fatal("WaitForResult returned nil result")
	}
	if res.Status != StatusCompleted {
		t.Fatalf("result status = %q, want %q", res.Status, StatusCompleted)
	}
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	select {
	case events <- SubagentEvent{Type: EventStarted, Message: cfg.Goal}:
	case <-ctx.Done():
	}
	<-ctx.Done()
	return &SubagentResult{
		Status:     StatusInterrupted,
		ExitReason: "ctx_cancelled",
		Error:      ctx.Err().Error(),
	}
}

func newBlockingManager(t *testing.T, depth int) (SubagentManager, context.CancelFunc) {
	t.Helper()

	parentCtx, cancel := context.WithCancel(context.Background())
	return NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     depth,
		Registry:  NewRegistry(),
		NewRunner: func() Runner { return blockingRunner{} },
	}), cancel
}

func TestManagerInterruptDeliversMessage(t *testing.T) {
	mgr, cancel := newBlockingManager(t, 0)
	defer cancel()

	sa, err := mgr.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if err := mgr.Interrupt(sa, "user_stop"); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	result, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if result.Status != StatusInterrupted {
		t.Errorf("Status: want %q, got %q", StatusInterrupted, result.Status)
	}

	var sawInterrupt bool
	var interruptMsg string
	for ev := range sa.Events() {
		if ev.Type == EventInterrupted {
			sawInterrupt = true
			interruptMsg = ev.Message
		}
	}
	if !sawInterrupt {
		t.Errorf("Events: want at least one EventInterrupted, got none")
	}
	if interruptMsg != "user_stop" {
		t.Errorf("EventInterrupted.Message: want %q, got %q", "user_stop", interruptMsg)
	}
}

func TestManagerInterruptUnknownReturnsErr(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()

	stranger := &Subagent{ID: "sa_stranger"}
	err := mgr.Interrupt(stranger, "nope")
	if err == nil || !errorsIs(err, ErrSubagentNotFound) {
		t.Errorf("err: want ErrSubagentNotFound, got %v", err)
	}
}

func errorsIs(err, target error) bool {
	if err == nil {
		return false
	}
	for {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
		if err == nil {
			return false
		}
	}
}

func TestManagerInterruptIsIdempotent(t *testing.T) {
	mgr, cancel := newBlockingManager(t, 0)
	defer cancel()

	sa, err := mgr.Spawn(context.Background(), SubagentConfig{Goal: "blocked"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if err := mgr.Interrupt(sa, "first"); err != nil {
		t.Fatalf("first Interrupt: %v", err)
	}
	if _, err := sa.WaitForResult(context.Background()); err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}

	err = mgr.Interrupt(sa, "second")
	if err == nil || !errorsIs(err, ErrSubagentNotFound) {
		t.Errorf("second Interrupt: want ErrSubagentNotFound, got %v", err)
	}
}
