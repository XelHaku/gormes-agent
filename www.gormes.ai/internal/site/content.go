package site

import (
	"encoding/json"
	"html/template"
)

func binarySizeMB() string {
	if len(benchmarksJSON) == 0 {
		return "17"
	}
	var data struct {
		Binary struct {
			SizeMB string `json:"size_mb"`
		} `json:"binary"`
	}
	if err := json.Unmarshal(benchmarksJSON, &data); err != nil {
		return "17"
	}
	return data.Binary.SizeMB
}

type NavLink struct {
	Label string
	Href  string
}

type Link struct {
	Label string
	Href  string
}

type InstallStep struct {
	Label   string
	Command string
}

type FeatureCard struct {
	Title string
	Body  string
}

// RoadmapItem is one sub-phase or work item inside a RoadmapPhase.
// Icon is the glyph shown at the start of the row — "✓" (shipped),
// "⏳" (pending), or "◌" (ongoing polish).
// Tone is the CSS-class suffix used by .roadmap-item-<tone>.
// Label is typed as template.HTML so that + and · characters render
// literally (html/template would otherwise escape + to &#43;). Must
// not carry user input; DefaultPage is the only writer.
type RoadmapItem struct {
	Icon  string
	Tone  string
	Label template.HTML
}

// RoadmapPhase groups sub-phase items under one phase header.
// StatusLabel is the pill text, e.g. "SHIPPED · EVOLVING" or
// "IN PROGRESS · 3/7" — picked to convey both the state and the
// shipped-count so visitors see granularity without hunting.
// StatusTone is the CSS-class suffix used by .roadmap-status-<tone>.
// Subtitle is optional one-line context shown below the title.
type RoadmapPhase struct {
	StatusLabel string
	StatusTone  string
	Title       string
	Subtitle    string
	Items       []RoadmapItem
}

type LandingPage struct {
	Title               string
	Description         string
	Nav                 []NavLink
	HeroKicker          string
	HeroHeadline        string
	HeroLines           []string
	// HeroFilterStamp + HeroFilterLine: the stamp ("Early-stage.") reads
	// as identity in accent-colored mono caps; the body line below
	// carries the filter caveat in muted body color.
	HeroFilterStamp string
	HeroFilterLine  string
	PrimaryCTA          Link
	SecondaryCTA        Link
	InstallSteps        []InstallStep
	InstallFootnote     string
	InstallFootnoteLink string
	InstallFootnoteHref string
	DocsNote            string
	DocsLinkLabel       string
	DocsLinkHref        string

	// "Why Gormes" section: pain frame + technical fix cards.
	WhyLabel        string
	WhyPainHeadline string
	WhyPainBullets  []string
	// WhyFixSubhead introduces the fix cards as a distinct sub-block
	// within the Why-Gormes section. v19 split the previous combined
	// "Why Hermes breaks in production — and how Gormes fixes it."
	// into two scannable headers: pain block has its own headline,
	// fix cards have this subhead.
	WhyFixSubhead string
	FeatureCards  []FeatureCard

	// Roadmap section: summary block (current focus + next milestone)
	// up top, then the full phase-by-phase checklist behind a <details>
	// disclosure. RoadmapPhases comes from progress.json via
	// buildRoadmapPhases — that wiring is unchanged.
	RoadmapLabel          string
	RoadmapHeadline       string
	RoadmapCurrentFocus   []string
	RoadmapNextMilestone  string
	RoadmapDetailsSummary string
	RoadmapPhases         []RoadmapPhase
	ProgressTracker       string
	ProgressTrackerURL    string

	FooterNav []NavLink
	// FooterLeft is typed as template.HTML so it can carry the anchor
	// tag linking to the TrebuchetDynamics company site. Must not
	// carry user input; DefaultPage is the only writer.
	FooterLeft  template.HTML
	FooterRight template.HTML
}

func DefaultPage() LandingPage {
	return LandingPage{
		Title:       "Gormes — One Go Binary. No Python. No Drift.",
		Description: "A Go-native runtime for AI agents — one static binary, no Python, no virtualenvs. Built for developers who care about reliability over polish. Under construction.",
		Nav: []NavLink{
			{Label: "Roadmap", Href: "#roadmap"},
			{Label: "GitHub", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		},
		HeroKicker:   "§ 01 · OPEN SOURCE · MIT LICENSE · UNDER CONSTRUCTION",
		HeroHeadline: "One Go Binary. No Python. No Drift.",
		HeroLines: []string{
			"Gormes is a Go-native runtime for AI agents.",
			"Built to solve the operations problem — not the AI problem.",
			"One static binary. No virtualenvs. No dependency hell.",
		},
		HeroFilterStamp: "Early-stage.",
		HeroFilterLine:  "Reliability-first runtime for developers who ship agents, not demos.",
		PrimaryCTA:     Link{Label: "Install", Href: "#install"},
		SecondaryCTA:   Link{Label: "View Source", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		InstallSteps: []InstallStep{
			{Label: "1. UNIX / MACOS / TERMUX", Command: "curl -fsSL https://gormes.ai/install.sh | sh"},
			{Label: "2. WINDOWS POWERSHELL", Command: "irm https://gormes.ai/install.ps1 | iex"},
			{Label: "3. RUN", Command: "gormes"},
		},
		InstallFootnote:     "Source-backed for now. Installers manage a checkout while binary releases settle.",
		InstallFootnoteLink: "Read the installer source →",
		InstallFootnoteHref: "https://github.com/TrebuchetDynamics/gormes-agent/tree/main/scripts",
		DocsLinkLabel:       "docs.gormes.ai →",
		DocsLinkHref:        "https://docs.gormes.ai/",
		WhyLabel:            "§ 02 · WHY GORMES",
		WhyPainHeadline:     "Why Hermes breaks in production",
		WhyFixSubhead:       "How Gormes fixes it",
		WhyPainBullets: []string{
			"environments drift",
			"installs fail",
			"agents crash mid-run",
			"streams drop and lose work",
		},
		FeatureCards: []FeatureCard{
			{Title: "Single Static Binary", Body: "Zero CGO. ~" + binarySizeMB() + " MB. scp it to Termux, Alpine, a fresh VPS — it runs. No Python, no virtualenv, no Nix."},
			{Title: "No Runtime Drift", Body: "Pure Go. No runtime Node or npm, no pip, no env activation. The binary you tested is the binary that deploys."},
			{Title: "Streams That Don't Drop", Body: "Route-B reconnect treats SSE drops as recoverable, not fatal. Your agent doesn't lose work to a flaky network."},
			{Title: "Local Validation", Body: "gormes doctor --offline checks tool schemas before you burn tokens. Catch bad wiring before a model round-trip."},
		},
		RoadmapLabel:    "§ 04 · BUILD STATE",
		RoadmapHeadline: "What works today, and what's still being wired up.",
		RoadmapCurrentFocus: []string{
			"Gateway stability",
			"Memory system (Goncho/Honcho)",
			"Subagent safety and native dashboard contracts",
		},
		RoadmapNextMilestone:  "Full Go-native runtime, no Hermes",
		RoadmapDetailsSummary: "View full phase-by-phase checklist",
		ProgressTracker:       progressTrackerLabel(),
		ProgressTrackerURL:    "https://docs.gormes.ai/building-gormes/architecture_plan/",
		RoadmapPhases:         buildRoadmapPhases(loadEmbeddedProgress()),
		FooterNav: []NavLink{
			{Label: "Docs", Href: "https://docs.gormes.ai/"},
			{Label: "Company", Href: "https://trebuchetdynamics.com/"},
		},
		FooterLeft:  `Gormes v0.1.0 · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a>`,
		FooterRight: "MIT License · 2026",
	}
}
