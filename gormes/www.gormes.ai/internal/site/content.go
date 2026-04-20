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
		RoadmapLabel:    "§ 03 · SHIPPING STATE",
		RoadmapHeadline: "What ships now, what doesn't.",
		RoadmapPhases: []RoadmapPhase{
			{
				StatusLabel: "SHIPPED · EVOLVING",
				StatusTone:  "shipped",
				Title:       "Phase 1 — Dashboard",
				Items: []RoadmapItem{
					{Icon: "✓", Tone: "shipped", Label: "Bubble Tea TUI shell"},
					{Icon: "✓", Tone: "shipped", Label: "Kernel with 16 ms render mailbox (coalescing)"},
					{Icon: "✓", Tone: "shipped", Label: "Route-B SSE reconnect — dropped streams recover"},
					{Icon: "✓", Tone: "shipped", Label: "Wire Doctor — offline tool-registry validation"},
					{Icon: "✓", Tone: "shipped", Label: "Streaming token renderer"},
					{Icon: "◌", Tone: "ongoing", Label: "Ongoing: polish, bug fixes, TUI ergonomics"},
				},
			},
			{
				StatusLabel: "IN PROGRESS · 3/7",
				StatusTone:  "progress",
				Title:       "Phase 2 — Gateway",
				Items: []RoadmapItem{
					{Icon: "✓", Tone: "shipped", Label: "2.A Go-native tool registry + kernel tool loop"},
					{Icon: "✓", Tone: "shipped", Label: "2.B.1 Telegram adapter"},
					{Icon: "✓", Tone: "shipped", Label: "2.C Thin session persistence (bbolt)"},
					{Icon: "⏳", Tone: "pending", Label: "2.B.2+ Wider platforms (23 upstream connectors queued)"},
					{Icon: "⏳", Tone: "pending", Label: "2.D Cron / scheduled automations"},
					{Icon: "⏳", Tone: "pending", Label: "2.E Subagent delegation"},
					{Icon: "⏳", Tone: "pending", Label: "2.F Hooks + lifecycle"},
				},
			},
			{
				StatusLabel: "IN PROGRESS · 4/5",
				StatusTone:  "progress",
				Title:       "Phase 3 — Memory",
				Items: []RoadmapItem{
					{Icon: "✓", Tone: "shipped", Label: "3.A SQLite + FTS5 lattice"},
					{Icon: "✓", Tone: "shipped", Label: "3.B Ontological graph + LLM extractor"},
					{Icon: "✓", Tone: "shipped", Label: "3.C Neural recall + context injection"},
					{Icon: "✓", Tone: "shipped", Label: "3.D.5 USER.md mirror — Gormes-original, no upstream equivalent"},
					{Icon: "⏳", Tone: "pending", Label: "3.D Ollama embeddings + semantic fusion"},
				},
			},
			{
				StatusLabel: "PLANNED · 0/8",
				StatusTone:  "planned",
				Title:       "Phase 4 — Brain Transplant",
				Subtitle:    "Ships hermes-off after 4.A–4.D. Backend becomes optional.",
				Items: []RoadmapItem{
					{Icon: "⏳", Tone: "pending", Label: "4.A Provider adapters (Anthropic, Bedrock, Gemini, OpenRouter, Google Code Assist, Codex, xAI)"},
					{Icon: "⏳", Tone: "pending", Label: "4.B Context engine + compression"},
					{Icon: "⏳", Tone: "pending", Label: "4.C Native Go prompt builder"},
					{Icon: "⏳", Tone: "pending", Label: "4.D Smart model routing"},
					{Icon: "⏳", Tone: "pending", Label: "4.E Trajectory + insights"},
					{Icon: "⏳", Tone: "pending", Label: "4.F Title generation"},
					{Icon: "⏳", Tone: "pending", Label: "4.G Credentials + OAuth flows"},
					{Icon: "⏳", Tone: "pending", Label: "4.H Rate / retry / prompt caching"},
				},
			},
			{
				StatusLabel: "LATER · 0/16",
				StatusTone:  "later",
				Title:       "Phase 5 — Final Purge (100% Go)",
				Subtitle:    "Delete the last Python dependency. Ship entirely in Go.",
				Items: []RoadmapItem{
					{Icon: "⏳", Tone: "pending", Label: "5.A–5.P — tool surface, sandboxing, browser, vision, voice, skills, MCP, ACP, plugins, security, code exec, file ops, CLI, packaging. See ARCH_PLAN §7."},
				},
			},
			{
				StatusLabel: "PLANNED · 0/6",
				StatusTone:  "planned",
				Title:       "Phase 6 — Learning Loop (The Soul)",
				Subtitle:    "Compounding intelligence. The feature Hermes doesn't have.",
				Items: []RoadmapItem{
					{Icon: "⏳", Tone: "pending", Label: "6.A Complexity detector — when is a task worth learning from?"},
					{Icon: "⏳", Tone: "pending", Label: "6.B Skill extractor — LLM-assisted pattern distillation"},
					{Icon: "⏳", Tone: "pending", Label: "6.C Skill storage format — portable Markdown + metadata"},
					{Icon: "⏳", Tone: "pending", Label: "6.D Skill retrieval + matching — lexical + semantic"},
					{Icon: "⏳", Tone: "pending", Label: "6.E Feedback loop — did the skill help? adjust weight"},
					{Icon: "⏳", Tone: "pending", Label: "6.F Skill surface — TUI + Telegram browsing and editing"},
				},
			},
		},
		FooterLeft:  `Gormes v0.1.0 · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a>`,
		FooterRight: "MIT License · 2026",
	}
}
