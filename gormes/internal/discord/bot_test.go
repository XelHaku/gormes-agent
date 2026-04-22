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

func newIdleKernel() *kernel.Kernel {
	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)
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

func TestBot_BindTurnForFrame_ClaimsReservedChannelBeforeFastTurnFrames(t *testing.T) {
	b := New(Config{}, newMockClient("bot-1"), newIdleKernel(), nil)

	ticket := b.reserveTurn("chan-fast")
	if ticket == 0 {
		t.Fatal("reserveTurn returned 0, want non-zero ticket")
	}

	if got := b.bindTurnForFrame(); got != "chan-fast" {
		t.Fatalf("bindTurnForFrame = %q, want chan-fast", got)
	}
	if got := b.currentTurnChannel(); got != "chan-fast" {
		t.Fatalf("currentTurnChannel = %q, want chan-fast", got)
	}
}

func TestBot_PersistIfChanged_UsesTurnOwnedChannel(t *testing.T) {
	smap := session.NewMemMap()
	b := New(Config{SessionMap: smap}, newMockClient("bot-1"), newIdleKernel(), nil)

	b.reserveTurn("chan-a")
	if got := b.bindTurnForFrame(); got != "chan-a" {
		t.Fatalf("first bindTurnForFrame = %q, want chan-a", got)
	}

	b.persistIfChanged(context.Background(), "chan-a", kernel.RenderFrame{SessionID: "sess-a"})
	if got, err := smap.Get(context.Background(), SessionKey("chan-a")); err != nil || got != "sess-a" {
		t.Fatalf("SessionMap[chan-a] = %q, %v, want sess-a, nil", got, err)
	}
	if got, _ := smap.Get(context.Background(), SessionKey("chan-b")); got != "" {
		t.Fatalf("SessionMap[chan-b] = %q, want empty before second turn", got)
	}

	b.finishTurn()
	b.reserveTurn("chan-b")
	if got := b.bindTurnForFrame(); got != "chan-b" {
		t.Fatalf("second bindTurnForFrame = %q, want chan-b", got)
	}
	b.persistIfChanged(context.Background(), "chan-b", kernel.RenderFrame{SessionID: "sess-b"})

	if got, _ := smap.Get(context.Background(), SessionKey("chan-a")); got != "sess-a" {
		t.Fatalf("SessionMap[chan-a] = %q, want sess-a", got)
	}
	if got, _ := smap.Get(context.Background(), SessionKey("chan-b")); got != "sess-b" {
		t.Fatalf("SessionMap[chan-b] = %q, want sess-b", got)
	}
}

func TestBot_NewCommandResetsSessionAndClearsMap(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newIdleKernel()
	smap := session.NewMemMap()
	if err := smap.Put(context.Background(), SessionKey("chan-1"), "sess-old"); err != nil {
		t.Fatalf("seed SessionMap: %v", err)
	}

	b := New(Config{
		AllowedGuildID:   "guild-1",
		AllowedChannelID: "chan-1",
		MentionRequired:  true,
		SessionMap:       smap,
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
		Content:      "<@bot-1> /new",
		MentionedBot: true,
	})

	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(mc.lastSentText(), "Session reset") {
		t.Fatalf("last sent text = %q, want Session reset reply", mc.lastSentText())
	}
	if got, _ := smap.Get(context.Background(), SessionKey("chan-1")); got != "" {
		t.Fatalf("SessionMap[chan-1] = %q, want cleared entry", got)
	}
}

func TestBot_StopCommandDoesNotEmitBusy(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newIdleKernel()
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
		Content:      "<@bot-1> /stop",
		MentionedBot: true,
	})

	time.Sleep(100 * time.Millisecond)
	if strings.Contains(mc.lastSentText(), "Busy") {
		t.Fatalf("last sent text = %q, want no Busy error on /stop", mc.lastSentText())
	}
}

func TestBot_UnsupportedSlashRepliesUnknownCommand(t *testing.T) {
	mc := newMockClient("bot-1")
	k := newIdleKernel()
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
		Content:      "<@bot-1> /wat",
		MentionedBot: true,
	})

	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(mc.lastSentText(), "unknown command") {
		t.Fatalf("last sent text = %q, want unknown command reply", mc.lastSentText())
	}
}

func TestCoalescer_FlushImmediate_EditFailureFallsBackToSend(t *testing.T) {
	mc := newMockClient("bot-1")
	mc.EditErr = errEditFailed()

	c := newCoalescer(mc, time.Second, "chan-1")
	c.flushImmediate("⏳")
	c.flushImmediate("final")

	if got := len(mc.sendCalls()); got != 2 {
		t.Fatalf("send calls = %d, want 2 with fallback send", got)
	}
	if got := len(mc.editCalls()); got != 1 {
		t.Fatalf("edit calls = %d, want 1", got)
	}
	if got := mc.lastSentText(); got != "final" {
		t.Fatalf("last sent text = %q, want final", got)
	}
}
