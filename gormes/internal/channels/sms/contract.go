package sms

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/gateway"
)

const (
	platformName       = "sms"
	MaxSegmentLength   = 1600
	defaultCountryCode = "1"
)

// InboundMessage is the Twilio/webhook-neutral SMS ingress contract.
type InboundMessage struct {
	From      string
	To        string
	Body      string
	MessageID string
}

// ReplyTarget captures the canonical SMS reply addresses.
type ReplyTarget struct {
	To   string
	From string
}

// SessionIdentity freezes the canonical identifiers used for gateway routing.
type SessionIdentity struct {
	ChatID      string
	UserID      string
	RecipientID string
}

// NormalizedInbound is the SMS adapter output consumed by the shared gateway
// plus the explicit reply target needed for outbound delivery.
type NormalizedInbound struct {
	Event    gateway.InboundEvent
	Identity SessionIdentity
	Reply    ReplyTarget
}

// Delivery is the outbound SMS contract future transport code can send
// directly through Twilio or any compatible bridge.
type Delivery struct {
	To       string
	From     string
	Segments []string
}

// NormalizeInbound maps a raw SMS event onto the shared gateway contract.
func NormalizeInbound(msg InboundMessage, ownNumber string) (NormalizedInbound, bool) {
	from := normalizeNumber(msg.From)
	to := normalizeNumber(msg.To)
	own := normalizeNumber(ownNumber)
	body := strings.TrimSpace(msg.Body)
	if from == "" || body == "" {
		return NormalizedInbound{}, false
	}

	recipientID := firstNonEmpty(to, own)
	if recipientID == "" {
		return NormalizedInbound{}, false
	}
	if own != "" && from == own {
		return NormalizedInbound{}, false
	}

	kind, parsedBody := gateway.ParseInboundText(body)
	return NormalizedInbound{
		Event: gateway.InboundEvent{
			Platform: platformName,
			ChatID:   from,
			ChatName: from,
			UserID:   from,
			UserName: from,
			MsgID:    strings.TrimSpace(msg.MessageID),
			Kind:     kind,
			Text:     parsedBody,
		},
		Identity: SessionIdentity{
			ChatID:      from,
			UserID:      from,
			RecipientID: recipientID,
		},
		Reply: ReplyTarget{
			To:   from,
			From: recipientID,
		},
	}, true
}

// BuildDelivery normalizes outbound addressing and splits long SMS replies into
// transport-safe segments while preserving the full message body.
func BuildDelivery(target ReplyTarget, text string) (Delivery, error) {
	to := normalizeNumber(target.To)
	from := normalizeNumber(target.From)
	if to == "" || from == "" {
		return Delivery{}, fmt.Errorf("sms: reply target requires to/from numbers")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return Delivery{}, fmt.Errorf("sms: delivery requires text")
	}

	return Delivery{
		To:       to,
		From:     from,
		Segments: splitSegments(text, MaxSegmentLength),
	}, nil
}

func splitSegments(text string, maxRunes int) []string {
	remaining := []rune(strings.TrimSpace(text))
	segments := make([]string, 0, 1)
	for len(remaining) > 0 {
		if len(remaining) <= maxRunes {
			segments = append(segments, string(remaining))
			break
		}

		splitAt := preferredSplit(remaining, maxRunes)
		segment := strings.TrimSpace(string(remaining[:splitAt]))
		if segment == "" {
			splitAt = maxRunes
			segment = string(remaining[:splitAt])
		}
		segments = append(segments, segment)
		remaining = []rune(strings.TrimSpace(string(remaining[splitAt:])))
	}
	return segments
}

func preferredSplit(text []rune, maxRunes int) int {
	if len(text) <= maxRunes {
		return len(text)
	}

	region := text[:maxRunes]
	if splitAt := lastIndex(region, '\n'); splitAt >= maxRunes/2 {
		return splitAt
	}
	if splitAt := lastSpace(region); splitAt >= maxRunes/2 {
		return splitAt
	}
	return maxRunes
}

func lastIndex(text []rune, target rune) int {
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == target {
			return i
		}
	}
	return -1
}

func lastSpace(text []rune) int {
	for i := len(text) - 1; i >= 0; i-- {
		if unicode.IsSpace(text[i]) {
			return i
		}
	}
	return -1
}

func normalizeNumber(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	digits := strings.Builder{}
	hasPlus := false
	for _, r := range raw {
		switch {
		case r == '+' && digits.Len() == 0 && !hasPlus:
			hasPlus = true
		case unicode.IsDigit(r):
			digits.WriteRune(r)
		}
	}
	value := digits.String()
	if value == "" {
		return ""
	}

	switch {
	case hasPlus:
		return "+" + value
	case strings.HasPrefix(value, "00") && len(value) > 2:
		return "+" + value[2:]
	case len(value) == 10:
		return "+" + defaultCountryCode + value
	default:
		return "+" + value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
