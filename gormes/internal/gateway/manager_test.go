package gateway

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

func TestManager_RegisterChannel(t *testing.T) {
	m := NewManager(ManagerConfig{}, nil, slog.Default())

	tg := newFakeChannel("telegram")
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register telegram: %v", err)
	}

	dc := newFakeChannel("discord")
	if err := m.Register(dc); err != nil {
		t.Fatalf("Register discord: %v", err)
	}

	if got := m.ChannelCount(); got != 2 {
		t.Errorf("ChannelCount() = %d, want 2", got)
	}
}

func TestManager_RegisterDuplicateName(t *testing.T) {
	m := NewManager(ManagerConfig{}, nil, slog.Default())

	if err := m.Register(newFakeChannel("telegram")); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := m.Register(newFakeChannel("telegram"))
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

func TestManager_RegisterEmptyName(t *testing.T) {
	m := NewManager(ManagerConfig{}, nil, slog.Default())
	if err := m.Register(newFakeChannel("")); err == nil {
		t.Fatal("expected empty-name error, got nil")
	}
}

type fakeKernel struct {
	mu        sync.Mutex
	submits   []kernel.PlatformEvent
	resets    int
	submitErr error
	resetErr  error
}

func (f *fakeKernel) Submit(e kernel.PlatformEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.submitErr != nil {
		return f.submitErr
	}
	f.submits = append(f.submits, e)
	return nil
}

func (f *fakeKernel) ResetSession() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resetErr != nil {
		return f.resetErr
	}
	f.resets++
	return nil
}

func (f *fakeKernel) Render() <-chan kernel.RenderFrame {
	return nil
}

func (f *fakeKernel) submitsSnapshot() []kernel.PlatformEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]kernel.PlatformEvent, len(f.submits))
	copy(out, f.submits)
	return out
}

func TestManager_Inbound_AllowedChat_Submit(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", UserID: "u", MsgID: "m",
		Kind: EventSubmit, Text: "hello",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})
	got := fk.submitsSnapshot()[0]
	if got.Kind != kernel.PlatformEventSubmit || got.Text != "hello" {
		t.Errorf("kernel submit = %+v, want %+v", got, kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "hello"})
	}
}

func TestManager_Inbound_BlockedChat_NoSubmit(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "999", Kind: EventSubmit, Text: "hi",
	})

	time.Sleep(50 * time.Millisecond)
	if n := len(fk.submitsSnapshot()); n != 0 {
		t.Errorf("blocked chat should produce 0 submits, got %d", n)
	}
}

func TestManager_Inbound_Cancel(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", Kind: EventCancel,
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		s := fk.submitsSnapshot()
		return len(s) == 1 && s[0].Kind == kernel.PlatformEventCancel
	})
}

func TestManager_Inbound_Reset(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", Kind: EventReset,
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		fk.mu.Lock()
		defer fk.mu.Unlock()
		return fk.resets == 1
	})
}

func TestManager_Inbound_Start_RepliesHelp(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", Kind: EventStart,
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		sent := tg.sentSnapshot()
		return len(sent) == 1 &&
			sent[0].ChatID == "42" &&
			strings.Contains(sent[0].Text, "/help") &&
			strings.Contains(sent[0].Text, "/new") &&
			strings.Contains(sent[0].Text, "/stop")
	})
	if n := len(fk.submitsSnapshot()); n != 0 {
		t.Errorf("EventStart should not submit to kernel, got %d", n)
	}
}

func TestManager_Inbound_SubmitUsesChatScopedSessionOverride(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		SessionMap:   session.NewMemMap(),
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m",
		Kind: EventSubmit, Text: "hello",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})
	got := fk.submitsSnapshot()[0]
	if got.SessionID != "telegram:42" {
		t.Errorf("SessionID override = %q, want %q", got.SessionID, "telegram:42")
	}
}

func TestManager_Outbound_StreamsToPinnedChannel(t *testing.T) {
	tg := newFakeChannel("telegram")
	frames := make(chan kernel.RenderFrame, 8)
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		CoalesceMs:   10,
	}, fk, slog.Default())
	m.setRenderChan(frames)
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "hi",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	frames <- kernel.RenderFrame{
		Phase: kernel.PhaseStreaming, DraftText: "partial",
	}

	waitFor(t, 500*time.Millisecond, func() bool {
		return len(tg.sentSnapshot()) >= 1 && len(tg.editsSnapshot()) >= 1
	})
}

func TestManager_Outbound_FinalFrameClearsTurn(t *testing.T) {
	tg := newFakeChannel("telegram")
	frames := make(chan kernel.RenderFrame, 8)
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		CoalesceMs:   10,
	}, fk, slog.Default())
	m.setRenderChan(frames)
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "hi",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	frames <- kernel.RenderFrame{Phase: kernel.PhaseStreaming, DraftText: "p1"}
	frames <- kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello back"},
		},
	}

	waitFor(t, 500*time.Millisecond, func() bool {
		m.turnMu.Lock()
		defer m.turnMu.Unlock()
		return m.turnPlatform == ""
	})
}

func TestManager_ShutdownWaitsForActiveTurn(t *testing.T) {
	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.Default())
	m.pinTurn("telegram", "42", "m1")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- m.Shutdown(shutdownCtx)
	}()

	time.Sleep(20 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Shutdown returned before turn cleared: %v", err)
	default:
	}

	m.clearTurn()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Shutdown did not return after turn cleared")
	}
}

func TestManager_ShutdownTimesOutWhileTurnActive(t *testing.T) {
	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.Default())
	m.pinTurn("telegram", "42", "m1")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := m.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want deadline exceeded", err)
	}
}

func TestManager_Inbound_SubmitRejectedDuringShutdown(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "hello",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(tg.sentSnapshot()) == 1
	})

	if n := len(fk.submitsSnapshot()); n != 0 {
		t.Fatalf("submit count = %d, want 0", n)
	}
	if got := tg.sentSnapshot()[0].Text; !strings.Contains(strings.ToLower(got), "shutting down") {
		t.Fatalf("shutdown reply = %q, want shutdown notice", got)
	}
}

func TestManager_Inbound_CancelAllowedDuringShutdown(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", Kind: EventCancel,
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		s := fk.submitsSnapshot()
		return len(s) == 1 && s[0].Kind == kernel.PlatformEventCancel
	})

	if n := len(tg.sentSnapshot()); n != 0 {
		t.Fatalf("shutdown cancel should not send a reply, got %d messages", n)
	}
}
