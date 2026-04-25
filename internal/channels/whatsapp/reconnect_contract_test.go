package whatsapp

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

func TestBotSend_RetriesWithBoundedBackoffAndPreservesOriginalTarget(t *testing.T) {
	client := newReconnectContractClient()
	client.sendErrs = []error{
		errors.New("connectionreset: bridge socket closed"),
		errors.New("connectionrefused: bridge restarting"),
		nil,
	}
	bot := New(client, IdentityContext{Runtime: RuntimeKindBridge}, nil)
	clock := &fakeBackoffClock{}
	bot.SetSendRetryPolicy(SendRetryPolicy{
		MaxRetries: 2,
		Backoff:    []time.Duration{10 * time.Millisecond, 20 * time.Millisecond},
		Sleep:      clock.Sleep,
	})
	pairReconnectTarget(bot)

	clock.onSleep = func(time.Duration) {
		status := bot.OutboundStatus()
		if status.State != OutboundStateReconnectPending {
			t.Fatalf("pending status = %+v, want reconnect pending", status)
		}
		if status.ChatID != "15551234567" || status.RawChatID != "999999999999999@lid" {
			t.Fatalf("pending target = chat %q raw %q, want original normalized/raw target", status.ChatID, status.RawChatID)
		}
		if status.Attempts < 1 || status.ErrorMessage == "" {
			t.Fatalf("pending status attempts/error = %d/%q, want visible retry state", status.Attempts, status.ErrorMessage)
		}
	}

	msgID, err := bot.Send(context.Background(), "15551234567", "terminal reply")
	if err != nil {
		t.Fatalf("Send() error = %v, want nil after reconnect retry", err)
	}
	if msgID != "wam-send-3" {
		t.Fatalf("Send() msgID = %q, want wam-send-3", msgID)
	}
	if client.reconnectCount() != 2 {
		t.Fatalf("reconnect count = %d, want 2", client.reconnectCount())
	}
	if !reflect.DeepEqual(clock.delays, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}) {
		t.Fatalf("backoff delays = %v, want deterministic bounded delays", clock.delays)
	}

	sends := client.sendsSnapshot()
	if len(sends) != 3 {
		t.Fatalf("transport send attempts = %d, want initial plus 2 retries", len(sends))
	}
	for i, got := range sends {
		assertOriginalReconnectRequest(t, i, got)
	}

	status := bot.OutboundStatus()
	if status.State != OutboundStatePaired {
		t.Fatalf("final status = %+v, want paired after successful retry", status)
	}
	if status.ChatID != "15551234567" || status.RawChatID != "999999999999999@lid" {
		t.Fatalf("final target = chat %q raw %q, want original normalized/raw target", status.ChatID, status.RawChatID)
	}
}

func TestBotSend_ReconnectExhaustionReturnsSingleDegradedTerminalResult(t *testing.T) {
	client := newReconnectContractClient()
	client.sendErrs = []error{
		errors.New("connectionreset: bridge socket closed"),
		errors.New("connectionreset: bridge socket closed"),
		errors.New("connectionreset: bridge socket closed"),
	}
	bot := New(client, IdentityContext{Runtime: RuntimeKindBridge}, nil)
	clock := &fakeBackoffClock{}
	bot.SetSendRetryPolicy(SendRetryPolicy{
		MaxRetries: 2,
		Backoff:    []time.Duration{5 * time.Millisecond, 10 * time.Millisecond},
		Sleep:      clock.Sleep,
	})
	pairReconnectTarget(bot)

	_, err := bot.Send(context.Background(), "15551234567", "terminal reply")
	if err == nil {
		t.Fatal("Send() error = nil, want reconnect exhaustion degraded error")
	}
	var degraded DegradedSendError
	if !errors.As(err, &degraded) {
		t.Fatalf("Send() error = %T, want DegradedSendError", err)
	}
	if degraded.Reason != SendDegradedReconnectExhausted {
		t.Fatalf("degraded reason = %q, want %q", degraded.Reason, SendDegradedReconnectExhausted)
	}
	if degraded.ChatID != "15551234567" || degraded.RawChatID != "999999999999999@lid" {
		t.Fatalf("degraded target = chat %q raw %q, want original normalized/raw target", degraded.ChatID, degraded.RawChatID)
	}
	if !strings.Contains(degraded.Error(), "connectionreset") {
		t.Fatalf("degraded error = %q, want original send failure", degraded.Error())
	}

	status := bot.OutboundStatus()
	if status.State != OutboundStateReconnectExhausted {
		t.Fatalf("OutboundStatus() = %+v, want reconnect exhausted", status)
	}
	if status.Reason != SendDegradedReconnectExhausted {
		t.Fatalf("status reason = %q, want %q", status.Reason, SendDegradedReconnectExhausted)
	}
	if status.ChatID != "15551234567" || status.RawChatID != "999999999999999@lid" {
		t.Fatalf("status target = chat %q raw %q, want original normalized/raw target", status.ChatID, status.RawChatID)
	}
	if status.Attempts != 3 || !strings.Contains(status.ErrorMessage, "connectionreset") {
		t.Fatalf("status attempts/error = %d/%q, want exhausted send state", status.Attempts, status.ErrorMessage)
	}
	if client.reconnectCount() != 2 {
		t.Fatalf("reconnect count = %d, want 2", client.reconnectCount())
	}
	if !reflect.DeepEqual(clock.delays, []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}) {
		t.Fatalf("backoff delays = %v, want deterministic bounded delays", clock.delays)
	}

	sends := client.sendsSnapshot()
	if len(sends) != 3 {
		t.Fatalf("transport send attempts = %d, want initial plus 2 retries", len(sends))
	}
	for i, got := range sends {
		assertOriginalReconnectRequest(t, i, got)
		if got.Text != "terminal reply" {
			t.Fatalf("send attempt %d text = %q, want only the original terminal reply", i, got.Text)
		}
	}
}

func pairReconnectTarget(bot *Bot) {
	bot.PairOutboundTarget(InboundResult{
		Decision: InboundDecisionRoute,
		Event: gateway.InboundEvent{
			Platform: platformName,
			ChatID:   "15551234567",
			MsgID:    "wamid.original",
		},
		Reply: ReplyTarget{
			ChatID:   "999999999999999@lid",
			ChatKind: ChatKindDirect,
		},
	})
}

func assertOriginalReconnectRequest(t *testing.T, attempt int, got SendRequest) {
	t.Helper()

	if got.Runtime != RuntimeKindBridge {
		t.Fatalf("send attempt %d runtime = %q, want %q", attempt, got.Runtime, RuntimeKindBridge)
	}
	if got.ChatID != "999999999999999@lid" {
		t.Fatalf("send attempt %d raw ChatID = %q, want original raw peer", attempt, got.ChatID)
	}
	if got.ChatKind != ChatKindDirect {
		t.Fatalf("send attempt %d ChatKind = %q, want direct", attempt, got.ChatKind)
	}
	if got.Options.ReplyToMessageID != "wamid.original" {
		t.Fatalf("send attempt %d reply metadata = %q, want wamid.original", attempt, got.Options.ReplyToMessageID)
	}
	if got.Text != "terminal reply" {
		t.Fatalf("send attempt %d text = %q, want terminal reply", attempt, got.Text)
	}
}

type fakeBackoffClock struct {
	delays  []time.Duration
	onSleep func(time.Duration)
}

func (c *fakeBackoffClock) Sleep(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.delays = append(c.delays, delay)
	if c.onSleep != nil {
		c.onSleep(delay)
	}
	return nil
}

type reconnectContractClient struct {
	events     chan InboundMessage
	sendErrs   []error
	sends      []SendRequest
	reconnects int
}

func newReconnectContractClient() *reconnectContractClient {
	return &reconnectContractClient{events: make(chan InboundMessage, 1)}
}

func (c *reconnectContractClient) Events() <-chan InboundMessage { return c.events }

func (c *reconnectContractClient) SendWhatsApp(_ context.Context, req SendRequest) (string, error) {
	c.sends = append(c.sends, req)
	attempt := len(c.sends)
	if len(c.sendErrs) > 0 {
		err := c.sendErrs[0]
		c.sendErrs = c.sendErrs[1:]
		if err != nil {
			return "", err
		}
	}
	return "wam-send-" + strconv.Itoa(attempt), nil
}

func (c *reconnectContractClient) ReconnectWhatsApp(context.Context) error {
	c.reconnects++
	return nil
}

func (c *reconnectContractClient) Close() error { return nil }

func (c *reconnectContractClient) reconnectCount() int {
	return c.reconnects
}

func (c *reconnectContractClient) sendsSnapshot() []SendRequest {
	out := make([]SendRequest, len(c.sends))
	copy(out, c.sends)
	return out
}
