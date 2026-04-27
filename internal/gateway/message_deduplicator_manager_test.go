package gateway

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

func TestGatewayInboundDedup_DropsRepeatedMessageID(t *testing.T) {
	ctx := context.Background()
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	status := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats:  map[string]string{"telegram": "chat-1"},
		RuntimeStatus: status,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ev := InboundEvent{
		Platform:  "telegram",
		ChatID:    "chat-1",
		ThreadID:  "thread-1",
		MsgID:     "gateway-msg-1",
		MessageID: "platform-msg-1",
		Kind:      EventSubmit,
		Text:      "first",
	}
	if err := m.handleInbound(ctx, ev); err != nil {
		t.Fatalf("first handleInbound: %v", err)
	}
	if err := m.handleInbound(ctx, ev); err != nil {
		t.Fatalf("duplicate handleInbound: %v", err)
	}
	m.drainNextFollowUp(ctx)

	submits := fk.submitsSnapshot()
	if len(submits) != 1 {
		t.Fatalf("kernel submits = %d, want only the first turn; submits=%#v", len(submits), submits)
	}
	if submits[0].Kind != kernel.PlatformEventSubmit || submits[0].Text != "first" {
		t.Fatalf("kernel submit = %+v, want first submit", submits[0])
	}
	assertGatewayDedupEvidence(t, status, "telegram", MessageDeduplicatorEvidenceDuplicate)
}

func TestGatewayInboundDedup_ScopesByChannelChatAndThread(t *testing.T) {
	ctx := context.Background()
	tg := newFakeChannel("telegram")
	dc := newFakeChannel("discord")
	fk := &fakeKernel{}
	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{
			"telegram": "chat-1",
			"discord":  "chat-1",
		},
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register telegram: %v", err)
	}
	if err := m.Register(dc); err != nil {
		t.Fatalf("Register discord: %v", err)
	}

	events := []InboundEvent{
		{
			Platform:  "telegram",
			ChatID:    "chat-1",
			ThreadID:  "thread-1",
			MsgID:     "gateway-msg-1",
			MessageID: "platform-msg-1",
			Kind:      EventSubmit,
			Text:      "base",
		},
		{
			Platform:  "telegram",
			ChatID:    "chat-2",
			ThreadID:  "thread-1",
			MsgID:     "gateway-msg-2",
			MessageID: "platform-msg-1",
			Kind:      EventSubmit,
			Text:      "different chat",
		},
		{
			Platform:  "telegram",
			ChatID:    "chat-1",
			ThreadID:  "thread-2",
			MsgID:     "gateway-msg-3",
			MessageID: "platform-msg-1",
			Kind:      EventSubmit,
			Text:      "different thread",
		},
		{
			Platform:  "discord",
			ChatID:    "chat-1",
			ThreadID:  "thread-1",
			MsgID:     "gateway-msg-4",
			MessageID: "platform-msg-1",
			Kind:      EventSubmit,
			Text:      "different channel",
		},
	}

	for _, ev := range events {
		m.cfg.AllowedChats[ev.Platform] = ev.ChatID
		if err := m.handleInbound(ctx, ev); err != nil {
			t.Fatalf("%s handleInbound: %v", ev.Text, err)
		}
		m.drainNextFollowUp(ctx)
	}

	submits := fk.submitsSnapshot()
	if len(submits) != len(events) {
		t.Fatalf("kernel submits = %d, want %d scoped turns; submits=%#v", len(submits), len(events), submits)
	}
	for i, ev := range events {
		if submits[i].Text != ev.Text {
			t.Fatalf("submit[%d].Text = %q, want %q; submits=%#v", i, submits[i].Text, ev.Text, submits)
		}
	}
}

func TestGatewayInboundDedup_MissingMessageIDDegrades(t *testing.T) {
	ctx := context.Background()
	tg := newFakeChannel("telegram")
	fk := &fakeKernel{}
	status := NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats:  map[string]string{"telegram": "chat-1"},
		RuntimeStatus: status,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, text := range []string{"first", "second"} {
		if err := m.handleInbound(ctx, InboundEvent{
			Platform:  "telegram",
			ChatID:    "chat-1",
			ThreadID:  "thread-1",
			MsgID:     "gateway-" + text,
			MessageID: "",
			Kind:      EventSubmit,
			Text:      text,
		}); err != nil {
			t.Fatalf("%s handleInbound: %v", text, err)
		}
	}
	m.drainNextFollowUp(ctx)

	submits := fk.submitsSnapshot()
	if len(submits) != 2 {
		t.Fatalf("kernel submits = %d, want both missing-ID submissions; submits=%#v", len(submits), submits)
	}
	for i, want := range []string{"first", "second"} {
		if submits[i].Text != want {
			t.Fatalf("submit[%d].Text = %q, want %q; submits=%#v", i, submits[i].Text, want, submits)
		}
	}
	assertGatewayDedupEvidence(t, status, "telegram", MessageDeduplicatorEvidenceMissingMessageID)
}

func assertGatewayDedupEvidence(t *testing.T, status *RuntimeStatusStore, platform string, want MessageDeduplicatorEvidence) {
	t.Helper()

	gotStatus, err := status.ReadRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("ReadRuntimeStatus: %v", err)
	}
	platformStatus, ok := gotStatus.Platforms[platform]
	if !ok {
		t.Fatalf("runtime status missing platform %q: %+v", platform, gotStatus.Platforms)
	}
	if platformStatus.ErrorMessage != string(want) {
		t.Fatalf("dedup evidence for %s = %q, want %q", platform, platformStatus.ErrorMessage, want)
	}
}
