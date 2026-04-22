package sms

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestNormalizeInbound_NormalizesNumbersAndBuildsReplyTarget(t *testing.T) {
	got, ok := NormalizeInbound(InboundMessage{
		From:      "1 (555) 123-4567",
		To:        "+1 555 000 1111",
		Body:      " hello from sms ",
		MessageID: "SM123",
	}, "+1 (555) 000-1111")
	if !ok {
		t.Fatal("NormalizeInbound() ok = false, want true")
	}

	if got.Event.Platform != "sms" {
		t.Fatalf("Platform = %q, want sms", got.Event.Platform)
	}
	if got.Event.ChatID != "+15551234567" {
		t.Fatalf("ChatID = %q, want +15551234567", got.Event.ChatID)
	}
	if got.Event.UserID != "+15551234567" {
		t.Fatalf("UserID = %q, want +15551234567", got.Event.UserID)
	}
	if got.Event.ChatName != "+15551234567" {
		t.Fatalf("ChatName = %q, want +15551234567", got.Event.ChatName)
	}
	if got.Event.UserName != "+15551234567" {
		t.Fatalf("UserName = %q, want +15551234567", got.Event.UserName)
	}
	if got.Event.MsgID != "SM123" {
		t.Fatalf("MsgID = %q, want SM123", got.Event.MsgID)
	}
	if got.Event.Kind != gateway.EventSubmit {
		t.Fatalf("Kind = %v, want %v", got.Event.Kind, gateway.EventSubmit)
	}
	if got.Event.Text != "hello from sms" {
		t.Fatalf("Text = %q, want hello from sms", got.Event.Text)
	}
	if got.Identity.ChatID != "+15551234567" {
		t.Fatalf("Identity.ChatID = %q, want +15551234567", got.Identity.ChatID)
	}
	if got.Identity.UserID != "+15551234567" {
		t.Fatalf("Identity.UserID = %q, want +15551234567", got.Identity.UserID)
	}
	if got.Identity.RecipientID != "+15550001111" {
		t.Fatalf("Identity.RecipientID = %q, want +15550001111", got.Identity.RecipientID)
	}
	if got.Reply.To != "+15551234567" {
		t.Fatalf("Reply.To = %q, want +15551234567", got.Reply.To)
	}
	if got.Reply.From != "+15550001111" {
		t.Fatalf("Reply.From = %q, want +15550001111", got.Reply.From)
	}
}

func TestNormalizeInbound_DelegatesCommandsAndRejectsEchoOrEmptyBodies(t *testing.T) {
	got, ok := NormalizeInbound(InboundMessage{
		From:      "+1 555 123 4567",
		To:        "+1 555 000 1111",
		Body:      " /help ",
		MessageID: "SM124",
	}, "+15550001111")
	if !ok {
		t.Fatal("NormalizeInbound(command) ok = false, want true")
	}
	if got.Event.Kind != gateway.EventStart {
		t.Fatalf("Kind = %v, want %v", got.Event.Kind, gateway.EventStart)
	}
	if got.Event.Text != "" {
		t.Fatalf("Text = %q, want empty command body", got.Event.Text)
	}

	if _, ok := NormalizeInbound(InboundMessage{
		From:      "+15550001111",
		To:        "+15550001111",
		Body:      "echo",
		MessageID: "SM125",
	}, "+1 (555) 000-1111"); ok {
		t.Fatal("NormalizeInbound(echo) ok = true, want false")
	}

	if _, ok := NormalizeInbound(InboundMessage{
		From:      "+15551234567",
		To:        "+15550001111",
		Body:      "   ",
		MessageID: "SM126",
	}, "+15550001111"); ok {
		t.Fatal("NormalizeInbound(empty) ok = true, want false")
	}
}

func TestBuildDelivery_SplitsAtNaturalBoundariesAndPreservesTargets(t *testing.T) {
	first := strings.Repeat("a", MaxSegmentLength-8)
	text := first + "\nsecond block"

	got, err := BuildDelivery(ReplyTarget{
		To:   "+15551234567",
		From: "+15550001111",
	}, text)
	if err != nil {
		t.Fatalf("BuildDelivery() error = %v", err)
	}

	if got.To != "+15551234567" {
		t.Fatalf("To = %q, want +15551234567", got.To)
	}
	if got.From != "+15550001111" {
		t.Fatalf("From = %q, want +15550001111", got.From)
	}
	if len(got.Segments) != 2 {
		t.Fatalf("segment count = %d, want 2", len(got.Segments))
	}
	if got.Segments[0] != first {
		t.Fatalf("segment[0] = %q, want first paragraph", got.Segments[0])
	}
	if got.Segments[1] != "second block" {
		t.Fatalf("segment[1] = %q, want second block", got.Segments[1])
	}
	for i, segment := range got.Segments {
		if len(segment) > MaxSegmentLength {
			t.Fatalf("segment[%d] length = %d, want <= %d", i, len(segment), MaxSegmentLength)
		}
	}
}

func TestBuildDelivery_HardSplitsUnbrokenText(t *testing.T) {
	text := strings.Repeat("x", MaxSegmentLength+25)

	got, err := BuildDelivery(ReplyTarget{
		To:   "+15551234567",
		From: "+15550001111",
	}, text)
	if err != nil {
		t.Fatalf("BuildDelivery() error = %v", err)
	}

	if len(got.Segments) != 2 {
		t.Fatalf("segment count = %d, want 2", len(got.Segments))
	}
	if len(got.Segments[0]) != MaxSegmentLength {
		t.Fatalf("segment[0] length = %d, want %d", len(got.Segments[0]), MaxSegmentLength)
	}
	if len(got.Segments[1]) != 25 {
		t.Fatalf("segment[1] length = %d, want 25", len(got.Segments[1]))
	}
}
