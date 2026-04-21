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

	mu              sync.Mutex
	activeChannelID string
	lastSID         string
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
		if err := b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text}); err != nil {
			_, _ = b.client.Send(msg.ChannelID, "Busy - try again in a second.")
			return
		}
		b.setActiveChannel(msg.ChannelID)
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

func (b *Bot) setActiveChannel(channelID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.activeChannelID != channelID {
		b.activeChannelID = channelID
		b.lastSID = ""
	}
}

func (b *Bot) clearSessionState(channelID string) {
	b.mu.Lock()
	if b.activeChannelID == channelID {
		b.lastSID = ""
	}
	b.mu.Unlock()

	if b.cfg.SessionMap != nil {
		_ = b.cfg.SessionMap.Put(context.Background(), SessionKey(channelID), "")
	}
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

			b.persistIfChanged(ctx, f)
			channelID := b.currentChannelID()
			if channelID == "" {
				continue
			}

			switch f.Phase {
			case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseFinalizing, kernel.PhaseReconnecting:
				if c == nil {
					window := time.Duration(b.cfg.CoalesceMs) * time.Millisecond
					turnCtx, cancel := context.WithCancel(ctx)
					turnCancel = cancel
					c = newCoalescer(b.client, window, channelID)
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
					c.flushImmediate(formatFinal(f))
				}
				stopTurn()
			case kernel.PhaseFailed, kernel.PhaseCancelling:
				if c != nil {
					c.flushImmediate(formatError(f))
				} else {
					_, _ = b.client.Send(channelID, formatError(f))
				}
				stopTurn()
			}
		}
	}
}

func (b *Bot) persistIfChanged(ctx context.Context, f kernel.RenderFrame) {
	if b.cfg.SessionMap == nil {
		return
	}

	channelID := b.currentChannelID()
	if channelID == "" {
		return
	}

	b.mu.Lock()
	if f.SessionID == b.lastSID {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	if err := b.cfg.SessionMap.Put(ctx, SessionKey(channelID), f.SessionID); err != nil {
		b.log.Warn("failed to persist discord session", "channel_id", channelID, "session_id", f.SessionID, "err", err)
		return
	}

	b.mu.Lock()
	b.lastSID = f.SessionID
	b.mu.Unlock()
}

func (b *Bot) currentChannelID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activeChannelID
}
