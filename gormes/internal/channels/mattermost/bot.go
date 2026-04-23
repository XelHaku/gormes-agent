package mattermost

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/channels/threadtext"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// Config captures the first Mattermost transport wiring surface.
type Config struct {
	ReplyMode threadtext.ReplyMode
}

// PostEvent is the Mattermost API-neutral inbound post shape.
type PostEvent struct {
	ChannelID   string
	ChannelName string
	UserID      string
	UserName    string
	PostID      string
	Message     string
	RootID      string
	FromSelf    bool
}

// SendOptions carries Mattermost-native post metadata.
type SendOptions struct {
	RootID string
}

// Client is the minimal Mattermost transport surface used by the shared channel.
type Client interface {
	Events() <-chan PostEvent
	CreatePost(ctx context.Context, channelID, message string, opts SendOptions) (string, error)
	Close() error
}

// Bot adapts Mattermost traffic into the shared gateway channel contract.
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

func (b *Bot) Name() string { return "mattermost" }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	events := b.client.Events()
	for {
		select {
		case <-ctx.Done():
			b.closeClient()
			return nil
		case post, ok := <-events:
			if !ok {
				b.closeClient()
				return nil
			}
			if post.FromSelf {
				continue
			}

			msg := post.toThreadText()
			ev, ok := threadtext.NormalizeInbound("mattermost", msg)
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
		return "", fmt.Errorf("mattermost: send requires channel ID")
	}
	if text == "" {
		return "", fmt.Errorf("mattermost: send requires text")
	}

	opts := SendOptions{}
	if target, ok := b.replyTargets.Lookup(chatID); ok {
		opts.RootID = target.ThreadID
	}
	return b.client.CreatePost(ctx, chatID, text, opts)
}

func (e PostEvent) toThreadText() threadtext.InboundMessage {
	return threadtext.InboundMessage{
		ChatID:       e.ChannelID,
		ChatName:     e.ChannelName,
		UserID:       e.UserID,
		UserName:     e.UserName,
		MessageID:    e.PostID,
		Text:         e.Message,
		ThreadRootID: e.RootID,
	}
}

func (b *Bot) closeClient() {
	_ = b.client.Close()
}

func trimSpace(value string) string {
	return strings.TrimSpace(value)
}
