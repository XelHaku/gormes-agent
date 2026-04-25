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
const followUpQueueFullNotice = "Busy — follow-up queue is full; try again after the current turn."
const followUpQueueCap = kernel.PlatformEventMailboxCap

type DrainTimeoutReason string

const (
	DrainReasonRestartTimeout  DrainTimeoutReason = session.ResumeReasonRestartTimeout
	DrainReasonShutdownTimeout DrainTimeoutReason = session.ResumeReasonShutdownTimeout
)

type sessionMetadataReader interface {
	GetMetadata(context.Context, string) (session.Metadata, bool, error)
}

type sessionMetadataWriter interface {
	PutMetadata(context.Context, session.Metadata) error
}

type sessionResumeClearer interface {
	ClearResumePending(context.Context, string) (bool, error)
}

type activeTurnSnapshot struct {
	Platform   string
	ChatID     string
	MsgID      string
	SessionKey string
	SessionID  string
	Source     SessionSource
	Cancelled  bool
}

// ManagerConfig drives the shared gateway manager.
type ManagerConfig struct {
	AllowedChats   map[string]string
	AllowDiscovery map[string]bool
	CoalesceMs     int
	SessionMap     session.Map
	Hooks          *Hooks
	RuntimeStatus  RuntimeStatusWriter
	Now            func() time.Time
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

	turnMu         sync.Mutex
	turnPlatform   string
	turnChatID     string
	turnMsgID      string
	turnSessionKey string
	turnSessionID  string
	turnSource     SessionSource
	turnCancelled  bool
	shuttingDown   bool
	followUps      []InboundEvent

	renderChan <-chan kernel.RenderFrame
}

type channelRunFailure struct {
	channel Channel
	err     error
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
		h.manager.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
			Platform:      h.platform,
			PlatformState: PlatformStateFailed,
			ErrorMessage:  err.Error(),
		})
		h.manager.fireHook(ctx, HookEvent{
			Point:    HookOnError,
			Platform: h.platform,
			ChatID:   chatID,
			Text:     placeholderText,
			Err:      err,
		})
		return "", err
	}

	h.manager.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		Platform:      h.platform,
		PlatformState: PlatformStateRunning,
	})
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

func (m *Manager) now() time.Time {
	if m.cfg.Now != nil {
		return m.cfg.Now().UTC()
	}
	return time.Now().UTC()
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
	return m.ShutdownWithDrainReason(ctx, DrainReasonShutdownTimeout)
}

func (m *Manager) ShutdownWithDrainReason(ctx context.Context, reason DrainTimeoutReason) error {
	if reason == "" {
		reason = DrainReasonShutdownTimeout
	}
	m.turnMu.Lock()
	m.shuttingDown = true
	m.turnMu.Unlock()

	activeAgents := m.activeAgentCount()
	m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		GatewayState: GatewayStateDraining,
		ActiveAgents: &activeAgents,
	})

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !m.hasActiveTurn() {
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				m.markDrainTimeoutResumePending(context.Background(), reason)
			}
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

	m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{GatewayState: GatewayStateStarting})

	inbox := make(chan InboundEvent, len(channels)*4)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	failures := make(chan channelRunFailure, len(channels))

	var wg sync.WaitGroup
	for _, ch := range channels {
		m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
			Platform:      ch.Name(),
			PlatformState: PlatformStateStarting,
		})
		wg.Add(1)
		go func(c Channel) {
			defer wg.Done()
			m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
				Platform:      c.Name(),
				PlatformState: PlatformStateRunning,
			})
			if err := c.Run(runCtx, inbox); err != nil && !errors.Is(err, context.Canceled) {
				m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
					Platform:      c.Name(),
					PlatformState: PlatformStateFailed,
					ErrorMessage:  err.Error(),
				})
				m.fireHook(runCtx, HookEvent{
					Point:    HookOnError,
					Platform: c.Name(),
					Err:      err,
				})
				m.log.Warn("channel exited with error", "channel", c.Name(), "err", err)
				failures <- channelRunFailure{channel: c, err: err}
				return
			}
			m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
				Platform:      c.Name(),
				PlatformState: PlatformStateStopped,
			})
		}(ch)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.runOutbound(runCtx)
	}()

	m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{GatewayState: GatewayStateRunning})

	activeChannels := len(channels)
	var firstFailure error
	for {
		select {
		case <-ctx.Done():
			cancel()
			wg.Wait()
			zero := 0
			m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
				GatewayState: GatewayStateStopped,
				ActiveAgents: &zero,
			})
			return nil
		case failure := <-failures:
			m.safeChannelDisconnect(ctx, failure.channel)
			if firstFailure == nil {
				firstFailure = failure.err
			}
			activeChannels--
			if activeChannels <= 0 {
				cancel()
				wg.Wait()
				reason := ""
				if firstFailure != nil {
					reason = firstFailure.Error()
				}
				zero := 0
				m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
					GatewayState: GatewayStateStartupFailed,
					ExitReason:   reason,
					ActiveAgents: &zero,
				})
				return firstFailure
			}
		case ev := <-inbox:
			m.handleInbound(runCtx, ev)
		}
	}
}

func (m *Manager) safeChannelDisconnect(ctx context.Context, ch Channel) {
	if disconnecter, ok := ch.(DisconnectCapable); ok {
		if err := disconnecter.Disconnect(ctx); err != nil {
			m.log.Debug("defensive channel disconnect after failed startup raised", "channel", ch.Name(), "err", err)
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
		m.markTurnCancelled()
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
		queued, full := m.queueFollowUpIfActive(ev)
		if queued {
			return
		}
		if full {
			_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, followUpQueueFullNotice)
			return
		}
		m.pinTurn(ev.Platform, ev.ChatID, ev.MsgID)
		m.submitPinned(ctx, ch, ev)
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
		if m.sendNoEdit(ctx, ch, f, chatID) {
			m.drainNextFollowUp(ctx)
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
		m.drainNextFollowUp(ctx)
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
		m.drainNextFollowUp(ctx)
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

func (m *Manager) sendNoEdit(ctx context.Context, ch Channel, f kernel.RenderFrame, chatID string) bool {
	switch f.Phase {
	case kernel.PhaseIdle:
		_, _ = m.sendWithHooks(ctx, ch, chatID, m.formatFinal(ch.Name(), f))
		return true
	case kernel.PhaseFailed, kernel.PhaseCancelling:
		_, _ = m.sendWithHooks(ctx, ch, chatID, m.formatError(ch.Name(), f))
		return true
	case kernel.PhaseConnecting, kernel.PhaseStreaming, kernel.PhaseReconnecting, kernel.PhaseFinalizing:
		if text := m.formatStream(ch.Name(), f); text != "" {
			_, _ = m.sendWithHooks(ctx, ch, chatID, text)
		}
	}
	return false
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
		m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
			Platform:      ch.Name(),
			PlatformState: PlatformStateFailed,
			ErrorMessage:  err.Error(),
		})
		m.fireHook(ctx, HookEvent{
			Point:    HookOnError,
			Platform: ch.Name(),
			ChatID:   chatID,
			Text:     text,
			Err:      err,
		})
		return "", err
	}

	m.writeRuntimeStatus(context.Background(), RuntimeStatusUpdate{
		Platform:      ch.Name(),
		PlatformState: PlatformStateRunning,
	})
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

func (m *Manager) writeRuntimeStatus(ctx context.Context, update RuntimeStatusUpdate) {
	if m.cfg.RuntimeStatus == nil {
		return
	}
	if err := m.cfg.RuntimeStatus.UpdateRuntimeStatus(ctx, update); err != nil && !errors.Is(err, context.Canceled) {
		m.log.Debug("write gateway runtime status", "err", err)
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
	m.turnSessionKey = ""
	m.turnSessionID = ""
	m.turnSource = SessionSource{}
	m.turnCancelled = false
}

func (m *Manager) clearTurn() {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	m.turnPlatform = ""
	m.turnChatID = ""
	m.turnMsgID = ""
	m.turnSessionKey = ""
	m.turnSessionID = ""
	m.turnSource = SessionSource{}
	m.turnCancelled = false
}

func (m *Manager) hasActiveTurn() bool {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	return m.hasActiveTurnLocked()
}

func (m *Manager) isShuttingDown() bool {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	return m.shuttingDown
}

func (m *Manager) activeAgentCount() int {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	if m.turnPlatform == "" {
		return 0
	}
	return 1
}

func (m *Manager) setPinnedTurnSession(sessionKey, sessionID string, source SessionSource) {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	m.turnSessionKey = sessionKey
	m.turnSessionID = sessionID
	m.turnSource = source
}

func (m *Manager) markTurnCancelled() {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	m.turnCancelled = true
}

func (m *Manager) activeTurnSnapshot() (activeTurnSnapshot, bool) {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	if m.turnPlatform == "" || m.turnChatID == "" {
		return activeTurnSnapshot{}, false
	}
	return activeTurnSnapshot{
		Platform:   m.turnPlatform,
		ChatID:     m.turnChatID,
		MsgID:      m.turnMsgID,
		SessionKey: m.turnSessionKey,
		SessionID:  m.turnSessionID,
		Source:     m.turnSource,
		Cancelled:  m.turnCancelled,
	}, true
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

func (m *Manager) hasActiveTurnLocked() bool {
	return m.turnPlatform != "" || m.turnChatID != "" || m.turnMsgID != "" || len(m.followUps) > 0
}

func (m *Manager) queueFollowUpIfActive(ev InboundEvent) (queued bool, full bool) {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	if !m.hasActiveTurnLocked() {
		return false, false
	}
	if len(m.followUps) >= followUpQueueCap {
		return false, true
	}
	m.followUps = append(m.followUps, ev)
	return true, false
}

func (m *Manager) drainNextFollowUp(ctx context.Context) {
	for {
		next, ok := m.popNextFollowUpAsActive()
		if !ok {
			return
		}
		ch := m.lookupChannel(next.Platform)
		if ch == nil {
			m.log.Warn("queued follow-up for unknown channel", "platform", next.Platform)
			m.clearTurn()
			continue
		}
		if m.submitPinned(ctx, ch, next) {
			return
		}
	}
}

func (m *Manager) popNextFollowUpAsActive() (InboundEvent, bool) {
	m.turnMu.Lock()
	defer m.turnMu.Unlock()
	if len(m.followUps) == 0 {
		m.turnPlatform = ""
		m.turnChatID = ""
		m.turnMsgID = ""
		return InboundEvent{}, false
	}
	next := m.followUps[0]
	copy(m.followUps, m.followUps[1:])
	m.followUps[len(m.followUps)-1] = InboundEvent{}
	m.followUps = m.followUps[:len(m.followUps)-1]
	m.turnPlatform = next.Platform
	m.turnChatID = next.ChatID
	m.turnMsgID = next.MsgID
	m.turnSessionKey = ""
	m.turnSessionID = ""
	m.turnSource = SessionSource{}
	m.turnCancelled = false
	return next, true
}

func (m *Manager) markDrainTimeoutResumePending(ctx context.Context, reason DrainTimeoutReason) {
	state, ok := m.activeTurnSnapshot()
	if !ok {
		return
	}
	now := m.now()
	if state.Source.Platform == "" {
		state.Source.Platform = state.Platform
	}
	if state.Source.ChatID == "" {
		state.Source.ChatID = state.ChatID
	}
	if state.SessionKey == "" {
		state.SessionKey = state.Platform + ":" + state.ChatID
	}
	if state.SessionID == "" {
		resolved, err := resolveSession(ctx, m.cfg.SessionMap, state.SessionKey)
		if err != nil {
			m.log.Warn("resolve active session for drain timeout", "key", state.SessionKey, "err", err)
		}
		state.SessionID = resolved.SessionID
	}
	if state.SessionID == "" {
		return
	}

	timeoutEvidence := RuntimeDrainTimeoutEvidence{
		SessionKey:   state.SessionKey,
		SessionID:    state.SessionID,
		Source:       state.Source.Platform,
		ChatID:       state.Source.ChatID,
		UserID:       state.Source.UserID,
		Reason:       string(reason),
		TimeoutAt:    now.Format(time.RFC3339Nano),
		ActiveAgents: m.activeAgentCount(),
	}
	m.writeRuntimeStatus(ctx, RuntimeStatusUpdate{DrainTimeoutEvidence: &timeoutEvidence})

	if state.Cancelled {
		m.markNonResumable(ctx, state, session.NonResumableCancelled, now)
		return
	}
	if meta, ok := m.getSessionMetadata(ctx, state.SessionID); ok && meta.NonResumableReason != "" {
		m.writeNonResumableEvidence(ctx, RuntimeNonResumableEvidence{
			SessionKey: state.SessionKey,
			SessionID:  state.SessionID,
			Source:     state.Source.Platform,
			ChatID:     state.Source.ChatID,
			UserID:     state.Source.UserID,
			Reason:     meta.NonResumableReason,
			At:         now.Format(time.RFC3339Nano),
		})
		return
	}

	writer, ok := m.cfg.SessionMap.(sessionMetadataWriter)
	if !ok {
		return
	}
	meta := session.Metadata{
		SessionID:      state.SessionID,
		Source:         state.Source.Platform,
		ChatID:         state.Source.ChatID,
		UserID:         state.Source.UserID,
		ResumePending:  true,
		ResumeReason:   string(reason),
		ResumeMarkedAt: now.Unix(),
		UpdatedAt:      now.Unix(),
	}
	if err := writer.PutMetadata(ctx, meta); err != nil {
		m.log.Warn("mark resume pending", "session_id", state.SessionID, "err", err)
		return
	}
	evidence := RuntimeResumePendingEvidence{
		SessionKey: state.SessionKey,
		SessionID:  state.SessionID,
		Source:     state.Source.Platform,
		ChatID:     state.Source.ChatID,
		UserID:     state.Source.UserID,
		Reason:     string(reason),
		MarkedAt:   now.Format(time.RFC3339Nano),
	}
	m.writeRuntimeStatus(ctx, RuntimeStatusUpdate{ResumePendingEvidence: &evidence})
}

func (m *Manager) markNonResumable(ctx context.Context, state activeTurnSnapshot, reason string, at time.Time) {
	if writer, ok := m.cfg.SessionMap.(sessionMetadataWriter); ok && state.SessionID != "" {
		if err := writer.PutMetadata(ctx, session.Metadata{
			SessionID:          state.SessionID,
			Source:             state.Source.Platform,
			ChatID:             state.Source.ChatID,
			UserID:             state.Source.UserID,
			NonResumableReason: reason,
			NonResumableAt:     at.Unix(),
			UpdatedAt:          at.Unix(),
		}); err != nil {
			m.log.Warn("mark non-resumable session", "session_id", state.SessionID, "err", err)
		}
	}
	m.writeNonResumableEvidence(ctx, RuntimeNonResumableEvidence{
		SessionKey: state.SessionKey,
		SessionID:  state.SessionID,
		Source:     state.Source.Platform,
		ChatID:     state.Source.ChatID,
		UserID:     state.Source.UserID,
		Reason:     reason,
		At:         at.Format(time.RFC3339Nano),
	})
}

func (m *Manager) getSessionMetadata(ctx context.Context, sessionID string) (session.Metadata, bool) {
	reader, ok := m.cfg.SessionMap.(sessionMetadataReader)
	if !ok || sessionID == "" {
		return session.Metadata{}, false
	}
	meta, ok, err := reader.GetMetadata(ctx, sessionID)
	if err != nil {
		m.log.Warn("read session metadata", "session_id", sessionID, "err", err)
		return session.Metadata{}, false
	}
	if ok && meta.MigratedMemoryFlushed {
		m.writeExpiryFinalizedEvidence(ctx, RuntimeExpiryFinalizedEvidence{
			SessionID:             meta.SessionID,
			Source:                meta.Source,
			ChatID:                meta.ChatID,
			UserID:                meta.UserID,
			ExpiryFinalized:       meta.ExpiryFinalized,
			MigratedMemoryFlushed: meta.MigratedMemoryFlushed,
		})
	}
	return meta, ok
}

func (m *Manager) clearResumePending(ctx context.Context, sessionID string) {
	clearer, ok := m.cfg.SessionMap.(sessionResumeClearer)
	if !ok || sessionID == "" {
		return
	}
	if _, err := clearer.ClearResumePending(ctx, sessionID); err != nil {
		m.log.Warn("clear resume pending", "session_id", sessionID, "err", err)
	}
}

func (m *Manager) writeNonResumableEvidence(ctx context.Context, evidence RuntimeNonResumableEvidence) {
	m.writeRuntimeStatus(ctx, RuntimeStatusUpdate{NonResumableEvidence: &evidence})
}

func (m *Manager) writeExpiryFinalizedEvidence(ctx context.Context, evidence RuntimeExpiryFinalizedEvidence) {
	m.writeRuntimeStatus(ctx, RuntimeStatusUpdate{ExpiryFinalizedEvidence: &evidence})
}

func resumePendingNote(reason string) string {
	reasonPhrase := "a gateway interruption"
	switch reason {
	case session.ResumeReasonRestartTimeout:
		reasonPhrase = "a gateway restart"
	case session.ResumeReasonShutdownTimeout:
		reasonPhrase = "a gateway shutdown"
	}
	return "[System note: Your previous turn in this session was interrupted by " +
		reasonPhrase +
		". The conversation history below is intact. If it contains unfinished tool result(s), process them first and summarize what was accomplished, then address the user's new message below.]"
}

func (m *Manager) submitPinned(ctx context.Context, ch Channel, ev InboundEvent) bool {
	resolved, err := resolveSession(ctx, m.cfg.SessionMap, ev.ChatKey())
	if err != nil {
		m.log.Warn("load session mapping", "key", ev.ChatKey(), "err", err)
	}
	source := sessionSourceFromInbound(ev)
	submitText := ev.SubmitText()
	var clearPendingSessionID string
	var clearBlockedMapping bool
	if meta, ok := m.getSessionMetadata(ctx, resolved.SessionID); ok {
		if meta.NonResumableReason != "" {
			resolved.NonResumableSessionID = resolved.SessionID
			resolved.NonResumableReason = meta.NonResumableReason
			resolved.SessionID = ev.ChatKey()
			clearBlockedMapping = true
			m.writeNonResumableEvidence(ctx, RuntimeNonResumableEvidence{
				SessionKey: ev.ChatKey(),
				SessionID:  resolved.NonResumableSessionID,
				Source:     source.Platform,
				ChatID:     source.ChatID,
				UserID:     source.UserID,
				Reason:     meta.NonResumableReason,
				At:         m.now().Format(time.RFC3339Nano),
			})
		} else if meta.ResumePending {
			reason := meta.ResumeReason
			if reason == "" {
				reason = session.ResumeReasonRestartTimeout
			}
			submitText = resumePendingNote(reason) + "\n\n" + submitText
			clearPendingSessionID = resolved.SessionID
		}
	}
	m.setPinnedTurnSession(ev.ChatKey(), resolved.SessionID, source)
	sessionContext := BuildSessionContextPrompt(SessionContext{
		Source:                source,
		SessionKey:            ev.ChatKey(),
		SessionID:             resolved.SessionID,
		RequestedSessionID:    resolved.RequestedSessionID,
		ResumePath:            resolved.ResumePath,
		ResumeStatus:          resolved.ResumeStatus,
		NonResumableSessionID: resolved.NonResumableSessionID,
		NonResumableReason:    resolved.NonResumableReason,
		ConnectedPlatforms:    m.connectedPlatforms(),
	})
	if err := m.kernel.Submit(kernel.PlatformEvent{
		Kind:           kernel.PlatformEventSubmit,
		Text:           submitText,
		SessionID:      resolved.SessionID,
		SessionContext: sessionContext,
	}); err != nil {
		m.clearTurn()
		_, _ = m.sendWithHooks(ctx, ch, ev.ChatID, "Busy — try again in a second.")
		return false
	}
	if clearPendingSessionID != "" {
		m.clearResumePending(ctx, clearPendingSessionID)
	}
	if clearBlockedMapping && m.cfg.SessionMap != nil {
		if err := m.cfg.SessionMap.Put(ctx, ev.ChatKey(), ""); err != nil {
			m.log.Warn("clear non-resumable session mapping", "key", ev.ChatKey(), "err", err)
		}
	}
	return true
}
