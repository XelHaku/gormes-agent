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

func TestBot_Run_RoutesResolvedImageAndFileAttachments(t *testing.T) {
	client := newMediaMockClient()
	client.downloadURLs = map[string]string{
		"img-code-1":  "https://media.dingtalk.example/image.png",
		"file-code-1": "https://media.dingtalk.example/report.pdf",
	}
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
		Text:             "please inspect these",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=reply",
		ImageContent: &MediaContent{
			DownloadCode: "img-code-1",
		},
		RichTextContent: []RichTextItem{
			{
				Type:         "file",
				DownloadCode: "file-code-1",
				FileName:     "report.pdf",
			},
		},
	})

	var ev gateway.InboundEvent
	select {
	case ev = <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected inbound event")
	}

	if got, want := ev.Text, "please inspect these"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	assertMediaFixture(t, "resolvedAttachments", ev.Attachments)
	assertMediaFixture(t, "downloadRequests", client.downloadRequests)
}

func TestBot_Run_RoutesMediaOnlyAttachmentWithDownloadFailureFallback(t *testing.T) {
	client := newMediaMockClient()
	client.downloadErrByCode = map[string]error{
		"file-code-timeout": errors.New("429 rate limit"),
	}
	b := New(Config{RobotCode: "robot-code"}, client, nil)
	inbox := make(chan gateway.InboundEvent, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, inbox) }()

	client.push(InboundMessage{
		MessageID:        "user-msg-2",
		ConversationID:   "dm-42",
		ConversationType: "1",
		SenderStaffID:    "staff-1",
		SessionWebhook:   "https://api.dingtalk.com/robot/send?access_token=reply",
		RichTextContent: []RichTextItem{
			{
				Type:         "file",
				DownloadCode: "file-code-timeout",
				FileName:     "report.pdf",
			},
		},
	})

	var ev gateway.InboundEvent
	select {
	case ev = <-inbox:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected media-only inbound event")
	}

	if ev.Kind != gateway.EventSubmit {
		t.Fatalf("Kind = %v, want %v", ev.Kind, gateway.EventSubmit)
	}
	if ev.Text != "" {
		t.Fatalf("Text = %q, want empty media-only text", ev.Text)
	}
	assertMediaFixture(t, "fallbackAttachments", ev.Attachments)
}

type mediaMockClient struct {
	*mockClient

	downloadURLs      map[string]string
	downloadRequests  []MediaDownloadRequest
	downloadErrByCode map[string]error
}

func newMediaMockClient() *mediaMockClient {
	return &mediaMockClient{mockClient: newMockClient()}
}

func (m *mediaMockClient) DownloadMedia(_ context.Context, req MediaDownloadRequest) (MediaDownloadResult, error) {
	m.downloadRequests = append(m.downloadRequests, req)
	if err := m.downloadErrByCode[req.DownloadCode]; err != nil {
		return MediaDownloadResult{}, err
	}
	return MediaDownloadResult{DownloadURL: m.downloadURLs[req.DownloadCode]}, nil
}

func assertMediaFixture(t *testing.T, key string, got any) {
	t.Helper()

	fixtures := loadMediaFixtures(t)
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
		t.Fatalf("%s mismatch\ngot:  %s\nwant: %s", key, gotRaw, wantRaw)
	}
}

func loadMediaFixtures(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	raw, err := os.ReadFile("testdata/media_attachment_contract.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("unmarshal fixture file: %v", err)
	}
	return fixtures
}
