package slack

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
	AllowedChannelID string
	ReplyInThread    bool
	CoalesceMs       int
	SessionMap       session.Map
}

type Bot struct {
	cfg    Config
	client Client
	kernel *kernel.Kernel
	log    *slog.Logger

	selfUserID string

	mu         sync.Mutex
	nextTicket uint64
	reserved   *reservedTurn
	current    *turnBinding
}

type reservedTurn struct {
	ticket    uint64
	channelID string
	threadTS  string
}

type turnBinding struct {
	channelID     string
	threadTS      string
	placeholderTS string
	lastSID       string
	lastUpdate    time.Time
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
	selfID, err := b.client.AuthTest(ctx)
	if err != nil {
		return err
	}
	b.selfUserID = selfID

	runCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	ready := make(chan struct{}, 1)
	wg.Add(1)
	go b.runOutbound(runCtx, &wg, ready)

	select {
	case <-ready:
	case <-runCtx.Done():
		cancel()
		wg.Wait()
		if err := runCtx.Err(); err != nil {
			return err
		}
		return nil
	}

	err = b.client.Run(runCtx, func(e Event) {
		b.handleEvent(ctx, e)
	})
	cancel()
	wg.Wait()
	return err
}

func (b *Bot) handleEvent(ctx context.Context, e Event) {
	if err := b.client.Ack(e.RequestID); err != nil {
		b.log.Warn("slack ack failed", "request_id", e.RequestID, "err", err)
		return
	}

	if e.UserID == "" || e.UserID == b.selfUserID {
		return
	}
	if ignoreSubtype(e.SubType) {
		return
	}
	if b.cfg.AllowedChannelID == "" || e.ChannelID != b.cfg.AllowedChannelID {
		return
	}

	threadTS := b.replyThreadTS(e)
	text := strings.TrimSpace(e.Text)
	switch {
	case text == "/start":
		_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS,
			"Gormes is online. Send a message to start a turn. Commands: /stop /new")
	case text == "/stop":
		_ = b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
	case text == "/new":
		if b.hasTurnInFlight() {
			_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS,
				"Cannot reset during active turn - send /stop first.")
			return
		}
		if err := b.kernel.ResetSession(); err != nil {
			if errors.Is(err, kernel.ErrResetDuringTurn) {
				_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS,
					"Cannot reset during active turn - send /stop first.")
			} else {
				_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS,
					"Session reset failed: "+err.Error())
			}
			return
		}
		if err := b.clearSessionState(e.ChannelID); err != nil {
			_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS,
				"Session reset completed, but failed to clear persisted session: "+err.Error())
			return
		}
		_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS, "Session reset. Next message starts fresh.")
	case strings.HasPrefix(text, "/"):
		_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS, "unknown command")
	case text == "":
		return
	default:
		ticket := b.reserveTurn(e.ChannelID, threadTS)
		if ticket == 0 {
			_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS, "Busy - try again in a second.")
			return
		}
		if err := b.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: text}); err != nil {
			b.cancelReservedTurn(ticket)
			_, _ = b.client.PostMessage(ctx, e.ChannelID, threadTS, "Busy - try again in a second.")
		}
	}
}

func (b *Bot) runOutbound(ctx context.Context, wg *sync.WaitGroup, ready chan<- struct{}) {
	defer wg.Done()

	frames := b.kernel.Render()
	select {
	case <-ctx.Done():
		close(ready)
		b.finishTurn()
		return
	case _, ok := <-frames:
		if !ok {
			close(ready)
			b.finishTurn()
			return
		}
	}
	close(ready)

	for {
		select {
		case <-ctx.Done():
			b.finishTurn()
			return
		case f, ok := <-frames:
			if !ok {
				b.finishTurn()
				return
			}

			binding, ok := b.bindTurnForFrame()
			if !ok {
				continue
			}
			b.persistIfChanged(ctx, binding.channelID, f)

			switch f.Phase {
			case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseFinalizing, kernel.PhaseReconnecting:
				if err := b.ensurePlaceholder(ctx, binding); err != nil {
					b.log.Warn("slack placeholder send failed", "channel_id", binding.channelID, "err", err)
					continue
				}
				if !b.allowUpdate(f.Phase) {
					continue
				}
				if err := b.client.UpdateMessage(ctx, binding.channelID, b.placeholderTS(), formatStream(f)); err != nil {
					b.log.Warn("slack placeholder update failed", "channel_id", binding.channelID, "ts", b.placeholderTS(), "err", err)
				}
			case kernel.PhaseIdle:
				terminal := b.releaseCurrentBinding()
				if err := b.deliverBinding(ctx, terminal, formatFinal(f)); err != nil {
					b.log.Warn("slack final delivery failed", "channel_id", terminal.channelID, "err", err)
				}
			case kernel.PhaseFailed, kernel.PhaseCancelling:
				terminal := b.releaseCurrentBinding()
				if err := b.deliverBinding(ctx, terminal, formatError(f)); err != nil {
					b.log.Warn("slack error delivery failed", "channel_id", terminal.channelID, "err", err)
				}
			}
		}
	}
}

func (b *Bot) replyThreadTS(e Event) string {
	if e.ThreadTS != "" {
		return e.ThreadTS
	}
	if !b.cfg.ReplyInThread {
		return ""
	}
	return e.Timestamp
}

func ignoreSubtype(subtype string) bool {
	return subtype != "" && subtype != "file_share"
}

func (b *Bot) hasTurnInFlight() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reserved != nil || b.current != nil
}

func (b *Bot) reserveTurn(channelID, threadTS string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if channelID == "" || b.reserved != nil || b.current != nil {
		return 0
	}
	b.nextTicket++
	b.reserved = &reservedTurn{
		ticket:    b.nextTicket,
		channelID: channelID,
		threadTS:  threadTS,
	}
	return b.nextTicket
}

func (b *Bot) cancelReservedTurn(ticket uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reserved != nil && b.reserved.ticket == ticket {
		b.reserved = nil
	}
}

func (b *Bot) bindTurnForFrame() (turnBinding, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current != nil {
		return *b.current, true
	}
	if b.reserved == nil {
		return turnBinding{}, false
	}
	b.current = &turnBinding{
		channelID: b.reserved.channelID,
		threadTS:  b.reserved.threadTS,
	}
	b.reserved = nil
	return *b.current, true
}

func (b *Bot) persistIfChanged(ctx context.Context, channelID string, f kernel.RenderFrame) {
	if b.cfg.SessionMap == nil || channelID == "" {
		return
	}

	b.mu.Lock()
	lastSID := ""
	if b.current != nil && b.current.channelID == channelID {
		lastSID = b.current.lastSID
	}
	b.mu.Unlock()

	if f.SessionID == "" || f.SessionID == lastSID {
		return
	}
	if err := b.cfg.SessionMap.Put(ctx, SessionKey(channelID), f.SessionID); err != nil {
		b.log.Warn("failed to persist session_id", "key", SessionKey(channelID), "session_id", f.SessionID, "err", err)
		return
	}

	b.mu.Lock()
	if b.current != nil && b.current.channelID == channelID {
		b.current.lastSID = f.SessionID
	}
	b.mu.Unlock()
}

func (b *Bot) ensurePlaceholder(ctx context.Context, binding turnBinding) error {
	if ts := b.placeholderTS(); ts != "" {
		return nil
	}
	ts, err := b.client.PostMessage(ctx, binding.channelID, binding.threadTS, formatPending())
	if err != nil {
		return err
	}
	b.setPlaceholderTS(ts)
	return nil
}

func (b *Bot) deliverCurrent(ctx context.Context, text string) error {
	b.mu.Lock()
	var binding turnBinding
	if b.current != nil {
		binding = *b.current
	}
	b.mu.Unlock()

	return b.deliverBinding(ctx, binding, text)
}

func (b *Bot) deliverBinding(ctx context.Context, binding turnBinding, text string) error {
	if binding.channelID == "" {
		return nil
	}
	if binding.placeholderTS != "" {
		if err := b.client.UpdateMessage(ctx, binding.channelID, binding.placeholderTS, text); err == nil {
			return nil
		}
	}
	_, err := b.client.PostMessage(ctx, binding.channelID, binding.threadTS, text)
	return err
}

func (b *Bot) placeholderTS() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current == nil {
		return ""
	}
	return b.current.placeholderTS
}

func (b *Bot) setPlaceholderTS(ts string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current != nil {
		b.current.placeholderTS = ts
	}
}

func (b *Bot) allowUpdate(phase kernel.Phase) bool {
	if phase != kernel.PhaseStreaming || b.cfg.CoalesceMs <= 0 {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.current != nil {
			b.current.lastUpdate = time.Now()
		}
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current == nil {
		return false
	}
	now := time.Now()
	if !b.current.lastUpdate.IsZero() && now.Sub(b.current.lastUpdate) < time.Duration(b.cfg.CoalesceMs)*time.Millisecond {
		return false
	}
	b.current.lastUpdate = now
	return true
}

func (b *Bot) clearSessionState(channelID string) error {
	b.mu.Lock()
	if b.current != nil && b.current.channelID == channelID {
		b.current.lastSID = ""
	}
	b.mu.Unlock()

	if b.cfg.SessionMap != nil {
		if err := b.cfg.SessionMap.Put(context.Background(), SessionKey(channelID), ""); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bot) finishTurn() {
	b.mu.Lock()
	b.current = nil
	b.mu.Unlock()
}

func (b *Bot) releaseCurrentBinding() turnBinding {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current == nil {
		return turnBinding{}
	}
	binding := *b.current
	b.current = nil
	return binding
}
