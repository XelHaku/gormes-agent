package gateway

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestStatusModel_MarkRegistered_SetsPhaseAndTimestamp(t *testing.T) {
	fixed := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	sm := NewStatusModelWithClock(func() time.Time { return fixed })

	sm.MarkRegistered("telegram")

	got, ok := sm.Lookup("telegram")
	if !ok {
		t.Fatalf("Lookup(telegram): not found after MarkRegistered")
	}
	if got.Platform != "telegram" {
		t.Errorf("Platform = %q, want %q", got.Platform, "telegram")
	}
	if got.Phase != LifecyclePhaseRegistered {
		t.Errorf("Phase = %q, want %q", got.Phase, LifecyclePhaseRegistered)
	}
	if !got.LastUpdated.Equal(fixed) {
		t.Errorf("LastUpdated = %v, want %v", got.LastUpdated, fixed)
	}
	if got.LastError != "" {
		t.Errorf("LastError = %q, want empty", got.LastError)
	}
}

func TestStatusModel_MarkFailed_StoresLastError(t *testing.T) {
	sm := NewStatusModel()

	sm.MarkFailed("discord", errors.New("gateway hang-up"))

	got, ok := sm.Lookup("discord")
	if !ok {
		t.Fatalf("Lookup(discord): not found after MarkFailed")
	}
	if got.Phase != LifecyclePhaseFailed {
		t.Errorf("Phase = %q, want %q", got.Phase, LifecyclePhaseFailed)
	}
	if got.LastError != "gateway hang-up" {
		t.Errorf("LastError = %q, want %q", got.LastError, "gateway hang-up")
	}
}

func TestStatusModel_MarkDisconnected_ClearsLastErrorOnCleanExit(t *testing.T) {
	sm := NewStatusModel()
	sm.MarkFailed("slack", errors.New("boom"))

	sm.MarkDisconnected("slack", nil)

	got, _ := sm.Lookup("slack")
	if got.Phase != LifecyclePhaseDisconnected {
		t.Errorf("Phase = %q, want %q", got.Phase, LifecyclePhaseDisconnected)
	}
	if got.LastError != "" {
		t.Errorf("LastError after clean disconnect = %q, want empty", got.LastError)
	}
}

func TestStatusModel_Snapshot_OrderedByPlatform(t *testing.T) {
	sm := NewStatusModel()
	sm.MarkRegistered("telegram")
	sm.MarkRegistered("discord")
	sm.MarkRegistered("slack")

	got := sm.Snapshot()
	if len(got) != 3 {
		t.Fatalf("Snapshot length = %d, want 3", len(got))
	}
	want := []string{"discord", "slack", "telegram"}
	for i, entry := range got {
		if entry.Platform != want[i] {
			t.Errorf("Snapshot[%d].Platform = %q, want %q", i, entry.Platform, want[i])
		}
	}
}

func TestStatusModel_Lookup_Missing_ReturnsFalse(t *testing.T) {
	sm := NewStatusModel()
	if _, ok := sm.Lookup("nope"); ok {
		t.Fatalf("Lookup(nope): want ok=false on missing platform")
	}
}

func TestManager_Register_WritesRegisteredIntoStatus(t *testing.T) {
	m := NewManager(ManagerConfig{}, nil, slog.Default())

	if err := m.Register(newFakeChannel("telegram")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	st := m.Status()
	if st == nil {
		t.Fatalf("Manager.Status() returned nil")
	}
	entry, ok := st.Lookup("telegram")
	if !ok {
		t.Fatalf("Status.Lookup(telegram): not found after Register")
	}
	if entry.Phase != LifecyclePhaseRegistered {
		t.Errorf("Phase = %q, want %q", entry.Phase, LifecyclePhaseRegistered)
	}
}

func TestManager_Run_TransitionsToRunningThenDisconnected(t *testing.T) {
	tg := newFakeChannel("telegram")
	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- m.Run(ctx) }()

	// Wait until the per-channel goroutine has begun Run (fakeChannel closes started).
	<-tg.started

	waitFor(t, 200*time.Millisecond, func() bool {
		entry, ok := m.Status().Lookup("telegram")
		return ok && entry.Phase == LifecyclePhaseRunning
	})

	cancel()
	// Drain Run return to let per-channel goroutines finish.
	select {
	case <-runDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Manager.Run did not return after cancel")
	}

	waitFor(t, 200*time.Millisecond, func() bool {
		entry, ok := m.Status().Lookup("telegram")
		return ok && entry.Phase == LifecyclePhaseDisconnected
	})

	entry, _ := m.Status().Lookup("telegram")
	if entry.LastError != "" {
		t.Errorf("LastError after clean cancel = %q, want empty", entry.LastError)
	}
}

func TestManager_Run_MarksChannelFailedOnRunError(t *testing.T) {
	boom := errors.New("adapter exploded")
	failing := &errRunChannel{name: "discord", err: boom}

	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.Default())
	if err := m.Register(failing); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- m.Run(ctx) }()

	waitFor(t, 200*time.Millisecond, func() bool {
		entry, ok := m.Status().Lookup("discord")
		return ok && entry.Phase == LifecyclePhaseFailed
	})

	entry, _ := m.Status().Lookup("discord")
	if entry.LastError != boom.Error() {
		t.Errorf("LastError = %q, want %q", entry.LastError, boom.Error())
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Manager.Run did not return after cancel")
	}
}

// errRunChannel is a Channel whose Run exits immediately with the configured
// error. It deliberately ignores the inbox and context.
type errRunChannel struct {
	name string
	err  error
}

func (e *errRunChannel) Name() string { return e.name }
func (e *errRunChannel) Run(_ context.Context, _ chan<- InboundEvent) error {
	return e.err
}
func (e *errRunChannel) Send(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
