package weixin

import (
	"fmt"
	"strings"
	"sync"
)

const (
	DefaultBaseURL    = "https://ilinkai.weixin.qq.com"
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
)

// IngressMode selects the Weixin transport bootstrap shape without binding the
// real iLink client.
type IngressMode string

const (
	IngressModeLongPoll IngressMode = "long_poll"
)

// ReplyMode names Weixin's outbound continuity primitive.
type ReplyMode string

const (
	ReplyModeContextToken ReplyMode = "context_token"
)

// RuntimeConfig captures operator-controlled Weixin bootstrap inputs.
type RuntimeConfig struct {
	AccountID  string
	Token      string
	BaseURL    string
	CDNBaseURL string
}

// RuntimePlan is the test-frozen Weixin transport/bootstrap contract.
type RuntimePlan struct {
	Ingress  IngressPlan
	Outbound OutboundPlan
}

// IngressPlan describes the selected long-poll operating model.
type IngressPlan struct {
	Mode                 IngressMode
	BaseURL              string
	CDNBaseURL           string
	RequiresPublicURL    bool
	SinglePollerPerToken bool
}

// OutboundPlan describes Weixin context-token reply continuity.
type OutboundPlan struct {
	Mode                 ReplyMode
	RequiresContextToken bool
	PersistContextTokens bool
}

// DecideRuntime validates credentials and freezes the Weixin long-poll +
// context-token contract before a real iLink transport is bound.
func DecideRuntime(cfg RuntimeConfig) (RuntimePlan, error) {
	if strings.TrimSpace(cfg.AccountID) == "" || strings.TrimSpace(cfg.Token) == "" {
		return RuntimePlan{}, fmt.Errorf("weixin: account id and token are required for long-poll mode")
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	cdnBaseURL := strings.TrimSpace(cfg.CDNBaseURL)
	if cdnBaseURL == "" {
		cdnBaseURL = DefaultCDNBaseURL
	}

	return RuntimePlan{
		Ingress: IngressPlan{
			Mode:                 IngressModeLongPoll,
			BaseURL:              baseURL,
			CDNBaseURL:           cdnBaseURL,
			RequiresPublicURL:    false,
			SinglePollerPerToken: true,
		},
		Outbound: OutboundPlan{
			Mode:                 ReplyModeContextToken,
			RequiresContextToken: true,
			PersistContextTokens: true,
		},
	}, nil
}

// ContextTokens is the narrow persistence seam future disk-backed iLink state
// can implement.
type ContextTokens struct {
	values sync.Map
}

func NewContextTokens() *ContextTokens {
	return &ContextTokens{}
}

func (s *ContextTokens) Remember(chatID, token string) {
	if s == nil {
		return
	}
	chatID = strings.TrimSpace(chatID)
	token = strings.TrimSpace(token)
	if chatID == "" || token == "" {
		return
	}
	s.values.Store(chatID, token)
}

func (s *ContextTokens) Lookup(chatID string) (string, error) {
	chatID = strings.TrimSpace(chatID)
	if s == nil {
		return "", fmt.Errorf("weixin: no context token for chat %q", chatID)
	}
	raw, ok := s.values.Load(chatID)
	if !ok {
		return "", fmt.Errorf("weixin: no context token for chat %q", chatID)
	}
	token, ok := raw.(string)
	if !ok || strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("weixin: no context token for chat %q", chatID)
	}
	return strings.TrimSpace(token), nil
}
