package slack

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

func newSlackKernel(reply, sid string) *kernel.Kernel {
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

func TestBot_AcksEventsBeforeHandling(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("roger", "sess-slack")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-1",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello",
		Timestamp: "1711111111.000100",
		ThreadTS:  "1711111111.000100",
	})

	time.Sleep(100 * time.Millisecond)
	if !mc.wasAcked("req-1") {
		t.Fatal("expected request req-1 to be acked")
	}
}

func TestBot_UsesThreadTSForReplies(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("thread ok", "sess-thread")
	smap := session.NewMemMap()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
		SessionMap:       smap,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-2",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello thread",
		Timestamp: "1711111111.000200",
		ThreadTS:  "1711111111.000200",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastOutputText(), "thread ok") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if mc.lastThreadTS() != "1711111111.000200" {
		t.Fatalf("thread_ts = %q, want 1711111111.000200", mc.lastThreadTS())
	}
	gotSID, err := smap.Get(context.Background(), SessionKey("C123"))
	if err != nil {
		t.Fatalf("Get persisted session: %v", err)
	}
	if gotSID != "sess-thread" {
		t.Fatalf("persisted sid = %q, want sess-thread", gotSID)
	}
}

func TestBot_RejectsOtherChannels(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("unused", "sess-unused")
	b := New(Config{AllowedChannelID: "C123", ReplyInThread: true}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-3",
		ChannelID: "C999",
		UserID:    "U1",
		Text:      "wrong room",
		Timestamp: "1711111111.000300",
	})

	time.Sleep(100 * time.Millisecond)
	if got := len(mc.outputs()); got != 0 {
		t.Fatalf("outputs = %d, want 0", got)
	}
}
