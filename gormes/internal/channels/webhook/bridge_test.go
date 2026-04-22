package webhook

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

func TestBuildPromptDelivery_RendersPromptAndExplicitThreadTarget(t *testing.T) {
	got, err := BuildPromptDelivery("pr-bot", "gh-1", "pull_request", RouteConfig{
		Prompt:  "PR {number} in {repository.full_name}: {pull_request.title}",
		Deliver: "telegram",
		DeliverExtra: map[string]any{
			"chat_id":   "{meta.chat_id}",
			"thread_id": "{number}",
		},
	}, map[string]any{
		"number": 42,
		"meta": map[string]any{
			"chat_id": "-100123",
		},
		"repository": map[string]any{
			"full_name": "org/repo",
		},
		"pull_request": map[string]any{
			"title": "Fix bug",
		},
	})
	if err != nil {
		t.Fatalf("BuildPromptDelivery() error = %v", err)
	}
	if got.SessionChatID != "webhook:pr-bot:gh-1" {
		t.Fatalf("SessionChatID = %q, want %q", got.SessionChatID, "webhook:pr-bot:gh-1")
	}
	if got.Prompt != "PR 42 in org/repo: Fix bug" {
		t.Fatalf("Prompt = %q, want rendered prompt", got.Prompt)
	}
	want := gateway.DeliveryTarget{
		Platform:   "telegram",
		ChatID:     "-100123",
		ThreadID:   "42",
		IsExplicit: true,
	}
	if got.Target != want {
		t.Fatalf("Target = %+v, want %+v", got.Target, want)
	}
	if !got.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
}

func TestBuildPromptDelivery_MapsMessageThreadIDAlias(t *testing.T) {
	got, err := BuildPromptDelivery("alerts", "evt-2", "issues", RouteConfig{
		Prompt:  "Issue {number}",
		Deliver: "telegram",
		DeliverExtra: map[string]any{
			"chat_id":           "12345",
			"message_thread_id": "888",
		},
	}, map[string]any{"number": 7})
	if err != nil {
		t.Fatalf("BuildPromptDelivery() error = %v", err)
	}
	if got.Target.ThreadID != "888" {
		t.Fatalf("ThreadID = %q, want %q", got.Target.ThreadID, "888")
	}
}

func TestBuildPromptDelivery_DefaultPromptAndPlatformHomeTarget(t *testing.T) {
	got, err := BuildPromptDelivery("issue-hook", "evt-3", "issues", RouteConfig{
		Deliver: "discord",
	}, map[string]any{"action": "opened"})
	if err != nil {
		t.Fatalf("BuildPromptDelivery() error = %v", err)
	}
	if !strings.Contains(got.Prompt, "Webhook event 'issues'") {
		t.Fatalf("Prompt = %q, want default event header", got.Prompt)
	}
	if !strings.Contains(got.Prompt, "issue-hook") {
		t.Fatalf("Prompt = %q, want route name", got.Prompt)
	}
	want := gateway.DeliveryTarget{Platform: "discord"}
	if got.Target != want {
		t.Fatalf("Target = %+v, want %+v", got.Target, want)
	}
	if !got.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
}

func TestBuildPromptDelivery_NonGatewayDeliverySkipsTarget(t *testing.T) {
	got, err := BuildPromptDelivery("log-hook", "evt-4", "push", RouteConfig{
		Deliver: "log",
	}, map[string]any{"ref": "main"})
	if err != nil {
		t.Fatalf("BuildPromptDelivery() error = %v", err)
	}
	if got.HasTarget {
		t.Fatalf("HasTarget = true, want false with Target=%+v", got.Target)
	}
}

func TestBuildPromptDelivery_RejectsThreadWithoutChatID(t *testing.T) {
	_, err := BuildPromptDelivery("bad-hook", "evt-5", "push", RouteConfig{
		Prompt:  "hello",
		Deliver: "telegram",
		DeliverExtra: map[string]any{
			"thread_id": "77",
		},
	}, map[string]any{"ref": "main"})
	if err == nil {
		t.Fatal("BuildPromptDelivery() error = nil, want non-nil")
	}
}

func TestBuildPromptDelivery_RejectsMissingRouteOrDeliveryID(t *testing.T) {
	for _, tt := range []struct {
		name       string
		routeName  string
		deliveryID string
	}{
		{name: "missing route", routeName: "", deliveryID: "evt"},
		{name: "missing delivery", routeName: "hook", deliveryID: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildPromptDelivery(tt.routeName, tt.deliveryID, "push", RouteConfig{
				Prompt:  "hello",
				Deliver: "telegram",
			}, map[string]any{})
			if err == nil {
				t.Fatal("BuildPromptDelivery() error = nil, want non-nil")
			}
		})
	}
}
