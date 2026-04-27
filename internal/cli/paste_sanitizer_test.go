package cli

import "testing"

func TestStripLeakedBracketedPasteWrappers_CanonicalEscape(t *testing.T) {
	got := StripLeakedBracketedPasteWrappers("\x1b[200~hello\x1b[201~")
	if got != "hello" {
		t.Fatalf("StripLeakedBracketedPasteWrappers() = %q, want %q", got, "hello")
	}
}

func TestStripLeakedBracketedPasteWrappers_CaretEscape(t *testing.T) {
	got := StripLeakedBracketedPasteWrappers("^[[200~hello^[[201~")
	if got != "hello" {
		t.Fatalf("StripLeakedBracketedPasteWrappers() = %q, want %q", got, "hello")
	}
}

func TestStripLeakedBracketedPasteWrappers_DegradedBracketBoundaries(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "start and end", in: "[200~hello[201~", want: "hello"},
		{name: "after whitespace", in: "prefix [200~hello[201~ suffix", want: "prefix hello suffix"},
		{name: "ordinary inline text preserved", in: "literal[200~tag and literal[201~tag should stay", want: "literal[200~tag and literal[201~tag should stay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripLeakedBracketedPasteWrappers(tt.in)
			if got != tt.want {
				t.Fatalf("StripLeakedBracketedPasteWrappers() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripLeakedBracketedPasteWrappers_FragmentBoundaries(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "start and end", in: "00~hello01~", want: "hello"},
		{name: "after whitespace", in: "prefix 00~hello01~ suffix", want: "prefix hello suffix"},
		{name: "ordinary inline text preserved", in: "build00~tag should stay", want: "build00~tag should stay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripLeakedBracketedPasteWrappers(tt.in)
			if got != tt.want {
				t.Fatalf("StripLeakedBracketedPasteWrappers() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripLeakedBracketedPasteWrappers_MultilinePreserved(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "caret", in: "^[[200~line 1\nline 2\nline 3^[[201~", want: "line 1\nline 2\nline 3"},
		{name: "degraded bracket", in: "[200~line 1\nline 2\nline 3[201~", want: "line 1\nline 2\nline 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripLeakedBracketedPasteWrappers(tt.in)
			if got != tt.want {
				t.Fatalf("StripLeakedBracketedPasteWrappers() = %q, want %q", got, tt.want)
			}
		})
	}
}
