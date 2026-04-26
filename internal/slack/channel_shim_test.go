package slack

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

type slackGatewayKernel struct {
	submits chan kernel.PlatformEvent
	renders chan kernel.RenderFrame

	mu     sync.Mutex
	resets int
}

func newSlackGatewayKernel() *slackGatewayKernel {
	return &slackGatewayKernel{
		submits: make(chan kernel.PlatformEvent, 4),
		renders: make(chan kernel.RenderFrame),
	}
}

func (k *slackGatewayKernel) Submit(ev kernel.PlatformEvent) error {
	k.submits <- ev
	return nil
}

func (k *slackGatewayKernel) ResetSession() error {
	k.mu.Lock()
	k.resets++
	k.mu.Unlock()
	return nil
}

func (k *slackGatewayKernel) Render() <-chan kernel.RenderFrame {
	return k.renders
}

func TestSlackChannel_RunTranslatesSlackEvents(t *testing.T) {
	mc := newMockClient()
	ch := NewChannel(mc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbox := make(chan gateway.InboundEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- ch.Run(ctx, inbox)
	}()

	mc.pushEvent(Event{
		RequestID: "req-channel-1",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "  hello gateway  ",
		Timestamp: "1711111111.000200",
		ThreadTS:  "1711111111.000100",
	})

	select {
	case ev := <-inbox:
		if ev.Platform != "slack" {
			t.Fatalf("Platform = %q, want slack", ev.Platform)
		}
		if ev.ChatID != "C123" {
			t.Fatalf("ChatID = %q, want C123", ev.ChatID)
		}
		if ev.ThreadID != "1711111111.000100" {
			t.Fatalf("ThreadID = %q, want Slack thread timestamp", ev.ThreadID)
		}
		if ev.UserID != "U1" {
			t.Fatalf("UserID = %q, want U1", ev.UserID)
		}
		if ev.MsgID != "1711111111.000200" {
			t.Fatalf("MsgID = %q, want Slack timestamp", ev.MsgID)
		}
		if ev.MessageID != "1711111111.000200" {
			t.Fatalf("MessageID = %q, want Slack timestamp", ev.MessageID)
		}
		if ev.Kind != gateway.EventSubmit || ev.Text != "hello gateway" {
			t.Fatalf("kind/text = %s/%q, want submit/hello gateway", ev.Kind, ev.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for translated Slack event")
	}

	if !mc.wasAcked("req-channel-1") {
		t.Fatal("expected Slack request to be acked by channel shim")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestSlackChannel_SendPostsToRememberedThread(t *testing.T) {
	mc := newMockClient()
	ch := NewChannel(mc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbox := make(chan gateway.InboundEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- ch.Run(ctx, inbox)
	}()

	mc.pushEvent(Event{
		RequestID: "req-thread-route",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/help",
		Timestamp: "1711111111.000300",
		ThreadTS:  "1711111111.000301",
	})
	select {
	case <-inbox:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound event")
	}

	msgID, err := ch.Send(context.Background(), "C123", "thread reply")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	outputs := mc.outputs()
	if len(outputs) != 1 {
		t.Fatalf("outputs = %d, want 1", len(outputs))
	}
	if msgID != outputs[0].ts {
		t.Fatalf("msgID = %q, want Slack ts %q", msgID, outputs[0].ts)
	}
	if outputs[0].channelID != "C123" || outputs[0].threadTS != "1711111111.000301" || outputs[0].text != "thread reply" {
		t.Fatalf("posted output = %+v, want channel C123 thread 1711111111.000301 text thread reply", outputs[0])
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestSlackChannel_ManagerConsumesIngressAndSends(t *testing.T) {
	mc := newMockClient()
	ch := NewChannel(mc, nil)
	fk := newSlackGatewayKernel()
	m := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats: map[string]string{"slack": "C123"},
	}, fk, slog.Default())
	if err := m.Register(ch); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	mc.pushEvent(Event{
		RequestID: "req-manager-help",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/help",
		Timestamp: "1711111111.000400",
		ThreadTS:  "1711111111.000401",
	})

	waitForSlackCondition(t, time.Second, func() bool {
		return strings.Contains(mc.lastOutputText(), "Gormes is online")
	})
	outputs := mc.outputs()
	if len(outputs) != 1 {
		t.Fatalf("outputs = %d, want 1", len(outputs))
	}
	if outputs[0].channelID != "C123" || outputs[0].threadTS != "1711111111.000401" {
		t.Fatalf("manager send output = %+v, want Slack channel/thread", outputs[0])
	}

	mc.pushEvent(Event{
		RequestID: "req-manager-submit",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "manager submit",
		Timestamp: "1711111111.000500",
	})
	select {
	case got := <-fk.submits:
		if got.Kind != kernel.PlatformEventSubmit || got.Text != "manager submit" {
			t.Fatalf("kernel submit = %+v, want submit manager submit", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for manager kernel submit")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Manager Run after cancel = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Manager Run did not return after cancellation")
	}
}

func TestSlackChannel_ManagerReportsSendDegradation(t *testing.T) {
	mc := newMockClient()
	mc.PostErr = errors.New("slack post failed")
	ch := NewChannel(mc, nil)
	store := gateway.NewRuntimeStatusStore(filepath.Join(t.TempDir(), "gateway_state.json"))
	m := gateway.NewManagerWithSubmitter(gateway.ManagerConfig{
		AllowedChats:   map[string]string{"slack": "C123"},
		RuntimeStatus:  store,
		AllowDiscovery: map[string]bool{},
	}, newSlackGatewayKernel(), slog.Default())
	if err := m.Register(ch); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	mc.pushEvent(Event{
		RequestID: "req-send-fail",
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "/help",
		Timestamp: "1711111111.000600",
	})

	waitForSlackCondition(t, time.Second, func() bool {
		status, err := store.ReadRuntimeStatus(context.Background())
		if err != nil {
			return false
		}
		platform := status.Platforms["slack"]
		return platform.State == gateway.PlatformStateFailed &&
			strings.Contains(platform.ErrorMessage, "slack post failed")
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Manager Run after cancel = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Manager Run did not return after cancellation")
	}
}

func TestSlackChannel_RichTextIngress(t *testing.T) {
	mc := newMockClient()
	ch := NewChannel(mc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inbox := make(chan gateway.InboundEvent, 1)
	done := make(chan error, 1)
	go func() {
		done <- ch.Run(ctx, inbox)
	}()

	mc.pushEvent(Event{
		RequestID:   "req-rich-text",
		ChannelID:   "C123",
		UserID:      "U1",
		Text:        "Can you summarize this?",
		Timestamp:   "1711111111.000700",
		Blocks:      sampleRichTextBlocks(),
		Attachments: sampleAttachmentPreviews(),
	})

	select {
	case ev := <-inbox:
		if ev.Kind != gateway.EventSubmit {
			t.Fatalf("Kind = %s, want submit", ev.Kind)
		}
		for _, want := range []string{
			"Can you summarize this?",
			"> Quoted line",
			"- First bullet",
			"1. First ordered",
			"```go\nfmt.Println(\"hi\")\n```",
			"Link preview: Spec",
			"https://example.com/spec",
			"The latest product spec preview",
			"Notion",
		} {
			if !strings.Contains(ev.Text, want) {
				t.Fatalf("Event text missing %q:\n%s", want, ev.Text)
			}
		}
		if strings.Contains(ev.Text, "Thread copy") {
			t.Fatalf("Event text contains skipped message unfurl:\n%s", ev.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for rich-text Slack event")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestSlackChannel_QuotedSlashDoesNotRouteAsCommand(t *testing.T) {
	ch := NewChannel(newMockClient(), nil)

	ev, ok := ch.toInboundEvent(Event{
		ChannelID: "C123",
		UserID:    "U1",
		Text:      "please review",
		Timestamp: "1711111111.000800",
		Blocks: []SlackBlock{
			{
				"type": "rich_text",
				"elements": []any{
					SlackBlock{
						"type": "rich_text_quote",
						"elements": []any{
							SlackBlock{
								"type": "rich_text_section",
								"elements": []any{
									SlackBlock{"type": "text", "text": "/deploy now"},
								},
							},
						},
					},
				},
			},
		},
	})

	if !ok {
		t.Fatal("toInboundEvent dropped event, want submit")
	}
	if ev.Kind != gateway.EventSubmit {
		t.Fatalf("Kind = %s, want submit", ev.Kind)
	}
	if ev.Text != "please review\n> /deploy now" {
		t.Fatalf("Text = %q, want original text plus quoted slash as submit text", ev.Text)
	}
}

func TestSlackChannel_ReplyToText(t *testing.T) {
	ch := NewChannel(newMockClient(), nil)
	ch.selfUserID = "UBOT"

	ev, ok := ch.toInboundEvent(Event{
		ChannelID: "C123",
		TeamID:    "T_A",
		UserID:    "U1",
		Text:      "please show details",
		Timestamp: "1000.5",
		ThreadTS:  "1000.0",
		ThreadReplies: []ThreadMessage{
			{Timestamp: "1000.0", TeamID: "T_A", BotID: "B_CRON", Text: "cron summary: 3 new emails"},
			{Timestamp: "1000.1", TeamID: "T_A", UserID: "UBOT", BotID: "B_GORMES", Text: "self bot child echo"},
		},
	})
	if !ok {
		t.Fatal("toInboundEvent dropped thread reply, want submit")
	}
	if ev.ReplyToText != "cron summary: 3 new emails" {
		t.Fatalf("ReplyToText = %q, want raw parent text", ev.ReplyToText)
	}
	if ev.ThreadID != "1000.0" {
		t.Fatalf("ThreadID = %q, want thread timestamp", ev.ThreadID)
	}
	if ev.MessageID != "1000.5" {
		t.Fatalf("MessageID = %q, want triggering Slack message timestamp", ev.MessageID)
	}

	top, ok := ch.toInboundEvent(Event{
		ChannelID: "C123",
		TeamID:    "T_A",
		UserID:    "U1",
		Text:      "top-level",
		Timestamp: "2000.0",
		ThreadReplies: []ThreadMessage{
			{Timestamp: "2000.0", TeamID: "T_A", BotID: "B_CRON", Text: "top-level parent"},
		},
	})
	if !ok {
		t.Fatal("toInboundEvent dropped top-level message, want submit")
	}
	if top.ReplyToText != "" {
		t.Fatalf("top-level ReplyToText = %q, want empty", top.ReplyToText)
	}
}

func waitForSlackCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
