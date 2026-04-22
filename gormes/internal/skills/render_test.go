package skills

import "testing"

func TestRenderBlockStableOrder(t *testing.T) {
	block := RenderBlock([]Skill{
		{Name: "careful-review", Description: "Review changes carefully", Body: "Follow the review checklist."},
		{Name: "review-tests", Description: "Review tests and failure modes", Body: "Check assertions before implementation."},
	})

	want := "<skills>\n## careful-review\nReview changes carefully\n\nFollow the review checklist.\n\n## review-tests\nReview tests and failure modes\n\nCheck assertions before implementation.\n</skills>"
	if block != want {
		t.Fatalf("RenderBlock() = %q, want %q", block, want)
	}
}
