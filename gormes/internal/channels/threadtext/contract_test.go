package threadtext

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestCanonicalThreadID_PrefersThreadRootID(t *testing.T) {
	got := CanonicalThreadID(InboundMessage{
		ThreadID:     "reply-2",
		ThreadRootID: "thread-1",
	})
	if got != "thread-1" {
		t.Fatalf("CanonicalThreadID() = %q, want %q", got, "thread-1")
	}
}

func TestNormalizeInbound_UsesCanonicalThreadIDAndSharedCommandParser(t *testing.T) {
	ev, ok := NormalizeInbound(" mattermost ", InboundMessage{
		ChatID:       "town-square",
		ChatName:     "Town Square",
		UserID:       "user-1",
		UserName:     "Alice",
		MessageID:    "post-9",
		ThreadID:     "reply-9",
		ThreadRootID: "thread-1",
		Text:         " /start ",
	})
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if ev.Platform != "mattermost" {
		t.Fatalf("Platform = %q, want %q", ev.Platform, "mattermost")
	}
	if ev.ThreadID != "thread-1" {
		t.Fatalf("ThreadID = %q, want %q", ev.ThreadID, "thread-1")
	}
	if ev.Kind != gateway.EventStart {
		t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventStart)
	}
	if ev.Text != "" {
		t.Fatalf("Text = %q, want empty command body", ev.Text)
	}
}

func TestNormalizeInbound_RejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		msg      InboundMessage
	}{
		{
			name:     "missing platform",
			platform: " ",
			msg: InboundMessage{
				ChatID: "town-square",
				UserID: "user-1",
				Text:   "hello",
			},
		},
		{
			name:     "missing chat",
			platform: "mattermost",
			msg: InboundMessage{
				UserID: "user-1",
				Text:   "hello",
			},
		},
		{
			name:     "missing user",
			platform: "mattermost",
			msg: InboundMessage{
				ChatID: "town-square",
				Text:   "hello",
			},
		},
		{
			name:     "empty text",
			platform: "mattermost",
			msg: InboundMessage{
				ChatID: "town-square",
				UserID: "user-1",
				Text:   "   ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := NormalizeInbound(tt.platform, tt.msg); ok {
				t.Fatalf("NormalizeInbound(%q, %+v) ok = true, want false", tt.platform, tt.msg)
			}
		})
	}
}

func TestResolveReplyTarget_PreservesExistingThread(t *testing.T) {
	got, ok := ResolveReplyTarget(InboundMessage{
		ChatID:       "town-square",
		MessageID:    "post-9",
		ThreadID:     "reply-9",
		ThreadRootID: "thread-1",
	}, ReplyModeFlat)
	if !ok {
		t.Fatal("ResolveReplyTarget() ok = false, want true")
	}
	want := ReplyTarget{
		ChatID:           "town-square",
		ThreadID:         "thread-1",
		ReplyToMessageID: "post-9",
	}
	if got != want {
		t.Fatalf("ResolveReplyTarget() = %+v, want %+v", got, want)
	}
}

func TestResolveReplyTarget_StartsThreadFromRootMessageWhenConfigured(t *testing.T) {
	got, ok := ResolveReplyTarget(InboundMessage{
		ChatID:    "town-square",
		MessageID: "post-9",
	}, ReplyModeThread)
	if !ok {
		t.Fatal("ResolveReplyTarget() ok = false, want true")
	}
	want := ReplyTarget{
		ChatID:           "town-square",
		ThreadID:         "post-9",
		ReplyToMessageID: "post-9",
	}
	if got != want {
		t.Fatalf("ResolveReplyTarget() = %+v, want %+v", got, want)
	}
}

func TestResolveReplyTarget_LeavesRootReplyFlatWhenThreadModeDisabled(t *testing.T) {
	got, ok := ResolveReplyTarget(InboundMessage{
		ChatID:    "town-square",
		MessageID: "post-9",
	}, ReplyModeFlat)
	if !ok {
		t.Fatal("ResolveReplyTarget() ok = false, want true")
	}
	want := ReplyTarget{ChatID: "town-square"}
	if got != want {
		t.Fatalf("ResolveReplyTarget() = %+v, want %+v", got, want)
	}
}

func TestResolveReplyTarget_RejectsMissingChatID(t *testing.T) {
	if _, ok := ResolveReplyTarget(InboundMessage{MessageID: "post-9"}, ReplyModeThread); ok {
		t.Fatal("ResolveReplyTarget() ok = true, want false")
	}
}
