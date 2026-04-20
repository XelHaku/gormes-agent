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
		Description: "Gormes is the operational moat for Hermes: a 7.9 MB zero-CGO TUI, Go-native tools, Telegram Scout, and Route-B resilience.",
		Nav: []NavLink{
			{Label: "Quick Start", Href: "#quickstart"},
			{Label: "Roadmap", Href: "#roadmap"},
			{Label: "Contribute", Href: "#contribute"},
			{Label: "GitHub", Href: "https://github.com/XelHaku/golang-hermes-agent"},
		},
		HeroBadge:    "Open Source • MIT License • 7.9 MB Static Binary • Zero-CGO",
		HeroHeadline: "The Agent That GOes With You.",
		HeroCopy: []string{
			"You already love Hermes. Now run it through a surgical Go host that cuts startup tax, isolates platform adapters, and keeps the hot path typed.",
			"Today's trunk ships a Go-native tool registry, Route-B reconnect, a 16 ms replace-latest kernel mailbox, and a split-binary Telegram Scout. Phase 2.C adds thin bbolt session resume without pretending the SQLite memory lattice has already landed.",
		},
		PrimaryCTA:   Link{Label: "Run Gormes", Href: "#quickstart"},
		SecondaryCTA: Link{Label: "Read the Roadmap", Href: "#roadmap"},
		TertiaryCTA:  Link{Label: "View on GitHub", Href: "https://github.com/XelHaku/golang-hermes-agent"},
		PhaseNote:    "Phase 2 is live on trunk: 2.A Tool Registry, 2.B.1 Telegram Scout, and 2.C thin bbolt resume are shipped. Python still owns transcript memory until Phase 3.",
		QuickStart: []CodeBlock{
			{
				Title: "1. Start your Hermes backend",
				Lines: []string{"API_SERVER_ENABLED=true hermes gateway start"},
			},
			{
				Title: "2. Build the Go binaries",
				Lines: []string{"cd gormes", "make build"},
			},
			{
				Title: "3. Validate and run the current surfaces",
				Lines: []string{"./bin/gormes doctor --offline", "./bin/gormes", "GORMES_TELEGRAM_TOKEN=... GORMES_TELEGRAM_CHAT_ID=123456789 ./bin/gormes-telegram"},
			},
		},
		DemoTitle: "Tool-capable kernel. Honest current surface.",
		DemoLines: []string{
			"$ ./bin/gormes",
			"",
			"Gormes",
			"❯ Verify the registry is alive. Give me the current UTC time and a random canary number.",
			"",
			"  status   connected to Hermes backend",
			"  tool     now",
			"  tool     rand_int",
			"",
			"UTC time captured and canary 731 generated.",
		},
		FeatureCards: []FeatureCard{
			{
				Kicker: "Operational Moat",
				Title:  "7.9 MB zero-CGO TUI",
				Body:   "The terminal binary stays small, static, and isolated from platform SDK drift while the hot path remains Go-native.",
			},
			{
				Kicker: "Tool Registry",
				Title:  "Go-native tool loop",
				Body:   "The kernel accumulates streamed tool calls, executes built-ins in-process, and lets doctor validate the schema surface before tokens are spent.",
			},
			{
				Kicker: "Route-B",
				Title:  "Chaos-resilient turns",
				Body:   "Dropped SSE streams are treated as an engineering problem. Reconnect and coalescing keep the latest useful state alive under pressure.",
			},
			{
				Kicker: "Telegram Scout",
				Title:  "Split-binary platform hand",
				Body:   "Telegram lives in its own binary with a 1-second edit coalescer, preserving dependency, crash, and binary-size isolation from the TUI.",
			},
			{
				Kicker: "Phase 2.C",
				Title:  "Thin resume, honest scope",
				Body:   "bbolt stores only session handles. Python still owns transcript memory and prompt assembly until the Phase 3 lattice and later brain work land.",
			},
		},
		RoadmapIntro: "Gormes is not a mockup. Phase 1 established the kernel shell; Phase 2 has already shipped Go-native tools, Telegram, and thin session resume. Phase 3 is still the real memory handoff.",
		Phases: []Phase{
			{
				Name: "Phase 1 — The Dashboard",
				Body: "Complete. A Go Bubble Tea interface over the existing Hermes backend with the deterministic kernel, Route-B resilience, and the moat story in place.",
			},
			{
				Name: "Phase 2 — The Gateway",
				Body: "In progress. 2.A Tool Registry, 2.B.1 Telegram Scout, and 2.C thin bbolt session mapping are already shipped on trunk.",
			},
			{
				Name: "Phase 3 — Memory",
				Body: "Planned. SQLite + FTS5 transcript memory and the real lattice live here; the current bbolt layer stores only session handles.",
			},
			{
				Name: "Phase 4 — The Brain",
				Body: "Planned. Native agent orchestration and prompt-building move into Go after the memory boundary is real.",
			},
			{
				Name: "Phase 5 — The Final Purge",
				Body: "Remaining Python dependencies are removed and Hermes runs as a fully native Go system.",
			},
		},
		ContributorTitle: "Help Finish the Port",
		ContributorBody:  "Phase 2 is active on trunk. The next hard problems are finishing the wiring harness, landing the SQLite memory lattice, and cutting Python out of the brain path.",
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
		FooterLine: "Gormes already ships the moat layers. The remaining work is the memory lattice and the brain cutover.",
	}
}
