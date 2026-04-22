package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBot_Name(t *testing.T) {
	b := New(Config{AllowedChatID: 42}, newMockClient(), nil)
	if got := b.Name(); got != "telegram" {
		t.Errorf("Name() = %q, want telegram", got)
	}
}

func TestBot_ToInboundEvent_Submit(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedChatID: 42}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.pushTextUpdate(42, "hello there")

	select {
	case ev := <-inbox:
		if ev.Kind != gateway.EventSubmit || ev.Text != "hello there" {
			t.Errorf("got %+v", ev)
		}
		if ev.Platform != "telegram" || ev.ChatID != "42" {
			t.Errorf("got %+v", ev)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no inbound event")
	}
}

func TestBot_ToInboundEvent_Commands(t *testing.T) {
	cases := []struct {
		text string
		want gateway.EventKind
	}{
		{"/start", gateway.EventStart},
		{"/stop", gateway.EventCancel},
		{"/new", gateway.EventReset},
		{"/gibberish", gateway.EventUnknown},
		{"plain text", gateway.EventSubmit},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			mc := newMockClient()
			b := New(Config{AllowedChatID: 42}, mc, nil)
			inbox := make(chan gateway.InboundEvent, 1)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = b.Run(ctx, inbox) }()

			mc.pushTextUpdate(42, c.text)

			select {
			case ev := <-inbox:
				if ev.Kind != c.want {
					t.Errorf("text=%q got Kind=%v want=%v", c.text, ev.Kind, c.want)
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatal("no inbound event")
			}
		})
	}
}

func TestBot_Send(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedChatID: 42}, mc, nil)

	id, err := b.Send(context.Background(), "42", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatalf("empty msg ID")
	}

	sent := mc.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("sent count = %d", len(sent))
	}
	if _, ok := sent[0].(tgbotapi.MessageConfig); !ok {
		t.Errorf("sent type = %T", sent[0])
	}
	if mc.lastSentText() != "hello" {
		t.Errorf("lastSentText = %q", mc.lastSentText())
	}
}

func TestBot_SendPlaceholder(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedChatID: 42}, mc, nil)

	id, err := b.SendPlaceholder(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatalf("placeholder id empty")
	}
	if !strings.Contains(mc.lastSentText(), "⏳") {
		t.Errorf("placeholder text = %q", mc.lastSentText())
	}
}

func TestBot_EditMessage(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedChatID: 42}, mc, nil)

	if err := b.EditMessage(context.Background(), "42", "1234", "updated"); err != nil {
		t.Fatal(err)
	}

	sent := mc.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("sent count = %d", len(sent))
	}
	if _, ok := sent[0].(tgbotapi.EditMessageTextConfig); !ok {
		t.Errorf("sent type = %T want EditMessageTextConfig", sent[0])
	}
	if mc.lastSentText() != "updated" {
		t.Errorf("edit text = %q", mc.lastSentText())
	}
}

func TestBot_EditMessage_BadMsgID(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedChatID: 42}, mc, nil)
	if err := b.EditMessage(context.Background(), "42", "nope", "x"); err == nil {
		t.Fatal("expected error for non-numeric msgID")
	}
}

func TestBot_NilMessage_Skipped(t *testing.T) {
	mc := newMockClient()
	b := New(Config{AllowedChatID: 42}, mc, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	mc.updatesCh <- tgbotapi.Update{UpdateID: 7}

	select {
	case ev := <-inbox:
		t.Fatalf("expected no inbound, got %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}
