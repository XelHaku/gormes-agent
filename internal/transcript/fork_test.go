package transcript

import (
	"context"
	"testing"
	"time"
)

func TestForkTranscript_CopiesParentTurnsToChild(t *testing.T) {
	store := openTranscriptStore(t)
	defer store.Close(context.Background())

	mustInsertTurn(t, store.DB(), transcriptRow{
		SessionID: "sess-parent",
		Role:      "user",
		Content:   "hello?",
		TSUnix:    time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC).Unix(),
		ChatID:    "tui:default",
	})
	mustInsertTurn(t, store.DB(), transcriptRow{
		SessionID: "sess-parent",
		Role:      "assistant",
		Content:   "hello there",
		TSUnix:    time.Date(2026, 4, 22, 9, 0, 5, 0, time.UTC).Unix(),
		ChatID:    "tui:default",
		MetaJSON:  `{"tool_calls":[]}`,
	})
	// Unrelated session — must not leak into the fork.
	mustInsertTurn(t, store.DB(), transcriptRow{
		SessionID: "sess-other",
		Role:      "user",
		Content:   "ignored",
		TSUnix:    time.Date(2026, 4, 22, 9, 0, 1, 0, time.UTC).Unix(),
	})

	copied, err := ForkTurns(context.Background(), store.DB(), "sess-parent", "sess-child")
	if err != nil {
		t.Fatalf("ForkTurns: %v", err)
	}
	if copied != 2 {
		t.Fatalf("ForkTurns copied = %d, want 2", copied)
	}

	parent, err := loadTurns(context.Background(), store.DB(), "sess-parent")
	if err != nil {
		t.Fatalf("loadTurns parent: %v", err)
	}
	if len(parent) != 2 {
		t.Fatalf("parent turn count = %d, want 2 (parent must remain intact)", len(parent))
	}

	child, err := loadTurns(context.Background(), store.DB(), "sess-child")
	if err != nil {
		t.Fatalf("loadTurns child: %v", err)
	}
	if len(child) != 2 {
		t.Fatalf("child turn count = %d, want 2", len(child))
	}
	for i, want := range parent {
		if child[i].Role != want.Role || child[i].Content != want.Content {
			t.Fatalf("child[%d] = (%s, %q), want (%s, %q)",
				i, child[i].Role, child[i].Content, want.Role, want.Content)
		}
		if child[i].SessionID != "sess-child" {
			t.Fatalf("child[%d].SessionID = %q, want sess-child", i, child[i].SessionID)
		}
		if child[i].ChatID != want.ChatID {
			t.Fatalf("child[%d].ChatID = %q, want %q", i, child[i].ChatID, want.ChatID)
		}
		if child[i].MetaJSON != want.MetaJSON {
			t.Fatalf("child[%d].MetaJSON = %q, want %q", i, child[i].MetaJSON, want.MetaJSON)
		}
	}

	other, err := loadTurns(context.Background(), store.DB(), "sess-other")
	if err != nil {
		t.Fatalf("loadTurns other: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("unrelated session leaked or shrank: got %d turns, want 1", len(other))
	}
}

func TestForkTranscript_EmptyParentReturnsZero(t *testing.T) {
	store := openTranscriptStore(t)
	defer store.Close(context.Background())

	copied, err := ForkTurns(context.Background(), store.DB(), "sess-empty", "sess-child")
	if err != nil {
		t.Fatalf("ForkTurns empty parent: %v", err)
	}
	if copied != 0 {
		t.Fatalf("ForkTurns copied = %d, want 0 for empty parent", copied)
	}

	if turns, err := loadTurns(context.Background(), store.DB(), "sess-child"); err != nil {
		t.Fatalf("loadTurns child: %v", err)
	} else if len(turns) != 0 {
		t.Fatalf("child has %d turns, want 0", len(turns))
	}
}

func TestForkTranscript_RejectsEmptyOrSelfParent(t *testing.T) {
	store := openTranscriptStore(t)
	defer store.Close(context.Background())

	cases := []struct {
		name           string
		parent, child  string
	}{
		{"empty parent", "", "child"},
		{"empty child", "parent", ""},
		{"self parent", "same", "same"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ForkTurns(context.Background(), store.DB(), tc.parent, tc.child); err == nil {
				t.Fatalf("ForkTurns(%q,%q) err = nil, want non-nil", tc.parent, tc.child)
			}
		})
	}
}
