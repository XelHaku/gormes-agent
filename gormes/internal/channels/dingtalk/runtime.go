package dingtalk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// IngressMode freezes the inbound transport shape for the first DingTalk port.
type IngressMode string

const (
	IngressModeStream IngressMode = "stream"
)

// ReplyMode freezes how outbound replies are delivered back to DingTalk.
type ReplyMode string

const (
	ReplyModeSessionWebhook ReplyMode = "session_webhook"
)

// RuntimeConfig captures the operator-controlled DingTalk bootstrap inputs.
type RuntimeConfig struct {
	ClientID     string
	ClientSecret string
	ReplyRetry   ReplyRetryPolicy
}

// RuntimePlan is the narrow DingTalk transport/bootstrap contract frozen in
// tests before wiring a real SDK transport.
type RuntimePlan struct {
	Ingress IngressPlan
	Reply   ReplyPlan
}

// IngressPlan describes the inbound DingTalk operating mode.
type IngressPlan struct {
	Mode              IngressMode
	AutoReconnect     bool
	RequiresPublicURL bool
}

// ReplyPlan describes the callback-bound outbound reply contract.
type ReplyPlan struct {
	Mode  ReplyMode
	Retry ReplyRetryPolicy
}

// ReplyRetryPolicy controls the retry behavior for session-webhook sends.
type ReplyRetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DecideRuntime freezes the first-pass DingTalk operating model: Stream Mode
// ingress plus session-webhook replies.
func DecideRuntime(cfg RuntimeConfig) (RuntimePlan, error) {
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return RuntimePlan{}, fmt.Errorf("dingtalk: client id and client secret are required for stream mode")
	}

	return RuntimePlan{
		Ingress: IngressPlan{
			Mode:              IngressModeStream,
			AutoReconnect:     true,
			RequiresPublicURL: false,
		},
		Reply: ReplyPlan{
			Mode:  ReplyModeSessionWebhook,
			Retry: normalizedReplyRetryPolicy(cfg.ReplyRetry),
		},
	}, nil
}

// DefaultReplyRetryPolicy keeps reply retry behavior deterministic and bounded.
func DefaultReplyRetryPolicy() ReplyRetryPolicy {
	return ReplyRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Multiplier:   2,
	}
}

func normalizedReplyRetryPolicy(policy ReplyRetryPolicy) ReplyRetryPolicy {
	defaults := DefaultReplyRetryPolicy()
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaults.MaxAttempts
	}
	if policy.InitialDelay <= 0 {
		policy.InitialDelay = defaults.InitialDelay
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = defaults.MaxDelay
	}
	if policy.Multiplier < 1 {
		policy.Multiplier = defaults.Multiplier
	}
	return policy
}

// SessionWebhooks keeps the latest callback webhook per chat so the adapter can
// reply without inventing a global DingTalk send primitive that does not exist.
type SessionWebhooks struct {
	values sync.Map
}

func NewSessionWebhooks() *SessionWebhooks {
	return &SessionWebhooks{}
}

func (s *SessionWebhooks) Remember(chatID, webhook string) {
	if s == nil {
		return
	}
	chatID = strings.TrimSpace(chatID)
	webhook = strings.TrimSpace(webhook)
	if chatID == "" || webhook == "" {
		return
	}
	s.values.Store(chatID, webhook)
}

func (s *SessionWebhooks) Lookup(chatID string) (string, error) {
	if s == nil {
		return "", errors.New("dingtalk: session webhook store is nil")
	}
	chatID = strings.TrimSpace(chatID)
	raw, ok := s.values.Load(chatID)
	if !ok {
		return "", fmt.Errorf("dingtalk: no session webhook for chat %q", chatID)
	}
	webhook, ok := raw.(string)
	if !ok || strings.TrimSpace(webhook) == "" {
		return "", fmt.Errorf("dingtalk: invalid session webhook for chat %q", chatID)
	}
	return strings.TrimSpace(webhook), nil
}

// ReplyClient is the minimal outbound DingTalk client surface.
type ReplyClient interface {
	SendReply(ctx context.Context, webhook, text string) (string, error)
}

// ReplySender applies the runtime retry/error contract around session-webhook
// delivery.
type ReplySender struct {
	client ReplyClient
	retry  ReplyRetryPolicy
	sleep  func(context.Context, time.Duration) error
}

// ReplySenderOption customizes reply send behavior without widening the
// constructor.
type ReplySenderOption func(*ReplySender)

// WithReplySleep swaps out the backoff wait so tests stay fast.
func WithReplySleep(fn func(context.Context, time.Duration) error) ReplySenderOption {
	return func(s *ReplySender) {
		if fn != nil {
			s.sleep = fn
		}
	}
}

func NewReplySender(client ReplyClient, retry ReplyRetryPolicy, opts ...ReplySenderOption) *ReplySender {
	sender := &ReplySender{
		client: client,
		retry:  normalizedReplyRetryPolicy(retry),
		sleep:  sleepContext,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(sender)
		}
	}
	return sender
}

func (s *ReplySender) Send(ctx context.Context, store *SessionWebhooks, chatID, text string) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("dingtalk: reply sender requires a client")
	}

	var lastErr error
	for attempt := 1; attempt <= s.retry.MaxAttempts; attempt++ {
		webhook, err := store.Lookup(chatID)
		if err != nil {
			return "", err
		}
		msgID, err := s.client.SendReply(ctx, webhook, text)
		if err == nil {
			return msgID, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		if !temporary(err) {
			return "", fmt.Errorf("dingtalk: send reply: %w", err)
		}

		lastErr = err
		if attempt == s.retry.MaxAttempts {
			break
		}
		if err := s.sleep(ctx, s.retry.delay(attempt)); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("dingtalk: send reply failed after %d attempts: %w", s.retry.MaxAttempts, lastErr)
}

func (p ReplyRetryPolicy) delay(attempt int) time.Duration {
	delay := p.InitialDelay
	if attempt <= 1 {
		return delay
	}
	for i := 1; i < attempt; i++ {
		next := time.Duration(float64(delay) * p.Multiplier)
		if p.MaxDelay > 0 && next > p.MaxDelay {
			return p.MaxDelay
		}
		delay = next
	}
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

type temporaryError interface {
	Temporary() bool
}

func temporary(err error) bool {
	var target temporaryError
	return errors.As(err, &target) && target.Temporary()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
