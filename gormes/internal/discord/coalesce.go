package discord

import (
	"context"
	"sync"
	"time"
)

type coalescer struct {
	client    Client
	channelID string
	window    time.Duration

	mu           sync.Mutex
	pendingText  string
	messageID    string
	lastSentText string
	lastEditAt   time.Time
	wakeupCh     chan struct{}
}

func newCoalescer(client Client, window time.Duration, channelID string) *coalescer {
	if window <= 0 {
		window = time.Second
	}
	return &coalescer{
		client:    client,
		channelID: channelID,
		window:    window,
		wakeupCh:  make(chan struct{}, 1),
	}
}

func (c *coalescer) submit(text string) {
	c.mu.Lock()
	c.pendingText = text
	c.mu.Unlock()

	select {
	case c.wakeupCh <- struct{}{}:
	default:
	}
}

func (c *coalescer) flushImmediate(text string) {
	c.mu.Lock()
	msgID := c.messageID
	c.mu.Unlock()

	if msgID == "" {
		newID, err := c.client.Send(c.channelID, text)
		if err != nil {
			return
		}
		c.mu.Lock()
		c.messageID = newID
		c.lastSentText = text
		c.lastEditAt = time.Now()
		c.pendingText = ""
		c.mu.Unlock()
		return
	}

	if err := c.client.Edit(c.channelID, msgID, text); err != nil {
		return
	}

	c.mu.Lock()
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
			c.tryFlush()
		case <-c.wakeupCh:
			c.tryFlush()
		}
	}
}

func (c *coalescer) tryFlush() {
	c.mu.Lock()
	text := c.pendingText
	msgID := c.messageID
	last := c.lastSentText
	lastAt := c.lastEditAt
	c.mu.Unlock()

	if text == "" || text == last {
		return
	}
	if msgID != "" && time.Since(lastAt) < c.window {
		return
	}

	if msgID == "" {
		newID, err := c.client.Send(c.channelID, text)
		if err != nil {
			return
		}
		c.mu.Lock()
		c.messageID = newID
		c.lastSentText = text
		c.lastEditAt = time.Now()
		c.mu.Unlock()
		return
	}

	if err := c.client.Edit(c.channelID, msgID, text); err != nil {
		return
	}
	c.mu.Lock()
	c.lastSentText = text
	c.lastEditAt = time.Now()
	c.mu.Unlock()
}

func runTypingLoop(ctx context.Context, client Client, channelID string) {
	if channelID == "" {
		return
	}
	_ = client.Typing(channelID)

	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = client.Typing(channelID)
		}
	}
}
