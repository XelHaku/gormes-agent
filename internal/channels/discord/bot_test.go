package discord

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{AllowedChannelID: "42"}, newMockSession(), nil)
	if b.Name() != "discord" {
		t.Errorf("Name() = %q", b.Name())
	}
}

func TestBot_ToInboundEvent_Submit(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "42"}, ms, nil)

	inbox := make(chan gateway.InboundEvent, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	time.Sleep(10 * time.Millisecond)
	ms.deliver(&discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m99",
		ChannelID: "42",
		Content:   "hello from discord",
		Author:    &discordgo.User{ID: "u1", Bot: false},
	}})

	select {
	case ev := <-inbox:
		if ev.Platform != "discord" || ev.ChatID != "42" {
			t.Errorf("got %+v", ev)
		}
		if ev.Kind != gateway.EventSubmit || ev.Text != "hello from discord" {
			t.Errorf("got %+v", ev)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_ToInboundEvent_IgnoresBotMessages(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "42"}, ms, nil)

	inbox := make(chan gateway.InboundEvent, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliver(&discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m1",
		ChannelID: "42",
		Content:   "bot reply",
		Author:    &discordgo.User{ID: "b1", Bot: true},
	}})

	select {
	case ev := <-inbox:
		t.Fatalf("expected no inbound from bot, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBot_ToInboundEvent_Commands(t *testing.T) {
	cases := []struct {
		text string
		want gateway.EventKind
	}{
		{"/help", gateway.EventStart},
		{"/start", gateway.EventStart},
		{"/stop", gateway.EventCancel},
		{"/new", gateway.EventReset},
		{"/xyzzy", gateway.EventUnknown},
		{"ordinary words", gateway.EventSubmit},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			ms := newMockSession()
			b := New(Config{AllowedChannelID: "42"}, ms, nil)
			inbox := make(chan gateway.InboundEvent, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = b.Run(ctx, inbox) }()
			time.Sleep(10 * time.Millisecond)

			ms.deliver(&discordgo.MessageCreate{Message: &discordgo.Message{
				ID:        "1",
				ChannelID: "42",
				Content:   c.text,
				Author:    &discordgo.User{ID: "u", Bot: false},
			}})
			select {
			case ev := <-inbox:
				if ev.Kind != c.want {
					t.Errorf("text=%q got Kind=%v want=%v", c.text, ev.Kind, c.want)
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatal("no inbound event")
			}
		})
	}
}

func TestBot_Send(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "42"}, ms, nil)

	id, err := b.Send(context.Background(), "42", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Errorf("empty id")
	}
	sent := ms.sentSnapshot()
	if len(sent) != 1 || sent[0].Content != "hi" {
		t.Errorf("sent = %+v", sent)
	}
}

func TestBot_Send_ForwardsError(t *testing.T) {
	ms := newMockSession()
	ms.sendErr = errUnderlying
	b := New(Config{AllowedChannelID: "42"}, ms, nil)
	if _, err := b.Send(context.Background(), "42", "x"); err == nil {
		t.Fatal("expected send error")
	}
}

func TestBot_SendPlaceholder(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "42"}, ms, nil)

	id, err := b.SendPlaceholder(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Errorf("empty id")
	}
	sent := ms.sentSnapshot()
	if len(sent) != 1 || !strings.Contains(sent[0].Content, "⏳") {
		t.Errorf("placeholder content = %+v", sent)
	}
}

func TestBot_EditMessage(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "42"}, ms, nil)

	if err := b.EditMessage(context.Background(), "42", "m1", "updated"); err != nil {
		t.Fatal(err)
	}
	edits := ms.editsSnapshot()
	if len(edits) != 1 || edits[0].Content != "updated" {
		t.Errorf("edits = %+v", edits)
	}
}

func TestBot_ReactToMessage(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "42"}, ms, nil)

	undo, err := b.ReactToMessage(context.Background(), "42", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if undo == nil {
		t.Fatal("undo is nil")
	}
	reacts := ms.reactionsAddedSnapshot()
	if len(reacts) != 1 {
		t.Errorf("reactions added = %+v", reacts)
	}

	undo()
	undo()
}

func TestDiscordAdapter_ManagerCleansSessionWhenStartupOpenFails(t *testing.T) {
	startupErr := errors.New("permission denied")
	ms := newMockSession()
	ms.openErr = startupErr
	bot := New(Config{AllowedChannelID: "42"}, ms, nil)

	mgr := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{}, &smokeKernel{frames: make(chan kernel.RenderFrame)}, nil)
	if err := mgr.Register(bot); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := mgr.Run(ctx)
	if !errors.Is(err, startupErr) {
		t.Fatalf("Run error = %v, want startup error %v", err, startupErr)
	}
	if !ms.closedSnapshot() {
		t.Fatal("discord session was not closed after startup failure")
	}
}

type smokeKernel struct {
	mu      sync.Mutex
	submits []kernel.PlatformEvent
	frames  chan kernel.RenderFrame
}

func (s *smokeKernel) Submit(ev kernel.PlatformEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submits = append(s.submits, ev)
	return nil
}

func (s *smokeKernel) ResetSession() error { return nil }
func (s *smokeKernel) Render() <-chan kernel.RenderFrame {
	return s.frames
}

func (s *smokeKernel) submitsSnapshot() []kernel.PlatformEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]kernel.PlatformEvent, len(s.submits))
	copy(out, s.submits)
	return out
}

func TestDiscordAdapter_ManagerSmokeE2E(t *testing.T) {
	ms := newMockSession()
	bot := New(Config{AllowedChannelID: "42"}, ms, nil)
	k := &smokeKernel{frames: make(chan kernel.RenderFrame, 8)}

	mgr := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats: map[string]string{"discord": "42"},
		CoalesceMs:   10,
		SessionMap:   session.NewMemMap(),
	}, k, nil)
	if err := mgr.Register(bot); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mgr.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliver(&discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m99",
		ChannelID: "42",
		Content:   "hello from discord",
		Author:    &discordgo.User{ID: "u1", Bot: false},
	}})

	waitForDiscord(t, 200*time.Millisecond, func() bool {
		return len(k.submitsSnapshot()) == 1
	})

	k.frames <- kernel.RenderFrame{Phase: kernel.PhaseStreaming, DraftText: "partial"}
	k.frames <- kernel.RenderFrame{
		Phase:   kernel.PhaseIdle,
		History: []hermes.Message{{Role: "assistant", Content: "done"}},
	}

	waitForDiscord(t, 500*time.Millisecond, func() bool {
		return len(ms.sentSnapshot()) >= 1 && len(ms.editsSnapshot()) >= 1
	})
}

func waitForDiscord(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

var errUnderlying = &simpleErr{"underlying"}

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
