package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

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

// OutboundState reports whether outbound sends can resolve a paired target.
type OutboundState string

const (
	OutboundStatePaired   OutboundState = "paired"
	OutboundStateDegraded OutboundState = "degraded"
)

// SendDegradedReason identifies why a send was stopped before transport I/O.
type SendDegradedReason string

const (
	SendDegradedUnpairedTarget   SendDegradedReason = "unpaired_target"
	SendDegradedUnresolvedTarget SendDegradedReason = "unresolved_target"
)

// OutboundStatus is the send-gate status payload future gateway status views
// can surface for WhatsApp pairing problems.
type OutboundStatus struct {
	State     OutboundState
	Reason    SendDegradedReason
	ChatID    string
	RawChatID string
	ChatKind  ChatKind
	Runtime   RuntimeKind
}

// DegradedSendError is returned when the send gate blocks delivery before
// calling the WhatsApp transport.
type DegradedSendError struct {
	Reason SendDegradedReason
	ChatID string
}

func (e DegradedSendError) Error() string {
	return fmt.Sprintf("whatsapp: outbound target %q degraded: %s", e.ChatID, e.Reason)
}

// Bot adapts WhatsApp traffic into the shared gateway channel contract.
type Bot struct {
	client   Client
	identity IdentityContext
	log      *slog.Logger

	mu        sync.RWMutex
	pairings  map[string]outboundPairing
	lastState OutboundStatus
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
	msgID, err := b.client.SendWhatsApp(ctx, request)
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
