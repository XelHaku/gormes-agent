package threadtext

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

// ReplyMode captures whether a channel should reply flat in the room or start
// a thread from a root message when one does not already exist.
type ReplyMode string

const (
	ReplyModeFlat   ReplyMode = "flat"
	ReplyModeThread ReplyMode = "thread"
)

// InboundMessage is the transport-neutral threaded-text ingress contract for
// channels like Matrix and Mattermost.
type InboundMessage struct {
	ChatID       string
	ChatName     string
	UserID       string
	UserName     string
	MessageID    string
	Text         string
	ThreadID     string
	ThreadRootID string
}

// ReplyTarget is the shared outbound addressing contract for threaded-text
// channels. ThreadID is the canonical thread root when a threaded reply should
// be used.
type ReplyTarget struct {
	ChatID           string
	ThreadID         string
	ReplyToMessageID string
}

// CanonicalThreadID prefers a platform's explicit thread-root identifier when
// one exists, and otherwise falls back to the thread identifier already present
// on the event.
func CanonicalThreadID(msg InboundMessage) string {
	if root := trim(msg.ThreadRootID); root != "" {
		return root
	}
	return trim(msg.ThreadID)
}

// NormalizeInbound maps a threaded-text transport event onto the shared
// gateway contract and preserves the canonical thread ID for origin replies.
func NormalizeInbound(platform string, msg InboundMessage) (gateway.InboundEvent, bool) {
	platform = strings.ToLower(trim(platform))
	chatID := trim(msg.ChatID)
	userID := trim(msg.UserID)
	text := trim(msg.Text)
	if platform == "" || chatID == "" || userID == "" || text == "" {
		return gateway.InboundEvent{}, false
	}

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: platform,
		ChatID:   chatID,
		ChatName: trim(msg.ChatName),
		UserID:   userID,
		UserName: trim(msg.UserName),
		ThreadID: CanonicalThreadID(msg),
		MsgID:    trim(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

// ResolveReplyTarget freezes the shared outbound reply semantics:
// existing thread replies stay inside that thread, and root messages only
// create a new thread when thread mode is enabled.
func ResolveReplyTarget(msg InboundMessage, mode ReplyMode) (ReplyTarget, bool) {
	chatID := trim(msg.ChatID)
	if chatID == "" {
		return ReplyTarget{}, false
	}

	target := ReplyTarget{ChatID: chatID}
	msgID := trim(msg.MessageID)
	if threadID := CanonicalThreadID(msg); threadID != "" {
		target.ThreadID = threadID
		target.ReplyToMessageID = msgID
		return target, true
	}

	if normalizeReplyMode(mode) == ReplyModeThread && msgID != "" {
		target.ThreadID = msgID
		target.ReplyToMessageID = msgID
	}
	return target, true
}

func normalizeReplyMode(mode ReplyMode) ReplyMode {
	if strings.EqualFold(trim(string(mode)), string(ReplyModeThread)) {
		return ReplyModeThread
	}
	return ReplyModeFlat
}

func trim(value string) string {
	return strings.TrimSpace(value)
}
