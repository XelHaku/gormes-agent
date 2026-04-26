package slack

import (
	"strings"
	"testing"
)

func TestThreadParentContextIncludesBotParentAndScopesCacheByTeam(t *testing.T) {
	cache := newThreadContextCache("UBOT")

	got := cache.Store("C123", "1000.0", "T_A", []ThreadMessage{
		{Timestamp: "1000.0", TeamID: "T_A", BotID: "B_CRON", Text: "cron parent summary"},
		{Timestamp: "1000.1", TeamID: "T_A", UserID: "UBOT", BotID: "B_GORMES", Text: "self bot child echo"},
		{Timestamp: "1000.2", TeamID: "T_A", UserID: "U_HELPER", BotID: "B_HELPER", Text: "third-party bot child"},
	})
	if got.ParentText != "cron parent summary" {
		t.Fatalf("ParentText = %q, want bot-authored thread parent", got.ParentText)
	}
	if !strings.Contains(got.ContextText, "cron parent summary") {
		t.Fatalf("ContextText missing bot-authored parent:\n%s", got.ContextText)
	}
	if strings.Contains(got.ContextText, "self bot child echo") {
		t.Fatalf("ContextText included self-bot child reply:\n%s", got.ContextText)
	}
	if !strings.Contains(got.ContextText, "third-party bot child") {
		t.Fatalf("ContextText missing third-party bot child:\n%s", got.ContextText)
	}

	cache.Store("C123", "1000.0", "T_B", []ThreadMessage{
		{Timestamp: "1000.0", TeamID: "T_B", BotID: "B_CRON", Text: "workspace B parent"},
	})

	if parent := cache.ParentText("C123", "1000.0", "T_A"); parent != "cron parent summary" {
		t.Fatalf("team A cached parent = %q, want cron parent summary", parent)
	}
	if parent := cache.ParentText("C123", "1000.0", "T_B"); parent != "workspace B parent" {
		t.Fatalf("team B cached parent = %q, want workspace B parent", parent)
	}
}
