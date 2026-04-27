package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

func TestConversationViewportTail_OmitsEarlierHistory(t *testing.T) {
	history := make([]hermes.Message, 0, 120)
	for i := 0; i < 120; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history = append(history, hermes.Message{
			Role:    role,
			Content: fmt.Sprintf("turn-%03d-body-marker", i),
		})
	}

	got := conversationViewportTail(kernel.RenderFrame{History: history}, 80, 8)

	for _, want := range []string{
		"turn-116-body-marker",
		"turn-117-body-marker",
		"turn-118-body-marker",
		"turn-119-body-marker",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("conversationViewportTail() missing latest turn %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "turn-000-body-marker") {
		t.Fatalf("conversationViewportTail() included earliest turn body in:\n%s", got)
	}
	if !strings.Contains(got, "116 earlier history messages omitted") {
		t.Fatalf("conversationViewportTail() missing deterministic omitted-history sentinel in:\n%s", got)
	}
}

func TestConversationViewportTail_AlwaysIncludesDraftAndLastError(t *testing.T) {
	history := make([]hermes.Message, 0, 80)
	for i := 0; i < 80; i++ {
		history = append(history, hermes.Message{
			Role:    "user",
			Content: fmt.Sprintf("history-%03d", i),
		})
	}
	frame := kernel.RenderFrame{
		History:   history,
		DraftText: "streaming draft survives clipping",
		LastError: "last error survives clipping",
	}

	got := conversationViewportTail(frame, 72, 6)

	for _, want := range []string{
		"streaming draft survives clipping",
		"last error survives clipping",
		"earlier history messages omitted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("conversationViewportTail() missing %q in:\n%s", want, got)
		}
	}
}

func TestConversationViewportTail_HeightAndWidthClamp(t *testing.T) {
	frame := kernel.RenderFrame{
		History: []hermes.Message{
			{Role: "user", Content: "earliest body should stay hidden"},
			{Role: "assistant", Content: "latest tiny terminal marker"},
		},
	}

	for _, width := range []int{3, 5} {
		got := conversationViewportTail(frame, width, 1)

		if !strings.Contains(got, "latest") {
			t.Fatalf("conversationViewportTail(width=%d) did not render compact latest-message view in:\n%s", width, got)
		}
		if strings.Contains(got, "earliest body should stay hidden") {
			t.Fatalf("conversationViewportTail(width=%d) included clipped earliest body in:\n%s", width, got)
		}
		if !strings.Contains(got, "1 earlier history messages omitted") {
			t.Fatalf("conversationViewportTail(width=%d) missing tiny-view sentinel in:\n%s", width, got)
		}
		if gotLines := renderedLineCount(got); gotLines > 4 {
			t.Fatalf("conversationViewportTail(width=%d) rendered %d lines in tiny view, want <= 4:\n%s", width, gotLines, got)
		}
	}
}

func TestConversationViewportTail_RenderedLineBudget(t *testing.T) {
	history := make([]hermes.Message, 0, 120)
	for i := 0; i < 120; i++ {
		history = append(history, hermes.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("budget-turn-%03d", i),
		})
	}
	frame := kernel.RenderFrame{
		History:   history,
		DraftText: "budget draft evidence",
		LastError: "budget error evidence",
	}
	const height = 8

	got := conversationViewportTail(frame, 80, height)

	for _, want := range []string{
		"budget-turn-119",
		"budget draft evidence",
		"budget error evidence",
		"earlier history messages omitted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("conversationViewportTail() missing %q in:\n%s", want, got)
		}
	}
	if gotLines, maxLines := renderedLineCount(got), height+3; gotLines > maxLines {
		t.Fatalf("conversationViewportTail() rendered %d lines, want <= %d:\n%s", gotLines, maxLines, got)
	}
}
