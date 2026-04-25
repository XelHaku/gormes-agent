package discord

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

func TestDiscordSessionSourceMetadata_ForumPostFlowsIntoSessionContext(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "forum-100"}, ms, nil)
	fk := newSessionSourceMetadataKernel()
	m := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats: map[string]string{"discord": "forum-100"},
	}, fk, nil)
	if err := m.Register(b); err != nil {
		t.Fatalf("Register discord: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliverThreadCreate(&discordgo.ThreadCreate{Channel: loadDiscordChannelFixture(t, "forum_thread_create.json")})
	ms.deliver(loadDiscordMessageCreateFixture(t, "forum_thread_message.json"))

	waitForSessionSourceSubmits(t, fk, 1)
	got := fk.submitsSnapshot()[0]
	if got.Text != "follow up from the forum post" {
		t.Fatalf("submit Text = %q, want forum post body", got.Text)
	}
	for _, want := range []string{
		"**Source:** discord chat `forum-100`",
		"**Guild ID:** `guild-1`",
		"**Parent Chat ID:** `forum-100`",
		"**Thread ID:** `thread-200`",
		"**Message ID:** `msg-201`",
		"**Session Key:** `discord:forum-100`",
	} {
		if !strings.Contains(got.SessionContext, want) {
			t.Fatalf("SessionContext missing %q in:\n%s", want, got.SessionContext)
		}
	}
}

func TestDiscordSessionSourceMetadata_EventPreservesRoutingAndSourceIDs(t *testing.T) {
	b := New(Config{AllowedChannelID: "forum-100"}, newMockSession(), nil)
	b.rememberThread(loadDiscordChannelFixture(t, "forum_thread_create.json"))

	msg := loadDiscordMessageCreateFixture(t, "forum_thread_message.json")
	ev, ok := b.toInboundEvent(msg.Message)
	if !ok {
		t.Fatal("toInboundEvent returned ok=false")
	}

	if ev.ChatID != "forum-100" {
		t.Fatalf("ChatID = %q, want parent forum id forum-100", ev.ChatID)
	}
	if ev.ThreadID != "thread-200" {
		t.Fatalf("ThreadID = %q, want canonical thread id thread-200", ev.ThreadID)
	}
	if ev.MsgID != "msg-201" {
		t.Fatalf("MsgID = %q, want triggering message id msg-201", ev.MsgID)
	}
	if ev.GuildID != "guild-1" {
		t.Fatalf("GuildID = %q, want guild-1", ev.GuildID)
	}
	if ev.ParentChatID != "forum-100" {
		t.Fatalf("ParentChatID = %q, want forum-100", ev.ParentChatID)
	}
	if ev.MessageID != "msg-201" {
		t.Fatalf("MessageID = %q, want msg-201", ev.MessageID)
	}
}

func TestDiscordSessionSourceMetadata_DMOmitsUnavailableScopeIDs(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "dm-42"}, ms, nil)
	fk := newSessionSourceMetadataKernel()
	m := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats: map[string]string{"discord": "dm-42"},
	}, fk, nil)
	if err := m.Register(b); err != nil {
		t.Fatalf("Register discord: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliver(&discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "dm-msg-1",
		ChannelID: "dm-42",
		Content:   "hello from dm",
		Author:    &discordgo.User{ID: "user-1", Bot: false},
	}})

	waitForSessionSourceSubmits(t, fk, 1)
	got := fk.submitsSnapshot()[0]
	for _, want := range []string{
		"**Source:** discord chat `dm-42`",
		"**Message ID:** `dm-msg-1`",
		"**Session Key:** `discord:dm-42`",
	} {
		if !strings.Contains(got.SessionContext, want) {
			t.Fatalf("SessionContext missing %q in:\n%s", want, got.SessionContext)
		}
	}
	for _, forbidden := range []string{
		"**Guild ID:**",
		"**Parent Chat ID:**",
		"**Thread ID:**",
	} {
		if strings.Contains(got.SessionContext, forbidden) {
			t.Fatalf("SessionContext unexpectedly contains %q in:\n%s", forbidden, got.SessionContext)
		}
	}
}

type sessionSourceMetadataKernel struct {
	mu      sync.Mutex
	submits []kernel.PlatformEvent
	render  chan kernel.RenderFrame
}

func newSessionSourceMetadataKernel() *sessionSourceMetadataKernel {
	return &sessionSourceMetadataKernel{render: make(chan kernel.RenderFrame)}
}

func (k *sessionSourceMetadataKernel) Submit(ev kernel.PlatformEvent) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.submits = append(k.submits, ev)
	return nil
}

func (k *sessionSourceMetadataKernel) ResetSession() error {
	return nil
}

func (k *sessionSourceMetadataKernel) Render() <-chan kernel.RenderFrame {
	return k.render
}

func (k *sessionSourceMetadataKernel) submitsSnapshot() []kernel.PlatformEvent {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]kernel.PlatformEvent, len(k.submits))
	copy(out, k.submits)
	return out
}

func waitForSessionSourceSubmits(t *testing.T, fk *sessionSourceMetadataKernel, want int) {
	t.Helper()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(fk.submitsSnapshot()) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("kernel submits = %d, want at least %d", len(fk.submitsSnapshot()), want)
}
