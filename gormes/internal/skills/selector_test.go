package skills

import (
	"reflect"
	"testing"
)

func TestSelectDeterministicAndCapped(t *testing.T) {
	skills := []Skill{
		{Name: "careful-review", Description: "Review changes carefully", Body: "Work step by step."},
		{Name: "review-tests", Description: "Review tests and failure modes", Body: "Check assertions first."},
		{Name: "ship-checklist", Description: "Release checklist", Body: "Cut the tag after verification."},
	}

	first := Select(skills, "please review this carefully and check tests", 2)
	second := Select(skills, "please review this carefully and check tests", 2)

	if len(first) != 2 {
		t.Fatalf("len(first) = %d, want 2", len(first))
	}
	gotFirst := skillNames(first)
	gotSecond := skillNames(second)
	want := []string{"review-tests", "careful-review"}
	if !reflect.DeepEqual(gotFirst, want) {
		t.Fatalf("skillNames(first) = %#v, want %#v", gotFirst, want)
	}
	if !reflect.DeepEqual(gotSecond, want) {
		t.Fatalf("skillNames(second) = %#v, want %#v", gotSecond, want)
	}
}
