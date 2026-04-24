# Gormes Documentation Sync and Public Manifesto Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refresh the Gormes README, Hugo docs home, public `Why Gormes` manifesto page, and Phase 2 proof index/cross-links so the public docs match the shipped Go-native architecture and pass `go test ./docs`, per spec `2026-04-19-gormes-doc-sync-manifesto-design.md` (commit `a7b48df6`).

**Architecture:** Keep the documentation split strict. `README.md` remains the operator-facing quick start; `docs/content/why-gormes.md` becomes the public engineering manifesto; `docs/superpowers/specs/README.md` becomes the internal proof index. Preserve the current Hugo mirror discipline by explicitly allowlisting `why-gormes.md` as a Gormes-native content page instead of weakening the mirror test broadly.

**Tech Stack:** Markdown, Hugo/Goldmark docs harness, Go 1.22+ tests under `gormes/docs`, git.

---

## Prerequisites

- Work from `<repo>/gormes`.
- Keep edits inside `README.md` and `gormes/docs/**`; do not touch `../../website/docs` or any Python path.
- Expect `go test ./docs` to be the primary verification command.
- Treat public proof links in `docs/content/*.md` as GitHub URLs, not local Hugo links, because the roadmap/spec files live outside `docs/content`.

## File Structure Map

```
gormes/
├── README.md                                              # MODIFY — operator-facing quick start + doctor + moat bullets
├── docs/
│   ├── docs_test.go                                       # MODIFY — targets, nativeHugoPages allowlist, rendered-page assertions
│   ├── landing_page_docs_test.go                          # MODIFY — README/home/manifesto/spec-index assertions
│   ├── content/
│   │   ├── _index.md                                      # MODIFY — Gormes-branded public home
│   │   └── why-gormes.md                                  # NEW — public manifesto page
│   └── superpowers/
│       ├── specs/
│       │   ├── README.md                                  # NEW — proof index
│       │   ├── 2026-04-19-gormes-phase2-tools-design.md   # MODIFY — Related Documents block
│       │   └── 2026-04-19-gormes-phase2b-telegram.md      # MODIFY — Related Documents block
│       └── plans/
│           └── 2026-04-19-gormes-doc-sync-manifesto.md    # THIS FILE
```

---

### Task 1: Harden the docs harness for Gormes-native pages

**Files:**
- Modify: `gormes/docs/docs_test.go`
- Modify: `gormes/docs/landing_page_docs_test.go`
- Test: `gormes/docs/docs_test.go`
- Test: `gormes/docs/landing_page_docs_test.go`

- [ ] **Step 1: Write the failing tests**

Add the following tests to `gormes/docs/landing_page_docs_test.go`:

```go
func TestTargetsIncludeManifestoSyncDocs(t *testing.T) {
	want := map[string]bool{
		"superpowers/specs/2026-04-19-gormes-doc-sync-manifesto-design.md": false,
		"superpowers/plans/2026-04-19-gormes-doc-sync-manifesto.md":        false,
	}

	for _, target := range targets {
		if _, ok := want[target]; ok {
			want[target] = true
		}
	}

	for rel, seen := range want {
		if !seen {
			t.Fatalf("docs target missing %s", rel)
		}
	}
}

func TestDocsHarnessAllowsNativeGormesManifestoPage(t *testing.T) {
	if _, ok := nativeHugoPages["why-gormes.md"]; !ok {
		t.Fatalf("nativeHugoPages should explicitly allow why-gormes.md")
	}
}
```

- [ ] **Step 2: Run the docs tests to verify they fail**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
... landing_page_docs_test.go: docs target missing superpowers/specs/2026-04-19-gormes-doc-sync-manifesto-design.md
... undefined: nativeHugoPages
FAIL
```

- [ ] **Step 3: Implement the harness changes**

Update `gormes/docs/docs_test.go` so the targets list includes the new spec/plan and the mirror test explicitly allows the one Gormes-native page:

```go
var targets = []string{
	"ARCH_PLAN.md",
	"THEORETICAL_ADVANTAGES_GORMES_HERMES.md",
	"superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md",
	"superpowers/plans/2026-04-18-gormes-phase1-frontend-adapter.md",
	"superpowers/specs/2026-04-19-gormes-landing-page-design.md",
	"superpowers/plans/2026-04-19-gormes-landing-page.md",
	"superpowers/specs/2026-04-19-gormes-ai-cutover-design.md",
	"superpowers/plans/2026-04-19-gormes-ai-cutover.md",
	"superpowers/specs/2026-04-19-gormes-doc-sync-manifesto-design.md",
	"superpowers/plans/2026-04-19-gormes-doc-sync-manifesto.md",
}

var nativeHugoPages = map[string]struct{}{
	"why-gormes.md": {},
}
```

Replace the "unexpected content file" loop in `TestMirroredDocsCoverage` with:

```go
	for rel := range seen {
		if _, ok := expected[rel]; ok {
			continue
		}
		if _, ok := nativeHugoPages[rel]; ok {
			continue
		}
		t.Fatalf("unexpected content file %s", rel)
	}
```

- [ ] **Step 4: Run the docs tests to verify the harness passes**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
ok  	.../gormes/docs	0.xxxs
```

- [ ] **Step 5: Commit the harness work**

Run:

```bash
cd gormes
git add docs/docs_test.go docs/landing_page_docs_test.go
git commit -m "test(docs): allow native gormes manifesto page"
```

---

### Task 2: Refresh the repository README as the operator-facing front door

**Files:**
- Modify: `gormes/README.md`
- Modify: `gormes/docs/landing_page_docs_test.go`
- Test: `gormes/docs/landing_page_docs_test.go`

- [ ] **Step 1: Write the failing README assertion**

Append this test to `gormes/docs/landing_page_docs_test.go`:

```go
func TestReadmeDocumentsDoctorAndArchitecturalEdge(t *testing.T) {
	raw := readDoc(t, "../README.md")
	wants := []string{
		"# Gormes",
		"7.9 MB static binary",
		"Go 1.22+",
		"Zero-dependencies inside the process boundary",
		"./bin/gormes doctor --offline",
		"Route-B reconnect",
		"16 ms coalescing mailbox",
		"[Why Gormes](docs/content/why-gormes.md)",
		"[Phase 2.A — Tool Registry](docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)",
		"[Phase 2.B.1 — Telegram Scout](docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("README is missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run the docs tests to verify the README assertion fails**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
... README is missing "7.9 MB static binary"
FAIL
```

- [ ] **Step 3: Rewrite `README.md` with the approved quick-start and moat copy**

Replace `gormes/README.md` with:

````md
# Gormes

Gormes is the operational moat strategy for Hermes: a Go-native agent host for the era where runtime quality matters more than demo quality. The current `cmd/gormes` build fits in a 7.9 MB static binary built with Go 1.22+, with Zero-dependencies inside the process boundary: no Python runtime, no Node runtime, and no per-host dependency stack once the binary is built.

Phase 1 is a tactical bridge, not the final shape. Today Gormes renders a Bubble Tea dashboard and talks to Python's OpenAI-compatible `api_server` on port 8642. That bridge exists to give immediate value to existing Hermes users while the long-term target stays fixed: a pure Go runtime that owns the full agent lifecycle.

## Quick Start

Start the existing Hermes backend:

```bash
API_SERVER_ENABLED=true hermes gateway start
```

Build the Go binary:

```bash
cd gormes
make build
```

Validate the local tool wiring before spending a cent on API traffic:

```bash
./bin/gormes doctor --offline
```

Run Gormes:

```bash
./bin/gormes
```

## Architectural Edge

- **Wire Doctor** — `gormes doctor` validates the local Go-native tool registry and schema shape before a live turn burns tokens.
- **Route-B reconnect** — dropped SSE streams are treated as a resilience problem to solve, not a happy-path omission to ignore.
- **16 ms coalescing mailbox** — the kernel uses a replace-latest render mailbox so stalled consumers do not trigger a thundering herd of stale frames.

## Further Reading

- [Why Gormes](docs/content/why-gormes.md)
- [Executive Roadmap](docs/ARCH_PLAN.md)
- [Phase 2.A — Tool Registry](docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)
- [Phase 2.B.1 — Telegram Scout](docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)

## License

MIT — see `../LICENSE`.
````

- [ ] **Step 4: Run the docs tests to verify the README passes**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
ok  	.../gormes/docs	0.xxxs
```

- [ ] **Step 5: Commit the README refresh**

Run:

```bash
cd gormes
git add README.md docs/landing_page_docs_test.go
git commit -m "docs(gormes): refresh readme moat copy"
```

---

### Task 3: Publish the public Hugo surface for Gormes

**Files:**
- Modify: `gormes/docs/docs_test.go`
- Modify: `gormes/docs/landing_page_docs_test.go`
- Modify: `gormes/docs/content/_index.md`
- Create: `gormes/docs/content/why-gormes.md`
- Test: `gormes/docs/docs_test.go`
- Test: `gormes/docs/landing_page_docs_test.go`

- [ ] **Step 1: Write the failing public-surface tests**

Add these tests to `gormes/docs/landing_page_docs_test.go`:

```go
func TestDocsHomePageIsGormesBranded(t *testing.T) {
	raw := readDoc(t, "content/_index.md")
	wants := []string{
		`title: "Gormes Documentation"`,
		"# Gormes",
		"[Why Gormes](why-gormes)",
		"Route-B reconnect",
		"Quick Start on GitHub",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("docs home is missing %q", want)
		}
	}

	rejects := []string{
		"Hermes Agent Documentation",
		"# Hermes Agent",
		"The self-improving AI agent built by Nous Research.",
	}
	for _, reject := range rejects {
		if strings.Contains(raw, reject) {
			t.Fatalf("docs home should not contain stale copy %q", reject)
		}
	}
}

func TestWhyGormesManifestoPageExistsAndCarriesApprovedSections(t *testing.T) {
	raw := readDoc(t, "content/why-gormes.md")
	wants := []string{
		`title: "Why Gormes"`,
		"## Operational Moat",
		"## Wire Doctor",
		"## Chaos Resilience",
		"## Surgical Architecture",
		"thundering herd",
		"Tool Registry",
		"Telegram Scout",
	}
	for _, want := range wants {
		if !strings.Contains(raw, want) {
			t.Fatalf("why-gormes page is missing %q", want)
		}
	}
}
```

Update the rendered-content check in `gormes/docs/docs_test.go` to:

```go
	checks := map[string][]string{
		"index.html": {
			"Gormes Documentation",
			"Why Gormes",
			"Quick Start on GitHub",
		},
		filepath.Join("why-gormes", "index.html"): {
			"Operational Moat",
			"Wire Doctor",
			"Chaos Resilience",
			"Surgical Architecture",
		},
		filepath.Join("user-guide", "cli", "index.html"): {
			"Stylized preview of the Hermes CLI layout",
			"The Hermes CLI banner, conversation stream, and fixed input prompt",
		},
		filepath.Join("user-guide", "sessions", "index.html"): {
			"Stylized preview of the Previous Conversation recap panel",
			"Resume mode shows a compact recap panel",
		},
	}
```

- [ ] **Step 2: Run the docs tests to verify the public-surface tests fail**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
... read content/why-gormes.md: no such file or directory
... docs home should not contain stale copy "Hermes Agent Documentation"
... rendered index.html missing "Gormes Documentation"
FAIL
```

- [ ] **Step 3: Implement the public content pages**

Replace `gormes/docs/content/_index.md` with:

```md
---
title: "Gormes Documentation"
description: "Go-native documentation for Gormes: operational moat, wire doctor, resilience, and surgical binaries."
weight: 0
slug: "/"
---

# Gormes

Gormes is the Go-native operational moat for Hermes: a current 7.9 MB static binary, a tool-aware doctor, and a resilience model built around Route-B reconnect plus a 16 ms coalescing mailbox.

- [Why Gormes](why-gormes)
- [Quick Start on GitHub](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/README.md)
- [Roadmap](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/ARCH_PLAN.md)

## What lives here?

This docs surface explains the public engineering case for Gormes. The operator-facing install and run flow stays in the repository README; the roadmap and phase specs stay in the repository docs where contributors can inspect the proof directly.

## Quick Links

| | |
|---|---|
| **[Why Gormes](why-gormes)** | Public technical manifesto: operational moat, wire doctor, resilience, and surgical binaries |
| **[Quick Start on GitHub](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/README.md)** | Build, validate, and run the current Go binary |
| **[Roadmap](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/ARCH_PLAN.md)** | The five-phase path from tactical bridge to pure Go runtime |
| **[Tool Registry Spec](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)** | Phase 2.A proof doc for Go-native tools |
| **[Telegram Scout Spec](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)** | Phase 2.B proof doc for split-binary messaging architecture |
```

Create `gormes/docs/content/why-gormes.md` with:

```md
---
title: "Why Gormes"
description: "The Go-native philosophy behind Gormes: operational moat, wire doctor, chaos resilience, and surgical binaries."
weight: 1
---

# Why Gormes

Gormes exists because the bottleneck has moved. Model quality keeps improving; operational friction is what now breaks agent systems in the field. The goal is not to wrap Hermes in another shell. The goal is to move the runtime surfaces that matter most into a Go-native host that is easier to ship, easier to reason about, and harder to kill by accident.

## Operational Moat

The current `cmd/gormes` build fits in a 7.9 MB static binary built with Go 1.22+. That matters because deployment friction is architecture, not cosmetics. A single binary with Zero-dependencies inside the process boundary is easier to copy to a VPS, easier to audit, and easier to recover in the middle of a bad day than a wrapper that drags a Python or Node runtime into every host.

## Wire Doctor

`gormes doctor` exists to catch wiring mistakes before a live turn burns tokens. The Go-native tool registry and schema surface can be validated locally, including the `--offline` path, before the model ever sees a tool definition. That is not a convenience flag. It is a control point that turns schema drift into a local failure instead of a paid production failure.

## Chaos Resilience

Real systems drop streams. Gormes treats that as a first-class architectural problem. Route-B reconnect keeps a turn alive when the SSE stream goes sideways, and the 16 ms coalescing mailbox prevents a stalled renderer from creating a thundering herd of stale frames. The kernel pushes the latest useful state, not every intermediate twitch.

## Surgical Architecture

Gormes is deliberately split into focused binaries. `gormes` stays small and terminal-first. `gormes-telegram` exists as a separate artifact because platform adapters should not bloat the TUI binary or couple unrelated dependency graphs. This is a surgical-strike architecture: clear ownership, smaller binaries, cleaner crash boundaries, and less hidden weight.

## Further Reading

- [Quick Start on GitHub](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/README.md)
- [Executive Roadmap](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/ARCH_PLAN.md)
- [Phase 2.A — Tool Registry](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)
- [Phase 2.B.1 — Telegram Scout](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)
```

- [ ] **Step 4: Run the docs tests to verify the public surface passes**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
ok  	.../gormes/docs	0.xxxs
```

- [ ] **Step 5: Commit the public docs pages**

Run:

```bash
cd gormes
git add docs/docs_test.go docs/landing_page_docs_test.go docs/content/_index.md docs/content/why-gormes.md
git commit -m "docs(gormes): publish why gormes public surface"
```

---

### Task 4: Create the Phase 2 proof index and reciprocal links

**Files:**
- Create: `gormes/docs/superpowers/specs/README.md`
- Modify: `gormes/docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md`
- Modify: `gormes/docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md`
- Modify: `gormes/docs/landing_page_docs_test.go`
- Test: `gormes/docs/landing_page_docs_test.go`

- [ ] **Step 1: Write the failing spec-index test**

Append this test to `gormes/docs/landing_page_docs_test.go`:

```go
func TestSpecIndexAndPhase2SpecsCrossLink(t *testing.T) {
	indexRaw := readDoc(t, "superpowers/specs/README.md")
	indexWants := []string{
		"# Gormes Specs Index",
		"2026-04-19-gormes-doc-sync-manifesto-design.md",
		"2026-04-19-gormes-phase2-tools-design.md",
		"2026-04-19-gormes-phase2b-telegram.md",
		"../../ARCH_PLAN.md",
	}
	for _, want := range indexWants {
		if !strings.Contains(indexRaw, want) {
			t.Fatalf("spec index is missing %q", want)
		}
	}

	toolsRaw := readDoc(t, "superpowers/specs/2026-04-19-gormes-phase2-tools-design.md")
	telegramRaw := readDoc(t, "superpowers/specs/2026-04-19-gormes-phase2b-telegram.md")
	for _, raw := range []string{toolsRaw, telegramRaw} {
		for _, want := range []string{
			"## Related Documents",
			"../../ARCH_PLAN.md",
			"README.md",
		} {
			if !strings.Contains(raw, want) {
				t.Fatalf("phase-2 spec is missing %q", want)
			}
		}
	}

	if !strings.Contains(toolsRaw, "2026-04-19-gormes-phase2b-telegram.md") {
		t.Fatalf("phase2 tools spec should link to the telegram spec")
	}
	if !strings.Contains(telegramRaw, "2026-04-19-gormes-phase2-tools-design.md") {
		t.Fatalf("telegram spec should link to the tool registry spec")
	}
}
```

- [ ] **Step 2: Run the docs tests to verify the spec-index test fails**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
... read superpowers/specs/README.md: no such file or directory
FAIL
```

- [ ] **Step 3: Implement the proof index and reciprocal links**

Create `gormes/docs/superpowers/specs/README.md` with:

```md
# Gormes Specs Index

This directory is the design-proof set for the Go-native port. Public-facing manifesto copy should trace back here, to `docs/ARCH_PLAN.md`, or to shipped code.

## Active Milestones

- [2026-04-19-gormes-doc-sync-manifesto-design.md](2026-04-19-gormes-doc-sync-manifesto-design.md) — public-surface plan for README, manifesto page, and proof indexing.
- [2026-04-19-gormes-phase2-tools-design.md](2026-04-19-gormes-phase2-tools-design.md) — Phase 2.A Tool Registry design for Go-native tools.
- [2026-04-19-gormes-phase2b-telegram.md](2026-04-19-gormes-phase2b-telegram.md) — Phase 2.B.1 Telegram Scout design for split-binary messaging.

## Phase 2 Cross-Links

- [Executive Roadmap](../../ARCH_PLAN.md)
- [Phase 2.A — Tool Registry](2026-04-19-gormes-phase2-tools-design.md)
- [Phase 2.B.1 — Telegram Scout](2026-04-19-gormes-phase2b-telegram.md)

## Chronological Index

- [2026-04-18-gormes-frontend-adapter-design.md](2026-04-18-gormes-frontend-adapter-design.md) — Phase 1 frontend adapter/TUI foundation.
- [2026-04-18-gormes-ignition-design.md](2026-04-18-gormes-ignition-design.md) — initial tactical bridge design.
- [2026-04-18-gormes-ignition-deterministic-kernel-design.md](2026-04-18-gormes-ignition-deterministic-kernel-design.md) — deterministic kernel ownership rules.
- [2026-04-19-gormes-ai-cutover-design.md](2026-04-19-gormes-ai-cutover-design.md) — `.ai` website hard cutover.
- [2026-04-19-gormes-doc-sync-manifesto-design.md](2026-04-19-gormes-doc-sync-manifesto-design.md) — documentation synchronization and public manifesto.
- [2026-04-19-gormes-landing-page-design.md](2026-04-19-gormes-landing-page-design.md) — public landing-page direction.
- [2026-04-19-gormes-phase1-5-tdd-rig-design.md](2026-04-19-gormes-phase1-5-tdd-rig-design.md) — compatibility probe and discipline rig.
- [2026-04-19-gormes-phase2-tools-design.md](2026-04-19-gormes-phase2-tools-design.md) — Go-native tool execution.
- [2026-04-19-gormes-phase2b-telegram.md](2026-04-19-gormes-phase2b-telegram.md) — Telegram Scout binary.
```

Insert this block after the vocabulary decision in `gormes/docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md`:

```md
## Related Documents

- [Executive Roadmap](../../ARCH_PLAN.md)
- [Specs Index](README.md)
- [Phase 2.B.1 — Telegram Scout](2026-04-19-gormes-phase2b-telegram.md)

---
```

Insert this block after the vocabulary decision in `gormes/docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md`:

```md
## Related Documents

- [Executive Roadmap](../../ARCH_PLAN.md)
- [Specs Index](README.md)
- [Phase 2.A — Tool Registry](2026-04-19-gormes-phase2-tools-design.md)

---
```

- [ ] **Step 4: Run the docs tests to verify the proof index passes**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
ok  	.../gormes/docs	0.xxxs
```

- [ ] **Step 5: Commit the proof-index work**

Run:

```bash
cd gormes
git add docs/superpowers/specs/README.md docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md docs/landing_page_docs_test.go
git commit -m "docs(gormes): index phase 2 proof docs"
```

---

### Task 5: Final verification sweep

**Files:**
- Modify: `gormes/docs/superpowers/plans/2026-04-19-gormes-doc-sync-manifesto.md` (check boxes only if you track progress in-file)
- Test: `gormes/docs`

- [ ] **Step 1: Run the full docs verification one more time**

Run:

```bash
cd gormes
go test ./docs
```

Expected:

```text
ok  	.../gormes/docs	0.xxxs
```

- [ ] **Step 2: Check for whitespace and patch-format issues**

Run:

```bash
cd gormes
git diff --check -- README.md docs/content/_index.md docs/content/why-gormes.md docs/docs_test.go docs/landing_page_docs_test.go docs/superpowers/specs/README.md docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md
```

Expected:

```text
<no output>
```

- [ ] **Step 3: Review the final file set**

Run:

```bash
cd gormes
git status --short -- README.md docs/content/_index.md docs/content/why-gormes.md docs/docs_test.go docs/landing_page_docs_test.go docs/superpowers/specs/README.md docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md
```

Expected:

```text
<no output>
```

- [ ] **Step 4: Confirm the task-level commits are the full history for this change**

Run:

```bash
cd gormes
git log --oneline -n 4
```

Expected:

```text
... docs(gormes): index phase 2 proof docs
... docs(gormes): publish why gormes public surface
... docs(gormes): refresh readme moat copy
... test(docs): allow native gormes manifesto page
```
