package email

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestNormalizeInbound_MultipartPrefersPlainTextAndBuildsReplyTarget(t *testing.T) {
	raw := strings.Join([]string{
		"From: Alice Example <alice@example.com>",
		"To: hermes@example.com",
		"Subject: Deploy to production",
		"Message-ID: <msg-1@example.com>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello from plain",
		"--b1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>hello from html</p>",
		"--b1--",
		"",
	}, "\r\n")

	got, ok, err := NormalizeInbound([]byte(raw))
	if err != nil {
		t.Fatalf("NormalizeInbound() error = %v", err)
	}
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if got.Event.Platform != "email" {
		t.Fatalf("Platform = %q, want email", got.Event.Platform)
	}
	if got.Event.ChatID != "alice@example.com" {
		t.Fatalf("ChatID = %q, want alice@example.com", got.Event.ChatID)
	}
	if got.Event.UserID != "alice@example.com" {
		t.Fatalf("UserID = %q, want alice@example.com", got.Event.UserID)
	}
	if got.Event.UserName != "Alice Example" {
		t.Fatalf("UserName = %q, want Alice Example", got.Event.UserName)
	}
	if got.Event.MsgID != "<msg-1@example.com>" {
		t.Fatalf("MsgID = %q, want <msg-1@example.com>", got.Event.MsgID)
	}
	if got.Event.Kind != gateway.EventSubmit {
		t.Fatalf("Kind = %v, want %v", got.Event.Kind, gateway.EventSubmit)
	}
	wantText := "[Subject: Deploy to production]\n\nhello from plain"
	if got.Event.Text != wantText {
		t.Fatalf("Text = %q, want %q", got.Event.Text, wantText)
	}
	if got.Reply.To != "alice@example.com" {
		t.Fatalf("Reply.To = %q, want alice@example.com", got.Reply.To)
	}
	if got.Reply.Subject != "Re: Deploy to production" {
		t.Fatalf("Reply.Subject = %q, want %q", got.Reply.Subject, "Re: Deploy to production")
	}
	if got.Reply.InReplyTo != "<msg-1@example.com>" {
		t.Fatalf("Reply.InReplyTo = %q, want <msg-1@example.com>", got.Reply.InReplyTo)
	}
	if got.Reply.References != "<msg-1@example.com>" {
		t.Fatalf("Reply.References = %q, want <msg-1@example.com>", got.Reply.References)
	}
}

func TestNormalizeInbound_HTMLOnlyReplyStripsTagsAndParsesCommand(t *testing.T) {
	raw := strings.Join([]string{
		"From: Bob Example <bob@example.com>",
		"To: hermes@example.com",
		"Subject: Re: Existing thread",
		"Message-ID: <msg-2@example.com>",
		"In-Reply-To: <root@example.com>",
		"References: <older@example.com> <root@example.com>",
		"MIME-Version: 1.0",
		`Content-Type: text/html; charset=utf-8`,
		"",
		"<div><b>/help</b></div>",
		"",
	}, "\r\n")

	got, ok, err := NormalizeInbound([]byte(raw))
	if err != nil {
		t.Fatalf("NormalizeInbound() error = %v", err)
	}
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}
	if got.Event.Kind != gateway.EventStart {
		t.Fatalf("Kind = %v, want %v", got.Event.Kind, gateway.EventStart)
	}
	if got.Event.Text != "" {
		t.Fatalf("Text = %q, want empty command body", got.Event.Text)
	}
	if got.Reply.Subject != "Re: Existing thread" {
		t.Fatalf("Reply.Subject = %q, want %q", got.Reply.Subject, "Re: Existing thread")
	}
	wantRefs := "<older@example.com> <root@example.com> <msg-2@example.com>"
	if got.Reply.References != wantRefs {
		t.Fatalf("Reply.References = %q, want %q", got.Reply.References, wantRefs)
	}
	if got.Reply.InReplyTo != "<msg-2@example.com>" {
		t.Fatalf("Reply.InReplyTo = %q, want %q", got.Reply.InReplyTo, "<msg-2@example.com>")
	}
}

func TestNormalizeInbound_RejectsMissingSenderAndEmptyNormalizedBody(t *testing.T) {
	missingSender := strings.Join([]string{
		"To: hermes@example.com",
		"Subject: Missing sender",
		"Message-ID: <msg-3@example.com>",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello",
		"",
	}, "\r\n")

	if _, ok, err := NormalizeInbound([]byte(missingSender)); err != nil {
		t.Fatalf("NormalizeInbound(missing sender) error = %v", err)
	} else if ok {
		t.Fatal("NormalizeInbound(missing sender) ok = true, want false")
	}

	emptyHTML := strings.Join([]string{
		"From: Carol Example <carol@example.com>",
		"To: hermes@example.com",
		"Subject: Empty body",
		"Message-ID: <msg-4@example.com>",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<div><br/></div>",
		"",
	}, "\r\n")

	if _, ok, err := NormalizeInbound([]byte(emptyHTML)); err != nil {
		t.Fatalf("NormalizeInbound(empty html) error = %v", err)
	} else if ok {
		t.Fatal("NormalizeInbound(empty html) ok = true, want false")
	}
}
