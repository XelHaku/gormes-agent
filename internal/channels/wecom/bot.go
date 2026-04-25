package wecom

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

const (
	ChatTypeDirect = "direct"
	ChatTypeGroup  = "group"
)

// Config captures the first-pass WeCom contract surface.
type Config struct {
	DMPolicy       string
	AllowFrom      []string
	GroupPolicy    string
	GroupAllowFrom []string
}

// InboundMessage is the SDK-neutral WeCom event shape.
type InboundMessage struct {
	ChatType  string
	ChatID    string
	UserID    string
	UserName  string
	MessageID string
	Text      string
	RequestID string
}

// Client is the minimal WeCom surface used by the adapter.
type Client interface {
	Events() <-chan InboundMessage
	SendReply(ctx context.Context, requestID, text string) (string, error)
	SendPush(ctx context.Context, chatID, chatType, text string) (string, error)
	Close() error
}

type route struct {
	chatType  string
	requestID string
}

// Bot adapts WeCom traffic into the shared gateway channel contract.
type Bot struct {
	cfg           Config
	client        Client
	log           *slog.Logger
	allowedDMs    map[string]struct{}
	allowedGroups map[string]struct{}

	mu     sync.Mutex
	routes map[string]route
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
		routes:        map[string]route{},
	}
}

func (b *Bot) Name() string { return "wecom" }

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
	meta, ok := b.lookupRoute(chatID)
	if !ok {
		return "", fmt.Errorf("wecom: no route for chat %q", chatID)
	}
	decision := DecideOutbound(OutboundContext{
		ChatID:    chatID,
		ChatType:  meta.chatType,
		RequestID: meta.requestID,
	})
	if decision.Primary == OutboundModeReply {
		msgID, err := b.client.SendReply(ctx, decision.RequestID, text)
		if err == nil {
			return msgID, nil
		}
	}
	return b.client.SendPush(ctx, decision.ChatID, decision.ChatType, text)
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
		if !allowedByPolicy(b.cfg.DMPolicy, b.allowedDMs, userID) {
			return gateway.InboundEvent{}, false
		}
	case ChatTypeGroup:
		if !allowedByPolicy(b.cfg.GroupPolicy, b.allowedGroups, chatID) {
			return gateway.InboundEvent{}, false
		}
	default:
		return gateway.InboundEvent{}, false
	}

	b.rememberRoute(chatID, route{
		chatType:  strings.TrimSpace(msg.ChatType),
		requestID: strings.TrimSpace(msg.RequestID),
	})

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: "wecom",
		ChatID:   chatID,
		UserID:   userID,
		UserName: strings.TrimSpace(msg.UserName),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) rememberRoute(chatID string, meta route) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routes[chatID] = meta
}

func (b *Bot) lookupRoute(chatID string) (route, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	meta, ok := b.routes[chatID]
	return meta, ok
}

func allowedByPolicy(policy string, allowed map[string]struct{}, value string) bool {
	switch normalizedPolicy(policy) {
	case "disabled":
		return false
	case "allowlist":
		_, ok := allowed[strings.TrimSpace(value)]
		return ok
	default:
		return true
	}
}

func normalizedPolicy(policy string) string {
	policy = strings.TrimSpace(strings.ToLower(policy))
	if policy == "" {
		return "open"
	}
	return policy
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
