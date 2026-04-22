package feishu

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const (
	ModeWebsocket = "websocket"
	ModeWebhook   = "webhook"

	SourceWebsocket = "websocket"
	SourceWebhook   = "webhook"

	ChatTypeDirect = "direct"
	ChatTypeGroup  = "group"
)

// Config captures the first-pass Feishu contract surface.
type Config struct {
	ConnectionMode    string
	VerificationToken string
	GroupPolicy       string
	AllowedUserIDs    []string
}

// InboundMessage is the SDK-neutral Feishu event shape.
type InboundMessage struct {
	Source       string
	ChatID       string
	ChatType     string
	UserID       string
	UserName     string
	MessageID    string
	Text         string
	Mentioned    bool
	VerifyToken  string
	ThreadRootID string
}

// SendOptions captures reply-target metadata for outbound sends.
type SendOptions struct {
	ReplyToMessageID string
	ThreadRootID     string
}

// Client is the minimal Feishu surface used by the adapter.
type Client interface {
	Events() <-chan InboundMessage
	SendRichText(ctx context.Context, chatID, text string, opts SendOptions) (string, error)
	SendText(ctx context.Context, chatID, text string, opts SendOptions) (string, error)
	Close() error
}

type replyTarget struct {
	messageID    string
	threadRootID string
}

// Bot adapts Feishu traffic into the shared gateway channel contract.
type Bot struct {
	cfg     Config
	client  Client
	log     *slog.Logger
	allowed map[string]struct{}

	mu      sync.Mutex
	replies map[string]replyTarget
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:     cfg,
		client:  client,
		log:     log,
		allowed: toSet(cfg.AllowedUserIDs),
		replies: map[string]replyTarget{},
	}
}

func (b *Bot) Name() string { return "feishu" }

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
	opts, ok := b.replyOptions(chatID)
	if !ok {
		return "", fmt.Errorf("feishu: no reply target for chat %q", chatID)
	}
	if looksLikeMarkdown(text) {
		msgID, err := b.client.SendRichText(ctx, chatID, text, opts)
		if err == nil {
			return msgID, nil
		}
	}
	return b.client.SendText(ctx, chatID, text, opts)
}

func (b *Bot) toInboundEvent(msg InboundMessage) (gateway.InboundEvent, bool) {
	if !b.sourceAllowed(msg.Source) {
		return gateway.InboundEvent{}, false
	}
	if msg.Source == SourceWebhook &&
		strings.TrimSpace(b.cfg.VerificationToken) != "" &&
		strings.TrimSpace(msg.VerifyToken) != strings.TrimSpace(b.cfg.VerificationToken) {
		return gateway.InboundEvent{}, false
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return gateway.InboundEvent{}, false
	}

	chatID := strings.TrimSpace(msg.ChatID)
	userID := strings.TrimSpace(msg.UserID)
	if chatID == "" || userID == "" {
		return gateway.InboundEvent{}, false
	}

	switch strings.TrimSpace(msg.ChatType) {
	case ChatTypeGroup:
		if normalizedPolicy(b.cfg.GroupPolicy) == "disabled" {
			return gateway.InboundEvent{}, false
		}
		if !msg.Mentioned {
			return gateway.InboundEvent{}, false
		}
		if normalizedPolicy(b.cfg.GroupPolicy) == "allowlist" && !b.allowedUser(userID) {
			return gateway.InboundEvent{}, false
		}
		text = stripLeadingMentions(text)
		if text == "" {
			return gateway.InboundEvent{}, false
		}
	case ChatTypeDirect:
		if len(b.allowed) > 0 && !b.allowedUser(userID) {
			return gateway.InboundEvent{}, false
		}
	default:
		return gateway.InboundEvent{}, false
	}

	b.rememberReplyTarget(chatID, replyTarget{
		messageID:    strings.TrimSpace(msg.MessageID),
		threadRootID: strings.TrimSpace(msg.ThreadRootID),
	})

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: "feishu",
		ChatID:   chatID,
		UserID:   userID,
		UserName: strings.TrimSpace(msg.UserName),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) sourceAllowed(source string) bool {
	switch normalizedMode(b.cfg.ConnectionMode) {
	case ModeWebhook:
		return strings.TrimSpace(source) == SourceWebhook
	case ModeWebsocket:
		return strings.TrimSpace(source) == SourceWebsocket
	default:
		return source == SourceWebhook || source == SourceWebsocket
	}
}

func (b *Bot) allowedUser(userID string) bool {
	if len(b.allowed) == 0 {
		return true
	}
	_, ok := b.allowed[strings.TrimSpace(userID)]
	return ok
}

func (b *Bot) rememberReplyTarget(chatID string, target replyTarget) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.replies[chatID] = target
}

func (b *Bot) replyOptions(chatID string) (SendOptions, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	target, ok := b.replies[chatID]
	if !ok {
		return SendOptions{}, false
	}
	return SendOptions{
		ReplyToMessageID: target.messageID,
		ThreadRootID:     target.threadRootID,
	}, true
}

func looksLikeMarkdown(text string) bool {
	return strings.ContainsAny(text, "#*_`[")
}

func normalizedMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return ModeWebsocket
	}
	return mode
}

func normalizedPolicy(policy string) string {
	policy = strings.TrimSpace(strings.ToLower(policy))
	if policy == "" {
		return "open"
	}
	return policy
}

func stripLeadingMentions(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	for len(fields) > 0 {
		if !strings.HasPrefix(fields[0], "@") {
			break
		}
		fields = fields[1:]
	}
	return strings.TrimSpace(strings.Join(fields, " "))
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
