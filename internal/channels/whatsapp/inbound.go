package whatsapp

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

const platformName = "whatsapp"

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
	ChatID      string
	ChatName    string
	ChatKind    ChatKind
	ReplyChatID string
	UserID      string
	UserName    string
	MessageID   string
	Text        string
	Mentioned   bool
	FromMe      bool
	BotIDs      []string
}

// NormalizeInbound maps a WhatsApp transport event onto the shared gateway
// contract. Slash commands are intentionally routed through
// gateway.ParseInboundText so the adapter never consumes generic commands
// locally.
func NormalizeInbound(msg InboundMessage) (gateway.InboundEvent, bool) {
	result := NormalizeInboundWithIdentity(msg, IdentityContext{})
	if !result.Routed() {
		return gateway.InboundEvent{}, false
	}
	return result.Event, true
}

// NormalizeInboundWithIdentity maps a WhatsApp transport event onto the shared
// gateway contract while preserving the identity and raw reply peer metadata
// future send code needs.
func NormalizeInboundWithIdentity(msg InboundMessage, identity IdentityContext) InboundResult {
	status := resolveBotIdentity(identity, msg)

	rawUserID := strings.TrimSpace(msg.UserID)
	if _, safe, evidence := NormalizeSafeWhatsAppIdentifier(rawUserID); !safe {
		return unsafeInboundIdentifierResult(status, SessionIdentity{}, evidence)
	}
	userID := canonicalWhatsAppUserID(rawUserID, identity.AliasMappings)
	if userID == "" {
		return InboundResult{Decision: InboundDecisionDrop, Status: status}
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return InboundResult{Decision: InboundDecisionDrop, Status: status}
	}
	if msg.Mentioned {
		text = stripLeadingMentions(text)
		if text == "" {
			return InboundResult{Decision: InboundDecisionDrop, Status: status}
		}
	}

	rawChatID := strings.TrimSpace(msg.ChatID)
	if rawChatID == "" {
		rawChatID = rawUserID
	}
	if _, safe, evidence := NormalizeSafeWhatsAppIdentifier(rawChatID); !safe {
		return unsafeInboundIdentifierResult(status, SessionIdentity{}, evidence)
	}
	chatKind := normalizedChatKind(msg.ChatKind, rawChatID)
	chatID := canonicalWhatsAppChatID(rawChatID, chatKind, identity.AliasMappings)
	if chatID == "" {
		chatID = userID
	}

	rawReplyChatID := strings.TrimSpace(msg.ReplyChatID)
	if rawReplyChatID == "" {
		rawReplyChatID = rawChatID
	}

	result := InboundResult{
		Identity: SessionIdentity{
			ChatKind:          chatKind,
			ChatID:            chatID,
			UserID:            userID,
			RawChatID:         rawChatID,
			RawUserID:         rawUserID,
			BotID:             status.BotID,
			RawBotID:          status.RawBotID,
			BotIdentitySource: status.Source,
		},
		Reply: ReplyTarget{
			ChatID:   rawReplyChatID,
			ChatKind: chatKind,
		},
		Status: status,
	}
	if _, safe, evidence := NormalizeSafeWhatsAppIdentifier(rawReplyChatID); !safe {
		return unsafeInboundIdentifierResult(status, result.Identity, evidence)
	}
	if suppression, ok := selfChatSuppression(msg, text, identity, result.Identity, status); ok {
		result.Decision = InboundDecisionSuppressSelfChat
		if suppression.Reason == SelfChatSuppressionBotIdentityUnresolved {
			result.Decision = InboundDecisionUnresolvedIdentity
		}
		result.Suppression = suppression
		return result
	}

	kind, body := gateway.ParseInboundText(text)
	result.Event = gateway.InboundEvent{
		Platform: platformName,
		ChatID:   chatID,
		ChatName: strings.TrimSpace(msg.ChatName),
		UserID:   userID,
		UserName: strings.TrimSpace(msg.UserName),
		MsgID:    strings.TrimSpace(msg.MessageID),
		Kind:     kind,
		Text:     body,
	}
	result.Decision = InboundDecisionRoute
	return result
}

func unsafeInboundIdentifierResult(status IdentityStatus, identity SessionIdentity, evidence WhatsAppIdentifierEvidence) InboundResult {
	status.Resolved = false
	status.BotID = ""
	status.RawBotID = ""
	status.Reason = string(evidence)
	return InboundResult{
		Decision: InboundDecisionUnresolvedIdentity,
		Identity: identity,
		Status:   status,
	}
}

func selfChatSuppression(msg InboundMessage, text string, ctx IdentityContext, identity SessionIdentity, status IdentityStatus) (SelfChatSuppression, bool) {
	if !msg.FromMe && (status.BotID == "" || identity.UserID != status.BotID) {
		return SelfChatSuppression{}, false
	}

	suppression := SelfChatSuppression{
		ChatID:    identity.ChatID,
		UserID:    identity.UserID,
		MessageID: strings.TrimSpace(msg.MessageID),
	}
	if msg.FromMe && !status.Resolved {
		suppression.Reason = SelfChatSuppressionBotIdentityUnresolved
		return suppression, true
	}

	switch normalizedAccountMode(ctx.AccountMode) {
	case AccountModeBot:
		if msg.FromMe || (status.BotID != "" && identity.UserID == status.BotID) {
			suppression.Reason = SelfChatSuppressionBotOwnMessage
			return suppression, true
		}
	case AccountModeSelfChat:
		recent := recentMessageIDSet(ctx.RecentSentMessageIDs)
		replyPrefix := ctx.ReplyPrefix
		if msg.FromMe && ((replyPrefix != "" && strings.HasPrefix(text, replyPrefix)) || recent[suppression.MessageID]) {
			suppression.Reason = SelfChatSuppressionAgentEcho
			return suppression, true
		}
	}
	return SelfChatSuppression{}, false
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
