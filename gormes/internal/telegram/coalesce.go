package telegram

import (
	"context"
	"errors"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// coalescer batches outbound edits. One per active turn. Caller pushes the
// latest text via setPending; a goroutine running run() flushes at most
// once per window. flushImmediate bypasses the window for semantic edges
// (final answer, error, cancel).
//
// Not goroutine-safe across instances; each turn gets its own coalescer.
// Fields are protected by mu since setPending, flushImmediate, and the
// run-loop reader all touch them from different goroutines.
type coalescer struct {
	client telegramClient
	window time.Duration
	chatID int64

	mu           sync.Mutex
	pendingText  string
	pendingMsgID int       // 0 means "no placeholder yet; next send creates the message"
	lastSentText string
	lastEditAt   time.Time
	retryAfter   time.Time // set on 429; ticks before this are skipped
	wakeupCh     chan struct{}
}

func newCoalescer(c telegramClient, window time.Duration, chatID int64) *coalescer {
	if window <= 0 {
		window = time.Second
	}
	return &coalescer{
		client:   c,
		window:   window,
		chatID:   chatID,
		wakeupCh: make(chan struct{}, 1),
	}
}

// setPending stores the latest text the caller wants visible in the bot
// message. The run loop picks this up on its next tick.
func (c *coalescer) setPending(text string) {
	c.mu.Lock()
	c.pendingText = text
	c.mu.Unlock()

	select {
	case c.wakeupCh <- struct{}{}:
	default:
	}
}

// currentMessageID exposes the pendingMsgID for outbound-goroutine wiring.
func (c *coalescer) currentMessageID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pendingMsgID
}

// flushImmediate sends text right now (Send if there's no placeholder yet,
// Edit if there is). Bypasses the 1s window. Used for Idle/Failed/
// Cancelling finalisation.
func (c *coalescer) flushImmediate(text string) {
	c.mu.Lock()
	msgID := c.pendingMsgID
	c.mu.Unlock()

	var msg tgbotapi.Message
	var err error
	if msgID == 0 {
		msg, err = c.client.Send(tgbotapi.NewMessage(c.chatID, text))
	} else {
		msg, err = c.client.Send(tgbotapi.NewEditMessageText(c.chatID, msgID, text))
	}
	if err != nil {
		c.handleSendErr(err)
		return
	}

	c.mu.Lock()
	if msgID == 0 {
		c.pendingMsgID = msg.MessageID
	}
	c.lastSentText = text
	c.lastEditAt = time.Now()
	c.pendingText = ""
	c.mu.Unlock()
}

// run is the flush loop. Exits on ctx cancellation.
func (c *coalescer) run(ctx context.Context) {
	ticker := time.NewTicker(c.window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tryFlush()
		case <-c.wakeupCh:
			c.tryFlush()
		}
	}
}

// tryFlush inspects state and sends an edit if the window permits.
func (c *coalescer) tryFlush() {
	c.mu.Lock()
	text := c.pendingText
	msgID := c.pendingMsgID
	last := c.lastSentText
	lastAt := c.lastEditAt
	retryAfter := c.retryAfter
	c.mu.Unlock()

	if text == "" || text == last {
		return
	}
	now := time.Now()
	if now.Before(retryAfter) {
		return // 429 backoff in effect
	}
	if msgID != 0 && now.Sub(lastAt) < c.window {
		return // too soon for this message
	}

	var msg tgbotapi.Message
	var err error
	if msgID == 0 {
		msg, err = c.client.Send(tgbotapi.NewMessage(c.chatID, text))
	} else {
		msg, err = c.client.Send(tgbotapi.NewEditMessageText(c.chatID, msgID, text))
	}
	if err != nil {
		c.handleSendErr(err)
		return
	}

	c.mu.Lock()
	if msgID == 0 {
		c.pendingMsgID = msg.MessageID
	}
	c.lastSentText = text
	c.lastEditAt = time.Now()
	c.mu.Unlock()
}

// handleSendErr inspects a tgbotapi error; if it's a 429 with a
// RetryAfter, defers subsequent sends. All other errors are swallowed
// (coalescer doesn't die on transient issues; caller can observe via
// lastSentText not advancing).
func (c *coalescer) handleSendErr(err error) {
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) && tgErr.Code == 429 {
		retryAfter := time.Duration(tgErr.ResponseParameters.RetryAfter) * time.Second
		if retryAfter <= 0 {
			retryAfter = c.window // sensible fallback
		}
		c.mu.Lock()
		c.retryAfter = time.Now().Add(retryAfter)
		c.mu.Unlock()
	}
}
