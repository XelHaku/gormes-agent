package gateway

import (
	"strconv"
	"strings"
)

const MessageDeduplicatorEvidenceMissingMessageID MessageDeduplicatorEvidence = "dedup_unavailable_missing_message_id"

// InboundDedupKeyResult reports the bounded-deduplicator tracking key or why
// one cannot be derived for an inbound event.
type InboundDedupKeyResult struct {
	Key      string
	Evidence MessageDeduplicatorEvidence
}

// InboundDedupKey derives the key used to track inbound platform message IDs.
func InboundDedupKey(ev InboundEvent) InboundDedupKeyResult {
	if ev.MessageID == "" {
		return InboundDedupKeyResult{Evidence: MessageDeduplicatorEvidenceMissingMessageID}
	}
	return InboundDedupKeyResult{Key: inboundDedupKeyParts(ev.Platform, ev.ChatID, ev.ThreadID, ev.MessageID)}
}

func inboundDedupKeyParts(parts ...string) string {
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}
