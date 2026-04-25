package gateway

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
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

func TestManager_Inbound_AppendsAttachmentsToSubmittedText(t *testing.T) {
	tg := newFakeChannel("dingtalk")
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"dingtalk": "dm-42"},
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "dingtalk",
		ChatID:   "dm-42",
		UserID:   "staff-1",
		MsgID:    "msg-1",
		Kind:     EventSubmit,
		Text:     "please inspect",
		Attachments: []Attachment{
			{
				Kind:      "image",
				URL:       "https://media.dingtalk.example/image.png",
				MediaType: "image",
				SourceID:  "img-code-1",
			},
			{
				Kind:      "file",
				URL:       "file-code-timeout",
				MediaType: "application/octet-stream",
				FileName:  "report.pdf",
				SourceID:  "file-code-timeout",
				Error:     "dingtalk: media download: 429 rate limit",
			},
		},
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})
	got := fk.submitsSnapshot()[0]
	want := "please inspect\n\nAttachments:\n- image: https://media.dingtalk.example/image.png (mediaType=image, sourceId=img-code-1)\n- file report.pdf: file-code-timeout (mediaType=application/octet-stream, sourceId=file-code-timeout, error=dingtalk: media download: 429 rate limit)"
	if got.Text != want {
		t.Fatalf("submitted text = %q, want %q", got.Text, want)
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

func TestManager_Outbound_NonEditableChannelUsesPlainSendForInterimAndFinal(t *testing.T) {
	ch := newChannelOnlyFake("plainchat")
	if _, ok := any(ch).(placeholderEditor); ok {
		t.Fatal("channel-only fixture unexpectedly implements placeholder editing")
	}
	if _, ok := any(ch).(PlaceholderCapable); ok {
		t.Fatal("channel-only fixture unexpectedly implements SendPlaceholder")
	}
	if _, ok := any(ch).(MessageEditor); ok {
		t.Fatal("channel-only fixture unexpectedly implements EditMessage")
	}

	frames := make(chan kernel.RenderFrame, 8)
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"plainchat": "thread-42"},
		CoalesceMs:   10,
	}, fk, slog.Default())
	m.setRenderChan(frames)
	if err := m.Register(ch); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	ch.pushInbound(InboundEvent{
		Platform: "plainchat", ChatID: "thread-42", MsgID: "origin-msg",
		Kind: EventSubmit, Text: "hi",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	frames <- kernel.RenderFrame{
		Phase:     kernel.PhaseStreaming,
		DraftText: "I'll inspect the repo first.",
	}
	frames <- kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "done"},
		},
	}

	waitFor(t, 500*time.Millisecond, func() bool {
		return len(ch.sentSnapshot()) == 2
	})

	got := ch.sentSnapshot()
	wantTexts := []string{"I'll inspect the repo first.", "done"}
	for i, want := range wantTexts {
		if got[i].ChatID != "thread-42" {
			t.Fatalf("sent[%d].ChatID = %q, want original chat target %q", i, got[i].ChatID, "thread-42")
		}
		if got[i].Text != want {
			t.Fatalf("sent[%d].Text = %q, want %q; sends=%#v", i, got[i].Text, want, got)
		}
	}
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

func TestManager_ActiveTurnQueuesFollowUpUntilTerminalFrame(t *testing.T) {
	tg := newFakeChannel("telegram")
	dc := newFakeChannel("discord")
	frames := make(chan kernel.RenderFrame, 8)
	fk := &fakeKernel{}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{
			"telegram": "42",
			"discord":  "99",
		},
		CoalesceMs: 10,
	}, fk, slog.Default())
	m.setRenderChan(frames)
	_ = m.Register(tg)
	_ = m.Register(dc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m1",
		Kind: EventSubmit, Text: "first",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	dc.pushInbound(InboundEvent{
		Platform: "discord", ChatID: "99", MsgID: "m2",
		Kind: EventSubmit, Text: "second",
	})
	time.Sleep(30 * time.Millisecond)
	if got := fk.submitsSnapshot(); len(got) != 1 {
		t.Fatalf("active-turn follow-up submitted immediately: submits=%#v", got)
	}

	frames <- kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "first answer"},
		},
	}

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 2 && len(tg.sentSnapshot()) == 1
	})
	if got := tg.sentSnapshot()[0].Text; got != "first answer" {
		t.Fatalf("first terminal reply routed to telegram = %q, want first answer", got)
	}
	if sent := dc.sentSnapshot(); len(sent) != 0 {
		t.Fatalf("discord received terminal reply for active telegram turn: %#v", sent)
	}
	got := fk.submitsSnapshot()
	if got[1].Text != "second" {
		t.Fatalf("drained follow-up text = %q, want second", got[1].Text)
	}
}

func TestManager_LateArrivalDuringFollowUpDrainQueuesBehindDrainedTurn(t *testing.T) {
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
		Kind: EventSubmit, Text: "first",
	})
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m2",
		Kind: EventSubmit, Text: "second",
	})
	time.Sleep(30 * time.Millisecond)
	if got := fk.submitsSnapshot(); len(got) != 1 {
		t.Fatalf("queued follow-up submitted before active turn drained: submits=%#v", got)
	}

	frames <- kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "first answer"},
		},
	}
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 2
	})

	tg.pushInbound(InboundEvent{
		Platform: "telegram", ChatID: "42", MsgID: "m3",
		Kind: EventSubmit, Text: "third",
	})
	time.Sleep(30 * time.Millisecond)
	if got := fk.submitsSnapshot(); len(got) != 2 {
		t.Fatalf("late arrival submitted during drained turn: submits=%#v", got)
	}

	frames <- kernel.RenderFrame{
		Phase: kernel.PhaseIdle,
		History: []hermes.Message{
			{Role: "user", Content: "second"},
			{Role: "assistant", Content: "second answer"},
		},
	}
	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 3
	})

	got := fk.submitsSnapshot()
	for i, want := range []string{"first", "second", "third"} {
		if got[i].Text != want {
			t.Fatalf("submit %d text = %q, want %q; submits=%#v", i, got[i].Text, want, got)
		}
	}
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

func TestManager_RunCleansStartupFailedChannelAndReturnsOriginalError(t *testing.T) {
	startupErr := errors.New("discord: open session: denied")
	ch := &startupFailedChannel{name: "discord", runErr: startupErr}

	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err := m.Register(ch); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := m.Run(ctx)
	if !errors.Is(err, startupErr) {
		t.Fatalf("Run error = %v, want startup error %v", err, startupErr)
	}
	if got := ch.disconnectCount(); got != 1 {
		t.Fatalf("disconnect count = %d, want 1", got)
	}
}

func TestManager_RunLogsCleanupErrorWithoutMaskingStartupFailure(t *testing.T) {
	startupErr := errors.New("discord: open session: denied")
	cleanupErr := errors.New("discord: close partial session: denied")
	ch := &startupFailedChannel{name: "discord", runErr: startupErr, disconnectErr: cleanupErr}
	var logs bytes.Buffer

	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	if err := m.Register(ch); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := m.Run(ctx)
	if !errors.Is(err, startupErr) {
		t.Fatalf("Run error = %v, want startup error %v", err, startupErr)
	}
	gotLogs := logs.String()
	if !strings.Contains(gotLogs, "defensive channel disconnect after failed startup raised") ||
		!strings.Contains(gotLogs, "discord") ||
		!strings.Contains(gotLogs, cleanupErr.Error()) {
		t.Fatalf("cleanup failure log = %q, want channel name and cleanup error", gotLogs)
	}
}

func TestManager_RunCleansFailedStartupChannelWithoutStoppingHealthyChannels(t *testing.T) {
	startupErr := errors.New("discord: open session: denied")
	failed := &startupFailedChannel{name: "discord", runErr: startupErr}
	healthy := newFakeChannel("telegram")

	m := NewManagerWithSubmitter(ManagerConfig{}, &fakeKernel{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err := m.Register(failed); err != nil {
		t.Fatalf("Register failed channel: %v", err)
	}
	if err := m.Register(healthy); err != nil {
		t.Fatalf("Register healthy channel: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	waitFor(t, 200*time.Millisecond, func() bool {
		return failed.disconnectCount() == 1
	})

	select {
	case err := <-done:
		t.Fatalf("Run returned while a healthy channel was still running: %v", err)
	default:
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after context cancellation = %v, want nil", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run did not return after context cancellation")
	}
}

type startupFailedChannel struct {
	name          string
	runErr        error
	disconnectErr error

	mu          sync.Mutex
	disconnects int
}

func (c *startupFailedChannel) Name() string { return c.name }

func (c *startupFailedChannel) Run(context.Context, chan<- InboundEvent) error {
	return c.runErr
}

func (c *startupFailedChannel) Send(context.Context, string, string) (string, error) {
	return "", errors.New("startup failed channel cannot send")
}

func (c *startupFailedChannel) Disconnect(context.Context) error {
	c.mu.Lock()
	c.disconnects++
	c.mu.Unlock()
	return c.disconnectErr
}

func (c *startupFailedChannel) disconnectCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnects
}
