package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

func TestBot_Run_FiresThinkingReactionForAcceptedInboundMessage(t *testing.T) {
	client := newEmojiMockClient()
	b := New(Config{RobotCode: "robot-code"}, client, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	client.push(InboundMessage{
		MessageID:        "user-msg-1",
		ConversationID:   "dm-42",
		ConversationType: "1",
		SenderStaffID:    "staff-1",
		Text:             "hello",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=reply",
	})

	select {
	case <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event")
	}

	replies := client.replyEmotionsSnapshot()
	if len(replies) != 1 {
		t.Fatalf("reply emotion count = %d, want 1", len(replies))
	}
	assertEmojiFixture(t, "thinkingReply", replies[0])
}

func TestAICardBot_EditMessageFinalSwapsThinkingToDoneOnce(t *testing.T) {
	replyClient := newEmojiMockClient()
	base := New(Config{RobotCode: "robot-code"}, replyClient, nil)
	cardClient := &recordingAICardClient{}
	bot := NewAICardBot(base, AICardConfig{
		TemplateID: "tmpl-1",
		RobotCode:  "robot-code",
	}, cardClient,
		WithAICardTrackID(func() string { return "hermes-test" }),
		WithAICardGUID(sequence("guid-1", "guid-2")),
	)

	ev, ok := base.toInboundEvent(InboundMessage{
		MessageID:        "user-msg-1",
		ConversationID:   "dm-42",
		ConversationType: "1",
		SenderStaffID:    "staff-1",
		Text:             "hello",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=reply",
	})
	if !ok {
		t.Fatal("toInboundEvent() rejected valid direct message")
	}

	msgID, err := bot.SendPlaceholder(context.Background(), ev.ChatID)
	if err != nil {
		t.Fatalf("SendPlaceholder() error = %v", err)
	}
	if err := bot.EditMessage(context.Background(), ev.ChatID, msgID, "partial answer"); err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if err := bot.EditMessageFinal(context.Background(), ev.ChatID, msgID, "final answer", true); err != nil {
		t.Fatalf("EditMessageFinal() error = %v", err)
	}
	base.fireDoneReaction(context.Background(), ev.ChatID)

	recalls := replyClient.recallEmotionsSnapshot()
	if len(recalls) != 1 {
		t.Fatalf("recall emotion count = %d, want 1", len(recalls))
	}
	assertEmojiFixture(t, "thinkingRecall", recalls[0])

	replies := replyClient.replyEmotionsSnapshot()
	if len(replies) != 1 {
		t.Fatalf("reply emotion count = %d, want 1", len(replies))
	}
	assertEmojiFixture(t, "doneReply", replies[0])
}

func TestBot_SendAttemptsDoneReactionButKeepsSessionWebhookFallbackOnReactionFailure(t *testing.T) {
	client := newEmojiMockClient()
	client.emotionErr = errors.New("emotion unavailable")
	b := New(Config{RobotCode: "robot-code"}, client, nil)

	ev, ok := b.toInboundEvent(InboundMessage{
		MessageID:        "user-msg-1",
		ConversationID:   "dm-42",
		ConversationType: "1",
		SenderStaffID:    "staff-1",
		Text:             "hello",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=reply",
	})
	if !ok {
		t.Fatal("toInboundEvent() rejected valid direct message")
	}

	msgID, err := b.Send(context.Background(), ev.ChatID, "final answer")
	if err != nil {
		t.Fatalf("Send() error = %v, want session-webhook fallback to succeed", err)
	}
	if msgID != "send-1" {
		t.Fatalf("Send() msgID = %q, want send-1", msgID)
	}

	sent := client.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("fallback send count = %d, want 1", len(sent))
	}
	if sent[0].Text != "final answer" {
		t.Fatalf("fallback text = %q, want final answer", sent[0].Text)
	}

	if recalls := client.recallEmotionsSnapshot(); len(recalls) != 1 {
		t.Fatalf("recall emotion count = %d, want 1", len(recalls))
	}
	if replies := client.replyEmotionsSnapshot(); len(replies) != 1 {
		t.Fatalf("reply emotion count = %d, want 1", len(replies))
	}
}

func TestBot_IgnoredGroupMessageDoesNotOverwriteReactionContext(t *testing.T) {
	client := newEmojiMockClient()
	b := New(Config{RobotCode: "robot-code"}, client, nil)

	ev, ok := b.toInboundEvent(InboundMessage{
		MessageID:        "accepted-msg",
		ConversationID:   "group-1",
		ConversationType: "2",
		SenderStaffID:    "staff-1",
		Text:             "@Hermes hello",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=accepted",
		Mentioned:        true,
	})
	if !ok {
		t.Fatal("toInboundEvent() rejected accepted group message")
	}

	if _, ok := b.toInboundEvent(InboundMessage{
		MessageID:        "ignored-msg",
		ConversationID:   "group-1",
		ConversationType: "2",
		SenderStaffID:    "staff-2",
		Text:             "background chatter",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=ignored",
		Mentioned:        false,
	}); ok {
		t.Fatal("toInboundEvent() accepted unmentioned group message")
	}

	b.fireDoneReaction(context.Background(), ev.ChatID)

	recalls := client.recallEmotionsSnapshot()
	if len(recalls) != 1 {
		t.Fatalf("recall emotion count = %d, want 1", len(recalls))
	}
	if recalls[0].OpenMessageID != "accepted-msg" {
		t.Fatalf("done reaction recalled message %q, want accepted-msg", recalls[0].OpenMessageID)
	}
}

type emojiMockClient struct {
	*mockClient

	replyEmotions  []EmojiReactionRequest
	recallEmotions []EmojiReactionRequest
	emotionErr     error
}

func newEmojiMockClient() *emojiMockClient {
	return &emojiMockClient{mockClient: newMockClient()}
}

func (m *emojiMockClient) ReplyEmotion(_ context.Context, req EmojiReactionRequest) error {
	m.replyEmotions = append(m.replyEmotions, req)
	return m.emotionErr
}

func (m *emojiMockClient) RecallEmotion(_ context.Context, req EmojiReactionRequest) error {
	m.recallEmotions = append(m.recallEmotions, req)
	return m.emotionErr
}

func (m *emojiMockClient) replyEmotionsSnapshot() []EmojiReactionRequest {
	out := make([]EmojiReactionRequest, len(m.replyEmotions))
	copy(out, m.replyEmotions)
	return out
}

func (m *emojiMockClient) recallEmotionsSnapshot() []EmojiReactionRequest {
	out := make([]EmojiReactionRequest, len(m.recallEmotions))
	copy(out, m.recallEmotions)
	return out
}

func assertEmojiFixture(t *testing.T, key string, got any) {
	t.Helper()

	fixtures := loadEmojiReactionFixtures(t)
	wantRaw, ok := fixtures[key]
	if !ok {
		t.Fatalf("missing fixture key %q", key)
	}

	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}

	var wantValue any
	if err := json.Unmarshal(wantRaw, &wantValue); err != nil {
		t.Fatalf("unmarshal fixture %q: %v", key, err)
	}
	var gotValue any
	if err := json.Unmarshal(gotRaw, &gotValue); err != nil {
		t.Fatalf("unmarshal got %q: %v", key, err)
	}

	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("%s request mismatch\ngot:  %s\nwant: %s", key, gotRaw, wantRaw)
	}
}

func loadEmojiReactionFixtures(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	raw, err := os.ReadFile("testdata/emoji_reaction_contract.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("unmarshal fixture file: %v", err)
	}
	return fixtures
}
