package discord

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

func TestDiscordForumChannelDetection_Fixture(t *testing.T) {
	forum := loadDiscordChannelFixture(t, "forum_channel.json")
	if !isForumChannel(forum) {
		t.Fatalf("isForumChannel(%+v) = false, want true", forum)
	}

	thread := &discordgo.Channel{ID: "thread-1", Type: discordgo.ChannelTypeGuildPublicThread, ParentID: forum.ID}
	if isForumChannel(thread) {
		t.Fatalf("isForumChannel(public thread) = true, want false")
	}
}

func TestBot_ForumPostMessageUsesParentChatAndCanonicalThreadID(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "forum-100"}, ms, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliverThreadCreate(&discordgo.ThreadCreate{Channel: loadDiscordChannelFixture(t, "forum_thread_create.json")})
	drainOptionalEvent(inbox)
	ms.deliver(loadDiscordMessageCreateFixture(t, "forum_thread_message.json"))

	select {
	case ev := <-inbox:
		if ev.Platform != "discord" {
			t.Fatalf("Platform = %q, want discord", ev.Platform)
		}
		if ev.ChatID != "forum-100" {
			t.Fatalf("ChatID = %q, want parent forum id forum-100", ev.ChatID)
		}
		if ev.ThreadID != "thread-200" {
			t.Fatalf("ThreadID = %q, want canonical forum post thread id thread-200", ev.ThreadID)
		}
		if ev.ChatName != "Support case 123" {
			t.Fatalf("ChatName = %q, want forum post name", ev.ChatName)
		}
		if ev.Kind != gateway.EventSubmit || ev.Text != "follow up from the forum post" {
			t.Fatalf("event payload = %+v, want submit with forum post text", ev)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_ThreadLifecycleEventsNormalizeOpenCloseArchive(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "forum-100"}, ms, nil)
	inbox := make(chan gateway.InboundEvent, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliverThreadCreate(&discordgo.ThreadCreate{Channel: loadDiscordChannelFixture(t, "forum_thread_create.json")})
	assertThreadLifecycle(t, inbox, gateway.ThreadLifecycleOpen)

	ms.deliverThreadUpdate(&discordgo.ThreadUpdate{Channel: loadDiscordChannelFixture(t, "forum_thread_archived.json")})
	assertThreadLifecycle(t, inbox, gateway.ThreadLifecycleArchived)

	ms.deliverThreadUpdate(&discordgo.ThreadUpdate{Channel: loadDiscordChannelFixture(t, "forum_thread_locked.json")})
	assertThreadLifecycle(t, inbox, gateway.ThreadLifecycleClosed)
}

func TestBot_ThreadDeleteNormalizesClosedLifecycle(t *testing.T) {
	ms := newMockSession()
	b := New(Config{AllowedChannelID: "forum-100"}, ms, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()
	time.Sleep(10 * time.Millisecond)

	ms.deliverThreadDelete(&discordgo.ThreadDelete{Channel: loadDiscordChannelFixture(t, "forum_thread_delete.json")})
	assertThreadLifecycle(t, inbox, gateway.ThreadLifecycleClosed)
}

func loadDiscordChannelFixture(t *testing.T, name string) *discordgo.Channel {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	var channel discordgo.Channel
	if err := json.Unmarshal(raw, &channel); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
	return &channel
}

func loadDiscordMessageCreateFixture(t *testing.T, name string) *discordgo.MessageCreate {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	var msg discordgo.MessageCreate
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
	return &msg
}

func assertThreadLifecycle(t *testing.T, inbox <-chan gateway.InboundEvent, want gateway.ThreadLifecycleState) {
	t.Helper()

	select {
	case ev := <-inbox:
		if ev.Kind != gateway.EventThreadLifecycle {
			t.Fatalf("Kind = %v, want %v in event %+v", ev.Kind, gateway.EventThreadLifecycle, ev)
		}
		if ev.ChatID != "forum-100" || ev.ThreadID != "thread-200" {
			t.Fatalf("thread address = chat:%q thread:%q, want forum-100/thread-200", ev.ChatID, ev.ThreadID)
		}
		if ev.ThreadLifecycle == nil {
			t.Fatalf("ThreadLifecycle = nil in event %+v", ev)
		}
		if ev.ThreadLifecycle.State != want {
			t.Fatalf("ThreadLifecycle.State = %q, want %q", ev.ThreadLifecycle.State, want)
		}
		if ev.ThreadLifecycle.Name != "Support case 123" {
			t.Fatalf("ThreadLifecycle.Name = %q, want Support case 123", ev.ThreadLifecycle.Name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("no lifecycle event for state %q", want)
	}
}

func drainOptionalEvent(inbox <-chan gateway.InboundEvent) {
	select {
	case <-inbox:
	case <-time.After(20 * time.Millisecond):
	}
}
