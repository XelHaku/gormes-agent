package slack

import (
	"strings"
	"sync"
)

// ThreadMessage is the fakeable subset of Slack conversations.replies data
// needed to derive thread parent context without wiring a live API fetcher.
type ThreadMessage struct {
	TeamID    string
	Timestamp string
	UserID    string
	BotID     string
	SubType   string
	Username  string
	Text      string
}

type ThreadContext struct {
	ContextText  string
	ParentText   string
	MessageCount int
}

type threadContextKey struct {
	channelID string
	threadTS  string
	teamID    string
}

type ThreadContextCache struct {
	mu         sync.RWMutex
	selfUserID string
	entries    map[threadContextKey]ThreadContext
}

func newThreadContextCache(selfUserID string) *ThreadContextCache {
	return &ThreadContextCache{
		selfUserID: strings.TrimSpace(selfUserID),
		entries:    make(map[threadContextKey]ThreadContext),
	}
}

func (c *ThreadContextCache) SetSelfUserID(selfUserID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.selfUserID = strings.TrimSpace(selfUserID)
}

func (c *ThreadContextCache) Store(channelID, threadTS, teamID string, messages []ThreadMessage) ThreadContext {
	if c == nil {
		return ThreadContext{}
	}

	key := makeThreadContextKey(channelID, threadTS, teamID)
	if key.channelID == "" || key.threadTS == "" {
		return ThreadContext{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := buildThreadContextLocked(strings.TrimSpace(c.selfUserID), key.threadTS, messages)
	c.entries[key] = ctx
	return ctx
}

func (c *ThreadContextCache) ParentText(channelID, threadTS, teamID string) string {
	if c == nil {
		return ""
	}
	key := makeThreadContextKey(channelID, threadTS, teamID)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[key].ParentText
}

func makeThreadContextKey(channelID, threadTS, teamID string) threadContextKey {
	return threadContextKey{
		channelID: strings.TrimSpace(channelID),
		threadTS:  strings.TrimSpace(threadTS),
		teamID:    strings.TrimSpace(teamID),
	}
}

func buildThreadContextLocked(selfUserID, threadTS string, messages []ThreadMessage) ThreadContext {
	var parts []string
	var parentText string
	for _, msg := range messages {
		msgTS := strings.TrimSpace(msg.Timestamp)
		text := strings.TrimSpace(msg.Text)
		if msgTS == "" || text == "" {
			continue
		}

		isParent := msgTS == threadTS
		if isParent {
			parentText = text
		}
		if isSelfBotChild(selfUserID, isParent, msg) {
			continue
		}

		prefix := ""
		if isParent {
			prefix = "[thread parent] "
		}
		parts = append(parts, prefix+threadMessageAuthor(msg)+": "+text)
	}

	return ThreadContext{
		ContextText:  strings.Join(parts, "\n"),
		ParentText:   parentText,
		MessageCount: len(parts),
	}
}

func isSelfBotChild(selfUserID string, isParent bool, msg ThreadMessage) bool {
	if isParent || selfUserID == "" {
		return false
	}
	if !isBotMessage(msg) {
		return false
	}
	return strings.TrimSpace(msg.UserID) == selfUserID
}

func isBotMessage(msg ThreadMessage) bool {
	return strings.TrimSpace(msg.BotID) != "" || strings.TrimSpace(msg.SubType) == "bot_message"
}

func threadMessageAuthor(msg ThreadMessage) string {
	if userID := strings.TrimSpace(msg.UserID); userID != "" {
		return userID
	}
	if isBotMessage(msg) {
		if username := strings.TrimSpace(msg.Username); username != "" {
			return username
		}
		return "bot"
	}
	return "unknown"
}
