package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// Mode captures the bridge-level WhatsApp operating mode.
type Mode string

const (
	ModeBot      Mode = "bot"
	ModeSelfChat Mode = "self-chat"
)

// SendOptions carries WhatsApp reply metadata for bridge sends.
type SendOptions struct {
	ReplyToMessageID string
}

// BridgeEvent represents bridge-side lifecycle or inbound-message events.
type BridgeEvent struct {
	Message     *InboundMessage
	PairingCode string
}

// Bridge is the bridge-first WhatsApp transport seam selected for Phase 2.B.4.
type Bridge interface {
	Start(ctx context.Context, mode Mode) (<-chan BridgeEvent, error)
	Send(ctx context.Context, chatID, text string, opts SendOptions) (string, error)
	Close() error
}

// Config configures the transport-neutral WhatsApp bridge adapter.
type Config struct {
	Mode              Mode
	ReconnectSchedule []time.Duration
	Wait              func(context.Context, time.Duration) error
}

// Bot adapts a bridge-first WhatsApp runtime into the shared gateway contract.
type Bot struct {
	bridge Bridge
	cfg    Config
	log    *slog.Logger

	mu           sync.RWMutex
	replyTargets map[string]replyTarget
}

type replyTarget struct {
	transportChatID string
	options         SendOptions
}

var _ gateway.Channel = (*Bot)(nil)

func New(bridge Bridge, cfg Config, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Wait == nil {
		cfg.Wait = waitWithContext
	}
	if len(cfg.ReconnectSchedule) == 0 {
		cfg.ReconnectSchedule = []time.Duration{
			1 * time.Second,
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
			16 * time.Second,
		}
	}

	return &Bot{
		bridge:       bridge,
		cfg:          cfg,
		log:          log,
		replyTargets: map[string]replyTarget{},
	}
}

func (b *Bot) Name() string { return platformName }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	defer b.closeBridge()

	attempt := 0
	for {
		if ctx.Err() != nil {
			return nil
		}

		events, err := b.bridge.Start(ctx, normalizeMode(b.cfg.Mode))
		if err != nil {
			if err := b.waitReconnect(ctx, attempt); err != nil {
				return nil
			}
			attempt++
			continue
		}

		attempt = 0
		disconnected, err := b.consume(ctx, events, inbox)
		if err != nil {
			return err
		}
		if !disconnected {
			return nil
		}

		if err := b.waitReconnect(ctx, attempt); err != nil {
			return nil
		}
		attempt++
	}
}

func (b *Bot) Send(ctx context.Context, chatID, text string) (string, error) {
	target, ok := b.lookupReplyTarget(chatID)
	if !ok {
		return "", fmt.Errorf("whatsapp: no reply target for chat %q", chatID)
	}
	return b.bridge.Send(ctx, target.transportChatID, text, target.options)
}

func (b *Bot) consume(ctx context.Context, events <-chan BridgeEvent, inbox chan<- gateway.InboundEvent) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return false, nil
		case evt, ok := <-events:
			if !ok {
				return true, nil
			}
			if evt.Message == nil {
				continue
			}

			normalized, ok := NormalizeInbound(*evt.Message)
			if !ok {
				continue
			}
			b.recordReplyTarget(*evt.Message, normalized)

			select {
			case inbox <- normalized:
			case <-ctx.Done():
				return false, nil
			}
		}
	}
}

func (b *Bot) recordReplyTarget(msg InboundMessage, normalized gateway.InboundEvent) {
	target, ok := replyTargetForMessage(msg, normalized)
	if !ok {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.replyTargets[normalized.ChatID] = target
}

func (b *Bot) lookupReplyTarget(chatID string) (replyTarget, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	target, ok := b.replyTargets[chatID]
	return target, ok
}

func (b *Bot) waitReconnect(ctx context.Context, attempt int) error {
	delay := b.reconnectDelay(attempt)
	if delay <= 0 {
		return nil
	}
	b.log.Info("whatsapp bridge reconnect", "attempt", attempt+1, "delay", delay.String())
	return b.cfg.Wait(ctx, delay)
}

func (b *Bot) reconnectDelay(attempt int) time.Duration {
	schedule := b.cfg.ReconnectSchedule
	if len(schedule) == 0 {
		return 0
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(schedule) {
		return schedule[len(schedule)-1]
	}
	return schedule[attempt]
}

func (b *Bot) closeBridge() {
	if b.bridge == nil {
		return
	}
	_ = b.bridge.Close()
}

func replyTargetForMessage(msg InboundMessage, normalized gateway.InboundEvent) (replyTarget, bool) {
	transportChatID := strings.TrimSpace(msg.ChatID)
	if transportChatID == "" {
		transportChatID = strings.TrimSpace(msg.UserID)
	}
	if transportChatID == "" {
		return replyTarget{}, false
	}
	return replyTarget{
		transportChatID: transportChatID,
		options: SendOptions{
			ReplyToMessageID: normalized.MsgID,
		},
	}, true
}

func normalizeMode(mode Mode) Mode {
	switch strings.TrimSpace(strings.ToLower(string(mode))) {
	case string(ModeSelfChat):
		return ModeSelfChat
	default:
		return ModeBot
	}
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
