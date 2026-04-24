package cli

import (
	"fmt"
	"math"
	"strings"
)

// Deterministic, dependency-free helpers ported from hermes_cli/banner.py
// (Phase 5.O). The Rich console renderer, the skin-engine color overrides,
// and the subprocess-backed update check remain tracked as follow-on work
// under 5.O; only the pure formatting helpers land here so the test surface
// stays hermetic.

// FormatContextLength mirrors hermes_cli/banner.py::_format_context_length.
// It compacts a token count into a short banner label:
//
//   - tokens < 1_000: raw decimal string ("500", "0").
//   - 1_000 <= tokens < 1_000_000: value in thousands. If the value rounds
//     cleanly within a 0.05 tolerance the integer form is used ("128K");
//     otherwise one decimal place is kept ("1.5K").
//   - tokens >= 1_000_000: value in millions with the same
//     rounding-with-tolerance rule ("1M" vs "1.2M").
//
// The tolerance is strictly less than 0.05 to match the upstream
// `abs(val - rounded) < 0.05` predicate.
func FormatContextLength(tokens int) string {
	const tolerance = 0.05
	switch {
	case tokens >= 1_000_000:
		return formatScaled(float64(tokens)/1_000_000, tolerance, "M")
	case tokens >= 1_000:
		return formatScaled(float64(tokens)/1_000, tolerance, "K")
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// formatScaled applies the rounding-with-tolerance rule used by upstream
// _format_context_length. When the scaled value is within tolerance of the
// nearest integer, the integer form is returned; otherwise one decimal
// place is retained.
func formatScaled(val, tolerance float64, suffix string) string {
	rounded := math.Round(val)
	if math.Abs(val-rounded) < tolerance {
		return fmt.Sprintf("%d%s", int(rounded), suffix)
	}
	return fmt.Sprintf("%.1f%s", val, suffix)
}

// DisplayToolsetName mirrors hermes_cli/banner.py::_display_toolset_name.
// An empty input returns "unknown" (the banner never wants a blank cell);
// a trailing "_tools" suffix is stripped to produce the short label. All
// other names pass through unchanged.
func DisplayToolsetName(name string) string {
	if name == "" {
		return "unknown"
	}
	const suffix = "_tools"
	if strings.HasSuffix(name, suffix) {
		return name[:len(name)-len(suffix)]
	}
	return name
}

// BannerGitState is the deterministic input that FormatBannerVersionLabel
// consumes. Upstream populates it from `git rev-parse --short=8` against
// origin/main and HEAD; the Go port keeps it pure so tests never shell out
// to git. A nil state means no git info is available and the label falls
// back to the bare "agent vX.Y.Z (date)" form.
type BannerGitState struct {
	// Upstream is the short SHA (typically 8 chars) for origin/main.
	Upstream string
	// Local is the short SHA for HEAD.
	Local string
	// Ahead is the number of commits HEAD is ahead of origin/main. Values
	// <= 0 cause FormatBannerVersionLabel to suppress the local-carried
	// segment, matching the upstream `ahead <= 0` guard.
	Ahead int
}

// FormatBannerVersionLabel mirrors hermes_cli/banner.py::format_banner_version_label.
// It builds the banner title used by the startup banner:
//
//   - nil state → "<agent> v<version> (<releaseDate>)".
//   - state with ahead <= 0 OR upstream == local →
//     "<base> · upstream <Upstream>".
//   - state with ahead > 0 AND upstream != local →
//     "<base> · upstream <Upstream> · local <Local> (+<Ahead> carried <commit|commits>)".
//
// The agent display name, version, and release date are caller-provided so
// the Go port doesn't ship its own upstream-tied constants.
func FormatBannerVersionLabel(agent, version, releaseDate string, state *BannerGitState) string {
	base := fmt.Sprintf("%s v%s (%s)", agent, version, releaseDate)
	if state == nil {
		return base
	}
	if state.Ahead <= 0 || state.Upstream == state.Local {
		return fmt.Sprintf("%s · upstream %s", base, state.Upstream)
	}
	carried := "commits"
	if state.Ahead == 1 {
		carried = "commit"
	}
	return fmt.Sprintf("%s · upstream %s · local %s (+%d carried %s)",
		base, state.Upstream, state.Local, state.Ahead, carried)
}
