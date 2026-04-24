package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestAICardBot_StreamingUpdateContract(t *testing.T) {
	replyClient := newMockClient()
	base := New(Config{}, replyClient, nil)
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
	if msgID != "hermes-test" {
		t.Fatalf("SendPlaceholder() msgID = %q, want hermes-test", msgID)
	}
	if err := bot.EditMessage(context.Background(), ev.ChatID, msgID, "partial answer"); err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if err := bot.EditMessageFinal(context.Background(), ev.ChatID, msgID, "final answer", true); err != nil {
		t.Fatalf("EditMessageFinal() error = %v", err)
	}

	assertJSONFixture(t, "dmCreate", cardClient.creates[0])
	assertJSONFixture(t, "dmDeliver", cardClient.delivers[0])
	assertJSONFixture(t, "streamOpen", cardClient.updates[0])
	assertJSONFixture(t, "streamFinal", cardClient.updates[1])
}

func TestAICardBot_FallsBackToSessionWebhookWhenStreamingUpdateFails(t *testing.T) {
	replyClient := newMockClient()
	base := New(Config{}, replyClient, nil)
	cardClient := &recordingAICardClient{
		updateErrs: []error{errors.New("card is not exist")},
	}
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
	if err := bot.EditMessage(context.Background(), ev.ChatID, msgID, "partial answer"); err == nil {
		t.Fatal("EditMessage() error = nil, want card update failure")
	}
	if err := bot.EditMessageFinal(context.Background(), ev.ChatID, msgID, "final answer", true); err != nil {
		t.Fatalf("EditMessageFinal() fallback error = %v", err)
	}

	sent := replyClient.sentSnapshot()
	if len(sent) != 1 {
		t.Fatalf("fallback sends = %d, want 1", len(sent))
	}
	if sent[0].Webhook != "https://api.dingtalk.com/robot/send?access_token=reply" {
		t.Fatalf("fallback webhook = %q, want stored session webhook", sent[0].Webhook)
	}
	if sent[0].Text != "final answer" {
		t.Fatalf("fallback text = %q, want final answer", sent[0].Text)
	}
}

type recordingAICardClient struct {
	creates    []AICardCreateRequest
	delivers   []AICardDeliverRequest
	updates    []AICardStreamingUpdateRequest
	createErr  error
	deliverErr error
	updateErrs []error
}

func (r *recordingAICardClient) CreateCard(_ context.Context, req AICardCreateRequest) error {
	r.creates = append(r.creates, req)
	return r.createErr
}

func (r *recordingAICardClient) DeliverCard(_ context.Context, req AICardDeliverRequest) error {
	r.delivers = append(r.delivers, req)
	return r.deliverErr
}

func (r *recordingAICardClient) StreamingUpdate(_ context.Context, req AICardStreamingUpdateRequest) error {
	r.updates = append(r.updates, req)
	if len(r.updateErrs) == 0 {
		return nil
	}
	err := r.updateErrs[0]
	r.updateErrs = r.updateErrs[1:]
	return err
}

func sequence(values ...string) func() string {
	i := 0
	return func() string {
		if i >= len(values) {
			return values[len(values)-1]
		}
		value := values[i]
		i++
		return value
	}
}

func assertJSONFixture(t *testing.T, key string, got any) {
	t.Helper()

	fixtures := loadAICardFixtures(t)
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

func loadAICardFixtures(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	raw, err := os.ReadFile("testdata/ai_card_streaming_contract.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("unmarshal fixture file: %v", err)
	}
	return fixtures
}
