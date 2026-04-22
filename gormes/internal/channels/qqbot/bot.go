package qqbot

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

// Config controls the first-pass QQ Bot ingress and delivery policy.
type Config struct {
	DMPolicy       string
	AllowFrom      []string
	GroupPolicy    string
	GroupAllowFrom []string
}

// InboundMessage is the SDK-neutral QQ Bot event shape the adapter consumes.
type InboundMessage struct {
	ChatType  string
	ChatID    string
	UserID    string
	UserName  string
	MessageID string
	Text      string
	Mentioned bool
}

// SendOptions captures QQ passive-reply metadata.
type SendOptions struct {
	ReplyToMessageID string
	Sequence         int
}

// Client is the minimal QQ Bot surface used by the adapter.
type Client interface {
	Events() <-chan InboundMessage
	SendDirect(ctx context.Context, chatID, text string, opts SendOptions) (string, error)
	SendGroup(ctx context.Context, chatID, text string, opts SendOptions) (string, error)
	Close() error
}

// Bot adapts QQ Bot traffic into the shared gateway channel contract.
type Bot struct {
	cfg            Config
	client         Client
	log            *slog.Logger
	allowedDMs     map[string]struct{}
	allowedGroups  map[string]struct{}
	mu             sync.Mutex
	chatTypes      map[string]string
	lastMessageIDs map[string]string
	sequences      map[string]int
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:            cfg,
		client:         client,
		log:            log,
		allowedDMs:     toSet(cfg.AllowFrom),
		allowedGroups:  toSet(cfg.GroupAllowFrom),
		chatTypes:      map[string]string{},
		lastMessageIDs: map[string]string{},
		sequences:      map[string]int{},
	}
}

func (b *Bot) Name() string { return "qqbot" }

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
	opts, chatType, err := b.nextSendOptions(chatID)
	if err != nil {
		return "", err
	}

	switch chatType {
	case ChatTypeDirect:
		return b.client.SendDirect(ctx, chatID, text, opts)
	case ChatTypeGroup:
		return b.client.SendGroup(ctx, chatID, text, opts)
	default:
		return "", fmt.Errorf("qqbot: unsupported chat type %q", chatType)
	}
}

func (b *Bot) toInboundEvent(msg InboundMessage) (gateway.InboundEvent, bool) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return gateway.InboundEvent{}, false
	}
	chatID := strings.TrimSpace(msg.ChatID)
	if chatID == "" {
		return gateway.InboundEvent{}, false
	}

	chatType := strings.TrimSpace(msg.ChatType)
	switch chatType {
	case ChatTypeDirect:
		if !b.allowDirect(strings.TrimSpace(msg.UserID)) {
			return gateway.InboundEvent{}, false
		}
	case ChatTypeGroup:
		if !b.allowGroup(chatID) || !msg.Mentioned {
			return gateway.InboundEvent{}, false
		}
		text = stripLeadingMentions(text)
		if text == "" {
			return gateway.InboundEvent{}, false
		}
	default:
		return gateway.InboundEvent{}, false
	}

	b.recordInbound(chatID, chatType, strings.TrimSpace(msg.MessageID))

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: "qqbot",
		ChatID:   chatID,
		UserID:   strings.TrimSpace(msg.UserID),
		UserName: strings.TrimSpace(msg.UserName),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) allowDirect(userID string) bool {
	switch normalizedPolicy(b.cfg.DMPolicy) {
	case "disabled":
		return false
	case "allowlist":
		_, ok := b.allowedDMs[userID]
		return ok
	default:
		return true
	}
}

func (b *Bot) allowGroup(chatID string) bool {
	switch normalizedPolicy(b.cfg.GroupPolicy) {
	case "disabled":
		return false
	case "allowlist":
		_, ok := b.allowedGroups[chatID]
		return ok
	default:
		return true
	}
}

func (b *Bot) recordInbound(chatID, chatType, msgID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chatTypes[chatID] = chatType
	if msgID != "" {
		b.lastMessageIDs[chatID] = msgID
	}
}

func (b *Bot) nextSendOptions(chatID string) (SendOptions, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chatType, ok := b.chatTypes[chatID]
	if !ok {
		return SendOptions{}, "", fmt.Errorf("qqbot: no reply metadata for chat %q", chatID)
	}
	b.sequences[chatID]++
	return SendOptions{
		ReplyToMessageID: b.lastMessageIDs[chatID],
		Sequence:         b.sequences[chatID],
	}, chatType, nil
}

func normalizedPolicy(policy string) string {
	if strings.TrimSpace(policy) == "" {
		return "open"
	}
	return strings.ToLower(strings.TrimSpace(policy))
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out[item] = struct{}{}
	}
	return out
}

func stripLeadingMentions(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	for len(fields) > 0 {
		token := fields[0]
		if !strings.HasPrefix(token, "@") {
			break
		}
		fields = fields[1:]
	}
	return strings.TrimSpace(strings.Join(fields, " "))
}
