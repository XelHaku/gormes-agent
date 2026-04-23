package dingtalk

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDecideRuntime_StreamModeDefaults(t *testing.T) {
	plan, err := DecideRuntime(RuntimeConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Ingress.Mode != IngressModeStream {
		t.Fatalf("Ingress.Mode = %q, want %q", plan.Ingress.Mode, IngressModeStream)
	}
	if !plan.Ingress.AutoReconnect {
		t.Fatal("Ingress.AutoReconnect = false, want true")
	}
	if plan.Ingress.RequiresPublicURL {
		t.Fatal("Ingress.RequiresPublicURL = true, want false")
	}
	if plan.Reply.Mode != ReplyModeSessionWebhook {
		t.Fatalf("Reply.Mode = %q, want %q", plan.Reply.Mode, ReplyModeSessionWebhook)
	}
	if plan.Reply.Retry.MaxAttempts != 3 {
		t.Fatalf("Reply.Retry.MaxAttempts = %d, want 3", plan.Reply.Retry.MaxAttempts)
	}
}

func TestDecideRuntime_RequiresCredentials(t *testing.T) {
	_, err := DecideRuntime(RuntimeConfig{ClientID: "client-id"})
	if err == nil {
		t.Fatal("DecideRuntime() error = nil, want credential validation failure")
	}
	if got := err.Error(); got != "dingtalk: client id and client secret are required for stream mode" {
		t.Fatalf("DecideRuntime() error = %q, want credential validation failure", got)
	}
}

func TestSessionWebhooks_RememberRefreshesLatestWebhook(t *testing.T) {
	store := NewSessionWebhooks()

	store.Remember("chat-1", "https://example.invalid/old")
	store.Remember("chat-1", "https://example.invalid/new")

	webhook, err := store.Lookup("chat-1")
	if err != nil {
		t.Fatalf("Lookup() error = %v, want nil", err)
	}
	if webhook != "https://example.invalid/new" {
		t.Fatalf("Lookup() = %q, want latest webhook", webhook)
	}
}

func TestReplySender_RetriesTemporaryFailuresAndUsesLatestWebhook(t *testing.T) {
	store := NewSessionWebhooks()
	store.Remember("chat-1", "https://example.invalid/old")
	store.Remember("chat-1", "https://example.invalid/new")

	client := &replyClientStub{
		results: []replyResult{
			{err: temporaryReplyError{err: errors.New("dial timeout")}},
			{err: temporaryReplyError{err: errors.New("502 bad gateway")}},
			{msgID: "send-3"},
		},
	}

	var sleeps []time.Duration
	sender := NewReplySender(client, ReplyRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2,
	}, WithReplySleep(func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}))

	msgID, err := sender.Send(context.Background(), store, "chat-1", "hello")
	if err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if msgID != "send-3" {
		t.Fatalf("Send() msgID = %q, want send-3", msgID)
	}
	if client.attempts != 3 {
		t.Fatalf("attempts = %d, want 3", client.attempts)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleep count = %d, want 2", len(sleeps))
	}
	if got := client.webhooks(); len(got) != 3 || got[0] != "https://example.invalid/new" || got[2] != "https://example.invalid/new" {
		t.Fatalf("webhooks = %v, want latest webhook on every attempt", got)
	}
}

func TestReplySender_StopsOnPermanentFailure(t *testing.T) {
	store := NewSessionWebhooks()
	store.Remember("chat-1", "https://example.invalid/live")

	client := &replyClientStub{
		results: []replyResult{{err: errors.New("403 forbidden")}},
	}

	sender := NewReplySender(client, ReplyRetryPolicy{
		MaxAttempts:  4,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
		Multiplier:   2,
	}, nil)

	_, err := sender.Send(context.Background(), store, "chat-1", "hello")
	if err == nil {
		t.Fatal("Send() error = nil, want permanent failure")
	}
	if got := err.Error(); got != "dingtalk: send reply: 403 forbidden" {
		t.Fatalf("Send() error = %q, want permanent failure wrap", got)
	}
	if client.attempts != 1 {
		t.Fatalf("attempts = %d, want 1", client.attempts)
	}
}

type replyResult struct {
	msgID string
	err   error
}

type replyCall struct {
	webhook string
	text    string
}

type replyClientStub struct {
	results  []replyResult
	attempts int
	calls    []replyCall
}

func (r *replyClientStub) SendReply(_ context.Context, webhook, text string) (string, error) {
	r.attempts++
	r.calls = append(r.calls, replyCall{webhook: webhook, text: text})
	if len(r.results) == 0 {
		return "", errors.New("no result queued")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result.msgID, result.err
}

func (r *replyClientStub) webhooks() []string {
	out := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		out = append(out, call.webhook)
	}
	return out
}

type temporaryReplyError struct {
	err error
}

func (e temporaryReplyError) Error() string {
	return e.err.Error()
}

func (e temporaryReplyError) Temporary() bool {
	return true
}
