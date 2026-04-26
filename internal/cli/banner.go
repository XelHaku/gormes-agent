package cli

import (
	"fmt"
	"math"
	"strings"
)

// BannerVersion is the pure input model for startup banner version text.
type BannerVersion struct {
	AgentName   string
	Version     string
	ReleaseDate string
	GitState    *BannerGitState
}

// BannerGitState captures the already-resolved git facts shown in the banner.
type BannerGitState struct {
	Upstream string
	Local    string
	Ahead    int
}

// FormatContextLength formats a model context window using Hermes' compact
// K/M display rules.
func FormatContextLength(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return formatCompactTokenCount(float64(tokens)/1_000_000, "M")
	case tokens >= 1_000:
		return formatCompactTokenCount(float64(tokens)/1_000, "K")
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// DisplayToolsetName normalizes internal and legacy toolset names for display.
func DisplayToolsetName(toolsetName string) string {
	if toolsetName == "" {
		return "unknown"
	}
	return strings.TrimSuffix(toolsetName, "_tools")
}

// FormatBannerVersionLabel returns the deterministic version label used in the
// CLI startup banner. Git state is injected by callers so this helper remains
// file, subprocess, and network inert.
func FormatBannerVersionLabel(version BannerVersion) string {
	agentName := version.AgentName
	if agentName == "" {
		agentName = "Hermes Agent"
	}
	base := fmt.Sprintf("%s v%s (%s)", agentName, version.Version, version.ReleaseDate)
	if version.GitState == nil || version.GitState.Upstream == "" || version.GitState.Local == "" {
		return base
	}

	state := version.GitState
	if state.Ahead <= 0 || state.Upstream == state.Local {
		return fmt.Sprintf("%s · upstream %s", base, state.Upstream)
	}

	carriedWord := "commit"
	if state.Ahead != 1 {
		carriedWord = "commits"
	}
	return fmt.Sprintf("%s · upstream %s · local %s (+%d carried %s)", base, state.Upstream, state.Local, state.Ahead, carriedWord)
}

func formatCompactTokenCount(value float64, suffix string) string {
	rounded := math.Round(value)
	if math.Abs(value-rounded) < 0.05 {
		return fmt.Sprintf("%.0f%s", rounded, suffix)
	}
	return fmt.Sprintf("%.1f%s", value, suffix)
}
