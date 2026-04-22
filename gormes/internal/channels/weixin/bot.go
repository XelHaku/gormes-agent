package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const (
	ChatTypeDirect = "direct"
	ChatTypeGroup  = "group"
)

// Config captures the first-pass Weixin contract surface.
type Config struct {
	DMPolicy       string
	AllowFrom      []string
	GroupPolicy    string
	GroupAllowFrom []string
}

// InboundMessage is the SDK-neutral Weixin poll event shape.
type InboundMessage struct {
	ChatType     string
	ChatID       string
	UserID       string
	UserName     string
	MessageID    string
	Text         string
	ContextToken string
}

// Client is the minimal Weixin surface used by the adapter.
type Client interface {
	Events() <-chan InboundMessage
	SendWithContext(ctx context.Context, chatID, contextToken, text string) (string, error)
	Close() error
}

type sendContext struct {
	chatType     string
	contextToken string
}

// Bot adapts Weixin traffic into the shared gateway channel contract.
type Bot struct {
	cfg           Config
	client        Client
	log           *slog.Logger
	allowedDMs    map[string]struct{}
	allowedGroups map[string]struct{}

	mu       sync.Mutex
	contexts map[string]sendContext
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:           cfg,
		client:        client,
		log:           log,
		allowedDMs:    toSet(cfg.AllowFrom),
		allowedGroups: toSet(cfg.GroupAllowFrom),
		contexts:      map[string]sendContext{},
	}
}

func (b *Bot) Name() string { return "weixin" }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	events := b.client.Events()
	for {
		select {
		case <-ctx.Done():
			_ = b.client.Close()
			return nil
		case msg, ok := <-events:
			if !ok {
				_ = b.client.Close()
				return nil
			}
			ev, ok := b.toInboundEvent(msg)
			if !ok {
				continue
			}
			select {
			case inbox <- ev:
			case <-ctx.Done():
				_ = b.client.Close()
				return nil
			}
		}
	}
}

func (b *Bot) Send(ctx context.Context, chatID, text string) (string, error) {
	meta, ok := b.lookupContext(chatID)
	if !ok || meta.contextToken == "" {
		return "", fmt.Errorf("weixin: no context token for chat %q", chatID)
	}
	return b.client.SendWithContext(ctx, chatID, meta.contextToken, text)
}

func (b *Bot) toInboundEvent(msg InboundMessage) (gateway.InboundEvent, bool) {
	text := strings.TrimSpace(msg.Text)
	chatID := strings.TrimSpace(msg.ChatID)
	userID := strings.TrimSpace(msg.UserID)
	if text == "" || chatID == "" || userID == "" {
		return gateway.InboundEvent{}, false
	}

	switch strings.TrimSpace(msg.ChatType) {
	case ChatTypeDirect:
		if !allowedByPolicy(b.cfg.DMPolicy, b.allowedDMs, userID, true) {
			return gateway.InboundEvent{}, false
		}
	case ChatTypeGroup:
		if !allowedByPolicy(b.cfg.GroupPolicy, b.allowedGroups, chatID, false) {
			return gateway.InboundEvent{}, false
		}
	default:
		return gateway.InboundEvent{}, false
	}

	b.rememberContext(chatID, sendContext{
		chatType:     strings.TrimSpace(msg.ChatType),
		contextToken: strings.TrimSpace(msg.ContextToken),
	})

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: "weixin",
		ChatID:   chatID,
		UserID:   userID,
		UserName: strings.TrimSpace(msg.UserName),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) rememberContext(chatID string, meta sendContext) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.contexts[chatID] = meta
}

func (b *Bot) lookupContext(chatID string) (sendContext, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	meta, ok := b.contexts[chatID]
	return meta, ok
}

func allowedByPolicy(policy string, allowed map[string]struct{}, value string, isDM bool) bool {
	switch normalizedPolicy(policy, isDM) {
	case "disabled":
		return false
	case "allowlist":
		_, ok := allowed[strings.TrimSpace(value)]
		return ok
	default:
		return true
	}
}

func normalizedPolicy(policy string, isDM bool) string {
	policy = strings.TrimSpace(strings.ToLower(policy))
	if policy != "" {
		return policy
	}
	if isDM {
		return "open"
	}
	return "disabled"
}

func toSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}
