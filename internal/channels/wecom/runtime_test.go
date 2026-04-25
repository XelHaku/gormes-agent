package wecom

import (
	"strings"
	"testing"
)

func TestDecideRuntime_WebSocketBotDefaults(t *testing.T) {
	plan, err := DecideRuntime(RuntimeConfig{
		BotID:  "bot-1",
		Secret: "secret-1",
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Credentials.Kind != CredentialKindAIBot {
		t.Fatalf("Credentials.Kind = %q, want %q", plan.Credentials.Kind, CredentialKindAIBot)
	}
	if plan.Ingress.Mode != IngressModeWebSocket {
		t.Fatalf("Ingress.Mode = %q, want %q", plan.Ingress.Mode, IngressModeWebSocket)
	}
	if plan.Ingress.Endpoint != DefaultWebSocketURL {
		t.Fatalf("Ingress.Endpoint = %q, want %q", plan.Ingress.Endpoint, DefaultWebSocketURL)
	}
	if plan.Ingress.RequiresPublicURL {
		t.Fatal("Ingress.RequiresPublicURL = true, want false")
	}
	if !plan.Ingress.AutoReconnect {
		t.Fatal("Ingress.AutoReconnect = false, want true")
	}
	if plan.Outbound.Mode != OutboundModeReplyThenPush {
		t.Fatalf("Outbound.Mode = %q, want %q", plan.Outbound.Mode, OutboundModeReplyThenPush)
	}
	if !plan.Outbound.RequiresActiveRequest {
		t.Fatal("Outbound.RequiresActiveRequest = false, want true")
	}
	if !plan.Outbound.FallbackToPush {
		t.Fatal("Outbound.FallbackToPush = false, want true")
	}
}

func TestDecideRuntime_CallbackDefaults(t *testing.T) {
	plan, err := DecideRuntime(RuntimeConfig{
		Mode: IngressModeCallback,
		Callback: CallbackConfig{
			CorpID:         "ww123",
			AgentID:        "1000002",
			AppSecret:      "app-secret",
			Token:          "callback-token",
			EncodingAESKey: strings.Repeat("a", 43),
		},
	})
	if err != nil {
		t.Fatalf("DecideRuntime() error = %v, want nil", err)
	}

	if plan.Credentials.Kind != CredentialKindCallback {
		t.Fatalf("Credentials.Kind = %q, want %q", plan.Credentials.Kind, CredentialKindCallback)
	}
	if plan.Ingress.Mode != IngressModeCallback {
		t.Fatalf("Ingress.Mode = %q, want %q", plan.Ingress.Mode, IngressModeCallback)
	}
	if plan.Ingress.CallbackPath != DefaultCallbackPath {
		t.Fatalf("Ingress.CallbackPath = %q, want %q", plan.Ingress.CallbackPath, DefaultCallbackPath)
	}
	if !plan.Ingress.RequiresPublicURL {
		t.Fatal("Ingress.RequiresPublicURL = false, want true")
	}
	if plan.Ingress.AutoReconnect {
		t.Fatal("Ingress.AutoReconnect = true, want false")
	}
	if plan.Outbound.Mode != OutboundModeActivePush {
		t.Fatalf("Outbound.Mode = %q, want %q", plan.Outbound.Mode, OutboundModeActivePush)
	}
	if plan.Outbound.RequiresActiveRequest {
		t.Fatal("Outbound.RequiresActiveRequest = true, want false")
	}
}

func TestDecideRuntime_RequiresCredentialsForSelectedMode(t *testing.T) {
	if _, err := DecideRuntime(RuntimeConfig{BotID: "bot-1"}); err == nil {
		t.Fatal("DecideRuntime() error = nil, want websocket credential validation failure")
	} else if got := err.Error(); got != "wecom: bot id and secret are required for websocket mode" {
		t.Fatalf("websocket error = %q, want credential validation failure", got)
	}

	_, err := DecideRuntime(RuntimeConfig{
		Mode: IngressModeCallback,
		Callback: CallbackConfig{
			CorpID:    "ww123",
			AgentID:   "1000002",
			AppSecret: "app-secret",
		},
	})
	if err == nil {
		t.Fatal("DecideRuntime() error = nil, want callback credential validation failure")
	}
	if got := err.Error(); got != "wecom: corp id, agent id, app secret, callback token, and encoding aes key are required for callback mode" {
		t.Fatalf("callback error = %q, want credential validation failure", got)
	}
}

func TestDecideRuntime_RejectsInvalidCallbackAESKey(t *testing.T) {
	_, err := DecideRuntime(RuntimeConfig{
		Mode: IngressModeCallback,
		Callback: CallbackConfig{
			CorpID:         "ww123",
			AgentID:        "1000002",
			AppSecret:      "app-secret",
			Token:          "callback-token",
			EncodingAESKey: "too-short",
		},
	})
	if err == nil {
		t.Fatal("DecideRuntime() error = nil, want aes key validation failure")
	}
	if got := err.Error(); got != "wecom: callback encoding aes key must be 43 characters" {
		t.Fatalf("DecideRuntime() error = %q, want aes key validation failure", got)
	}
}

func TestDecideOutbound_UsesReplyOnlyWhenRequestIDIsActive(t *testing.T) {
	reply := DecideOutbound(OutboundContext{
		ChatID:    "chat-1",
		ChatType:  ChatTypeGroup,
		RequestID: "req-1",
	})
	if reply.Primary != OutboundModeReply {
		t.Fatalf("Primary = %q, want %q", reply.Primary, OutboundModeReply)
	}
	if reply.Fallback != OutboundModeActivePush {
		t.Fatalf("Fallback = %q, want %q", reply.Fallback, OutboundModeActivePush)
	}
	if reply.RequestID != "req-1" {
		t.Fatalf("RequestID = %q, want req-1", reply.RequestID)
	}

	push := DecideOutbound(OutboundContext{
		ChatID:   "chat-1",
		ChatType: ChatTypeGroup,
	})
	if push.Primary != OutboundModeActivePush {
		t.Fatalf("Primary = %q, want %q", push.Primary, OutboundModeActivePush)
	}
	if push.Fallback != "" {
		t.Fatalf("Fallback = %q, want empty fallback", push.Fallback)
	}
}
