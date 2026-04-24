package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

var startGreeting = gatewayHelpText()

const shutdownNotice = "Gateway is shutting down — send /stop to cancel the active turn or try again shortly."

// ManagerConfig drives the shared gateway manager.
type ManagerConfig struct {
	AllowedChats   map[string]string
	AllowDiscovery map[string]bool
	CoalesceMs     int
	SessionMap     session.Map
	Hooks          *Hooks
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
	shuttingDown bool

	renderChan <-chan kernel.RenderFrame
}

type hookedPlaceholderEditor struct {
	base     placeholderEditor
	manager  *Manager
	platform string
}

func (h hookedPlaceholderEditor) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	const placeholderText = "⏳"

	h.manager.fireHook(ctx, HookEvent{
		Point:    HookBeforeSend,
		Platform: h.platform,
		ChatID:   chatID,
		Text:     placeholderText,
	})

	msgID, err := h.base.SendPlaceholder(ctx, chatID)
	if err != nil {
		h.manager.fireHook(ctx, HookEvent{
			Point:    HookOnError,
			Platform: h.platform,
			ChatID:   chatID,
			Text:     placeholderText,
			Err:      err,
		})
		return "", err
	}

	h.manager.fireHook(ctx, HookEvent{
		Point:    HookAfterSend,
		Platform: h.platform,
		ChatID:   chatID,
		MsgID:    msgID,
		Text:     placeholderText,
	})
	return msgID, nil
}

func (h hookedPlaceholderEditor) EditMessage(ctx context.Context, chatID, msgID, text string) error {
	return h.base.EditMessage(ctx, chatID, msgID, text)
}

func (h hookedPlaceholderEditor) EditMessageFinal(ctx context.Context, chatID, msgID, text string, finalize bool) error {
	if finalizer, ok := h.base.(FinalizingMessageEditor); ok {
		return finalizer.EditMessageFinal(ctx, chatID, msgID, text, finalize)
	}
	return h.base.EditMessage(ctx, chatID, msgID, text)
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

// Shutdown prevents new work from starting and waits for the currently active
// turn, if any, to drain before returning or timing out.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.turnMu.Lock()
	m.shuttingDown = true
	m.turnMu.Unlock()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !m.hasActiveTurn() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
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
				m.fireHook(ctx, HookEvent{
					Point:    HookOnError,
					Platform: c.Name(),
					Err:      err,
				})
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
	m.fireHook(ctx, HookEvent{
		Point:    HookBeforeReceive,
		Platform: ev.Platform,
		ChatID:   ev.ChatID,
		MsgID:    ev.MsgID,
		Kind:     ev.Kind,
		Text:     ev.Text,
		Inbound:  &ev,
	})
	defer m.fireHook(ctx, HookEvent{
		Point:    HookAfterReceive,
		Platform: ev.Platform,
		ChatID:   ev.ChatID,
		MsgID:    ev.MsgID,
		Kind:     ev.Kind,
		Text:     ev.Text,
		Inbound:  &ev,
	})

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
	if m.isShuttingDown() && ev.Kind != EventCancel {
		_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, shutdownNotice)
		return
	}

	switch ev.Kind {
	case EventStart:
		if _, err := m.sendWithHooks(ctx, ch, ev.ChatID, startGreeting); err != nil {
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
				_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, "Cannot reset during active turn — send /stop first.")
			} else {
				_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, "Session reset failed: "+err.Error())
			}
			return
		}
		if m.cfg.SessionMap != nil {
			if err := m.cfg.SessionMap.Put(ctx, ev.ChatKey(), ""); err != nil {
				m.log.Warn("clear session mapping", "key", ev.ChatKey(), "err", err)
			}
		}
		_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, "Session reset. Next message starts fresh.")
	case EventSubmit:
		if m.kernel == nil {
			return
		}
		sessionID, err := resolveSessionID(ctx, m.cfg.SessionMap, ev.ChatKey())
		if err != nil {
			m.log.Warn("load session mapping", "key", ev.ChatKey(), "err", err)
		}
		sessionContext := BuildSessionContextPrompt(SessionContext{
			Source:             sessionSourceFromInbound(ev),
			SessionKey:         ev.ChatKey(),
			SessionID:          sessionID,
			ConnectedPlatforms: m.connectedPlatforms(),
		})
		m.pinTurn(ev.Platform, ev.ChatID, ev.MsgID)
		if err := m.kernel.Submit(kernel.PlatformEvent{
			Kind:           kernel.PlatformEventSubmit,
			Text:           ev.SubmitText(),
			SessionID:      sessionID,
			SessionContext: sessionContext,
		}); err != nil {
			m.clearTurn()
			_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, "Busy — try again in a second.")
			return
		}
	case EventUnknown:
		_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, "unknown command")
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
			(*co).flushImmediateFinal(ctx, m.formatFinal(platform, f), true)
			(*coCancel)()
			*co = nil
			*coCancel = nil
		} else {
			_, _ = m.sendWithHooks(ctx, ch, chatID, m.formatFinal(platform, f))
		}
		m.clearTurn()
	case kernel.PhaseFailed, kernel.PhaseCancelling:
		text := m.formatError(platform, f)
		if *co != nil {
			(*co).flushImmediateFinal(ctx, text, true)
			(*coCancel)()
			*co = nil
			*coCancel = nil
		} else {
			_, _ = m.sendWithHooks(ctx, ch, chatID, text)
		}
		m.clearTurn()
	case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseReconnecting, kernel.PhaseFinalizing:
		if *co == nil {
			cCtx, cancel := context.WithCancel(ctx)
			*coCancel = cancel
			nc := newCoalescer(hookedPlaceholderEditor{
				base:     pe,
				manager:  m,
				platform: platform,
			}, time.Duration(m.cfg.CoalesceMs)*time.Millisecond, chatID)
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
		_, _ = m.sendWithHooks(ctx, ch, chatID, m.formatFinal(ch.Name(), f))
	case kernel.PhaseFailed, kernel.PhaseCancelling:
		_, _ = m.sendWithHooks(ctx, ch, chatID, m.formatError(ch.Name(), f))
	}
}

func (m *Manager) sendWithHooks(ctx context.Context, ch Channel, chatID, text string) (string, error) {
	if ch == nil {
		return "", nil
	}
	ev := HookEvent{
		Point:    HookBeforeSend,
		Platform: ch.Name(),
		ChatID:   chatID,
		Text:     text,
	}
	m.fireHook(ctx, ev)

	msgID, err := ch.Send(ctx, chatID, text)
	if err != nil {
		m.fireHook(ctx, HookEvent{
			Point:    HookOnError,
			Platform: ch.Name(),
			ChatID:   chatID,
			Text:     text,
			Err:      err,
		})
		return "", err
	}

	m.fireHook(ctx, HookEvent{
		Point:    HookAfterSend,
		Platform: ch.Name(),
		ChatID:   chatID,
		MsgID:    msgID,
		Text:     text,
	})
	return msgID, nil
}

func (m *Manager) fireHook(ctx context.Context, ev HookEvent) {
	if m.cfg.Hooks == nil {
		return
	}
	m.cfg.Hooks.Fire(ctx, ev)
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

func (m *Manager) connectedPlatforms() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

func (m *Manager) hasActiveTurn() bool {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	return m.turnPlatform != "" || m.turnChatID != "" || m.turnMsgID != ""
}

func (m *Manager) isShuttingDown() bool {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	return m.shuttingDown
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
