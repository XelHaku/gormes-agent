package gateway

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

func TestManager_Inbound_SubmitInjectsSessionContext(t *testing.T) {
	tg := newFakeChannel("telegram")
	dc := newFakeChannel("discord")
	fk := &fakeKernel{}
	smap := session.NewMemMap()
	if err := smap.Put(context.Background(), "telegram:42", "sess-stored"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	m := NewManagerWithSubmitter(ManagerConfig{
		AllowedChats: map[string]string{"telegram": "42"},
		SessionMap:   smap,
	}, fk, slog.Default())
	if err := m.Register(tg); err != nil {
		t.Fatalf("Register telegram: %v", err)
	}
	if err := m.Register(dc); err != nil {
		t.Fatalf("Register discord: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()

	tg.pushInbound(InboundEvent{
		Platform: "telegram",
		ChatID:   "42",
		UserID:   "7",
		MsgID:    "m1",
		Kind:     EventSubmit,
		Text:     "hello",
	})

	waitFor(t, 200*time.Millisecond, func() bool {
		return len(fk.submitsSnapshot()) == 1
	})

	got := fk.submitsSnapshot()[0]
	if got.SessionID != "sess-stored" {
		t.Fatalf("SessionID = %q, want %q", got.SessionID, "sess-stored")
	}
	for _, want := range []string{
		"## Current Session Context",
		"**Source:** telegram chat `42`",
		"`origin`",
		"`local`",
		"`discord`",
		"`telegram`",
	} {
		if !strings.Contains(got.SessionContext, want) {
			t.Fatalf("SessionContext missing %q in:\n%s", want, got.SessionContext)
		}
	}
}
