package whatsapp

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const platformName = "whatsapp"

var peerIDSuffixes = []string{"@s.whatsapp.net", "@c.us", "@g.us"}

// ChatKind distinguishes direct chats from group chats without committing to a
// particular transport implementation.
type ChatKind string

const (
	ChatKindDirect ChatKind = "direct"
	ChatKindGroup  ChatKind = "group"
)

// InboundMessage is the transport-neutral WhatsApp ingress contract shared by
// future bridge and native runtimes.
type InboundMessage struct {
	ChatID    string
	ChatName  string
	ChatKind  ChatKind
	UserID    string
	UserName  string
	MessageID string
	Text      string
	Mentioned bool
}

// NormalizeInbound maps a WhatsApp transport event onto the shared gateway
// contract. Slash commands are intentionally routed through
// gateway.ParseInboundText so the adapter never consumes generic commands
// locally.
func NormalizeInbound(msg InboundMessage) (gateway.InboundEvent, bool) {
	userID := normalizePeerID(msg.UserID)
	if userID == "" {
		return gateway.InboundEvent{}, false
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return gateway.InboundEvent{}, false
	}
	if msg.Mentioned {
		text = stripLeadingMentions(text)
		if text == "" {
			return gateway.InboundEvent{}, false
		}
	}

	chatID := normalizePeerID(msg.ChatID)
	if chatID == "" {
		chatID = userID
	}

	kind, body := gateway.ParseInboundText(text)
	return gateway.InboundEvent{
		Platform: platformName,
		ChatID:   chatID,
		ChatName: strings.TrimSpace(msg.ChatName),
		UserID:   userID,
		UserName: strings.TrimSpace(msg.UserName),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}, true
}

func normalizePeerID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	lower := strings.ToLower(id)
	for _, suffix := range peerIDSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return id[:len(id)-len(suffix)]
		}
	}
	return id
}

func stripLeadingMentions(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	for len(fields) > 0 {
		if !strings.HasPrefix(fields[0], "@") {
			break
		}
		fields = fields[1:]
	}
	return strings.TrimSpace(strings.Join(fields, " "))
}
