package gateway

import (
	"context"
	"sync"
	"time"
)

type placeholderEditor interface {
	SendPlaceholder(ctx context.Context, chatID string) (msgID string, err error)
	EditMessage(ctx context.Context, chatID, msgID, text string) error
}

type coalescerMessageSender interface {
	Send(ctx context.Context, chatID, text string) (msgID string, err error)
}

type coalescerOption func(*coalescer)

func coalescerFreshFinalAfter(d time.Duration) coalescerOption {
	return func(c *coalescer) {
		c.freshFinalAfter = d
	}
}

func coalescerNow(now func() time.Time) coalescerOption {
	return func(c *coalescer) {
		if now != nil {
			c.now = now
		}
	}
}

// coalescer batches outbound edits for one turn. The manager owns one
// instance per active turn and tears it down on terminal phases.
type coalescer struct {
	sender placeholderEditor
	window time.Duration
	chatID string
	now    func() time.Time

	mu               sync.Mutex
	pendingText      string
	pendingMsgID     string
	messageCreatedAt time.Time
	lastSentText     string
	lastEditAt       time.Time
	retryAfter       time.Time
	freshFinalAfter  time.Duration
	wakeupCh         chan struct{}
}

func newCoalescer(pe placeholderEditor, window time.Duration, chatID string, opts ...coalescerOption) *coalescer {
	if window <= 0 {
		window = time.Second
	}
	c := &coalescer{
		sender:   pe,
		window:   window,
		chatID:   chatID,
		now:      time.Now,
		wakeupCh: make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *coalescer) setPending(text string) {
	c.mu.Lock()
	c.pendingText = text
	c.mu.Unlock()

	select {
	case c.wakeupCh <- struct{}{}:
	default:
	}
}

func (c *coalescer) currentMessageID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pendingMsgID
}

func (c *coalescer) flushImmediate(ctx context.Context, text string) {
	c.flushImmediateFinal(ctx, text, false)
}

func (c *coalescer) flushImmediateFinal(ctx context.Context, text string, finalize bool) {
	c.mu.Lock()
	msgID := c.pendingMsgID
	createdAt := c.messageCreatedAt
	freshFinalAfter := c.freshFinalAfter
	c.mu.Unlock()

	now := c.now()
	if shouldSendFreshFinal(finalize, msgID, createdAt, freshFinalAfter, now) {
		if sentID, ok := c.tryFreshFinal(ctx, msgID, text); ok {
			c.mu.Lock()
			c.pendingMsgID = sentID
			c.messageCreatedAt = now
			c.lastSentText = text
			c.lastEditAt = now
			c.pendingText = ""
			c.mu.Unlock()
			return
		}
	}

	var sentID string
	var err error
	if msgID == "" {
		sentAt := c.now()
		sentID, err = c.sender.SendPlaceholder(ctx, c.chatID)
		if err == nil {
			err = editCoalescedMessage(ctx, c.sender, c.chatID, sentID, text, finalize)
		}
		if err == nil {
			createdAt = sentAt
		}
	} else {
		sentID = msgID
		err = editCoalescedMessage(ctx, c.sender, c.chatID, msgID, text, finalize)
	}
	if err != nil {
		return
	}

	c.mu.Lock()
	if c.pendingMsgID == "" {
		c.pendingMsgID = sentID
		c.messageCreatedAt = createdAt
	}
	c.lastSentText = text
	c.lastEditAt = c.now()
	c.pendingText = ""
	c.mu.Unlock()
}

func (c *coalescer) run(ctx context.Context) {
	ticker := time.NewTicker(c.window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tryFlush(ctx)
		case <-c.wakeupCh:
			c.tryFlush(ctx)
		}
	}
}

func (c *coalescer) tryFlush(ctx context.Context) {
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
	now := c.now()
	if now.Before(retryAfter) {
		return
	}
	if msgID != "" && now.Sub(lastAt) < c.window {
		return
	}

	if msgID == "" {
		sentID, err := c.sender.SendPlaceholder(ctx, c.chatID)
		if err != nil {
			return
		}
		c.mu.Lock()
		if c.pendingMsgID == "" {
			c.pendingMsgID = sentID
			c.messageCreatedAt = now
		}
		c.pendingText = ""
		c.mu.Unlock()
		return
	}

	if err := editCoalescedMessage(ctx, c.sender, c.chatID, msgID, text, false); err != nil {
		return
	}

	c.mu.Lock()
	c.lastSentText = text
	c.lastEditAt = now
	c.pendingText = ""
	c.mu.Unlock()
}

func shouldSendFreshFinal(finalize bool, msgID string, createdAt time.Time, threshold time.Duration, now time.Time) bool {
	if !finalize || threshold <= 0 || msgID == "" || createdAt.IsZero() {
		return false
	}
	return now.Sub(createdAt) >= threshold
}

func (c *coalescer) tryFreshFinal(ctx context.Context, oldMsgID, text string) (string, bool) {
	sender, ok := c.sender.(coalescerMessageSender)
	if !ok {
		return "", false
	}
	msgID, err := sender.Send(ctx, c.chatID, text)
	if err != nil {
		return "", false
	}
	if deleter, ok := c.sender.(MessageDeleter); ok {
		_ = deleter.DeleteMessage(ctx, c.chatID, oldMsgID)
	}
	return msgID, true
}

func editCoalescedMessage(ctx context.Context, sender placeholderEditor, chatID, msgID, text string, finalize bool) error {
	if finalizer, ok := sender.(FinalizingMessageEditor); ok {
		return finalizer.EditMessageFinal(ctx, chatID, msgID, text, finalize)
	}
	return sender.EditMessage(ctx, chatID, msgID, text)
}
