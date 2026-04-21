package slack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
	slackevents "github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type failingSessionMap struct {
	putErr error
}

func (m failingSessionMap) Get(context.Context, string) (string, error) { return "", nil }
func (m failingSessionMap) Put(context.Context, string, string) error   { return m.putErr }
func (m failingSessionMap) Close() error                                { return nil }

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

func newIdleSlackKernel() *kernel.Kernel {
	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)
}

type failingStore struct {
	err error
}

func (s failingStore) Exec(context.Context, store.Command) (store.Ack, error) {
	return store.Ack{}, s.err
}

func newFailingSlackKernel(err error) *kernel.Kernel {
	return kernel.New(kernel.Config{
		Model:             "hermes-agent",
		Endpoint:          "http://mock",
		Admission:         kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
		MaxToolIterations: 10,
		MaxToolDuration:   5 * time.Second,
	}, hermes.NewMockClient(), failingStore{err: err}, telemetry.New(), nil)
}

func waitForSlackOutput(t *testing.T, mc *mockClient, needle string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(mc.lastOutputText(), needle) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("last output = %q, want to contain %q", mc.lastOutputText(), needle)
}

func TestBot_AcksEventsBeforeHandling(t *testing.T) {
	mc := newMockClient()
	k := newIdleSlackKernel()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-1",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/start",
		Timestamp: "1711111111.000100",
		ThreadTS:  "1711111111.000100",
	})

	waitForSlackOutput(t, mc, "Gormes is online")
	if !mc.wasAcked("req-1") {
		t.Fatal("expected request req-1 to be acked")
	}
	calls := mc.calls()
	if len(calls) < 2 {
		t.Fatalf("calls = %v, want ack and reply call", calls)
	}
	if calls[0] != "ack:req-1" {
		t.Fatalf("first call = %q, want ack:req-1", calls[0])
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
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-2",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello thread",
		Timestamp: "1711111111.000200",
		ThreadTS:  "1711111111.000200",
	})

	waitForSlackOutput(t, mc, "thread ok")

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

func TestBot_UsesTimestampFallbackForRepliesWhenThreadMissing(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("timestamp ok", "sess-ts")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-ts",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello timestamp",
		Timestamp: "1711111111.000250",
	})

	waitForSlackOutput(t, mc, "timestamp ok")
	if mc.lastThreadTS() != "1711111111.000250" {
		t.Fatalf("thread_ts = %q, want 1711111111.000250", mc.lastThreadTS())
	}
}

func TestBot_RejectsOtherChannels(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("unused", "sess-unused")
	b := New(Config{AllowedChannelID: "C123", ReplyInThread: true}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
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

func TestBot_RejectsAllChannelsWhenAllowedChannelUnset(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("unused", "sess-unused")
	b := New(Config{ReplyInThread: true}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-unset-channel",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/start",
		Timestamp: "1711111111.000301",
	})

	time.Sleep(100 * time.Millisecond)
	if !mc.wasAcked("req-unset-channel") {
		t.Fatal("expected request to be acked even when channel allowlist is unset")
	}
	if got := len(mc.outputs()); got != 0 {
		t.Fatalf("outputs = %d, want 0 when AllowedChannelID is unset", got)
	}
}

func TestBot_IgnoresThreadBroadcastSubtype(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("unused", "sess-unused")
	b := New(Config{AllowedChannelID: "C123", ReplyInThread: true}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-subtype",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "broadcast copy",
		Timestamp: "1711111111.000350",
		SubType:   "thread_broadcast",
	})

	time.Sleep(100 * time.Millisecond)
	if !mc.wasAcked("req-subtype") {
		t.Fatal("expected subtype event to be acked")
	}
	if got := len(mc.outputs()); got != 0 {
		t.Fatalf("outputs = %d, want 0 for ignored subtype", got)
	}
}

func TestBot_AllowsFileShareSubtype(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("file ok", "sess-file")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-file",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "see attached",
		Timestamp: "1711111111.000360",
		SubType:   "file_share",
	})

	waitForSlackOutput(t, mc, "file ok")
}

func TestBot_RootMessageDoesNotThreadWhenReplyInThreadDisabled(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("root ok", "sess-root")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    false,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-root",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello root",
		Timestamp: "1711111111.000370",
	})

	waitForSlackOutput(t, mc, "root ok")
	if mc.lastThreadTS() != "" {
		t.Fatalf("thread_ts = %q, want empty for root reply", mc.lastThreadTS())
	}
}

func TestBot_ThreadedInboundStillRepliesInThreadWhenReplyInThreadDisabled(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("thread keep", "sess-keep")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    false,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-thread-keep",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello thread keep",
		Timestamp: "1711111111.000380",
		ThreadTS:  "1711111111.000381",
	})

	waitForSlackOutput(t, mc, "thread keep")
	if mc.lastThreadTS() != "1711111111.000381" {
		t.Fatalf("thread_ts = %q, want 1711111111.000381", mc.lastThreadTS())
	}
}

func TestBot_DoesNotHandleEventWhenAckFails(t *testing.T) {
	mc := newMockClient()
	mc.AckErr = errors.New("ack failed")
	k := newIdleSlackKernel()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-ack-fail",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/start",
		Timestamp: "1711111111.000390",
	})

	time.Sleep(100 * time.Millisecond)
	if got := len(mc.outputs()); got != 0 {
		t.Fatalf("outputs = %d, want 0 when ack fails", got)
	}
	if got := mc.ackAttempts("req-ack-fail"); got != 1 {
		t.Fatalf("ackAttempts = %d, want 1", got)
	}
}

func TestBot_NewCommandRejectsReservedTurnWithoutKernelReset(t *testing.T) {
	mc := newMockClient()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
	}, mc, newIdleSlackKernel(), nil)

	if ticket := b.reserveTurn("C123", "1711111111.000392"); ticket == 0 {
		t.Fatal("reserveTurn returned 0, want non-zero ticket")
	}

	b.handleEvent(context.Background(), Event{
		RequestID: "req-new-reserved",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/new",
		Timestamp: "1711111111.000392",
		ThreadTS:  "1711111111.000392",
	})

	if got := mc.lastOutputText(); !strings.Contains(got, "Cannot reset during active turn") {
		t.Fatalf("last output = %q, want deterministic busy reset reply", got)
	}
	if got := mc.lastOutputText(); strings.Contains(got, "ack timeout") {
		t.Fatalf("last output = %q, want no queued reset ack-timeout path", got)
	}
}

func TestBot_NewCommandAllowsResetAfterTerminalBindingRelease(t *testing.T) {
	mc := newMockClient()
	k := newIdleSlackKernel()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)

	b.current = &turnBinding{
		channelID: "C123",
		threadTS:  "1711111111.000393",
		placeholderTS: "1711111111.999997",
	}
	released := b.releaseCurrentBinding()
	if released.channelID != "C123" {
		t.Fatalf("released.channelID = %q, want C123", released.channelID)
	}
	if b.hasTurnInFlight() {
		t.Fatal("hasTurnInFlight = true after terminal binding release, want false")
	}

	b.handleEvent(ctx, Event{
		RequestID: "req-new-current",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/new",
		Timestamp: "1711111111.000393",
		ThreadTS:  "1711111111.000393",
	})

	if got := mc.lastOutputText(); !strings.Contains(got, "Session reset. Next message starts fresh.") {
		t.Fatalf("last output = %q, want successful reset after terminal binding release", got)
	}
	if got := mc.lastOutputText(); strings.Contains(got, "ack timeout") {
		t.Fatalf("last output = %q, want no queued reset ack-timeout path", got)
	}
}

func TestBot_NewCommandReportsSessionCleanupFailure(t *testing.T) {
	mc := newMockClient()
	k := newIdleSlackKernel()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		SessionMap:       failingSessionMap{putErr: errors.New("persist down")},
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)

	b.handleEvent(ctx, Event{
		RequestID: "req-new-persist-fail",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/new",
		Timestamp: "1711111111.000394",
		ThreadTS:  "1711111111.000394",
	})

	got := mc.lastOutputText()
	if !strings.Contains(got, "Session reset completed, but failed to clear persisted session") {
		t.Fatalf("last output = %q, want honest cleanup failure reply", got)
	}
	if strings.Contains(got, "Next message starts fresh.") {
		t.Fatalf("last output = %q, want no clean-reset success claim", got)
	}
}

func TestBot_RunReturnsPromptlyWhenClientRunFails(t *testing.T) {
	mc := newMockClient()
	runErr := errors.New("socket mode failed")
	mc.RunErr = runErr
	k := newIdleSlackKernel()
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)

	done := make(chan error, 1)
	go func() {
		done <- b.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, runErr) {
			t.Fatalf("Run err = %v, want %v", err, runErr)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Bot.Run hung after client.Run failure")
	}
}

func TestBot_FirstInboundMessage_NotLostWhenStartupFrameIsUndrained(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("first ok", "sess-first")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       50,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-first-undrained",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello first",
		Timestamp: "1711111111.000395",
		ThreadTS:  "1711111111.000395",
	})

	waitForSlackOutput(t, mc, "first ok")
	if strings.Contains(mc.lastOutputText(), "(empty reply)") {
		t.Fatalf("last output = %q, want real first reply not empty final", mc.lastOutputText())
	}
}

func TestBot_RunOutbound_EmitsPendingStreamAndFinal(t *testing.T) {
	mc := newMockClient()
	k := newSlackKernel("roger", "sess-slack")
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       1,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-stream",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello stream",
		Timestamp: "1711111111.000400",
		ThreadTS:  "1711111111.000400",
	})

	waitForSlackOutput(t, mc, "roger")
	outputs := mc.outputs()
	if len(outputs) == 0 {
		t.Fatal("outputs = 0, want at least one delivered reply")
	}
	if mc.lastThreadTS() != "1711111111.000400" {
		t.Fatalf("thread_ts = %q, want 1711111111.000400", mc.lastThreadTS())
	}

	// The kernel render mailbox is capacity-1 with replace-latest semantics,
	// so a fast turn may expose only the latest observable frame. The adapter
	// must still deliver the final reply correctly; intermediate placeholder
	// and stream updates are opportunistic rather than guaranteed.
	foundFinal := false
	foundPending := false
	foundStreamingLike := false
	for _, out := range outputs {
		if out.text == "⏳" && !out.updated {
			foundPending = true
		}
		if out.updated && strings.Contains(out.text, "rog") {
			foundStreamingLike = true
		}
		if strings.Contains(out.text, "roger") {
			foundFinal = true
		}
	}
	if !foundFinal {
		t.Fatalf("outputs = %+v, want final reply containing roger", outputs)
	}
	if foundStreamingLike && !foundPending {
		t.Fatalf("outputs = %+v, want placeholder before any streamed update", outputs)
	}
}

func TestBot_RunOutbound_EmitsErrorReply(t *testing.T) {
	mc := newMockClient()
	k := newFailingSlackKernel(errors.New("store broke"))
	b := New(Config{
		AllowedChannelID: "C123",
		ReplyInThread:    true,
		CoalesceMs:       1,
	}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	go func() { _ = b.Run(ctx) }()

	mc.pushEvent(Event{
		RequestID: "req-fail",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "hello fail",
		Timestamp: "1711111111.000500",
		ThreadTS:  "1711111111.000500",
	})

	waitForSlackOutput(t, mc, "❌")
	if !strings.Contains(mc.lastOutputText(), "store ack timeout") {
		t.Fatalf("last output = %q, want store ack timeout error", mc.lastOutputText())
	}
}

func TestBot_DeliverCurrent_FallsBackToPostWhenFinalEditFails(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ReplyInThread: true}, mc, newIdleSlackKernel(), nil)
	b.current = &turnBinding{
		channelID:     "C123",
		threadTS:      "1711111111.000600",
		placeholderTS: "1711111111.999999",
	}
	mc.rememberThread("1711111111.999999", "1711111111.000600")
	mc.UpdateErr = errUpdateFailed()

	if err := b.deliverCurrent(context.Background(), "final fallback"); err != nil {
		t.Fatalf("deliverCurrent returned err = %v, want nil via post fallback", err)
	}
	outputs := mc.outputs()
	if len(outputs) != 1 {
		t.Fatalf("outputs = %d, want exactly one fallback post", len(outputs))
	}
	if outputs[0].updated {
		t.Fatalf("output = %+v, want fallback post not edit", outputs[0])
	}
	if outputs[0].threadTS != "1711111111.000600" {
		t.Fatalf("thread_ts = %q, want 1711111111.000600", outputs[0].threadTS)
	}
}

func TestBot_DeliverCurrent_FallsBackToPostWhenErrorEditFails(t *testing.T) {
	mc := newMockClient()
	b := New(Config{ReplyInThread: true}, mc, newIdleSlackKernel(), nil)
	b.current = &turnBinding{
		channelID:     "C123",
		threadTS:      "1711111111.000700",
		placeholderTS: "1711111111.999998",
	}
	mc.rememberThread("1711111111.999998", "1711111111.000700")
	mc.UpdateErr = errUpdateFailed()

	if err := b.deliverCurrent(context.Background(), formatError(kernel.RenderFrame{LastError: "boom"})); err != nil {
		t.Fatalf("deliverCurrent returned err = %v, want nil via post fallback", err)
	}
	outputs := mc.outputs()
	if len(outputs) != 1 {
		t.Fatalf("outputs = %d, want exactly one fallback post", len(outputs))
	}
	if outputs[0].updated {
		t.Fatalf("output = %+v, want fallback post not edit", outputs[0])
	}
	if !strings.Contains(outputs[0].text, "boom") {
		t.Fatalf("output text = %q, want boom", outputs[0].text)
	}
}

func TestBot_BindTurnForFrame_ClaimsReservedThreadBeforeFastTurnFrames(t *testing.T) {
	b := New(Config{}, newMockClient(), newIdleSlackKernel(), nil)

	ticket := b.reserveTurn("C123", "1711111111.000800")
	if ticket == 0 {
		t.Fatal("reserveTurn returned 0, want non-zero ticket")
	}

	binding, ok := b.bindTurnForFrame()
	if !ok {
		t.Fatal("bindTurnForFrame returned ok=false, want true")
	}
	if binding.channelID != "C123" {
		t.Fatalf("binding.channelID = %q, want C123", binding.channelID)
	}
	if binding.threadTS != "1711111111.000800" {
		t.Fatalf("binding.threadTS = %q, want 1711111111.000800", binding.threadTS)
	}
}

func TestBot_PersistIfChanged_RebindsSameChannelAcrossThreads(t *testing.T) {
	smap := session.NewMemMap()
	b := New(Config{SessionMap: smap}, newMockClient(), newIdleSlackKernel(), nil)

	if ticket := b.reserveTurn("C123", "thread-1"); ticket == 0 {
		t.Fatal("first reserveTurn returned 0")
	}
	binding, ok := b.bindTurnForFrame()
	if !ok {
		t.Fatal("first bindTurnForFrame returned ok=false")
	}
	if binding.threadTS != "thread-1" {
		t.Fatalf("first binding.threadTS = %q, want thread-1", binding.threadTS)
	}
	b.persistIfChanged(context.Background(), "C123", kernel.RenderFrame{SessionID: "sess-1"})

	b.finishTurn()

	if ticket := b.reserveTurn("C123", "thread-2"); ticket == 0 {
		t.Fatal("second reserveTurn returned 0")
	}
	binding, ok = b.bindTurnForFrame()
	if !ok {
		t.Fatal("second bindTurnForFrame returned ok=false")
	}
	if binding.threadTS != "thread-2" {
		t.Fatalf("second binding.threadTS = %q, want thread-2", binding.threadTS)
	}
	if binding.lastSID != "" || binding.placeholderTS != "" {
		t.Fatalf("second binding = %+v, want fresh per-turn state", binding)
	}
	b.persistIfChanged(context.Background(), "C123", kernel.RenderFrame{SessionID: "sess-2"})

	gotSID, err := smap.Get(context.Background(), SessionKey("C123"))
	if err != nil {
		t.Fatalf("Get persisted session: %v", err)
	}
	if gotSID != "sess-2" {
		t.Fatalf("persisted sid = %q, want sess-2", gotSID)
	}
}

func TestRealClient_HandleEventsAPI_PreservesSubtypeMetadata(t *testing.T) {
	rc := &realClient{pending: make(map[string]socketmode.Request)}
	var got Event

	rc.handleEventsAPI(socketmode.Event{
		Type: socketmode.EventTypeEventsAPI,
		Request: &socketmode.Request{
			EnvelopeID: "env-1",
		},
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Data: &slackevents.MessageEvent{
					Channel:         "C123",
					User:            "U1",
					Text:            "hello",
					TimeStamp:       "1711111111.001000",
					ThreadTimeStamp: "1711111111.001000",
					SubType:         "thread_broadcast",
				},
			},
		},
	}, func(e Event) {
		got = e
	})

	if got.SubType != "thread_broadcast" {
		t.Fatalf("SubType = %q, want thread_broadcast", got.SubType)
	}
	if _, ok := rc.pending["env-1"]; !ok {
		t.Fatal("pending request env-1 not recorded")
	}
}

func TestRealClient_AckFailureRetainsPendingAndSuccessClearsIt(t *testing.T) {
	ackCount := 0
	ackErr := errors.New("ack failed")
	rc := &realClient{
		pending: map[string]socketmode.Request{
			"env-1": {EnvelopeID: "env-1"},
		},
		ackFn: func(req socketmode.Request) error {
			ackCount++
			if req.EnvelopeID != "env-1" {
				t.Fatalf("EnvelopeID = %q, want env-1", req.EnvelopeID)
			}
			if ackCount == 1 {
				return ackErr
			}
			return nil
		},
	}

	if err := rc.Ack("env-1"); !errors.Is(err, ackErr) {
		t.Fatalf("Ack err = %v, want %v", err, ackErr)
	}
	if got := len(rc.pending); got != 1 {
		t.Fatalf("pending len after failed ack = %d, want 1", got)
	}

	if err := rc.Ack("env-1"); err != nil {
		t.Fatalf("Ack retry err = %v, want nil", err)
	}
	if got := len(rc.pending); got != 0 {
		t.Fatalf("pending len after successful ack = %d, want 0", got)
	}
	if ackCount != 2 {
		t.Fatalf("ackCount = %d, want 2", ackCount)
	}

	if err := rc.Ack("env-1"); err != nil {
		t.Fatalf("Ack on cleared request err = %v, want nil", err)
	}
	if ackCount != 2 {
		t.Fatalf("ackCount after idempotent ack = %d, want 2", ackCount)
	}
}

func TestRealClient_HandleSocketEvent_AutoAckUsesInjectedSeam(t *testing.T) {
	var acked []string
	rc := &realClient{
		pending: make(map[string]socketmode.Request),
		ackFn: func(req socketmode.Request) error {
			acked = append(acked, req.EnvelopeID)
			return nil
		},
	}

	rc.handleSocketEvent(socketmode.Event{
		Type: socketmode.EventTypeInteractive,
		Request: &socketmode.Request{
			EnvelopeID: "interactive-1",
		},
	}, nil)
	rc.handleSocketEvent(socketmode.Event{
		Type: socketmode.EventTypeSlashCommand,
		Request: &socketmode.Request{
			EnvelopeID: "slash-1",
		},
	}, nil)
	rc.handleEventsAPI(socketmode.Event{
		Type: socketmode.EventTypeEventsAPI,
		Request: &socketmode.Request{
			EnvelopeID: "not-message-1",
		},
		Data: slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Data: struct{}{},
			},
		},
	}, func(Event) {
		t.Fatal("unexpected message callback")
	})

	if len(acked) != 3 {
		t.Fatalf("acked = %v, want 3 acked envelopes", acked)
	}
	if acked[0] != "interactive-1" || acked[1] != "slash-1" || acked[2] != "not-message-1" {
		t.Fatalf("acked = %v, want injected ack order for interactive/slash/non-message events", acked)
	}
}
