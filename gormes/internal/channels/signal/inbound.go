package signal

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const platformName = "signal"

// ChatType distinguishes Signal direct chats from group chats.
type ChatType string

const (
	ChatTypeDirect ChatType = "direct"
	ChatTypeGroup  ChatType = "group"
)

// InboundMessage is the transport-neutral Signal ingress contract used by a
// future signal-cli bridge adapter.
type InboundMessage struct {
	ChatType   ChatType
	SenderID   string
	SenderUUID string
	SenderName string
	GroupID    string
	GroupName  string
	MessageID  string
	Text       string
}

// SessionIdentity captures the canonical and alternate identifiers the Signal
// edge exposes for session routing.
type SessionIdentity struct {
	ChatType  ChatType
	ChatID    string
	ChatIDAlt string
	UserID    string
	UserIDAlt string
}

// NormalizedInbound is the adapter output consumed by gateway.Manager plus
// the richer session-identity metadata retained for future routing work.
type NormalizedInbound struct {
	Event    gateway.InboundEvent
	Identity SessionIdentity
}

// NormalizeInbound maps a Signal transport event onto the shared gateway
// contract. Generic slash commands are intentionally delegated to the shared
// gateway parser instead of being consumed locally.
func NormalizeInbound(msg InboundMessage) (NormalizedInbound, bool) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return NormalizedInbound{}, false
	}

	userID, userIDAlt := primaryUserIdentity(msg.SenderID, msg.SenderUUID)
	if userID == "" {
		return NormalizedInbound{}, false
	}

	chatType := normalizedChatType(msg.ChatType)
	if chatType == "" {
		return NormalizedInbound{}, false
	}

	chatID, chatIDAlt, chatName, ok := normalizedChatIdentity(msg, chatType, userID)
	if !ok {
		return NormalizedInbound{}, false
	}

	kind, body := gateway.ParseInboundText(text)
	return NormalizedInbound{
		Event: gateway.InboundEvent{
			Platform: platformName,
			ChatID:   chatID,
			ChatName: chatName,
			UserID:   userID,
			UserName: strings.TrimSpace(msg.SenderName),
			MsgID:    strings.TrimSpace(msg.MessageID),
			Kind:     kind,
			Text:     body,
		},
		Identity: SessionIdentity{
			ChatType:  chatType,
			ChatID:    chatID,
			ChatIDAlt: chatIDAlt,
			UserID:    userID,
			UserIDAlt: userIDAlt,
		},
	}, true
}

func normalizedChatType(chatType ChatType) ChatType {
	switch ChatType(strings.ToLower(strings.TrimSpace(string(chatType)))) {
	case ChatTypeDirect:
		return ChatTypeDirect
	case ChatTypeGroup:
		return ChatTypeGroup
	default:
		return ""
	}
}

func primaryUserIdentity(senderID, senderUUID string) (primary, alternate string) {
	senderID = strings.TrimSpace(senderID)
	senderUUID = strings.TrimSpace(senderUUID)
	switch {
	case senderID != "":
		if senderUUID != "" && senderUUID != senderID {
			return senderID, senderUUID
		}
		return senderID, ""
	case senderUUID != "":
		return senderUUID, ""
	default:
		return "", ""
	}
}

func normalizedChatIdentity(msg InboundMessage, chatType ChatType, fallbackChatID string) (chatID, chatIDAlt, chatName string, ok bool) {
	switch chatType {
	case ChatTypeDirect:
		return fallbackChatID, "", strings.TrimSpace(msg.SenderName), true
	case ChatTypeGroup:
		groupID := strings.TrimSpace(msg.GroupID)
		if groupID == "" {
			return "", "", "", false
		}
		return "group:" + groupID, groupID, strings.TrimSpace(msg.GroupName), true
	default:
		return "", "", "", false
	}
}
