package cli

import "testing"

// Tests freeze the Go port of hermes_cli/banner.py (Phase 5.O). Only the
// deterministic, dependency-free helpers are ported at this step:
//
//   - _format_context_length(tokens) → FormatContextLength
//   - _display_toolset_name(name)    → DisplayToolsetName
//   - format_banner_version_label()  → FormatBannerVersionLabel
//
// The Rich/Console renderer and the subprocess-backed git banner state are
// intentionally out of scope here; they remain tracked under 5.O as follow-on.

func TestFormatContextLength_MatrixMatchesUpstream(t *testing.T) {
	// Reference values mirror the semantics of hermes_cli/banner.py::_format_context_length:
	// thresholds at 1_000_000 and 1_000, rounding-with-tolerance (0.05) before falling
	// back to one decimal place, and a raw decimal passthrough below 1_000.
	cases := []struct {
		name   string
		tokens int
		want   string
	}{
		{"zero tokens passthrough", 0, "0"},
		{"sub-thousand passthrough", 500, "500"},
		{"sub-thousand boundary minus one", 999, "999"},
		{"exactly one thousand rounds to K", 1000, "1K"},
		{"one-and-a-half K uses one decimal", 1500, "1.5K"},
		{"eight K clean", 8000, "8K"},
		{"one-twenty-eight K clean", 128000, "128K"},
		{"exactly one million rounds to M", 1000000, "1M"},
		{"1048576 rounds to 1M within tolerance", 1048576, "1M"},
		{"one-point-two M uses one decimal", 1200000, "1.2M"},
		{"two M clean", 2000000, "2M"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatContextLength(tc.tokens); got != tc.want {
				t.Fatalf("FormatContextLength(%d) = %q, want %q", tc.tokens, got, tc.want)
			}
		})
	}
}

func TestDisplayToolsetName_EmptyReturnsUnknown(t *testing.T) {
	if got, want := DisplayToolsetName(""), "unknown"; got != want {
		t.Fatalf("DisplayToolsetName(\"\") = %q, want %q", got, want)
	}
}

func TestDisplayToolsetName_StripsTrailingToolsSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hermes_tools", "hermes"},
		{"mcp_tools", "mcp"},
		{"code_tools", "code"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := DisplayToolsetName(tc.in); got != tc.want {
				t.Fatalf("DisplayToolsetName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDisplayToolsetName_LeavesNonMatchingNamesUnchanged(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hermes", "hermes"},
		{"tools", "tools"},             // 5 chars, doesn't end with "_tools"
		{"hermes_tool", "hermes_tool"}, // ends with "_tool", not "_tools"
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := DisplayToolsetName(tc.in); got != tc.want {
				t.Fatalf("DisplayToolsetName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatBannerVersionLabel_NoGitStateReturnsBase(t *testing.T) {
	got := FormatBannerVersionLabel("Gormes Agent", "1.2.3", "2026-04-24", nil)
	want := "Gormes Agent v1.2.3 (2026-04-24)"
	if got != want {
		t.Fatalf("FormatBannerVersionLabel(nil state) = %q, want %q", got, want)
	}
}

func TestFormatBannerVersionLabel_UpstreamEqualsLocalReturnsUpstreamOnly(t *testing.T) {
	state := &BannerGitState{Upstream: "abcdef12", Local: "abcdef12", Ahead: 0}
	got := FormatBannerVersionLabel("Gormes Agent", "1.2.3", "2026-04-24", state)
	want := "Gormes Agent v1.2.3 (2026-04-24) · upstream abcdef12"
	if got != want {
		t.Fatalf("FormatBannerVersionLabel(equal hashes) = %q, want %q", got, want)
	}
}

func TestFormatBannerVersionLabel_AheadNonPositiveReturnsUpstreamOnly(t *testing.T) {
	// Even if upstream and local differ, an ahead<=0 count means upstream is the
	// only thing worth reporting — mirrors the upstream guard.
	state := &BannerGitState{Upstream: "upstream1", Local: "local1", Ahead: 0}
	got := FormatBannerVersionLabel("Gormes Agent", "1.2.3", "2026-04-24", state)
	want := "Gormes Agent v1.2.3 (2026-04-24) · upstream upstream1"
	if got != want {
		t.Fatalf("FormatBannerVersionLabel(ahead=0) = %q, want %q", got, want)
	}
}

func TestFormatBannerVersionLabel_AheadSingleCommitUsesSingularCarriedWord(t *testing.T) {
	state := &BannerGitState{Upstream: "origmain", Local: "localsha", Ahead: 1}
	got := FormatBannerVersionLabel("Gormes Agent", "1.2.3", "2026-04-24", state)
	want := "Gormes Agent v1.2.3 (2026-04-24) · upstream origmain · local localsha (+1 carried commit)"
	if got != want {
		t.Fatalf("FormatBannerVersionLabel(ahead=1) = %q, want %q", got, want)
	}
}

func TestFormatBannerVersionLabel_AheadMultipleCommitsUsesPluralCarriedWord(t *testing.T) {
	state := &BannerGitState{Upstream: "origmain", Local: "localsha", Ahead: 3}
	got := FormatBannerVersionLabel("Gormes Agent", "1.2.3", "2026-04-24", state)
	want := "Gormes Agent v1.2.3 (2026-04-24) · upstream origmain · local localsha (+3 carried commits)"
	if got != want {
		t.Fatalf("FormatBannerVersionLabel(ahead=3) = %q, want %q", got, want)
	}
}

func TestFormatBannerVersionLabel_NegativeAheadCoercedToZero(t *testing.T) {
	// Upstream max(ahead, 0) is applied at the state-construction site; here we
	// pin the FormatBannerVersionLabel contract: a negative ahead falls through to
	// the upstream-only branch rather than producing a nonsense "+-N" label.
	state := &BannerGitState{Upstream: "origmain", Local: "localsha", Ahead: -1}
	got := FormatBannerVersionLabel("Gormes Agent", "1.2.3", "2026-04-24", state)
	want := "Gormes Agent v1.2.3 (2026-04-24) · upstream origmain"
	if got != want {
		t.Fatalf("FormatBannerVersionLabel(ahead=-1) = %q, want %q", got, want)
	}
}
