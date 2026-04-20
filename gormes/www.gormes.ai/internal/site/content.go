package site

import "html/template"

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

// ShipState renders one row of the shipping ledger.
// State is the display label ("SHIPPED", "NEXT", "LATER").
// Tone is the lowercase CSS-class suffix used by .status-<tone>.
// Name is typed as template.HTML so that + signs in copy render literally
// (html/template would otherwise escape + to &#43;).
type ShipState struct {
	State string
	Tone  string
	Name  template.HTML
}

type LandingPage struct {
	Title               string
	Description         string
	Nav                 []NavLink
	HeroKicker          string
	HeroHeadline        string
	HeroSubhead         string
	PrimaryCTA          Link
	SecondaryCTA        Link
	InstallSteps        []InstallStep
	InstallFootnote     string
	InstallFootnoteLink string
	InstallFootnoteHref string
	FeaturesLabel       string
	FeaturesHeadline    string
	FeatureCards        []FeatureCard
	LedgerLabel         string
	LedgerHeadline      string
	ShippingStates      []ShipState
	FooterLeft          string
	FooterRight         string
}

func DefaultPage() LandingPage {
	return LandingPage{
		Title:       "Gormes — One Go Binary. Same Hermes Brain.",
		Description: "Zero-CGO Go shell for Hermes Agent. One static binary, in-process tool loop, Route-B reconnect.",
		Nav: []NavLink{
			{Label: "Install", Href: "#install"},
			{Label: "Features", Href: "#features"},
			{Label: "Roadmap", Href: "#roadmap"},
			{Label: "GitHub", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		},
		HeroKicker:   "OPEN SOURCE · MIT LICENSE",
		HeroHeadline: "One Go Binary. Same Hermes Brain.",
		HeroSubhead:  "A static Go binary that talks to your Hermes backend over HTTP. scp it to Termux, Alpine, a fresh VPS — Gormes adds no runtime of its own on top of what Hermes already needs.",
		PrimaryCTA:   Link{Label: "Install", Href: "#install"},
		SecondaryCTA: Link{Label: "View Source", Href: "https://github.com/TrebuchetDynamics/gormes-agent"},
		InstallSteps: []InstallStep{
			{Label: "1. INSTALL", Command: "curl -fsSL https://gormes.ai/install.sh | sh"},
			{Label: "2. RUN", Command: "gormes"},
		},
		InstallFootnote:     "Requires Hermes backend at localhost:8642.",
		InstallFootnoteLink: "Install Hermes →",
		InstallFootnoteHref: "https://github.com/NousResearch/hermes-agent#quickstart",
		FeaturesLabel:       "FEATURES",
		FeaturesHeadline:    "Why a Go layer matters.",
		FeatureCards: []FeatureCard{
			{Title: "Single Static Binary", Body: "Zero CGO. ~17 MB. scp it to Termux, Alpine, a fresh VPS — it runs."},
			{Title: "Boots Like a Tool", Body: "No Python warmup. 16 ms render mailbox keeps the TUI responsive under load."},
			{Title: "In-Process Tool Loop", Body: "Streamed tool_calls execute against a Go-native registry. No bounce through Python."},
			{Title: "Survives Dropped Streams", Body: "Route-B reconnect treats SSE drops as a resilience problem, not a happy-path omission."},
		},
		LedgerLabel:    "SHIPPING STATE",
		LedgerHeadline: "What ships now, what doesn't.",
		ShippingStates: []ShipState{
			{State: "SHIPPED", Tone: "shipped", Name: "Phase 1 — Bubble Tea TUI shell."},
			{State: "SHIPPED", Tone: "shipped", Name: "Phase 2.A–C — Tool registry + Telegram adapter + session resume."},
			{State: "NEXT", Tone: "next", Name: "Phase 2.B.2+ — Wider gateway (Discord, Slack, more adapters)."},
			{State: "SHIPPED", Tone: "shipped", Name: "Phase 3.A–C + 3.D.5 — SQLite + FTS5 lattice, ontological graph, neural recall, USER.md mirror."},
			{State: "NEXT", Tone: "next", Name: "Phase 3.D — Ollama embeddings + semantic fusion."},
			{State: "LATER", Tone: "later", Name: "Phase 4 — Brain transplant. Hermes backend becomes optional."},
			{State: "LATER", Tone: "later", Name: "Phase 5 — 100% Go. Python tool scripts ported. Hermes-off."},
		},
		FooterLeft:  "Gormes v0.1.0 · TrebuchetDynamics",
		FooterRight: "MIT License · 2026",
	}
}
