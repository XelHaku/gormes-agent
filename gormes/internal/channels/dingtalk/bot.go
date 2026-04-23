package dingtalk

import (
	"context"
	"log/slog"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// Config controls the narrow first-pass DingTalk adapter contract.
type Config struct {
	AllowedUserIDs []string
}

// InboundMessage is the SDK-neutral DingTalk event shape the adapter consumes.
type InboundMessage struct {
	MessageID        string
	ConversationID   string
	ConversationType string
	SenderStaffID    string
	SenderID         string
	SenderNick       string
	Text             string
	SessionWebhook   string
	Mentioned        bool
}

// Client is the minimal DingTalk surface used by the adapter.
type Client interface {
	Events() <-chan InboundMessage
	SendReply(ctx context.Context, webhook, text string) (string, error)
	Close() error
}

// Bot adapts DingTalk events into the shared gateway channel contract.
type Bot struct {
	cfg     Config
	client  Client
	log     *slog.Logger
	allowed map[string]struct{}

	sessionWebhooks *SessionWebhooks
	replySender     *ReplySender
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedUserIDs))
	for _, id := range cfg.AllowedUserIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		allowed[id] = struct{}{}
	}
	return &Bot{
		cfg:             cfg,
		client:          client,
		log:             log,
		allowed:         allowed,
		sessionWebhooks: NewSessionWebhooks(),
		replySender:     NewReplySender(client, DefaultReplyRetryPolicy()),
	}
}

func (b *Bot) Name() string { return "dingtalk" }

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
	return b.replySender.Send(ctx, b.sessionWebhooks, chatID, text)
}

func (b *Bot) toInboundEvent(msg InboundMessage) (gateway.InboundEvent, bool) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return gateway.InboundEvent{}, false
	}

	senderID := strings.TrimSpace(msg.SenderStaffID)
	if senderID == "" {
		senderID = strings.TrimSpace(msg.SenderID)
	}
	if !b.allowedSender(senderID) {
		return gateway.InboundEvent{}, false
	}

	chatID := strings.TrimSpace(msg.ConversationID)
	if chatID == "" && strings.TrimSpace(msg.ConversationType) == "1" {
		chatID = senderID
	}
	if chatID == "" {
		return gateway.InboundEvent{}, false
	}

	b.sessionWebhooks.Remember(chatID, msg.SessionWebhook)

	if strings.TrimSpace(msg.ConversationType) != "1" {
		if !msg.Mentioned {
			return gateway.InboundEvent{}, false
		}
		text = stripLeadingMentions(text)
		if text == "" {
			return gateway.InboundEvent{}, false
		}
	}

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: "dingtalk",
		ChatID:   chatID,
		UserID:   senderID,
		UserName: strings.TrimSpace(msg.SenderNick),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) allowedSender(senderID string) bool {
	if len(b.allowed) == 0 {
		return true
	}
	_, ok := b.allowed[strings.TrimSpace(senderID)]
	return ok
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
