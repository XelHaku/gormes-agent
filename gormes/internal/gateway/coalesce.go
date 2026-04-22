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

// coalescer batches outbound edits for one turn. The manager owns one
// instance per active turn and tears it down on terminal phases.
type coalescer struct {
	sender placeholderEditor
	window time.Duration
	chatID string

	mu           sync.Mutex
	pendingText  string
	pendingMsgID string
	lastSentText string
	lastEditAt   time.Time
	retryAfter   time.Time
	wakeupCh     chan struct{}
}

func newCoalescer(pe placeholderEditor, window time.Duration, chatID string) *coalescer {
	if window <= 0 {
		window = time.Second
	}
	return &coalescer{
		sender:   pe,
		window:   window,
		chatID:   chatID,
		wakeupCh: make(chan struct{}, 1),
	}
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
	c.mu.Lock()
	msgID := c.pendingMsgID
	c.mu.Unlock()

	var sentID string
	var err error
	if msgID == "" {
		sentID, err = c.sender.SendPlaceholder(ctx, c.chatID)
		if err == nil {
			err = c.sender.EditMessage(ctx, c.chatID, sentID, text)
		}
	} else {
		sentID = msgID
		err = c.sender.EditMessage(ctx, c.chatID, msgID, text)
	}
	if err != nil {
		return
	}

	c.mu.Lock()
	if c.pendingMsgID == "" {
		c.pendingMsgID = sentID
	}
	c.lastSentText = text
	c.lastEditAt = time.Now()
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
	now := time.Now()
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
		}
		c.pendingText = ""
		c.mu.Unlock()
		return
	}

	if err := c.sender.EditMessage(ctx, c.chatID, msgID, text); err != nil {
		return
	}

	c.mu.Lock()
	c.lastSentText = text
	c.lastEditAt = time.Now()
	c.pendingText = ""
	c.mu.Unlock()
}
