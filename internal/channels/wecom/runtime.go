package wecom

import (
	"fmt"
	"strings"
)

const (
	DefaultWebSocketURL = "wss://openws.work.weixin.qq.com"
	DefaultCallbackPath = "/wecom/callback"
)

// IngressMode selects the WeCom transport bootstrap shape without binding a
// concrete SDK implementation.
type IngressMode string

const (
	IngressModeWebSocket IngressMode = "websocket"
	IngressModeCallback  IngressMode = "callback"
)

// CredentialKind records which credential set was validated for the plan.
type CredentialKind string

const (
	CredentialKindAIBot    CredentialKind = "ai_bot"
	CredentialKindCallback CredentialKind = "callback"
)

// OutboundMode names the platform-specific delivery primitive selected for a
// message.
type OutboundMode string

const (
	OutboundModeReplyThenPush OutboundMode = "reply_then_push"
	OutboundModeReply         OutboundMode = "reply"
	OutboundModeActivePush    OutboundMode = "active_push"
)

// RuntimeConfig captures operator-controlled WeCom bootstrap inputs.
type RuntimeConfig struct {
	Mode         IngressMode
	BotID        string
	Secret       string
	WebSocketURL string
	Callback     CallbackConfig
}

// CallbackConfig captures the self-built-app callback credential set.
type CallbackConfig struct {
	CorpID         string
	AgentID        string
	AppSecret      string
	Token          string
	EncodingAESKey string
	Path           string
}

// RuntimePlan is the test-frozen WeCom transport/bootstrap contract.
type RuntimePlan struct {
	Credentials CredentialPlan
	Ingress     IngressPlan
	Outbound    OutboundPlan
}

type CredentialPlan struct {
	Kind CredentialKind
}

// IngressPlan describes the selected inbound operating model.
type IngressPlan struct {
	Mode              IngressMode
	Endpoint          string
	CallbackPath      string
	AutoReconnect     bool
	RequiresPublicURL bool
}

// OutboundPlan describes how replies leave the adapter.
type OutboundPlan struct {
	Mode                  OutboundMode
	RequiresActiveRequest bool
	FallbackToPush        bool
}

// DecideRuntime validates credentials and freezes WeCom's runtime seams before
// a concrete WebSocket or HTTP callback transport is bound.
func DecideRuntime(cfg RuntimeConfig) (RuntimePlan, error) {
	mode := normalizedIngressMode(cfg.Mode)
	if mode == "" {
		if hasCallbackConfig(cfg.Callback) {
			mode = IngressModeCallback
		} else {
			mode = IngressModeWebSocket
		}
	}

	switch mode {
	case IngressModeWebSocket:
		if strings.TrimSpace(cfg.BotID) == "" || strings.TrimSpace(cfg.Secret) == "" {
			return RuntimePlan{}, fmt.Errorf("wecom: bot id and secret are required for websocket mode")
		}
		endpoint := strings.TrimSpace(cfg.WebSocketURL)
		if endpoint == "" {
			endpoint = DefaultWebSocketURL
		}
		return RuntimePlan{
			Credentials: CredentialPlan{Kind: CredentialKindAIBot},
			Ingress: IngressPlan{
				Mode:              IngressModeWebSocket,
				Endpoint:          endpoint,
				AutoReconnect:     true,
				RequiresPublicURL: false,
			},
			Outbound: OutboundPlan{
				Mode:                  OutboundModeReplyThenPush,
				RequiresActiveRequest: true,
				FallbackToPush:        true,
			},
		}, nil
	case IngressModeCallback:
		callback := normalizeCallbackConfig(cfg.Callback)
		if callback.CorpID == "" ||
			callback.AgentID == "" ||
			callback.AppSecret == "" ||
			callback.Token == "" ||
			callback.EncodingAESKey == "" {
			return RuntimePlan{}, fmt.Errorf("wecom: corp id, agent id, app secret, callback token, and encoding aes key are required for callback mode")
		}
		if len(callback.EncodingAESKey) != 43 {
			return RuntimePlan{}, fmt.Errorf("wecom: callback encoding aes key must be 43 characters")
		}
		if callback.Path == "" {
			callback.Path = DefaultCallbackPath
		}
		return RuntimePlan{
			Credentials: CredentialPlan{Kind: CredentialKindCallback},
			Ingress: IngressPlan{
				Mode:              IngressModeCallback,
				CallbackPath:      callback.Path,
				AutoReconnect:     false,
				RequiresPublicURL: true,
			},
			Outbound: OutboundPlan{
				Mode:                  OutboundModeActivePush,
				RequiresActiveRequest: false,
				FallbackToPush:        false,
			},
		}, nil
	default:
		return RuntimePlan{}, fmt.Errorf("wecom: unsupported ingress mode %q", cfg.Mode)
	}
}

// OutboundContext is the SDK-neutral routing state remembered from inbound
// WeCom messages.
type OutboundContext struct {
	ChatID    string
	ChatType  string
	RequestID string
}

// OutboundDecision chooses reply-mode only while an inbound request id is
// available; otherwise delivery is a proactive push.
type OutboundDecision struct {
	Primary   OutboundMode
	Fallback  OutboundMode
	ChatID    string
	ChatType  string
	RequestID string
}

func DecideOutbound(ctx OutboundContext) OutboundDecision {
	decision := OutboundDecision{
		ChatID:    strings.TrimSpace(ctx.ChatID),
		ChatType:  strings.TrimSpace(ctx.ChatType),
		RequestID: strings.TrimSpace(ctx.RequestID),
	}
	if decision.RequestID == "" {
		decision.Primary = OutboundModeActivePush
		return decision
	}
	decision.Primary = OutboundModeReply
	decision.Fallback = OutboundModeActivePush
	return decision
}

func normalizedIngressMode(mode IngressMode) IngressMode {
	return IngressMode(strings.TrimSpace(strings.ToLower(string(mode))))
}

func hasCallbackConfig(cfg CallbackConfig) bool {
	return strings.TrimSpace(cfg.CorpID) != "" ||
		strings.TrimSpace(cfg.AgentID) != "" ||
		strings.TrimSpace(cfg.AppSecret) != "" ||
		strings.TrimSpace(cfg.Token) != "" ||
		strings.TrimSpace(cfg.EncodingAESKey) != "" ||
		strings.TrimSpace(cfg.Path) != ""
}

func normalizeCallbackConfig(cfg CallbackConfig) CallbackConfig {
	return CallbackConfig{
		CorpID:         strings.TrimSpace(cfg.CorpID),
		AgentID:        strings.TrimSpace(cfg.AgentID),
		AppSecret:      strings.TrimSpace(cfg.AppSecret),
		Token:          strings.TrimSpace(cfg.Token),
		EncodingAESKey: strings.TrimSpace(cfg.EncodingAESKey),
		Path:           strings.TrimSpace(cfg.Path),
	}
}
