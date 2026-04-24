package gateway

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingRestartStore struct {
	mu         sync.Mutex
	marker     TakeoverMarker
	markerSet  bool
	saveCalls  int
	loadCalls  int
	saveErr    error
	loadErr    error
	preloaded  bool
	preloadVal TakeoverMarker
}

func (r *recordingRestartStore) LoadTakeoverMarker(_ context.Context) (TakeoverMarker, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadCalls++
	if r.loadErr != nil {
		return TakeoverMarker{}, false, r.loadErr
	}
	if r.preloaded && !r.markerSet {
		return r.preloadVal, true, nil
	}
	if !r.markerSet {
		return TakeoverMarker{}, false, nil
	}
	return r.marker, true, nil
}

func (r *recordingRestartStore) SaveTakeoverMarker(_ context.Context, m TakeoverMarker) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.marker = m
	r.markerSet = true
	return nil
}

func (r *recordingRestartStore) saveSnapshot() (TakeoverMarker, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.marker, r.saveCalls
}

type recordingRestartFn struct {
	mu    sync.Mutex
	calls int
	last  InboundEvent
}

func (r *recordingRestartFn) Do(_ context.Context, ev InboundEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.last = ev
	return nil
}

func (r *recordingRestartFn) snapshot() (int, InboundEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.last
}

func TestManager_Inbound_Restart_FiresRestartFuncOnce(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	store := &recordingRestartStore{}
	rf := &recordingRestartFn{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats:      map[string]string{"telegram": "42"},
		RestartMarkers:    store,
		RestartFunc:       rf.Do,
		RestartGracePause: time.Millisecond,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", UserID: "u", MsgID: "r1",
		Kind: EventRestart,
	})

	waitFor(t, 500*time.Millisecond, func() bool {
		c, _ := rf.snapshot()
		return c == 1
	})

	calls, gotEv := rf.snapshot()
	if calls != 1 {
		t.Fatalf("RestartFunc calls = %d, want 1", calls)
	}
	if gotEv.MsgID != "r1" || gotEv.ChatID != "42" || gotEv.Platform != "telegram" {
		t.Fatalf("RestartFunc event = %+v, want platform=telegram chat=42 msg=r1", gotEv)
	}

	// Takeover marker saved with the triggering (Platform, ChatID, MsgID).
	marker, saves := store.saveSnapshot()
	if saves != 1 {
		t.Fatalf("TakeoverMarker saves = %d, want 1", saves)
	}
	if marker.Platform != "telegram" || marker.ChatID != "42" || marker.MsgID != "r1" {
		t.Fatalf("TakeoverMarker = %+v, want platform=telegram chat=42 msg=r1", marker)
	}

	// User-visible restart notice must be sent.
	sent := tg.sentSnapshot()
	if len(sent) == 0 {
		t.Fatal("restart notice not sent")
	}
	notice := sent[0].Text
	if !strings.Contains(strings.ToLower(notice), "restart") {
		t.Fatalf("restart notice = %q, want contains 'restart'", notice)
	}

	// No kernel submit/reset for a restart event.
	if n := len(fk.submitsSnapshot()); n != 0 {
		t.Fatalf("kernel submits = %d, want 0", n)
	}
}

func TestManager_Inbound_Restart_RedeliveredIsDeduped(t *testing.T) {
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	store := &recordingRestartStore{
		preloaded: true,
		preloadVal: TakeoverMarker{
			Platform: "telegram", ChatID: "42", MsgID: "r1",
		},
	}
	rf := &recordingRestartFn{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats:   map[string]string{"telegram": "42"},
		RestartMarkers: store,
		RestartFunc:    rf.Do,
	}, fk, slog.Default())
	_ = m.Register(tg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "r1",
		Kind: EventRestart,
	})

	// Give the manager a beat to (not) act.
	time.Sleep(75 * time.Millisecond)

	if calls, _ := rf.snapshot(); calls != 0 {
		t.Fatalf("RestartFunc should not fire on a redelivered /restart, got calls=%d", calls)
	}

	// A fresh marker must not overwrite the preloaded one.
	_, saves := store.saveSnapshot()
	if saves != 0 {
		t.Fatalf("TakeoverMarker saves = %d on redelivered restart, want 0", saves)
	}

	// No user-visible duplicate notice should go out.
	if n := len(tg.sentSnapshot()); n != 0 {
		t.Fatalf("deduped redelivered restart sent %d reply messages, want 0", n)
	}
}

func TestManager_Inbound_Restart_WithoutRestartFuncRepliesUnsupported(t *testing.T) {
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
		Platform: "telegram", ChatID: "42", MsgID: "r1",
		Kind: EventRestart,
	})

	waitFor(t, 500*time.Millisecond, func() bool {
		return len(tg.sentSnapshot()) >= 1
	})

	sent := tg.sentSnapshot()
	if len(sent) == 0 {
		t.Fatal("expected unsupported reply, got none")
	}
	body := strings.ToLower(sent[0].Text)
	if !strings.Contains(body, "restart") || !strings.Contains(body, "unsupported") {
		t.Fatalf("unsupported reply = %q, want mention of restart and unsupported", sent[0].Text)
	}
	if n := len(fk.submitsSnapshot()); n != 0 {
		t.Fatalf("kernel submits on unsupported /restart = %d, want 0", n)
	}
}
