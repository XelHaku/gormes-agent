package cron

import "context"

// DeliverySink is the abstraction between the cron executor and the
// actual outbound channel. cmd/gormes/telegram.go provides a Telegram
// implementation; future Slack/Discord adapters plug in the same way
// without the cron package learning about them.
//
// Implementations:
//   - Own their rate limiting + retries internally (cron won't retry
//     on failure — it records the delivery_status and moves on).
//   - Return a non-nil error on failure so the executor can log it.
type DeliverySink interface {
	Deliver(ctx context.Context, text string) error
}

// FuncSink adapts a plain function to the DeliverySink interface —
// convenient for test injections and for wrapping the Telegram bot's
// existing send method without wrapping in a struct.
type FuncSink func(ctx context.Context, text string) error

// Deliver satisfies DeliverySink.
func (f FuncSink) Deliver(ctx context.Context, text string) error {
	return f(ctx, text)
}
