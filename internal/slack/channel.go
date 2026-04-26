package slack

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

// Channel adapts the Slack Client seam onto the shared gateway manager.
type Channel struct {
	client Client
	log    *slog.Logger

	selfUserID string

	mu              sync.RWMutex
	threadByChannel map[string]string
	threadContext   *ThreadContextCache
}

var _ gateway.Channel = (*Channel)(nil)

func NewChannel(client Client, log *slog.Logger) *Channel {
	if log == nil {
		log = slog.Default()
	}
	return &Channel{
		client:          client,
		log:             log,
		threadByChannel: map[string]string{},
		threadContext:   newThreadContextCache(""),
	}
}

func (c *Channel) Name() string { return "slack" }

func (c *Channel) Run(ctx context.Context, inbox chan<- gateway.InboundEvent) error {
	selfID, err := c.client.AuthTest(ctx)
	if err != nil {
		return err
	}
	c.selfUserID = selfID
	c.threadContext.SetSelfUserID(selfID)

	return c.client.Run(ctx, func(e Event) {
		c.handleEvent(ctx, inbox, e)
	})
}

func (c *Channel) handleEvent(ctx context.Context, inbox chan<- gateway.InboundEvent, e Event) {
	if err := c.client.Ack(e.RequestID); err != nil {
		c.log.Warn("slack ack failed", "request_id", e.RequestID, "err", err)
		return
	}

	ev, ok := c.toInboundEvent(e)
	if !ok {
		return
	}
	select {
	case inbox <- ev:
	case <-ctx.Done():
	}
}

func (c *Channel) toInboundEvent(e Event) (gateway.InboundEvent, bool) {
	channelID := strings.TrimSpace(e.ChannelID)
	userID := strings.TrimSpace(e.UserID)
	if channelID == "" || userID == "" || userID == c.selfUserID {
		return gateway.InboundEvent{}, false
	}
	if ignoreSubtype(e.SubType) {
		return gateway.InboundEvent{}, false
	}

	threadTS := strings.TrimSpace(e.ThreadTS)
	c.rememberThread(channelID, threadTS)
	ts := strings.TrimSpace(e.Timestamp)
	replyToText := ""
	if isSlackThreadReply(threadTS, ts) {
		if len(e.ThreadReplies) > 0 {
			replyToText = c.threadContext.Store(channelID, threadTS, e.TeamID, e.ThreadReplies).ParentText
		} else {
			replyToText = c.threadContext.ParentText(channelID, threadTS, e.TeamID)
		}
	}

	kind, body := gateway.ParseInboundText(strings.TrimSpace(e.Text))
	if kind == gateway.EventSubmit {
		var evidence []SlackRichTextEvidence
		body, evidence = augmentInboundText(body, e.Blocks, e.Attachments)
		for _, ev := range evidence {
			c.log.Warn(slackRichTextUnavailableCode, "source", ev.Source, "reason", ev.Reason)
		}
	}
	return gateway.InboundEvent{
		Platform:    "slack",
		ChatID:      channelID,
		UserID:      userID,
		ThreadID:    threadTS,
		MsgID:       ts,
		MessageID:   ts,
		ReplyToText: replyToText,
		Kind:        kind,
		Text:        body,
	}, true
}

func (c *Channel) Send(ctx context.Context, chatID, text string) (string, error) {
	return c.client.PostMessage(ctx, chatID, c.threadForChannel(chatID), text)
}

func (c *Channel) rememberThread(channelID, threadTS string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if threadTS == "" {
		delete(c.threadByChannel, channelID)
		return
	}
	c.threadByChannel[channelID] = threadTS
}

func (c *Channel) threadForChannel(channelID string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.threadByChannel[channelID]
}

func isSlackThreadReply(threadTS, ts string) bool {
	return strings.TrimSpace(threadTS) != "" && strings.TrimSpace(threadTS) != strings.TrimSpace(ts)
}
