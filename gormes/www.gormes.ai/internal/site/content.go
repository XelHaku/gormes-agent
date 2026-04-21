package site

import (
	"encoding/json"
	"html/template"
	"strconv"
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

func binarySizeMBFloat() float64 {
	if len(benchmarksJSON) == 0 {
		return 17.0
	}
	var data struct {
		Binary struct {
			SizeMB string `json:"size_mb"`
		} `json:"binary"`
	}
	if err := json.Unmarshal(benchmarksJSON, &data); err != nil {
		return 17.0
	}
	size, _ := strconv.ParseFloat(data.Binary.SizeMB, 64)
	return size
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
	HeroSubhead         string
	HeroImage           string
	PrimaryCTA          Link
	SecondaryCTA        Link
	InstallSteps        []InstallStep
	InstallFootnote     string
	InstallFootnoteLink string
	InstallFootnoteHref string
	DocsNote            string
	DocsLinkLabel       string
	DocsLinkHref        string
	FeaturesLabel       string
	FeaturesHeadline    string
	FeatureCards        []FeatureCard
	RoadmapLabel        string
	RoadmapHeadline     string
	RoadmapPhases       []RoadmapPhase
	ProgressTracker     string
	ProgressTrackerURL  string
	// FooterLeft is typed as template.HTML so it can carry the anchor
	// tag linking to the TrebuchetDynamics company site. Must not
	// carry user input; DefaultPage is the only writer.
	FooterLeft  template.HTML
	FooterRight string
}

func DefaultPage() LandingPage {
	return LandingPage{
		Title:       "Gormes — One Go Binary. Same Hermes Brain.",
		Description: "Zero-CGO Go shell for Hermes Agent. One static binary, in-process tool loop, Route-B reconnect.",
		Nav: []NavLink{
			{Label: "Install", Href: "#install"},
			{Label: "Features", Href: "#features"},
			{Label: "Docs", Href: "https://docs.gormes.ai/"},
			{Label: "Roadmap", Href: "#roadmap"},
			{Label: "GitHub", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
			{Label: "Company", Href: "https://trebuchetdynamics.com/"},
		},
		HeroKicker:   "§ 01 · OPEN SOURCE · MIT LICENSE",
		HeroHeadline: "One Go Binary. Same Hermes Brain.",
		HeroSubhead:  "A static Go binary that talks to your Hermes backend over HTTP. scp it to Termux, Alpine, a fresh VPS — Gormes adds no runtime of its own on top of what Hermes already needs.",
		HeroImage:    "/static/go-gopher-bear-lowpoly.png",
		PrimaryCTA:   Link{Label: "Install", Href: "#install"},
		SecondaryCTA: Link{Label: "View Source", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		InstallSteps: []InstallStep{
			{Label: "1. INSTALL", Command: "curl -fsSL https://gormes.ai/install.sh | sh"},
			{Label: "2. RUN", Command: "gormes"},
		},
		InstallFootnote:     "Requires Hermes backend at localhost:8642.",
		InstallFootnoteLink: "Install Hermes →",
		InstallFootnoteHref: "https://github.com/NousResearch/hermes-agent#quickstart",
		DocsNote:            "Deeper reference material lives at",
		DocsLinkLabel:       "docs.gormes.ai →",
		DocsLinkHref:        "https://docs.gormes.ai/",
		FeaturesLabel:       "§ 02 · FEATURES",
		FeaturesHeadline:    "Why a Go layer matters.",
		FeatureCards: []FeatureCard{
			{Title: "Single Static Binary", Body: "Zero CGO. ~" + binarySizeMB() + " MB. scp it to Termux, Alpine, a fresh VPS — it runs."},
			{Title: "Boots Like a Tool", Body: "No Python warmup. 16 ms render mailbox keeps the TUI responsive under load."},
			{Title: "In-Process Tool Loop", Body: "Streamed tool_calls execute against a Go-native registry. No bounce through Python."},
			{Title: "Survives Dropped Streams", Body: "Route-B reconnect treats SSE drops as a resilience problem, not a happy-path omission."},
		},
		RoadmapLabel:       "§ 03 · SHIPPING STATE",
		RoadmapHeadline:    "What ships now, what doesn't.",
		ProgressTracker:    progressTrackerLabel(),
		ProgressTrackerURL: "https://docs.gormes.ai/building-gormes/architecture_plan/",
		RoadmapPhases:      buildRoadmapPhases(loadEmbeddedProgress()),
		FooterLeft:  `Gormes v0.1.0 · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a>`,
		FooterRight: "MIT License · 2026",
	}
}
