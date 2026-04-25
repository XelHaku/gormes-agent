package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

var defaultSendRetryBackoff = []time.Duration{2 * time.Second, 4 * time.Second}

// SendOptions carries WhatsApp reply metadata that must survive the gateway's
// normalized chat/session handle.
type SendOptions struct {
	ReplyToMessageID string
}

// SendRequest is the bridge/native-neutral outbound WhatsApp delivery shape.
type SendRequest struct {
	Runtime  RuntimeKind
	ChatID   string
	ChatKind ChatKind
	Text     string
	Options  SendOptions
}

// Client is the transport-neutral WhatsApp surface used by the gateway bot.
type Client interface {
	Events() <-chan InboundMessage
	SendWhatsApp(ctx context.Context, req SendRequest) (string, error)
	Close() error
}

// ReconnectCapable is implemented by transports that can refresh their bridge
// or native session before a retry.
type ReconnectCapable interface {
	ReconnectWhatsApp(ctx context.Context) error
}

// SendRetryPolicy bounds reconnect retries after transient send failures.
type SendRetryPolicy struct {
	MaxRetries int
	Backoff    []time.Duration
	Sleep      func(context.Context, time.Duration) error
}

// OutboundState reports whether outbound sends can resolve a paired target.
type OutboundState string

const (
	OutboundStatePaired             OutboundState = "paired"
	OutboundStateDegraded           OutboundState = "degraded"
	OutboundStateReconnectPending   OutboundState = "reconnect_pending"
	OutboundStateReconnectExhausted OutboundState = "reconnect_exhausted"
)

// SendDegradedReason identifies why a send was stopped before transport I/O.
type SendDegradedReason string

const (
	SendDegradedUnpairedTarget     SendDegradedReason = "unpaired_target"
	SendDegradedUnresolvedTarget   SendDegradedReason = "unresolved_target"
	SendDegradedReconnectExhausted SendDegradedReason = "reconnect_exhausted"
)

// OutboundStatus is the send-gate status payload future gateway status views
// can surface for WhatsApp pairing problems.
type OutboundStatus struct {
	State        OutboundState
	Reason       SendDegradedReason
	ChatID       string
	RawChatID    string
	ChatKind     ChatKind
	Runtime      RuntimeKind
	Attempts     int
	ErrorMessage string
}

// DegradedSendError is returned when the send gate blocks delivery before
// calling the WhatsApp transport.
type DegradedSendError struct {
	Reason    SendDegradedReason
	ChatID    string
	RawChatID string
	Cause     error
}

func (e DegradedSendError) Error() string {
	msg := fmt.Sprintf("whatsapp: outbound target %q degraded: %s", e.ChatID, e.Reason)
	if e.RawChatID != "" {
		msg = fmt.Sprintf("%s raw=%q", msg, e.RawChatID)
	}
	if e.Cause != nil {
		msg = msg + ": " + e.Cause.Error()
	}
	return msg
}

func (e DegradedSendError) Unwrap() error {
	return e.Cause
}

// Bot adapts WhatsApp traffic into the shared gateway channel contract.
type Bot struct {
	client   Client
	identity IdentityContext
	log      *slog.Logger

	mu        sync.RWMutex
	pairings  map[string]outboundPairing
	lastState OutboundStatus
	retry     SendRetryPolicy
}

type outboundPairing struct {
	rawChatID        string
	chatKind         ChatKind
	replyToMessageID string
}

var _ gateway.Channel = (*Bot)(nil)

// New returns a WhatsApp gateway channel backed by a transport-neutral client.
func New(client Client, identity IdentityContext, log *slog.Logger) *Bot {
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		client:   client,
		identity: identity,
		log:      log,
		pairings: map[string]outboundPairing{},
		retry:    DefaultSendRetryPolicy(),
	}
}

func (b *Bot) Name() string { return platformName }

func (b *Bot) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	if b.client == nil {
		return fmt.Errorf("whatsapp: nil client")
	}

	events := b.client.Events()
	for {
		select {
		case <-ctx.Done():
			b.closeClient()
			return nil
		case msg, ok := <-events:
			if !ok {
				b.closeClient()
				return nil
			}

			normalized := NormalizeInboundWithIdentity(msg, b.identity)
			if !normalized.Routed() {
				continue
			}
			b.PairOutboundTarget(normalized)

			select {
			case inbox <- normalized.Event:
			case <-ctx.Done():
				b.closeClient()
				return nil
			}
		}
	}
}

func (b *Bot) Send(ctx context.Context, chatID, text string) (string, error) {
	normalizedChatID := strings.TrimSpace(chatID)
	target, ok := b.lookupOutboundPairing(normalizedChatID)
	if !ok {
		return "", b.degrade(normalizedChatID, SendDegradedUnpairedTarget)
	}
	if target.rawChatID == "" {
		return "", b.degrade(normalizedChatID, SendDegradedUnresolvedTarget)
	}
	if b.client == nil {
		return "", fmt.Errorf("whatsapp: nil client")
	}

	request := SendRequest{
		Runtime:  normalizedRuntimeKind(b.identity.Runtime),
		ChatID:   target.rawChatID,
		ChatKind: target.chatKind,
		Text:     text,
		Options: SendOptions{
			ReplyToMessageID: target.replyToMessageID,
		},
	}
	msgID, err := b.sendWithRetry(ctx, normalizedChatID, request)
	if err != nil {
		return "", err
	}

	b.recordOutboundStatus(OutboundStatus{
		State:     OutboundStatePaired,
		ChatID:    normalizedChatID,
		RawChatID: request.ChatID,
		ChatKind:  request.ChatKind,
		Runtime:   request.Runtime,
	})
	return msgID, nil
}

// SetSendRetryPolicy replaces the bounded reconnect retry policy. Tests use it
// to inject a fake clock; production callers can keep the default policy.
func (b *Bot) SetSendRetryPolicy(policy SendRetryPolicy) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.retry = normalizeSendRetryPolicy(policy)
}

// PairOutboundTarget records the raw WhatsApp peer required to send back to a
// normalized gateway chat handle.
func (b *Bot) PairOutboundTarget(result InboundResult) bool {
	if !result.Routed() {
		return false
	}

	chatID := strings.TrimSpace(result.Event.ChatID)
	if chatID == "" {
		return false
	}

	rawChatID := strings.TrimSpace(result.Reply.ChatID)
	chatKind := normalizedChatKind(result.Reply.ChatKind, rawChatID)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.pairings[chatID] = outboundPairing{
		rawChatID:        rawChatID,
		chatKind:         chatKind,
		replyToMessageID: strings.TrimSpace(result.Event.MsgID),
	}
	return true
}

// OutboundStatus returns the latest send-gate status.
func (b *Bot) OutboundStatus() OutboundStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastState
}

func (b *Bot) lookupOutboundPairing(chatID string) (outboundPairing, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	target, ok := b.pairings[chatID]
	return target, ok
}

func (b *Bot) sendWithRetry(ctx context.Context, chatID string, request SendRequest) (string, error) {
	policy := b.retryPolicy()
	maxAttempts := policy.MaxRetries + 1
	var lastErr error
	var attempts int

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		msgID, err := b.client.SendWhatsApp(ctx, request)
		if err == nil {
			return msgID, nil
		}
		lastErr = err
		if attempt == maxAttempts || !isRetryableSendError(err) {
			break
		}

		b.recordOutboundStatus(OutboundStatus{
			State:        OutboundStateReconnectPending,
			ChatID:       chatID,
			RawChatID:    request.ChatID,
			ChatKind:     request.ChatKind,
			Runtime:      request.Runtime,
			Attempts:     attempt,
			ErrorMessage: err.Error(),
		})

		reconnecter, ok := b.client.(ReconnectCapable)
		if !ok {
			lastErr = fmt.Errorf("whatsapp: reconnect unavailable after send failure: %w", err)
			break
		}
		if err := reconnecter.ReconnectWhatsApp(ctx); err != nil {
			lastErr = err
			break
		}
		if err := policy.Sleep(ctx, policy.backoff(attempt)); err != nil {
			lastErr = err
			break
		}
	}

	b.recordOutboundStatus(OutboundStatus{
		State:        OutboundStateReconnectExhausted,
		Reason:       SendDegradedReconnectExhausted,
		ChatID:       chatID,
		RawChatID:    request.ChatID,
		ChatKind:     request.ChatKind,
		Runtime:      request.Runtime,
		Attempts:     attempts,
		ErrorMessage: errorMessage(lastErr),
	})
	return "", DegradedSendError{
		Reason:    SendDegradedReconnectExhausted,
		ChatID:    chatID,
		RawChatID: request.ChatID,
		Cause:     lastErr,
	}
}

func (b *Bot) retryPolicy() SendRetryPolicy {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return normalizeSendRetryPolicy(b.retry)
}

func (b *Bot) degrade(chatID string, reason SendDegradedReason) error {
	b.recordOutboundStatus(OutboundStatus{
		State:   OutboundStateDegraded,
		Reason:  reason,
		ChatID:  chatID,
		Runtime: normalizedRuntimeKind(b.identity.Runtime),
	})
	return DegradedSendError{Reason: reason, ChatID: chatID}
}

func (b *Bot) recordOutboundStatus(status OutboundStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastState = status
}

func (b *Bot) closeClient() {
	if b.client != nil {
		_ = b.client.Close()
	}
}

func normalizedRuntimeKind(runtime RuntimeKind) RuntimeKind {
	if runtime == RuntimeKindNative {
		return RuntimeKindNative
	}
	return RuntimeKindBridge
}

// DefaultSendRetryPolicy returns the production reconnect retry policy. It is
// deterministic: retry count and backoff durations are bounded and contain no
// jitter so tests can replace only the sleep function.
func DefaultSendRetryPolicy() SendRetryPolicy {
	return SendRetryPolicy{
		MaxRetries: len(defaultSendRetryBackoff),
		Backoff:    append([]time.Duration(nil), defaultSendRetryBackoff...),
		Sleep:      sleepContext,
	}
}

func normalizeSendRetryPolicy(policy SendRetryPolicy) SendRetryPolicy {
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.Backoff == nil {
		policy.Backoff = append([]time.Duration(nil), defaultSendRetryBackoff...)
	} else {
		policy.Backoff = append([]time.Duration(nil), policy.Backoff...)
	}
	if policy.Sleep == nil {
		policy.Sleep = sleepContext
	}
	return policy
}

func (p SendRetryPolicy) backoff(attempt int) time.Duration {
	if attempt <= 0 || len(p.Backoff) == 0 {
		return 0
	}
	index := attempt - 1
	if index >= len(p.Backoff) {
		index = len(p.Backoff) - 1
	}
	if p.Backoff[index] < 0 {
		return 0
	}
	return p.Backoff[index]
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableSendError(err error) bool {
	if err == nil {
		return false
	}
	lowered := strings.ToLower(err.Error())
	if strings.Contains(lowered, "timed out") ||
		strings.Contains(lowered, "readtimeout") ||
		strings.Contains(lowered, "writetimeout") {
		return false
	}
	for _, pattern := range []string{
		"connecterror",
		"connectionerror",
		"connectionreset",
		"connectionrefused",
		"connecttimeout",
		"network",
		"broken pipe",
		"remotedisconnected",
		"eoferror",
		"not connected",
	} {
		if strings.Contains(lowered, pattern) {
			return true
		}
	}
	return false
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
