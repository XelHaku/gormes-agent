package site

type NavLink struct {
	Label string
	Href  string
}

type Link struct {
	Label string
	Href  string
}

type CodeBlock struct {
	Title string
	Lines []string
}

type FeatureCard struct {
	Kicker string
	Title  string
	Body   string
}

type Phase struct {
	Name string
	Body string
}

type LandingPage struct {
	Title            string
	Description      string
	Nav              []NavLink
	HeroBadge        string
	HeroHeadline     string
	HeroCopy         []string
	PrimaryCTA       Link
	SecondaryCTA     Link
	TertiaryCTA      Link
	PhaseNote        string
	QuickStart       []CodeBlock
	DemoTitle        string
	DemoLines        []string
	FeatureCards     []FeatureCard
	RoadmapIntro     string
	Phases           []Phase
	ContributorTitle string
	ContributorBody  string
	ContributorLinks []Link
	FooterLinks      []Link
	FooterLine       string
}

func DefaultPage() LandingPage {
	return LandingPage{
		Title:       "Gormes.ai | The Agent That GOes With You.",
		Description: "Gormes is the Phase 1 Go frontend for Hermes Agent: a faster terminal today and a public path to a pure-Go stack tomorrow.",
		Nav: []NavLink{
			{Label: "Quick Start", Href: "#quickstart"},
			{Label: "Roadmap", Href: "#roadmap"},
			{Label: "Contribute", Href: "#contribute"},
			{Label: "GitHub", Href: "https://github.com/XelHaku/golang-hermes-agent"},
		},
		HeroBadge:    "Open Source • MIT License • Phase 1 Go Port",
		HeroHeadline: "The Agent That GOes With You.",
		HeroCopy: []string{
			"You already love Hermes. Now run it through a faster, lighter Go terminal.",
			"Gormes is the Phase 1 Go frontend for Hermes Agent: a Bubble Tea dashboard and CLI facade that connects to your existing Python Hermes backend. Same agent. Same memory. Same workflows. A sharper terminal today, with the rewrite underway.",
		},
		PrimaryCTA:   Link{Label: "Run Gormes", Href: "#quickstart"},
		SecondaryCTA: Link{Label: "Read the Roadmap", Href: "#roadmap"},
		TertiaryCTA:  Link{Label: "View on GitHub", Href: "https://github.com/XelHaku/golang-hermes-agent"},
		PhaseNote:    "Phase 1 uses your existing Hermes backend. Pure single-binary Go arrives later in the roadmap.",
		QuickStart: []CodeBlock{
			{
				Title: "1. Start your Hermes backend",
				Lines: []string{"API_SERVER_ENABLED=true hermes gateway start"},
			},
			{
				Title: "2. Build and run Gormes",
				Lines: []string{"cd gormes", "make build", "./bin/gormes"},
			},
		},
		DemoTitle: "Same Hermes workflows. Sharper terminal.",
		DemoLines: []string{
			"$ API_SERVER_ENABLED=true hermes gateway start",
			"$ ./bin/gormes",
			"",
			"Gormes",
			"❯ Review the open PR and summarize the risks",
			"",
			"  status   connected to Hermes backend",
			"  tool     git diff main...feature-branch",
			"  tool     scripts/run_tests.sh tests/gateway/",
			"  tool     write_file ./notes/pr-review.md",
			"",
			"Found 2 risks and saved a review summary.",
		},
		FeatureCards: []FeatureCard{
			{
				Kicker: "Phase 1",
				Title:  "Same Hermes brain",
				Body:   "Keep the Python Hermes backend you already trust. Gormes upgrades the terminal surface first without rewriting the agent core out from under you.",
			},
			{
				Kicker: "Go UI",
				Title:  "Faster terminal",
				Body:   "Bubble Tea gives Gormes a tighter, lighter terminal feel for the workflows you already run every day.",
			},
			{
				Kicker: "Upgrade Path",
				Title:  "Drop-in adoption",
				Body:   "This is for current Hermes users. Same stack, same habits, less friction in the terminal.",
			},
			{
				Kicker: "Roadmap",
				Title:  "Honest migration",
				Body:   "Phase 1 ships today. Phases 2 through 5 move the gateway, memory, and agent core into Go in public.",
			},
			{
				Kicker: "Builders",
				Title:  "Built for contributors",
				Body:   "The port has clear seams and explicit phases, which makes it a serious target for Go developers who want to help finish the rewrite.",
			},
		},
		RoadmapIntro: "Gormes is not a mockup and not a futureware landing page. Phase 1 ships the Go user interface first, then each layer of Hermes moves across until the stack is pure Go.",
		Phases: []Phase{
			{
				Name: "Phase 1 — The Dashboard",
				Body: "A Go Bubble Tea interface over the existing Hermes backend. Faster terminal rendering, cleaner interaction loop, minimal migration risk.",
			},
			{
				Name: "Phase 2 — The Gateway",
				Body: "Platform adapters move into Go so the wiring layer no longer depends on Python.",
			},
			{
				Name: "Phase 3 — Memory",
				Body: "Persistence and recall move into Go, replacing the Python-owned state layer.",
			},
			{
				Name: "Phase 4 — The Brain",
				Body: "Agent orchestration and prompt-building move into Go. This is where the single-binary future starts to become real.",
			},
			{
				Name: "Phase 5 — The Final Purge",
				Body: "Remaining Python dependencies are removed and Hermes runs as a fully native Go system.",
			},
		},
		ContributorTitle: "Help Finish the Port",
		ContributorBody:  "Phase 1 is the user-facing proof. The next phases move the gateway, memory, and agent core into Go. If you want to help build a serious Go-native agent stack, this is the seam to join.",
		ContributorLinks: []Link{
			{Label: "Read ARCH_PLAN.md", Href: "https://github.com/XelHaku/golang-hermes-agent/blob/main/gormes/docs/ARCH_PLAN.md"},
			{Label: "Browse the Gormes source", Href: "https://github.com/XelHaku/golang-hermes-agent/tree/main/gormes"},
			{Label: "Open the implementation docs", Href: "https://github.com/XelHaku/golang-hermes-agent/tree/main/gormes/docs/superpowers"},
		},
		FooterLinks: []Link{
			{Label: "GitHub", Href: "https://github.com/XelHaku/golang-hermes-agent"},
			{Label: "ARCH_PLAN", Href: "https://github.com/XelHaku/golang-hermes-agent/blob/main/gormes/docs/ARCH_PLAN.md"},
			{Label: "Hermes Upstream", Href: "https://github.com/NousResearch/hermes-agent"},
			{Label: "MIT License", Href: "https://github.com/XelHaku/golang-hermes-agent/blob/main/LICENSE"},
		},
		FooterLine: "Gormes is the terminal upgrade for Hermes users today, and the public path to a pure-Go Hermes tomorrow.",
	}
}
