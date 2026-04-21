package subagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

type runnerFunc func(context.Context, Spec, func(Event)) (Result, error)

func (f runnerFunc) Run(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
	return f(ctx, spec, emit)
}

func TestManager_Start_WaitsAndStreamsEvents(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 7,
		DefaultTimeout:       3 * time.Second,
		MaxChildDepth:        1,
	}

	started := make(chan struct{})
	release := make(chan struct{})
	seenSpec := make(chan Spec, 1)

	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		seenSpec <- spec
		close(started)
		emit(Event{Type: EventStarted, Message: "delegated"})
		emit(Event{Type: EventProgress, Message: "working"})
		<-release
		emit(Event{Type: EventCompleted, Message: "done"})
		return Result{Status: StatusCompleted, Summary: "done"}, nil
	}), "")

	var (
		handle   *Handle
		startErr error
	)
	startDone := make(chan struct{})
	go func() {
		handle, startErr = mgr.Start(context.Background(), Spec{Goal: "  summarize  "})
		close(startDone)
	}()

	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("Start blocked waiting for runner")
	}
	if startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	if handle == nil {
		t.Fatal("Start returned nil handle")
	}
	if cap(handle.Events) == 0 {
		t.Fatal("Handle.Events must be buffered")
	}
	if cap(handle.done) != 1 {
		t.Fatalf("Handle.done capacity = %d, want 1", cap(handle.done))
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}

	spec := <-seenSpec
	if spec.Goal != "summarize" {
		t.Fatalf("Goal = %q, want trimmed summarize", spec.Goal)
	}
	if spec.MaxIterations != 7 {
		t.Fatalf("MaxIterations = %d, want 7", spec.MaxIterations)
	}
	if spec.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %v, want 3s", spec.Timeout)
	}

	close(release)

	res, err := handle.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}
	if res.RunID == "" {
		t.Fatal("RunID must be populated")
	}
	if res.RunID != handle.RunID {
		t.Fatalf("RunID = %q, want handle.RunID %q", res.RunID, handle.RunID)
	}

	var got []Event
	for ev := range handle.Events {
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Type != EventStarted || got[1].Type != EventProgress || got[2].Type != EventCompleted {
		t.Fatalf("events = %#v, want started/progress/completed", got)
	}

	mgr.mu.Lock()
	live := len(mgr.live)
	mgr.mu.Unlock()
	if live != 0 {
		t.Fatalf("live handles = %d, want 0", live)
	}
}

func TestManager_Cancel_ReturnsCancelledResult(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Hour,
		MaxChildDepth:        1,
	}

	canceled := make(chan struct{})
	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		emit(Event{Type: EventStarted, Message: "begin"})
		<-ctx.Done()
		close(canceled)
		return Result{Summary: "should be overridden"}, ctx.Err()
	}), "")

	handle, err := mgr.Start(context.Background(), Spec{Goal: "cancel me"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := handle.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()

	res, err := handle.Wait(waitCtx)
	if err != nil {
		t.Fatalf("Wait error = %v, want nil", err)
	}
	if res.Status != StatusCancelled {
		t.Fatalf("Status = %q, want cancelled", res.Status)
	}

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("runner did not observe cancellation")
	}

	mgr.mu.Lock()
	live := len(mgr.live)
	mgr.mu.Unlock()
	if live != 0 {
		t.Fatalf("live handles = %d, want 0", live)
	}
}

func TestManager_Timeout_ReturnsTimedOutResult(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Hour,
		MaxChildDepth:        1,
	}

	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		<-ctx.Done()
		return Result{Summary: "late"}, ctx.Err()
	}), "")

	handle, err := mgr.Start(context.Background(), Spec{
		Goal:    "timeout me",
		Timeout: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	res, err := handle.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait error = %v, want nil", err)
	}
	if res.Status != StatusTimedOut {
		t.Fatalf("Status = %q, want timed_out", res.Status)
	}
}

func TestManager_LogAppendFailure_PreservesResultError(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Second,
		MaxChildDepth:        1,
	}

	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		return Result{Status: StatusCompleted, Summary: "ok"}, nil
	}), t.TempDir())

	handle, err := mgr.Start(context.Background(), Spec{Goal: "log failure"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	res, err := handle.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait error = %v, want nil", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}
	if res.RunID == "" {
		t.Fatal("RunID must be populated")
	}
	if res.Error == "" {
		t.Fatal("Result.Error must contain the logging failure")
	}
}

func TestManager_Run_RecoversRunnerPanics(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Second,
		MaxChildDepth:        1,
	}

	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		panic("runner boom")
	}), "")

	handle, err := mgr.Start(context.Background(), Spec{Goal: "panic me"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	res, err := handle.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait error = %v, want nil", err)
	}
	if res.Status != StatusFailed {
		t.Fatalf("Status = %q, want failed", res.Status)
	}
	if res.Error == "" {
		t.Fatal("Error must be populated when runner panics")
	}
}

func TestManager_Wait_ReturnsContextErrorWhenWaitCtxCanceled(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Second,
		MaxChildDepth:        1,
	}

	blocked := make(chan struct{})
	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		<-blocked
		return Result{Status: StatusCompleted, Summary: "late"}, nil
	}), "")

	handle, err := mgr.Start(context.Background(), Spec{Goal: "wait ctx cancel"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()

	res, err := handle.Wait(waitCtx)
	if err == nil {
		t.Fatal("Wait error = nil, want context cancellation")
	}
	if res.RunID != handle.RunID {
		t.Fatalf("RunID = %q, want %q", res.RunID, handle.RunID)
	}

	close(blocked)

	waitDone := make(chan struct{})
	go func() {
		_, _ = handle.Wait(context.Background())
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("run did not finish after unblocking")
	}
}

func TestManager_EventBurst_CompletesBeforeConsumerReads(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Second,
		MaxChildDepth:        1,
	}

	const eventCount = handleEventBufferSize + 8
	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		for i := 0; i < eventCount; i++ {
			emit(Event{Type: EventProgress, Message: "tick"})
		}
		return Result{Status: StatusCompleted, Summary: "burst done"}, nil
	}), "")

	handle, err := mgr.Start(context.Background(), Spec{Goal: "burst"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitDone := make(chan struct{})
	var waitRes Result
	var waitErr error
	go func() {
		waitRes, waitErr = handle.Wait(context.Background())
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("Wait blocked while runner emitted more than the event buffer")
	}
	if waitErr != nil {
		t.Fatalf("Wait error = %v, want nil", waitErr)
	}
	if waitRes.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", waitRes.Status)
	}

	var got int
	for range handle.Events {
		got++
	}
	if got == 0 {
		t.Fatal("got no events, want bounded best-effort delivery")
	}
	if got > handleEventBufferSize {
		t.Fatalf("got %d events, want at most %d", got, handleEventBufferSize)
	}
}

func TestManager_WritesRunLogRecord(t *testing.T) {
	cfg := config.DelegationCfg{
		DefaultMaxIterations: 4,
		DefaultTimeout:       time.Second,
		MaxChildDepth:        1,
	}

	logPath := filepath.Join(t.TempDir(), "runs.jsonl")
	mgr := NewManager(cfg, runnerFunc(func(ctx context.Context, spec Spec, emit func(Event)) (Result, error) {
		emit(Event{Type: EventStarted, Message: "start"})
		return Result{Status: StatusCompleted, Summary: "logged"}, nil
	}), logPath)

	handle, err := mgr.Start(context.Background(), Spec{Goal: "log this"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	res, err := handle.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", res.Status)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var rec RunRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rec.RunID != handle.RunID {
		t.Fatalf("RunID = %q, want %q", rec.RunID, handle.RunID)
	}
	if rec.Status != StatusCompleted {
		t.Fatalf("Status = %q, want completed", rec.Status)
	}
	if rec.Summary != "logged" {
		t.Fatalf("Summary = %q, want logged", rec.Summary)
	}
	if rec.StartedAt.IsZero() || rec.FinishedAt.IsZero() {
		t.Fatal("timestamps must be populated")
	}
}
