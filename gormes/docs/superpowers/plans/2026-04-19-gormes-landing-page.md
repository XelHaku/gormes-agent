# Gormes Landing Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Go-rendered `www.gormes.io` landing page that targets existing Hermes users first, explains the Phase 1 Go port honestly, and invites contributors into the public roadmap without requiring JavaScript.

**Architecture:** Build `www.gormes.io` as a standalone, stdlib-only Go module that serves one server-rendered route (`/`) plus embedded static assets from an `embed.FS`. Keep the approved marketing copy in typed Go structs, render it with `html/template` partials, and harden the site with focused handler tests plus `gormes/docs` coverage for the design and plan artifacts.

**Tech Stack:** Go 1.22+, stdlib `net/http`, `html/template`, `embed`, `io/fs`, `flag`, `log/slog`, plain CSS, `go test`, existing Goldmark-based docs tests in `gormes/docs`.

---

## File Structure

Commands below assume you start at the repository root: `golang-hermes-agent/`.

- Create: `www.gormes.io/.gitignore`
  - Ignore `bin/` so local builds do not dirty the worktree.
- Create: `www.gormes.io/go.mod`
  - Standalone module for the public site, isolated from the `gormes/` application module.
- Create: `www.gormes.io/Makefile`
  - Standard `build`, `run`, `test`, `fmt`, and `clean` shortcuts.
- Create: `www.gormes.io/cmd/www-gormes/main.go`
  - Binary entrypoint that starts the HTTP server.
- Create: `www.gormes.io/internal/site/server.go`
  - Route setup and request handling for `/` and `/static/*`.
- Create: `www.gormes.io/internal/site/content.go`
  - Typed landing-page content model and approved Phase 1 copy.
- Create: `www.gormes.io/internal/site/assets.go`
  - `embed.FS` plumbing for templates and static assets.
- Create: `www.gormes.io/internal/site/server_smoke_test.go`
  - First smoke test for `GET /`.
- Create: `www.gormes.io/internal/site/render_test.go`
  - Tests for hero, quick start, roadmap, and contributor copy.
- Create: `www.gormes.io/internal/site/assets_test.go`
  - Tests for embedded CSS, script-free HTML, and `404` behavior.
- Create: `www.gormes.io/templates/layout.tmpl`
  - Document shell, nav, footer, stylesheet link.
- Create: `www.gormes.io/templates/index.tmpl`
  - Main landing-page sections in approved order.
- Create: `www.gormes.io/templates/partials/code_block.tmpl`
  - Reusable command block rendering.
- Create: `www.gormes.io/templates/partials/feature_card.tmpl`
  - Reusable feature card rendering.
- Create: `www.gormes.io/templates/partials/phase_item.tmpl`
  - Reusable roadmap phase rendering.
- Create: `www.gormes.io/static/site.css`
  - CSS variables, layout, typography, and motion.
- Create: `www.gormes.io/README.md`
  - Local development and deploy notes for the landing page.
- Create: `gormes/docs/landing_page_docs_test.go`
  - Assert the docs test target list includes the landing-page design and plan docs.
- Modify: `gormes/docs/docs_test.go`
  - Add the new landing-page design and plan files to Goldmark coverage.

The module stays intentionally small. There is no client-side framework, no API layer, no CMS, and no JavaScript asset pipeline.

### Task 1: Bootstrap the Standalone Go Site

**Files:**
- Create: `www.gormes.io/.gitignore`
- Create: `www.gormes.io/go.mod`
- Create: `www.gormes.io/Makefile`
- Create: `www.gormes.io/cmd/www-gormes/main.go`
- Create: `www.gormes.io/internal/site/server.go`
- Test: `www.gormes.io/internal/site/server_smoke_test.go`

- [ ] **Step 1: Write the failing test**

Create `www.gormes.io/internal/site/server_smoke_test.go`:

```go
package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_IndexRenders200(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "Gormes") {
		t.Fatalf("body %q does not mention Gormes", rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd www.gormes.io && go test ./... -run TestServer_IndexRenders200`

Expected: FAIL with module/bootstrap errors such as `go: go.mod file not found` or compile errors such as `undefined: NewServer`.

- [ ] **Step 3: Write minimal implementation**

Create `www.gormes.io/.gitignore`:

```gitignore
bin/
```

Create `www.gormes.io/go.mod`:

```go
module github.com/XelHaku/golang-hermes-agent/www.gormes.io

go 1.22

toolchain go1.26.1
```

Create `www.gormes.io/Makefile`:

```make
.PHONY: build run test fmt clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/www-gormes ./cmd/www-gormes

run:
	go run ./cmd/www-gormes -listen :8080

test:
	go test ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
```

Create `www.gormes.io/cmd/www-gormes/main.go`:

```go
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/XelHaku/golang-hermes-agent/www.gormes.io/internal/site"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	handler, err := site.NewServer()
	if err != nil {
		slog.Error("build server", "err", err)
		os.Exit(1)
	}

	slog.Info("www.gormes.io listening", "addr", *listen)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
```

Create `www.gormes.io/internal/site/server.go`:

```go
package site

import (
	"io"
	"net/http"
)

func NewServer() (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html lang=\"en\"><body><h1>Gormes</h1></body></html>")
	})
	return mux, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd www.gormes.io && go test ./... -run TestServer_IndexRenders200`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add www.gormes.io/.gitignore www.gormes.io/go.mod www.gormes.io/Makefile www.gormes.io/cmd/www-gormes/main.go www.gormes.io/internal/site/server.go www.gormes.io/internal/site/server_smoke_test.go
git commit -m "feat(gormes-www): bootstrap standalone Go landing page server"
```

### Task 2: Render the Approved Landing-Page Content

**Files:**
- Create: `www.gormes.io/internal/site/content.go`
- Create: `www.gormes.io/internal/site/assets.go`
- Create: `www.gormes.io/internal/site/render_test.go`
- Create: `www.gormes.io/templates/layout.tmpl`
- Create: `www.gormes.io/templates/index.tmpl`
- Create: `www.gormes.io/templates/partials/code_block.tmpl`
- Create: `www.gormes.io/templates/partials/feature_card.tmpl`
- Create: `www.gormes.io/templates/partials/phase_item.tmpl`
- Modify: `www.gormes.io/internal/site/server.go`

- [ ] **Step 1: Write the failing test**

Create `www.gormes.io/internal/site/render_test.go`:

```go
package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_RendersApprovedPhase1Story(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	wants := []string{
		"The Agent That GOes With You.",
		"Open Source • MIT License • Phase 1 Go Port",
		"API_SERVER_ENABLED=true hermes gateway start",
		"./bin/gormes",
		"Phase 1 uses your existing Hermes backend.",
		"The Port Is Already Moving",
		"Help Finish the Port",
		"Same agent. Same memory. Same workflows.",
	}

	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q\nbody:\n%s", want, body)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd www.gormes.io && go test ./... -run TestServer_RendersApprovedPhase1Story`

Expected: FAIL because the minimal inline HTML from Task 1 does not contain the approved hero, quick start, roadmap, or contributor copy.

- [ ] **Step 3: Write minimal implementation**

Create `www.gormes.io/internal/site/content.go`:

```go
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
	Title           string
	Description     string
	Nav             []NavLink
	HeroBadge       string
	HeroHeadline    string
	HeroCopy        []string
	PrimaryCTA      Link
	SecondaryCTA    Link
	TertiaryCTA     Link
	PhaseNote       string
	QuickStart      []CodeBlock
	DemoTitle       string
	DemoLines       []string
	FeatureCards    []FeatureCard
	RoadmapIntro    string
	Phases          []Phase
	ContributorTitle string
	ContributorBody string
	ContributorLinks []Link
	FooterLinks     []Link
	FooterLine      string
}

func DefaultPage() LandingPage {
	return LandingPage{
		Title:       "Gormes.io | The Agent That GOes With You.",
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
```

Create `www.gormes.io/internal/site/assets.go`:

```go
package site

import (
	"embed"
	"html/template"
)

//go:embed templates/*.tmpl templates/partials/*.tmpl
var templateFS embed.FS

func parseTemplates() (*template.Template, error) {
	return template.ParseFS(
		templateFS,
		"templates/*.tmpl",
		"templates/partials/*.tmpl",
	)
}
```

Replace `www.gormes.io/internal/site/server.go` with:

```go
package site

import (
	"html/template"
	"net/http"
)

type Server struct {
	page      LandingPage
	templates *template.Template
}

func NewServer() (http.Handler, error) {
	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	srv := &Server{
		page:      DefaultPage(),
		templates: templates,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	return mux, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "layout", s.page); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
		return
	}
}
```

Create `www.gormes.io/templates/layout.tmpl`:

```gotemplate
{{define "layout"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
</head>
<body>
  <header id="top">
    <nav aria-label="Primary">
      <a href="#top">Gormes</a>
      {{range .Nav}}
      <a href="{{.Href}}">{{.Label}}</a>
      {{end}}
    </nav>
  </header>

  <main>
    {{template "index" .}}
  </main>

  <footer>
    <p>{{.FooterLine}}</p>
    <ul>
      {{range .FooterLinks}}
      <li><a href="{{.Href}}">{{.Label}}</a></li>
      {{end}}
    </ul>
  </footer>
</body>
</html>
{{end}}
```

Create `www.gormes.io/templates/index.tmpl`:

```gotemplate
{{define "index"}}
<section aria-labelledby="hero-title">
  <p>Hermes<br>Agent</p>
  <p>{{.HeroBadge}}</p>
  <h1 id="hero-title">{{.HeroHeadline}}</h1>
  {{range .HeroCopy}}
  <p>{{.}}</p>
  {{end}}
  <p>
    <a href="{{.PrimaryCTA.Href}}">{{.PrimaryCTA.Label}}</a>
    <a href="{{.SecondaryCTA.Href}}">{{.SecondaryCTA.Label}}</a>
    <a href="{{.TertiaryCTA.Href}}">{{.TertiaryCTA.Label}}</a>
  </p>
  <p>{{.PhaseNote}}</p>
</section>

<section id="quickstart" aria-labelledby="quickstart-title">
  <h2 id="quickstart-title">Quick Start</h2>
  {{range .QuickStart}}
    {{template "code_block" .}}
  {{end}}
</section>

<section aria-labelledby="demo-title">
  <h2 id="demo-title">{{.DemoTitle}}</h2>
  <pre><code>{{range .DemoLines}}{{.}}
{{end}}</code></pre>
</section>

<section aria-labelledby="features-title">
  <h2 id="features-title">What Changes Today</h2>
  {{range .FeatureCards}}
    {{template "feature_card" .}}
  {{end}}
</section>

<section id="roadmap" aria-labelledby="roadmap-title">
  <h2 id="roadmap-title">The Port Is Already Moving</h2>
  <p>{{.RoadmapIntro}}</p>
  {{range .Phases}}
    {{template "phase_item" .}}
  {{end}}
</section>

<section id="contribute" aria-labelledby="contribute-title">
  <h2 id="contribute-title">{{.ContributorTitle}}</h2>
  <p>{{.ContributorBody}}</p>
  <ul>
    {{range .ContributorLinks}}
    <li><a href="{{.Href}}">{{.Label}}</a></li>
    {{end}}
  </ul>
</section>
{{end}}
```

Create `www.gormes.io/templates/partials/code_block.tmpl`:

```gotemplate
{{define "code_block"}}
<article>
  <h3>{{.Title}}</h3>
  <pre><code>{{range .Lines}}{{.}}
{{end}}</code></pre>
</article>
{{end}}
```

Create `www.gormes.io/templates/partials/feature_card.tmpl`:

```gotemplate
{{define "feature_card"}}
<article>
  <p>{{.Kicker}}</p>
  <h3>{{.Title}}</h3>
  <p>{{.Body}}</p>
</article>
{{end}}
```

Create `www.gormes.io/templates/partials/phase_item.tmpl`:

```gotemplate
{{define "phase_item"}}
<article>
  <h3>{{.Name}}</h3>
  <p>{{.Body}}</p>
</article>
{{end}}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd www.gormes.io && go test ./... -run TestServer_RendersApprovedPhase1Story`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add www.gormes.io/internal/site/content.go www.gormes.io/internal/site/assets.go www.gormes.io/internal/site/render_test.go www.gormes.io/internal/site/server.go www.gormes.io/templates/layout.tmpl www.gormes.io/templates/index.tmpl www.gormes.io/templates/partials/code_block.tmpl www.gormes.io/templates/partials/feature_card.tmpl www.gormes.io/templates/partials/phase_item.tmpl
git commit -m "feat(gormes-www): render approved Phase 1 landing page copy"
```

### Task 3: Embed CSS, Serve Static Assets, and Harden the Routes

**Files:**
- Create: `www.gormes.io/internal/site/assets_test.go`
- Create: `www.gormes.io/static/site.css`
- Modify: `www.gormes.io/internal/site/assets.go`
- Modify: `www.gormes.io/internal/site/server.go`
- Modify: `www.gormes.io/templates/layout.tmpl`

- [ ] **Step 1: Write the failing test**

Create `www.gormes.io/internal/site/assets_test.go`:

```go
package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_ServesEmbeddedCSS(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/static/site.css", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("content-type = %q, want text/css", ct)
	}
	if !strings.Contains(rr.Body.String(), "--page-bg") {
		t.Fatalf("css is missing expected design variables")
	}
}

func TestServer_IndexLinksCSSAndAvoidsScripts(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `href="/static/site.css"`) {
		t.Fatalf("index is missing stylesheet link\n%s", body)
	}
	if strings.Contains(strings.ToLower(body), "<script") {
		t.Fatalf("index must not require JavaScript\n%s", body)
	}
}

func TestServer_UnknownRoutesReturn404(t *testing.T) {
	handler, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest("GET", "/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 404 {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd www.gormes.io && go test ./... -run 'TestServer_ServesEmbeddedCSS|TestServer_IndexLinksCSSAndAvoidsScripts|TestServer_UnknownRoutesReturn404'`

Expected: FAIL because `/static/site.css` does not exist yet and the rendered HTML does not link a stylesheet.

- [ ] **Step 3: Write minimal implementation**

Replace `www.gormes.io/internal/site/assets.go` with:

```go
package site

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed templates/*.tmpl templates/partials/*.tmpl static/*
var siteFS embed.FS

func parseTemplates() (*template.Template, error) {
	return template.ParseFS(
		siteFS,
		"templates/*.tmpl",
		"templates/partials/*.tmpl",
	)
}

func staticFS() (fs.FS, error) {
	return fs.Sub(siteFS, "static")
}
```

Replace `www.gormes.io/internal/site/server.go` with:

```go
package site

import (
	"html/template"
	"net/http"
)

type Server struct {
	page      LandingPage
	templates *template.Template
}

func NewServer() (http.Handler, error) {
	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	files, err := staticFS()
	if err != nil {
		return nil, err
	}

	srv := &Server{
		page:      DefaultPage(),
		templates: templates,
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(files)))
	mux.HandleFunc("/", srv.handleIndex)
	return mux, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "layout", s.page); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
		return
	}
}
```

Replace `www.gormes.io/templates/layout.tmpl` with:

```gotemplate
{{define "layout"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
  <link rel="stylesheet" href="/static/site.css">
</head>
<body>
  <div class="page-shell">
    <header class="site-header" id="top">
      <nav class="nav" aria-label="Primary">
        <a class="brand" href="#top">Gormes</a>
        <div class="nav-links">
          {{range .Nav}}
          <a href="{{.Href}}">{{.Label}}</a>
          {{end}}
        </div>
      </nav>
    </header>

    <main class="content">
      {{template "index" .}}
    </main>

    <footer class="site-footer">
      <p>{{.FooterLine}}</p>
      <ul class="footer-links">
        {{range .FooterLinks}}
        <li><a href="{{.Href}}">{{.Label}}</a></li>
        {{end}}
      </ul>
    </footer>
  </div>
</body>
</html>
{{end}}
```

Create `www.gormes.io/static/site.css`:

```css
:root {
  --page-bg: #0f1218;
  --page-glow: rgba(87, 167, 255, 0.16);
  --panel-bg: rgba(248, 242, 232, 0.06);
  --panel-border: rgba(210, 177, 88, 0.26);
  --panel-solid: #171b22;
  --text: #f4efe6;
  --muted: #b6ada1;
  --accent: #d9b86c;
  --accent-cool: #7bc4ff;
  --code-bg: #12161d;
  --shadow: 0 24px 70px rgba(0, 0, 0, 0.28);
  --radius: 24px;
  --font-body: "Avenir Next", "Segoe UI", "Helvetica Neue", "Nimbus Sans", sans-serif;
  --font-display: "Iowan Old Style", "Palatino Linotype", "Book Antiqua", Georgia, serif;
  --font-mono: "Berkeley Mono", "SFMono-Regular", Menlo, Monaco, Consolas, monospace;
}

* {
  box-sizing: border-box;
}

html {
  scroll-behavior: smooth;
}

body {
  margin: 0;
  color: var(--text);
  font-family: var(--font-body);
  background:
    radial-gradient(circle at top, var(--page-glow), transparent 32rem),
    linear-gradient(180deg, #141922 0%, var(--page-bg) 44%, #0a0d12 100%);
}

a {
  color: inherit;
  text-decoration: none;
}

.page-shell {
  width: min(1120px, calc(100% - 32px));
  margin: 0 auto;
  padding: 24px 0 56px;
}

.site-header {
  position: sticky;
  top: 0;
  z-index: 10;
  padding: 12px 0 20px;
  backdrop-filter: blur(14px);
}

.nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 14px 18px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 999px;
  background: rgba(12, 15, 21, 0.72);
  box-shadow: var(--shadow);
}

.brand {
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.nav-links {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  color: var(--muted);
}

.content {
  display: grid;
  gap: 28px;
}

section,
.site-footer {
  padding: 28px;
  border: 1px solid var(--panel-border);
  border-radius: var(--radius);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.02));
  box-shadow: var(--shadow);
}

h1,
h2,
h3 {
  margin: 0 0 12px;
  font-family: var(--font-display);
  line-height: 1.05;
}

h1 {
  font-size: clamp(3rem, 8vw, 6.25rem);
  max-width: 10ch;
}

h2 {
  font-size: clamp(2rem, 4vw, 3.1rem);
}

h3 {
  font-size: 1.25rem;
}

p,
li {
  color: var(--muted);
  line-height: 1.65;
}

code,
pre {
  font-family: var(--font-mono);
}

pre {
  margin: 0;
  padding: 18px;
  overflow-x: auto;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 18px;
  background: var(--code-bg);
  color: #edf4ff;
}

.site-header + .content section:first-child {
  animation: rise 420ms ease-out both;
}

.content section:nth-child(2),
.content section:nth-child(3),
.content section:nth-child(4),
.content section:nth-child(5),
.content section:nth-child(6) {
  animation: rise 500ms ease-out both;
}

.content section:nth-child(2) { animation-delay: 40ms; }
.content section:nth-child(3) { animation-delay: 80ms; }
.content section:nth-child(4) { animation-delay: 120ms; }
.content section:nth-child(5) { animation-delay: 160ms; }
.content section:nth-child(6) { animation-delay: 200ms; }

.content section:first-child p:first-child {
  margin: 0 0 10px;
  color: var(--text);
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.content section:first-child p:nth-child(2) {
  display: inline-flex;
  margin: 0 0 16px;
  padding: 8px 12px;
  border-radius: 999px;
  background: rgba(217, 184, 108, 0.12);
  color: var(--accent);
}

.content section:first-child > p:last-child {
  margin-top: 18px;
  color: var(--accent-cool);
}

.content section:first-child a,
.site-footer a {
  color: var(--text);
}

.content section:first-child p a:first-child {
  display: inline-flex;
  margin-right: 12px;
  padding: 12px 18px;
  border-radius: 999px;
  background: var(--accent);
  color: #11151c;
  font-weight: 700;
}

.content section:first-child p a:nth-child(2),
.content section:first-child p a:nth-child(3) {
  display: inline-flex;
  margin-right: 12px;
  padding: 12px 18px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 999px;
}

#quickstart,
#roadmap,
#contribute {
  scroll-margin-top: 96px;
}

#quickstart article,
#roadmap article,
#features-title + article,
#features-title ~ article,
#contribute ul {
  margin-top: 18px;
}

#features-title ~ article {
  padding: 18px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 18px;
  background: rgba(10, 13, 18, 0.22);
}

#features-title ~ article p:first-child {
  margin-bottom: 8px;
  color: var(--accent);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

#roadmap article {
  padding-left: 18px;
  border-left: 2px solid rgba(123, 196, 255, 0.35);
}

.footer-links,
#contribute ul {
  display: flex;
  flex-wrap: wrap;
  gap: 14px 18px;
  padding: 0;
  list-style: none;
}

@keyframes rise {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 720px) {
  .page-shell {
    width: min(100% - 20px, 1120px);
    padding-top: 16px;
  }

  .nav {
    border-radius: 24px;
    align-items: flex-start;
    flex-direction: column;
  }

  section,
  .site-footer {
    padding: 22px;
  }

  h1 {
    max-width: none;
  }

  .content section:first-child p a:first-child,
  .content section:first-child p a:nth-child(2),
  .content section:first-child p a:nth-child(3) {
    margin-bottom: 10px;
  }
}

@media (prefers-reduced-motion: reduce) {
  html {
    scroll-behavior: auto;
  }

  *,
  *::before,
  *::after {
    animation: none !important;
    transition: none !important;
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd www.gormes.io && go test ./... -run 'TestServer_ServesEmbeddedCSS|TestServer_IndexLinksCSSAndAvoidsScripts|TestServer_UnknownRoutesReturn404'`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add www.gormes.io/internal/site/assets_test.go www.gormes.io/static/site.css www.gormes.io/internal/site/assets.go www.gormes.io/internal/site/server.go www.gormes.io/templates/layout.tmpl
git commit -m "feat(gormes-www): embed css and harden asset routing"
```

### Task 4: Wire Documentation Coverage and Local Developer Notes

**Files:**
- Create: `www.gormes.io/README.md`
- Create: `gormes/docs/landing_page_docs_test.go`
- Modify: `gormes/docs/docs_test.go`

- [ ] **Step 1: Write the failing test**

Create `gormes/docs/landing_page_docs_test.go`:

```go
package docs_test

import "testing"

func TestTargetsIncludeLandingPageDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-landing-page-design.md": false,
		"superpowers/plans/2026-04-19-gormes-landing-page.md":        false,
	}

	for _, target := range targets {
		if _, ok := want[target]; ok {
			want[target] = true
		}
	}

	for rel, seen := range want {
		if !seen {
			t.Fatalf("targets missing %s", rel)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd gormes && go test ./docs -run TestTargetsIncludeLandingPageDocs`

Expected: FAIL with `targets missing superpowers/specs/2026-04-19-gormes-landing-page-design.md` and `targets missing superpowers/plans/2026-04-19-gormes-landing-page.md`.

- [ ] **Step 3: Write minimal implementation**

Create `www.gormes.io/README.md`:

````markdown
# www.gormes.io

Go-rendered landing page for Gormes.

## Stack

- Go 1.22+
- stdlib `net/http`
- stdlib `html/template`
- stdlib `embed`
- plain CSS

No JavaScript is required for the first release.

## Development

```bash
make run
```

Then open `http://127.0.0.1:8080`.

## Test

```bash
make test
```

## Build

```bash
make build
```
````

Update the `targets` slice in `gormes/docs/docs_test.go` to:

```go
var targets = []string{
	"ARCH_PLAN.md",
	"superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md",
	"superpowers/specs/2026-04-19-gormes-landing-page-design.md",
	"superpowers/plans/2026-04-18-gormes-phase1-frontend-adapter.md",
	"superpowers/plans/2026-04-19-gormes-landing-page.md",
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd gormes && go test ./docs -run 'TestTargetsIncludeLandingPageDocs|TestMarkdownRendersCleanViaGoldmark|TestMarkdownAvoidsPortabilityHazards'`

Expected: PASS

Run: `cd www.gormes.io && go test ./...`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add www.gormes.io/README.md gormes/docs/landing_page_docs_test.go gormes/docs/docs_test.go
git commit -m "docs(gormes): cover landing page design and plan docs"
```

## Final Verification

After Task 4, run the full verification sequence from the repository root:

```bash
cd www.gormes.io && go test ./...
cd ../gormes && go test ./docs
cd ../www.gormes.io && make build
cd ../www.gormes.io && go run ./cmd/www-gormes -listen :8080
```

Manual checks at `http://127.0.0.1:8080`:

1. The first viewport clearly says `Phase 1 Go Port`, `The Agent That GOes With You.`, and the Phase 1 Python-backend clarifier.
2. The quick start shows the two exact command blocks from the approved spec.
3. The roadmap includes all five phases and the section header `The Port Is Already Moving`.
4. The contributor block contains links to `ARCH_PLAN.md`, the `gormes/` source, and the implementation docs.
5. Disable JavaScript in the browser and confirm the page still renders normally.
6. Check a narrow mobile viewport and confirm the nav wraps cleanly, the hero remains readable, and code blocks still scroll horizontally instead of breaking layout.

## Spec Coverage Checklist

- Audience lock: Task 2 content model and templates encode existing-Hermes-user-first copy.
- Truthfulness lock: Task 2 hero note and roadmap copy explicitly state the Python backend remains in Phase 1.
- Go-only implementation lock: Tasks 1 and 3 use stdlib HTTP/template/embed plus plain CSS, with Task 3 asserting no `<script>` tags.
- Required sections: Task 2 renders header, hero, quick start, demo, feature grid, roadmap, contributor block, and footer.
- Testing requirements: Tasks 1 through 4 cover `/`, key copy, static assets, no-script rendering, `404`, and docs portability coverage.
