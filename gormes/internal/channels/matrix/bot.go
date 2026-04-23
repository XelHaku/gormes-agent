package matrix

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/threadtext"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// Config captures the first Matrix transport wiring surface.
type Config struct {
	ReplyMode threadtext.ReplyMode
}

// Event is the Matrix SDK-neutral inbound event shape.
type Event struct {
	RoomID       string
	RoomName     string
	SenderID     string
	SenderName   string
	EventID      string
	Body         string
	ThreadID     string
	ThreadRootID string
	FromSelf     bool
}

// SendOptions carries Matrix-native reply metadata.
type SendOptions struct {
	ThreadID       string
	ReplyToEventID string
}

// Client is the minimal Matrix transport surface used by the shared channel.
type Client interface {
	Events() <-chan Event
	SendMessage(ctx context.Context, roomID, text string, opts SendOptions) (string, error)
	Close() error
}

// Bot adapts Matrix traffic into the shared gateway channel contract.
type Bot struct {
	cfg    Config
	client Client
	log    *slog.Logger

	replyTargets *threadtext.ReplyTracker
}

var _ gateway.Channel = (*Bot)(nil)

func New(cfg Config, client Client, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:          cfg,
		client:       client,
		log:          log,
		replyTargets: threadtext.NewReplyTracker(),
	}
}

func (b *Bot) Name() string { return "matrix" }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	events := b.client.Events()
	for {
		select {
		case <-ctx.Done():
			b.closeClient()
			return nil
		case event, ok := <-events:
			if !ok {
				b.closeClient()
				return nil
			}
			if event.FromSelf {
				continue
			}

			msg := event.toThreadText()
			ev, ok := threadtext.NormalizeInbound("matrix", msg)
			if !ok {
				continue
			}
			b.replyTargets.Record(msg, b.cfg.ReplyMode)

			select {
			case inbox <- ev:
			case <-ctx.Done():
				b.closeClient()
				return nil
			}
		}
	}
}

func (b *Bot) Send(ctx context.Context, chatID, text string) (string, error) {
	chatID = trimSpace(chatID)
	text = trimSpace(text)
	if chatID == "" {
		return "", fmt.Errorf("matrix: send requires room ID")
	}
	if text == "" {
		return "", fmt.Errorf("matrix: send requires text")
	}

	opts := SendOptions{}
	if target, ok := b.replyTargets.Lookup(chatID); ok {
		opts.ThreadID = target.ThreadID
		opts.ReplyToEventID = target.ReplyToMessageID
	}
	return b.client.SendMessage(ctx, chatID, text, opts)
}

func (e Event) toThreadText() threadtext.InboundMessage {
	return threadtext.InboundMessage{
		ChatID:       e.RoomID,
		ChatName:     e.RoomName,
		UserID:       e.SenderID,
		UserName:     e.SenderName,
		MessageID:    e.EventID,
		Text:         e.Body,
		ThreadID:     e.ThreadID,
		ThreadRootID: e.ThreadRootID,
	}
}

func (b *Bot) closeClient() {
	_ = b.client.Close()
}

func trimSpace(value string) string {
	return strings.TrimSpace(value)
}
