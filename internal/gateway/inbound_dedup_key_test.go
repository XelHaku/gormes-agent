package gateway

import "testing"

func TestInboundDedupKey_MissingMessageIDDegrades(t *testing.T) {
	result := InboundDedupKey(InboundEvent{
		Platform:  "telegram",
		ChatID:    "chat-1",
		ThreadID:  "thread-1",
		MessageID: "",
	})

	if result.Key != "" {
		t.Fatalf("InboundDedupKey missing message ID key = %q, want empty", result.Key)
	}
	if result.Evidence != MessageDeduplicatorEvidenceMissingMessageID {
		t.Fatalf("InboundDedupKey missing message ID evidence = %q, want %q", result.Evidence, MessageDeduplicatorEvidenceMissingMessageID)
	}
}

func TestInboundDedupKey_StableForSameEvent(t *testing.T) {
	ev := InboundEvent{
		Platform:  "telegram",
		ChatID:    "chat-1",
		ThreadID:  "thread-1",
		MessageID: "msg-1",
	}

	first := InboundDedupKey(ev)
	second := InboundDedupKey(ev)

	if first.Evidence != "" || second.Evidence != "" {
		t.Fatalf("InboundDedupKey evidence = %q then %q, want none", first.Evidence, second.Evidence)
	}
	if first.Key == "" {
		t.Fatalf("InboundDedupKey key is empty for event with MessageID")
	}
	if first.Key != second.Key {
		t.Fatalf("InboundDedupKey repeated key = %q then %q, want stable key", first.Key, second.Key)
	}
}

func TestInboundDedupKey_ScopesByPlatformChatThreadMessageID(t *testing.T) {
	events := map[string]InboundEvent{
		"base": {
			Platform:  "telegram",
			ChatID:    "chat-1",
			ThreadID:  "thread-1",
			MessageID: "msg-1",
		},
		"different platform": {
			Platform:  "discord",
			ChatID:    "chat-1",
			ThreadID:  "thread-1",
			MessageID: "msg-1",
		},
		"different chat": {
			Platform:  "telegram",
			ChatID:    "chat-2",
			ThreadID:  "thread-1",
			MessageID: "msg-1",
		},
		"different thread": {
			Platform:  "telegram",
			ChatID:    "chat-1",
			ThreadID:  "thread-2",
			MessageID: "msg-1",
		},
		"different message": {
			Platform:  "telegram",
			ChatID:    "chat-1",
			ThreadID:  "thread-1",
			MessageID: "msg-2",
		},
	}

	seen := map[string]string{}
	for name, ev := range events {
		result := InboundDedupKey(ev)
		if result.Evidence != "" {
			t.Fatalf("%s evidence = %q, want none", name, result.Evidence)
		}
		if result.Key == "" {
			t.Fatalf("%s key is empty", name)
		}
		if prior, ok := seen[result.Key]; ok {
			t.Fatalf("%s key %q matches %s, want scoped key", name, result.Key, prior)
		}
		seen[result.Key] = name
	}
}
