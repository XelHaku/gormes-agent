package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
)

const startGreeting = "Gormes is online. Send a message to start a turn. Commands: /stop /new"

// ManagerConfig drives the shared gateway manager.
type ManagerConfig struct {
	AllowedChats   map[string]string
	AllowDiscovery map[string]bool
	CoalesceMs     int
	SessionMap     session.Map
}

type kernelSubmitter interface {
	Submit(ev kernel.PlatformEvent) error
	ResetSession() error
	Render() <-chan kernel.RenderFrame
}

// Manager owns cross-channel gateway mechanics for one binary instance.
type Manager struct {
	cfg    ManagerConfig
	kernel kernelSubmitter
	log    *slog.Logger

	mu       sync.Mutex
	channels map[string]Channel

	turnMu       sync.Mutex
	turnPlatform string
	turnChatID   string
	turnMsgID    string

	renderChan <-chan kernel.RenderFrame
}

// ErrDuplicateChannel is returned when two registered channels share a name.
var ErrDuplicateChannel = errors.New("gateway: duplicate channel name")

// ErrEmptyChannelName is returned when a channel reports an empty Name.
var ErrEmptyChannelName = errors.New("gateway: channel Name() must be non-empty")

// NewManager constructs a manager backed by a concrete kernel.
func NewManager(cfg ManagerConfig, k *kernel.Kernel, log *slog.Logger) *Manager {
	return newManagerInternal(cfg, k, log)
}

// NewManagerWithSubmitter lets tests inject a fake kernel-compatible object.
func NewManagerWithSubmitter(cfg ManagerConfig, k kernelSubmitter, log *slog.Logger) *Manager {
	return newManagerInternal(cfg, k, log)
}

func newManagerInternal(cfg ManagerConfig, k kernelSubmitter, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	if cfg.CoalesceMs <= 0 {
		cfg.CoalesceMs = 1000
	}
	if cfg.AllowedChats == nil {
		cfg.AllowedChats = map[string]string{}
	}
	if cfg.AllowDiscovery == nil {
		cfg.AllowDiscovery = map[string]bool{}
	}
	return &Manager{
		cfg:      cfg,
		kernel:   k,
		log:      log,
		channels: map[string]Channel{},
	}
}

// Register adds a channel to the manager. It must be called before Run.
func (m *Manager) Register(ch Channel) error {
	name := ch.Name()
	if name == "" {
		return ErrEmptyChannelName
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.channels[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateChannel, name)
	}
	m.channels[name] = ch
	return nil
}

// ChannelCount reports how many channels are currently registered.
func (m *Manager) ChannelCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.channels)
}

func (m *Manager) setRenderChan(c <-chan kernel.RenderFrame) {
	m.renderChan = c
}

func (m *Manager) Run(ctx context.Context) error {
	m.mu.Lock()
	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	m.mu.Unlock()

	inbox := make(chan InboundEvent, len(channels)*4)

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(c Channel) {
			defer wg.Done()
			if err := c.Run(ctx, inbox); err != nil && !errors.Is(err, context.Canceled) {
				m.log.Warn("channel exited with error", "channel", c.Name(), "err", err)
			}
		}(ch)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.runOutbound(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case ev := <-inbox:
			m.handleInbound(ctx, ev)
		}
	}
}

func (m *Manager) runOutbound(ctx context.Context) {
	frames := m.renderChan
	if frames == nil && m.kernel != nil {
		frames = m.kernel.Render()
	}
	if frames == nil {
		<-ctx.Done()
		return
	}

	var (
		co       *coalescer
		coCancel context.CancelFunc
	)

	for {
		select {
		case <-ctx.Done():
			if coCancel != nil {
				coCancel()
			}
			return
		case f, ok := <-frames:
			if !ok {
				if coCancel != nil {
					coCancel()
				}
				return
			}
			m.persistSession(ctx, f)
			m.dispatchFrame(ctx, f, &co, &coCancel)
		}
	}
}

func (m *Manager) handleInbound(ctx context.Context, ev InboundEvent) {
	if !m.allowed(ev) {
		if m.cfg.AllowDiscovery[ev.Platform] {
			m.log.Info("first-run discovery: unknown chat", "platform", ev.Platform, "chat_id", ev.ChatID)
		} else {
			m.log.Warn("unauthorised chat blocked", "platform", ev.Platform, "chat_id", ev.ChatID)
		}
		return
	}

	ch := m.lookupChannel(ev.Platform)
	if ch == nil {
		m.log.Warn("inbound for unknown channel", "platform", ev.Platform)
		return
	}

	switch ev.Kind {
	case EventStart:
		if _, err := ch.Send(ctx, ev.ChatID, startGreeting); err != nil {
			m.log.Warn("send greeting", "platform", ev.Platform, "chat_id", ev.ChatID, "err", err)
		}
	case EventCancel:
		if m.kernel != nil {
			_ = m.kernel.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventCancel})
		}
	case EventReset:
		if m.kernel == nil {
			return
		}
		if err := m.kernel.ResetSession(); err != nil {
			if errors.Is(err, kernel.ErrResetDuringTurn) {
				_, _ = ch.Send(ctx, ev.ChatID, "Cannot reset during active turn — send /stop first.")
			} else {
				_, _ = ch.Send(ctx, ev.ChatID, "Session reset failed: "+err.Error())
			}
			return
		}
		if m.cfg.SessionMap != nil {
			if err := m.cfg.SessionMap.Put(ctx, ev.ChatKey(), ""); err != nil {
				m.log.Warn("clear session mapping", "key", ev.ChatKey(), "err", err)
			}
		}
		_, _ = ch.Send(ctx, ev.ChatID, "Session reset. Next message starts fresh.")
	case EventSubmit:
		if m.kernel == nil {
			return
		}
		sessionID := ev.ChatKey()
		if m.cfg.SessionMap != nil {
			if stored, err := m.cfg.SessionMap.Get(ctx, ev.ChatKey()); err != nil {
				m.log.Warn("load session mapping", "key", ev.ChatKey(), "err", err)
			} else if stored != "" {
				sessionID = stored
			}
		}
		if err := m.kernel.Submit(kernel.PlatformEvent{
			Kind:      kernel.PlatformEventSubmit,
			Text:      ev.Text,
			SessionID: sessionID,
		}); err != nil {
			_, _ = ch.Send(ctx, ev.ChatID, "Busy — try again in a second.")
			return
		}
		m.pinTurn(ev.Platform, ev.ChatID, ev.MsgID)
	case EventUnknown:
		_, _ = ch.Send(ctx, ev.ChatID, "unknown command")
	}
}

func (m *Manager) dispatchFrame(ctx context.Context, f kernel.RenderFrame, co **coalescer, coCancel *context.CancelFunc) {
	m.turnMu.Lock()
	platform := m.turnPlatform
	chatID := m.turnChatID
	m.turnMu.Unlock()

	if platform == "" || chatID == "" {
		return
	}

	ch := m.lookupChannel(platform)
	if ch == nil {
		return
	}
	pe, ok := ch.(placeholderEditor)
	if !ok {
		m.sendFinalNoStream(ctx, ch, f, chatID)
		if f.Phase == kernel.PhaseIdle || f.Phase == kernel.PhaseFailed || f.Phase == kernel.PhaseCancelling {
			m.clearTurn()
		}
		return
	}

	switch f.Phase {
	case kernel.PhaseIdle:
		if *co != nil {
			(*co).flushImmediate(ctx, m.formatFinal(platform, f))
			(*coCancel)()
			*co = nil
			*coCancel = nil
		} else {
			_, _ = ch.Send(ctx, chatID, m.formatFinal(platform, f))
		}
		m.clearTurn()
	case kernel.PhaseFailed, kernel.PhaseCancelling:
		text := m.formatError(platform, f)
		if *co != nil {
			(*co).flushImmediate(ctx, text)
			(*coCancel)()
			*co = nil
			*coCancel = nil
		} else {
			_, _ = ch.Send(ctx, chatID, text)
		}
		m.clearTurn()
	case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseReconnecting, kernel.PhaseFinalizing:
		if *co == nil {
			cCtx, cancel := context.WithCancel(ctx)
			*coCancel = cancel
			nc := newCoalescer(pe, time.Duration(m.cfg.CoalesceMs)*time.Millisecond, chatID)
			*co = nc
			go nc.run(cCtx)
			nc.flushImmediate(ctx, "⏳")
		}
		(*co).setPending(m.formatStream(platform, f))
	}
}

func (m *Manager) sendFinalNoStream(ctx context.Context, ch Channel, f kernel.RenderFrame, chatID string) {
	switch f.Phase {
	case kernel.PhaseIdle:
		_, _ = ch.Send(ctx, chatID, m.formatFinal(ch.Name(), f))
	case kernel.PhaseFailed, kernel.PhaseCancelling:
		_, _ = ch.Send(ctx, chatID, m.formatError(ch.Name(), f))
	}
}

func (m *Manager) allowed(ev InboundEvent) bool {
	want, ok := m.cfg.AllowedChats[ev.Platform]
	if !ok || want == "" {
		return false
	}
	return ev.ChatID == want
}

func (m *Manager) lookupChannel(name string) Channel {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.channels[name]
}

func (m *Manager) pinTurn(platform, chatID, msgID string) {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	m.turnPlatform = platform
	m.turnChatID = chatID
	m.turnMsgID = msgID
}

func (m *Manager) clearTurn() {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	m.turnPlatform = ""
	m.turnChatID = ""
	m.turnMsgID = ""
}

func (m *Manager) formatStream(platform string, f kernel.RenderFrame) string {
	if platform == "telegram" {
		return FormatStreamTelegram(f)
	}
	return FormatStreamPlain(f)
}

func (m *Manager) formatFinal(platform string, f kernel.RenderFrame) string {
	if platform == "telegram" {
		return FormatFinalTelegram(f)
	}
	return FormatFinalPlain(f)
}

func (m *Manager) formatError(platform string, f kernel.RenderFrame) string {
	if platform == "telegram" {
		return FormatErrorTelegram(f)
	}
	return FormatErrorPlain(f)
}

func (m *Manager) persistSession(ctx context.Context, f kernel.RenderFrame) {
	if m.cfg.SessionMap == nil {
		return
	}
	m.turnMu.Lock()
	platform := m.turnPlatform
	chatID := m.turnChatID
	m.turnMu.Unlock()
	if platform == "" || chatID == "" || f.SessionID == "" {
		return
	}
	key := platform + ":" + chatID
	if err := m.cfg.SessionMap.Put(ctx, key, f.SessionID); err != nil {
		m.log.Warn("persist session_id", "key", key, "session_id", f.SessionID, "err", err)
	}
}
