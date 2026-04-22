package discord

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

type Config struct {
	AllowedGuildID   string
	AllowedChannelID string
	MentionRequired  bool
	CoalesceMs       int
	SessionMap       session.Map
}

type Bot struct {
	cfg    Config
	client Client
	kernel *kernel.Kernel
	log    *slog.Logger

	mu         sync.Mutex
	nextTicket uint64
	reserved   *reservedTurn
	current    *turnBinding
}

type reservedTurn struct {
	ticket    uint64
	channelID string
}

type turnBinding struct {
	channelID string
	lastSID   string
}

func New(cfg Config, client Client, k *kernel.Kernel, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	if cfg.CoalesceMs <= 0 {
		cfg.CoalesceMs = 1000
	}
	return &Bot{cfg: cfg, client: client, kernel: k, log: log}
}

func (b *Bot) Run(ctx context.Context) error {
	b.client.SetMessageHandler(func(msg InboundMessage) {
		b.handleMessage(ctx, msg)
	})

	if err := b.client.Open(); err != nil {
		return err
	}
	defer b.client.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go b.runOutbound(ctx, &wg)

	<-ctx.Done()
	wg.Wait()
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, msg InboundMessage) {
	if !b.allowed(msg) {
		return
	}

	text := strings.TrimSpace(stripSelfMention(msg.Content, b.client.SelfID()))
	switch {
	case text == "/start":
		_, _ = b.client.Send(msg.ChannelID, "Gormes is online. Send a message to start a turn. Commands: /stop /new")
	case text == "/stop":
		_ = b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	case text == "/new":
		if err := b.kernel.ResetSession(); err != nil {
			if errors.Is(err, kernel.ErrResetDuringTurn) {
				_, _ = b.client.Send(msg.ChannelID, "Cannot reset during active turn - send /stop first.")
			} else {
				_, _ = b.client.Send(msg.ChannelID, "Session reset failed: "+err.Error())
			}
			return
		}
		b.clearSessionState(msg.ChannelID)
		_, _ = b.client.Send(msg.ChannelID, "Session reset. Next message starts fresh.")
	case strings.HasPrefix(text, "/"):
		_, _ = b.client.Send(msg.ChannelID, "unknown command")
	case text == "":
		return
	default:
		ticket := b.reserveTurn(msg.ChannelID)
		if ticket == 0 {
			_, _ = b.client.Send(msg.ChannelID, "Busy - try again in a second.")
			return
		}
		if err := b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text}); err != nil {
			b.cancelReservedTurn(ticket)
			_, _ = b.client.Send(msg.ChannelID, "Busy - try again in a second.")
			return
		}
	}

	_ = ctx
}

func (b *Bot) allowed(msg InboundMessage) bool {
	if msg.AuthorID == "" || msg.AuthorID == b.client.SelfID() {
		return false
	}

	if msg.IsDM {
		return b.cfg.AllowedChannelID == ""
	}

	if b.cfg.AllowedGuildID != "" && msg.GuildID != b.cfg.AllowedGuildID {
		return false
	}
	if b.cfg.AllowedChannelID != "" && msg.ChannelID != b.cfg.AllowedChannelID {
		return false
	}
	if b.cfg.MentionRequired && !(msg.MentionedBot || hasSelfMention(msg.Content, b.client.SelfID())) {
		return false
	}
	return true
}

func (b *Bot) reserveTurn(channelID string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if channelID == "" || b.reserved != nil || b.current != nil {
		return 0
	}
	b.nextTicket++
	b.reserved = &reservedTurn{ticket: b.nextTicket, channelID: channelID}
	return b.nextTicket
}

func (b *Bot) cancelReservedTurn(ticket uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reserved != nil && b.reserved.ticket == ticket {
		b.reserved = nil
	}
}

func (b *Bot) bindTurnForFrame() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current != nil {
		return b.current.channelID
	}
	if b.reserved == nil {
		return ""
	}
	b.current = &turnBinding{channelID: b.reserved.channelID}
	b.reserved = nil
	return b.current.channelID
}

func (b *Bot) clearSessionState(channelID string) {
	b.mu.Lock()
	if b.current != nil && b.current.channelID == channelID {
		b.current.lastSID = ""
	}
	b.mu.Unlock()

	if b.cfg.SessionMap != nil {
		_ = b.cfg.SessionMap.Put(context.Background(), SessionKey(channelID), "")
	}
}

func (b *Bot) currentTurnChannel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current == nil {
		return ""
	}
	return b.current.channelID
}

func (b *Bot) finishTurn() {
	b.mu.Lock()
	b.current = nil
	b.mu.Unlock()
}

func (b *Bot) runOutbound(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	frames := b.kernel.Render()
	var c *coalescer
	var turnCancel context.CancelFunc

	stopTurn := func() {
		if turnCancel != nil {
			turnCancel()
			turnCancel = nil
		}
		c = nil
		b.finishTurn()
	}

	for {
		select {
		case <-ctx.Done():
			stopTurn()
			return
		case f, ok := <-frames:
			if !ok {
				stopTurn()
				return
			}

			channelID := b.bindTurnForFrame()
			if channelID == "" {
				continue
			}
			b.persistIfChanged(ctx, channelID, f)

			switch f.Phase {
			case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseFinalizing, kernel.PhaseReconnecting:
				if c == nil {
					window := time.Duration(b.cfg.CoalesceMs) * time.Millisecond
					turnCtx, cancel := context.WithCancel(ctx)
					turnCancel = cancel
					c = newCoalescer(b.client, window, channelID)
					c.log = b.log
					c.flushImmediate("⏳")
					activeCoalescer := c
					activeChannelID := channelID

					wg.Add(2)
					go func() {
						defer wg.Done()
						activeCoalescer.run(turnCtx)
					}()
					go func() {
						defer wg.Done()
						runTypingLoop(turnCtx, b.client, activeChannelID)
					}()
				}
				c.submit(formatStream(f))
			case kernel.PhaseIdle:
				if c != nil {
					if err := c.flushImmediate(formatFinal(f)); err != nil {
						b.log.Warn("discord final delivery failed", "channel_id", channelID, "err", err)
					}
				}
				stopTurn()
			case kernel.PhaseFailed, kernel.PhaseCancelling:
				if c != nil {
					if err := c.flushImmediate(formatError(f)); err != nil {
						b.log.Warn("discord terminal error delivery failed", "channel_id", channelID, "err", err)
					}
				} else {
					if _, err := b.client.Send(channelID, formatError(f)); err != nil {
						b.log.Warn("discord terminal send failed", "channel_id", channelID, "err", err)
					}
				}
				stopTurn()
			}
		}
	}
}

func (b *Bot) persistIfChanged(ctx context.Context, channelID string, f kernel.RenderFrame) {
	if b.cfg.SessionMap == nil || channelID == "" {
		return
	}

	b.mu.Lock()
	if b.current == nil || b.current.channelID != channelID || f.SessionID == b.current.lastSID {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	if err := b.cfg.SessionMap.Put(ctx, SessionKey(channelID), f.SessionID); err != nil {
		b.log.Warn("failed to persist discord session", "channel_id", channelID, "session_id", f.SessionID, "err", err)
		return
	}

	b.mu.Lock()
	if b.current != nil && b.current.channelID == channelID {
		b.current.lastSID = f.SessionID
	}
	b.mu.Unlock()
}
