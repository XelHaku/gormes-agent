package gateway

import "context"

// Channel is the minimum every adapter implements. Additional capabilities are
// modeled as optional interfaces that the manager type-asserts at runtime.
type Channel interface {
	// Name returns the stable platform identifier ("telegram", "discord", ...).
	Name() string

	// Run starts the inbound loop and blocks until ctx cancellation. The
	// adapter must not close inbox; the manager owns the shared channel.
	Run(ctx context.Context, inbox chan<- InboundEvent) error

	// Send delivers a plain-text message to chatID and returns the platform
	// message ID when one exists.
	Send(ctx context.Context, chatID, text string) (msgID string, err error)
}

// MessageEditor is implemented by channels that can edit an existing message.
type MessageEditor interface {
	EditMessage(ctx context.Context, chatID, msgID, text string) error
}

// PlaceholderCapable is implemented by channels that can create a message
// placeholder for subsequent streaming edits.
type PlaceholderCapable interface {
	SendPlaceholder(ctx context.Context, chatID string) (msgID string, err error)
}

// TypingCapable is implemented by channels that can show a typing indicator.
// The returned stop function must be idempotent.
type TypingCapable interface {
	StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

// ReactionCapable is implemented by channels that can react to inbound
// messages. The returned undo function must be idempotent.
type ReactionCapable interface {
	ReactToMessage(ctx context.Context, chatID, msgID string) (undo func(), err error)
}
