package discord

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

func newScriptedKernel(reply, sid string) *kernel.Kernel {
	hc := hermes.NewMockClient()
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: len(reply)})
	hc.Script(events, sid)

	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hc, store.NewNoop(), telemetry.New(), nil)
}

func TestBot_SubmitsMentionedGuildMessage(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newScriptedKernel("roger", "sess-discord-guild")
	b := New(Config{
		AllowedGuildID:   "guild-1",
		AllowedChannelID: "chan-1",
		MentionRequired:  true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushMessage(InboundMessage{
		ChannelID:    "chan-1",
		GuildID:      "guild-1",
		AuthorID:     "user-1",
		Content:      "<@bot-1> hi",
		MentionedBot: true,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "roger") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("last sent text = %q, want streamed reply containing roger", mc.lastSentText())
}

func TestBot_RejectsGuildMessageWithoutMention(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newScriptedKernel("unused", "sess-unused")
	b := New(Config{
		AllowedGuildID:   "guild-1",
		AllowedChannelID: "chan-1",
		MentionRequired:  true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushMessage(InboundMessage{
		ChannelID:    "chan-1",
		GuildID:      "guild-1",
		AuthorID:     "user-1",
		Content:      "hi without mention",
		MentionedBot: false,
	})

	time.Sleep(100 * time.Millisecond)
	if got := len(mc.sentTexts()); got != 0 {
		t.Fatalf("sent texts = %d, want 0", got)
	}
}

func TestBot_AcceptsDMDiscoveryAndPersistsSession(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newScriptedKernel("dm ok", "sess-discord-dm")
	smap := session.NewMemMap()
	b := New(Config{
		AllowedChannelID: "",
		MentionRequired:  true,
		CoalesceMs:       50,
		SessionMap:       smap,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushMessage(InboundMessage{
		ChannelID: "dm-42",
		AuthorID:  "user-1",
		Content:   "hello in dm",
		IsDM:      true,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastSentText(), "dm ok") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	gotSID, err := smap.Get(context.Background(), SessionKey("dm-42"))
	if err != nil {
		t.Fatalf("Get persisted session: %v", err)
	}
	if gotSID != "sess-discord-dm" {
		t.Fatalf("persisted sid = %q, want sess-discord-dm", gotSID)
	}
}
