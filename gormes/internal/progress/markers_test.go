package progress

import (
	"strings"
	"testing"
)

func TestReplaceMarker_HappyPath(t *testing.T) {
	input := strings.Join([]string{
		"intro",
		"<!-- PROGRESS:START kind=readme-rollup -->",
		"STALE CONTENT",
		"<!-- PROGRESS:END -->",
		"outro",
	}, "\n")
	out, err := ReplaceMarker(input, "readme-rollup", "NEW CONTENT\n")
	if err != nil {
		t.Fatalf("ReplaceMarker() error = %v", err)
	}
	if !strings.Contains(out, "NEW CONTENT") {
		t.Errorf("output missing new content:\n%s", out)
	}
	if strings.Contains(out, "STALE CONTENT") {
		t.Errorf("output still contains stale content:\n%s", out)
	}
	if !strings.Contains(out, "intro") || !strings.Contains(out, "outro") {
		t.Errorf("output missing surrounding prose:\n%s", out)
	}
}

func TestReplaceMarker_MissingMarkers(t *testing.T) {
	_, err := ReplaceMarker("no markers here", "readme-rollup", "x")
	if err == nil {
		t.Errorf("want error on missing markers, got nil")
	}
}

func TestReplaceMarker_WrongKind(t *testing.T) {
	input := "<!-- PROGRESS:START kind=other -->\nx\n<!-- PROGRESS:END -->"
	_, err := ReplaceMarker(input, "readme-rollup", "x")
	if err == nil {
		t.Errorf("want error on wrong kind, got nil")
	}
}
