package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const (
	ackEmoji        = "👀"
	placeholderText = "⏳"
)

type Config struct {
	AllowedChannelID  string
	FirstRunDiscovery bool
}

type Bot struct {
	cfg     Config
	session discordSession
	log     *slog.Logger

	reactionsMu sync.Mutex
	reactions   map[string]bool
}

var (
	_ gateway.Channel            = (*Bot)(nil)
	_ gateway.MessageEditor      = (*Bot)(nil)
	_ gateway.PlaceholderCapable = (*Bot)(nil)
	_ gateway.ReactionCapable    = (*Bot)(nil)
)

func New(cfg Config, session discordSession, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		cfg:       cfg,
		session:   session,
		log:       log,
		reactions: map[string]bool{},
	}
}

func (b *Bot) Name() string { return "discord" }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	b.session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m == nil || m.Message == nil {
			return
		}
		if m.Author == nil || m.Author.Bot {
			return
		}
		ev, ok := b.toInboundEvent(m.Message)
		if !ok {
			return
		}
		select {
		case inbox <- ev:
		case <-ctx.Done():
		}
	})
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("discord: open session: %w", err)
	}
	<-ctx.Done()
	_ = b.session.Close()
	return nil
}

func (b *Bot) toInboundEvent(m *discordgo.Message) (gateway.InboundEvent, bool) {
	text := strings.TrimSpace(m.Content)

	kind := gateway.EventSubmit
	body := text
	switch {
	case text == "/start":
		kind = gateway.EventStart
		body = ""
	case text == "/stop":
		kind = gateway.EventCancel
		body = ""
	case text == "/new":
		kind = gateway.EventReset
		body = ""
	case strings.HasPrefix(text, "/"):
		kind = gateway.EventUnknown
		body = ""
	}

	userID := ""
	if m.Author != nil {
		userID = m.Author.ID
	}
	return gateway.InboundEvent{
		Platform: "discord",
		ChatID:   m.ChannelID,
		UserID:   userID,
		MsgID:    m.ID,
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) Send(_ context.Context, chatID, text string) (string, error) {
	msg, err := b.session.ChannelMessageSend(chatID, text)
	if err != nil {
		return "", fmt.Errorf("discord: send: %w", err)
	}
	return msg.ID, nil
}

func (b *Bot) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	return b.Send(ctx, chatID, placeholderText)
}

func (b *Bot) EditMessage(_ context.Context, chatID, msgID, text string) error {
	if _, err := b.session.ChannelMessageEdit(chatID, msgID, text); err != nil {
		return fmt.Errorf("discord: edit: %w", err)
	}
	return nil
}

func (b *Bot) ReactToMessage(_ context.Context, chatID, msgID string) (func(), error) {
	if err := b.session.MessageReactionAdd(chatID, msgID, ackEmoji); err != nil {
		return nil, fmt.Errorf("discord: reaction add: %w", err)
	}

	key := chatID + ":" + msgID
	return func() {
		b.reactionsMu.Lock()
		if b.reactions[key] {
			b.reactionsMu.Unlock()
			return
		}
		b.reactions[key] = true
		b.reactionsMu.Unlock()
		_ = b.session.MessageReactionRemoveMe(chatID, msgID, ackEmoji)
	}, nil
}
