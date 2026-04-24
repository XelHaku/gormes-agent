package dingtalk

import (
	"context"
	"strings"
	"sync"
)

const (
	thinkingEmojiName = "🤔Thinking"
	doneEmojiName     = "🥳Done"
	textEmotionID     = "2659900"
	textBackgroundID  = "im_bg_1"
)

// EmojiReactionRequest is the SDK-neutral DingTalk robot emotion payload. The
// real SDK binding should translate it to RobotReplyEmotionRequest or
// RobotRecallEmotionRequest without changing the bot seam.
type EmojiReactionRequest struct {
	RobotCode          string                   `json:"robotCode"`
	OpenMessageID      string                   `json:"openMsgId"`
	OpenConversationID string                   `json:"openConversationId"`
	EmotionType        int                      `json:"emotionType"`
	EmotionName        string                   `json:"emotionName"`
	TextEmotion        EmojiReactionTextEmotion `json:"textEmotion"`
}

type EmojiReactionTextEmotion struct {
	EmotionID    string `json:"emotionId"`
	EmotionName  string `json:"emotionName"`
	Text         string `json:"text"`
	BackgroundID string `json:"backgroundId"`
}

// EmojiReactionClient is the minimal SDK binding seam for DingTalk robot
// emotion reactions.
type EmojiReactionClient interface {
	ReplyEmotion(ctx context.Context, req EmojiReactionRequest) error
	RecallEmotion(ctx context.Context, req EmojiReactionRequest) error
}

type emojiReactionContexts struct {
	mu       sync.Mutex
	contexts map[string]emojiReactionContext
	done     map[string]bool
}

type emojiReactionContext struct {
	messageID      string
	conversationID string
}

func newEmojiReactionContexts() *emojiReactionContexts {
	return &emojiReactionContexts{
		contexts: map[string]emojiReactionContext{},
		done:     map[string]bool{},
	}
}

func (c *emojiReactionContexts) Remember(chatID string, ctx emojiReactionContext) {
	if c == nil {
		return
	}
	chatID = strings.TrimSpace(chatID)
	ctx.messageID = strings.TrimSpace(ctx.messageID)
	ctx.conversationID = strings.TrimSpace(ctx.conversationID)
	if chatID == "" || ctx.messageID == "" || ctx.conversationID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contexts[chatID] = ctx
	delete(c.done, chatID)
}

func (c *emojiReactionContexts) Lookup(chatID string) (emojiReactionContext, bool) {
	if c == nil {
		return emojiReactionContext{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, ok := c.contexts[strings.TrimSpace(chatID)]
	return ctx, ok
}

func (c *emojiReactionContexts) MarkDone(chatID string) (emojiReactionContext, bool) {
	if c == nil {
		return emojiReactionContext{}, false
	}
	chatID = strings.TrimSpace(chatID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done[chatID] {
		return emojiReactionContext{}, false
	}
	ctx, ok := c.contexts[chatID]
	if !ok {
		return emojiReactionContext{}, false
	}
	c.done[chatID] = true
	return ctx, true
}

func newEmojiReactionRequest(robotCode string, ctx emojiReactionContext, emojiName string) EmojiReactionRequest {
	emojiName = strings.TrimSpace(emojiName)
	return EmojiReactionRequest{
		RobotCode:          strings.TrimSpace(robotCode),
		OpenMessageID:      strings.TrimSpace(ctx.messageID),
		OpenConversationID: strings.TrimSpace(ctx.conversationID),
		EmotionType:        2,
		EmotionName:        emojiName,
		TextEmotion: EmojiReactionTextEmotion{
			EmotionID:    textEmotionID,
			EmotionName:  emojiName,
			Text:         emojiName,
			BackgroundID: textBackgroundID,
		},
	}
}
