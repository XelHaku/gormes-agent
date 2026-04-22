package bluebubbles

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// Config captures the first-pass BlueBubbles contract surface.
type Config struct {
	Password    string
	HomeChannel string
}

// InboundMessage is the webhook-neutral BlueBubbles event shape.
type InboundMessage struct {
	MessageID             string
	ChatGUID              string
	ChatIdentifier        string
	Sender                string
	SenderName            string
	Text                  string
	AuthToken             string
	IsFromMe              bool
	AssociatedMessageType int
}

// Client is the minimal BlueBubbles surface used by the adapter contract.
type Client interface {
	Events() <-chan InboundMessage
	ResolveChat(ctx context.Context, target string) (string, error)
	SendText(ctx context.Context, chatGUID, text string) (string, error)
	Close() error
}

// Bot adapts BlueBubbles webhooks into the shared gateway channel contract.
type Bot struct {
	cfg    Config
	client Client
	log    *slog.Logger

	mu        sync.Mutex
	guidCache map[string]string
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:       cfg,
		client:    client,
		log:       log,
		guidCache: map[string]string{},
	}
}

func (b *Bot) Name() string { return "bluebubbles" }

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
	text = stripMarkdown(text)
	if text == "" {
		return "", fmt.Errorf("bluebubbles: send requires text")
	}

	target := b.sendTarget(chatID)
	if target == "" {
		return "", fmt.Errorf("bluebubbles: no chat target or home channel configured")
	}

	guid, err := b.resolveGUID(ctx, target)
	if err != nil {
		return "", err
	}
	return b.client.SendText(ctx, guid, text)
}

func (b *Bot) toInboundEvent(msg InboundMessage) (gateway.InboundEvent, bool) {
	if password := trim(b.cfg.Password); password != "" && trim(msg.AuthToken) != password {
		return gateway.InboundEvent{}, false
	}
	if msg.IsFromMe || msg.AssociatedMessageType != 0 {
		return gateway.InboundEvent{}, false
	}

	text := trim(msg.Text)
	sender := trim(msg.Sender)
	if text == "" || sender == "" {
		return gateway.InboundEvent{}, false
	}

	chatGUID := trim(msg.ChatGUID)
	chatIdentifier := trim(msg.ChatIdentifier)
	chatID := firstNonEmpty(chatGUID, chatIdentifier, sender)
	if chatID == "" {
		return gateway.InboundEvent{}, false
	}

	if chatGUID != "" {
		b.rememberGUID(chatGUID, chatGUID)
		if chatIdentifier != "" {
			b.rememberGUID(chatIdentifier, chatGUID)
		}
		b.rememberGUID(sender, chatGUID)
	}

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: "bluebubbles",
		ChatID:   chatID,
		UserID:   sender,
		UserName: trim(msg.SenderName),
		MsgID:    trim(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) cachedGUID(target string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	guid, ok := b.guidCache[trim(target)]
	return guid, ok
}

func (b *Bot) rememberGUID(target, guid string) {
	target = trim(target)
	guid = trim(guid)
	if target == "" || guid == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.guidCache[target] = guid
}

func (b *Bot) sendTarget(chatID string) string {
	target := trim(chatID)
	if target != "" {
		return target
	}
	return trim(b.cfg.HomeChannel)
}

func (b *Bot) resolveGUID(ctx context.Context, target string) (string, error) {
	if guid, ok := b.cachedGUID(target); ok {
		return guid, nil
	}

	guid, err := b.client.ResolveChat(ctx, target)
	if err != nil {
		return "", fmt.Errorf("bluebubbles: resolve chat %q: %w", target, err)
	}
	guid = trim(guid)
	if guid == "" {
		return "", fmt.Errorf("bluebubbles: chat not found for target %q", target)
	}
	b.rememberGUID(target, guid)
	return guid, nil
}

func stripMarkdown(text string) string {
	text = trim(text)
	replacer := strings.NewReplacer(
		"**", "",
		"*", "",
		"__", "",
		"_", "",
		"`", "",
	)
	return trim(replacer.Replace(text))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = trim(value); value != "" {
			return value
		}
	}
	return ""
}

func trim(value string) string {
	return strings.TrimSpace(value)
}
