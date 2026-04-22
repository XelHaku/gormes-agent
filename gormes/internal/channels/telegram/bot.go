package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// Config drives the Telegram channel. AllowedChatID and discovery are still
// kept here so SDK-specific entrypoints can reuse the typed values.
type Config struct {
	AllowedChatID     int64
	FirstRunDiscovery bool
}

// Bot implements gateway.Channel plus the editing capabilities the shared
// manager uses for streamed responses.
type Bot struct {
	cfg    Config
	client telegramClient
	log    *slog.Logger
}

var _ gateway.Channel = (*Bot)(nil)
var _ gateway.MessageEditor = (*Bot)(nil)
var _ gateway.PlaceholderCapable = (*Bot)(nil)

func New(cfg Config, client telegramClient, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{cfg: cfg, client: client, log: log}
}

func (b *Bot) Name() string { return "telegram" }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	ucfg := tgbotapi.NewUpdate(0)
	ucfg.Timeout = 30
	updates := b.client.GetUpdatesChan(ucfg)

	for {
		select {
		case <-ctx.Done():
			b.client.StopReceivingUpdates()
			return nil
		case u, ok := <-updates:
			if !ok {
				return nil
			}
			if ev, ok := b.toInboundEvent(u); ok {
				select {
				case inbox <- ev:
				case <-ctx.Done():
					return nil
				}
			}
		}
	}
}

func (b *Bot) toInboundEvent(u tgbotapi.Update) (gateway.InboundEvent, bool) {
	if u.Message == nil {
		return gateway.InboundEvent{}, false
	}

	chatID := u.Message.Chat.ID
	text := strings.TrimSpace(u.Message.Text)

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

	var userID string
	if u.Message.From != nil {
		userID = strconv.FormatInt(u.Message.From.ID, 10)
	}

	return gateway.InboundEvent{
		Platform: "telegram",
		ChatID:   strconv.FormatInt(chatID, 10),
		UserID:   userID,
		MsgID:    strconv.Itoa(u.Message.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func (b *Bot) Send(ctx context.Context, chatID, text string) (string, error) {
	_ = ctx
	id, err := parseChatID(chatID)
	if err != nil {
		return "", err
	}
	msg, err := b.client.Send(tgbotapi.NewMessage(id, text))
	if err != nil {
		return "", err
	}
	return strconv.Itoa(msg.MessageID), nil
}

func (b *Bot) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	return b.Send(ctx, chatID, "⏳")
}

func (b *Bot) EditMessage(ctx context.Context, chatID, msgID, text string) error {
	_ = ctx
	cid, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	mid, err := strconv.Atoi(msgID)
	if err != nil {
		return fmt.Errorf("telegram: invalid msgID %q: %w", msgID, err)
	}
	_, err = b.client.Send(tgbotapi.NewEditMessageText(cid, mid, text))
	return err
}

// SendToChat is retained for the cron delivery sink, which addresses Telegram
// using the native int64 chat identifier.
func (b *Bot) SendToChat(ctx context.Context, chatID int64, text string) error {
	_ = ctx
	_, err := b.client.Send(tgbotapi.NewMessage(chatID, text))
	return err
}

func parseChatID(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telegram: invalid chat ID %q: %w", s, err)
	}
	return v, nil
}
