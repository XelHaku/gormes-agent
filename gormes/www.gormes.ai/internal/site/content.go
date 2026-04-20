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
		Title:       "Gormes.ai | Run Hermes Through a Go Operator Console",
		Description: "Gormes is the Go operator shell for Hermes users: zero-CGO, Go-native tools, split Telegram edge, and honest shipping state.",
		Nav: []NavLink{
			{Label: "Run Now", Href: "#quickstart"},
			{Label: "Shipping State", Href: "#roadmap"},
			{Label: "Source", Href: "#contribute"},
			{Label: "GitHub", Href: "https://github.com/XelHaku/gormes-agent"},
		},
		HeroBadge:    "Open Source • MIT License • Zero-CGO • Go Shell Shipping Now",
		HeroHeadline: "Run Hermes Through a Go Operator Console.",
		HeroCopy: []string{
			"Stop waiting for the clean-room rewrite. Gormes already ships a Go shell, a Go-native tool loop, Route-B resilience, and a split Telegram edge.",
			"Boot it locally. Judge the surface yourself. Keep the promises honest.",
		},
		PrimaryCTA:   Link{Label: "Boot Gormes", Href: "#quickstart"},
		SecondaryCTA: Link{Label: "See Shipping State", Href: "#roadmap"},
		TertiaryCTA:  Link{Label: "Inspect Source", Href: "https://github.com/XelHaku/gormes-agent"},
		PhaseNote:    "Current boundary: the Go shell ships now. Transcript memory stays on the later cutover path.",
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
				Kicker: "Operator Shell",
				Title:  "Go Shell Shipping Now",
				Body:   "The terminal shell is live, Go-native, and honest about the current boundary while the later memory cutover stays separate.",
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
			{Label: "Read ARCH_PLAN.md", Href: "https://github.com/XelHaku/gormes-agent/blob/main/gormes/docs/ARCH_PLAN.md"},
			{Label: "Browse the Gormes source", Href: "https://github.com/XelHaku/gormes-agent/tree/main/gormes"},
			{Label: "Open the implementation docs", Href: "https://github.com/XelHaku/gormes-agent/tree/main/gormes/docs/superpowers"},
		},
		FooterLinks: []Link{
			{Label: "GitHub", Href: "https://github.com/XelHaku/gormes-agent"},
			{Label: "ARCH_PLAN", Href: "https://github.com/XelHaku/gormes-agent/blob/main/gormes/docs/ARCH_PLAN.md"},
			{Label: "Hermes Upstream", Href: "https://github.com/NousResearch/hermes-agent"},
			{Label: "MIT License", Href: "https://github.com/XelHaku/gormes-agent/blob/main/LICENSE"},
		},
		FooterLine: "Gormes already ships the moat layers. The remaining work is the memory lattice and the brain cutover.",
	}
}
