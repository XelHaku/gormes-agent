package gateway

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

type hookRecorder struct {
	mu     sync.Mutex
	events []HookEvent
}

type immediateFrameKernel struct {
	mu      sync.Mutex
	entered chan struct{}
	render  chan kernel.RenderFrame
	release chan struct{}
	submits []kernel.PlatformEvent
}

func newImmediateFrameKernel() *immediateFrameKernel {
	return &immediateFrameKernel{
		entered: make(chan struct{}),
		render:  make(chan kernel.RenderFrame, 1),
		release: make(chan struct{}),
	}
}

func (k *immediateFrameKernel) Submit(ev kernel.PlatformEvent) error {
	k.mu.Lock()
	k.submits = append(k.submits, ev)
	k.mu.Unlock()

	close(k.entered)
	k.render <- kernel.RenderFrame{
		Phase:     kernel.PhaseStreaming,
		DraftText: "partial",
	}
	<-k.release
	return nil
}

func (k *immediateFrameKernel) ResetSession() error { return nil }

func (k *immediateFrameKernel) Render() <-chan kernel.RenderFrame { return k.render }

func (r *hookRecorder) record(_ context.Context, ev HookEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *hookRecorder) snapshot() []HookEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]HookEvent, len(r.events))
	copy(out, r.events)
	return out
}

func TestManagerHooksReceiveLifecycle(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	hooks := NewHooks()
	rec := &hookRecorder{}
	hooks.Add(HookBeforeReceive, rec.record)
	hooks.Add(HookAfterReceive, rec.record)

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		Hooks:        hooks,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", UserID: "u1", MsgID: "m1",
		Kind: EventSubmit, Text: "hello hooks",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(rec.snapshot()) == 2
	})

	got := rec.snapshot()
	if got[0].Point != HookBeforeReceive || got[1].Point != HookAfterReceive {
		t.Fatalf("hook points = %v, want before_receive then after_receive", []HookPoint{got[0].Point, got[1].Point})
	}
	if got[0].Platform != "telegram" || got[0].ChatID != "42" || got[0].Kind != EventSubmit {
		t.Fatalf("before_receive = %+v, want telegram chat 42 submit", got[0])
	}
}

func TestManagerHooksSendLifecycle(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	hooks := NewHooks()
	rec := &hookRecorder{}
	hooks.Add(HookBeforeSend, rec.record)
	hooks.Add(HookAfterSend, rec.record)

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		Hooks:        hooks,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventStart,
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(rec.snapshot()) == 2
	})

	got := rec.snapshot()
	if got[0].Point != HookBeforeSend || got[1].Point != HookAfterSend {
		t.Fatalf("hook points = %v, want before_send then after_send", []HookPoint{got[0].Point, got[1].Point})
	}
	if got[0].Text != startGreeting {
		t.Fatalf("before_send text = %q, want %q", got[0].Text, startGreeting)
	}
	if got[1].MsgID == "" {
		t.Fatal("after_send MsgID = empty, want sent message id")
	}
}

func TestManagerHooksOnError(t *testing.T) {
	tg := newFakeChannel("telegram")
	tg.sendErr = errors.New("send failed")
	fk := &fakeKernel{}
	hooks := NewHooks()
	rec := &hookRecorder{}
	hooks.Add(HookOnError, rec.record)

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		Hooks:        hooks,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventStart,
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(rec.snapshot()) == 1
	})

	got := rec.snapshot()[0]
	if got.Point != HookOnError {
		t.Fatalf("point = %q, want %q", got.Point, HookOnError)
	}
	if got.Platform != "telegram" || got.ChatID != "42" {
		t.Fatalf("hook event = %+v, want telegram chat 42", got)
	}
	if got.Err == nil || got.Err.Error() != "send failed" {
		t.Fatalf("Err = %v, want send failed", got.Err)
	}
}

func TestManagerHooksObservePlaceholderSendDuringStreaming(t *testing.T) {
	tg := newFakeChannel("telegram")
	frames := make(chan kernel.RenderFrame, 8)
	fk := &fakeKernel{}
	hooks := NewHooks()
	rec := &hookRecorder{}
	hooks.Add(HookBeforeSend, rec.record)
	hooks.Add(HookAfterSend, rec.record)

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		CoalesceMs:   10,
		Hooks:        hooks,
	}, fk, slog.Default())
	m.setRenderChan(frames)
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "stream please",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	frames <- kernel.RenderFrame{
		Phase:     kernel.PhaseStreaming,
		DraftText: "partial",
	}

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(rec.snapshot()) >= 2
	})

	got := rec.snapshot()
	if got[0].Point != HookBeforeSend || got[1].Point != HookAfterSend {
		t.Fatalf("hook points = %v, want before_send then after_send", []HookPoint{got[0].Point, got[1].Point})
	}
	if got[0].Text != "⏳" {
		t.Fatalf("before_send text = %q, want placeholder marker", got[0].Text)
	}
	if got[1].MsgID == "" {
		t.Fatal("after_send MsgID = empty, want placeholder message id")
	}
}

func TestManagerHooksObservePlaceholderSendWhenFirstFrameArrivesDuringSubmit(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := newImmediateFrameKernel()
	defer close(fk.release)

	hooks := NewHooks()
	rec := &hookRecorder{}
	hooks.Add(HookBeforeSend, rec.record)
	hooks.Add(HookAfterSend, rec.record)

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		CoalesceMs:   10,
		Hooks:        hooks,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	go tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "stream immediately",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(rec.snapshot()) >= 2
	})

	got := rec.snapshot()
	if got[0].Point != HookBeforeSend || got[1].Point != HookAfterSend {
		t.Fatalf("hook points = %v, want before_send then after_send", []HookPoint{got[0].Point, got[1].Point})
	}
	if got[0].Text != "⏳" {
		t.Fatalf("before_send text = %q, want placeholder marker", got[0].Text)
	}
	if got[1].MsgID == "" {
		t.Fatal("after_send MsgID = empty, want placeholder message id")
	}
}

func TestManagerPinsTurnBeforeSubmitReturns(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := newImmediateFrameKernel()
	defer close(fk.release)

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	go tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "pin first",
	})

	select {
	case <-fk.entered:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Submit was not entered within 200ms")
	}

	m.turnMu.Lock()
	platform := m.turnPlatform
	chatID := m.turnChatID
	m.turnMu.Unlock()

	if platform != "telegram" || chatID != "42" {
		t.Fatalf("turn pinned during Submit = (%q, %q), want (%q, %q)", platform, chatID, "telegram", "42")
	}
}
