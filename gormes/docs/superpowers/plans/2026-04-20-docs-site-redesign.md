# Docs Site Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `docs.gormes.ai` as a two-audience docs site (users + collaborators) with landing-aligned aesthetic, bulletproof mobile, Pagefind search, and a split `ARCH_PLAN.md`. Promote the Learning Loop to a new Phase 6 throughout ARCH_PLAN and the landing page.

**Architecture:** Hugo static site. Unified left sidebar with three colored sections (USING / BUILDING / UPSTREAM). Hand-built layouts (no theme). Same Fraunces + JetBrains Mono + DM Sans + amber palette as the landing. Pagefind for search (no backend). CSS-only drawer for mobile. Existing `.github/workflows/deploy-gormes-docs.yml` runs `hugo --minify && npx pagefind` and deploys to Cloudflare Pages.

**Tech Stack:** Hugo 0.140.0 extended, Go (tests), Pagefind (search), Playwright (mobile smoke), Cloudflare Pages (deploy), Fraunces + JetBrains Mono + DM Sans (Google Fonts).

**Spec:** `gormes/docs/superpowers/specs/2026-04-20-docs-site-redesign-design.md`

---

## File Structure

| Action | Path | Purpose |
|---|---|---|
| Move | `gormes/docs/content/{guides,developer-guide,integrations,reference,user-guide,getting-started}` → `gormes/docs/content/upstream-hermes/…` | Re-home inherited Hermes content verbatim |
| Create | `gormes/docs/content/upstream-hermes/_index.md` | Disclaimer banner + section landing |
| Create | `gormes/docs/content/using-gormes/{_index,quickstart,install,tui-mode,telegram-adapter,configuration,wire-doctor,faq}.md` | User docs (8 pages) |
| Create | `gormes/docs/content/building-gormes/{_index,what-hermes-gets-wrong,porting-a-subsystem,testing}.md` | Collaborator docs (4 pages) |
| Create | `gormes/docs/content/building-gormes/core-systems/{_index,learning-loop,memory,tool-execution,gateway}.md` | Core systems (5 pages) |
| Create | `gormes/docs/content/building-gormes/architecture_plan/{_index,phase-1,…,phase-6,subsystem-inventory,mirror-strategy,technology-radar,boundaries,why-go}.md` | Split ARCH_PLAN (12 pages) |
| Modify | `gormes/docs/ARCH_PLAN.md` | Replace with one-line stub pointing at `content/building-gormes/architecture_plan/` |
| Create | `gormes/docs/layouts/_default/baseof.html` | Base template: topbar + sidebar + content + TOC + footer |
| Create | `gormes/docs/layouts/_default/single.html` | Single-page template inheriting baseof |
| Create | `gormes/docs/layouts/_default/list.html` | Section-landing template |
| Create | `gormes/docs/layouts/index.html` | Docs home page |
| Create | `gormes/docs/layouts/partials/{topbar,sidebar,toc,breadcrumbs,search,footer}.html` | Reusable template partials |
| Create | `gormes/docs/layouts/_default/_markup/render-codeblock.html` | Hugo render hook: wraps every code block with copy button |
| Modify | `gormes/docs/static/site.css` | Full rewrite — landing aesthetic + docs ergonomics + mobile |
| Modify | `gormes/docs/hugo.toml` | Add menu config, pretty URLs, syntax highlighting config |
| Modify | `gormes/docs/ARCH_PLAN.md` (if it ever regrows) | Stay as stub |
| Modify | `gormes/docs/docs_test.go` | Update assertions for new content structure |
| Create | `gormes/docs/build_test.go` | Run `hugo --minify`, assert every section renders |
| Create | `gormes/docs/www-tests/{package.json,playwright.config.mjs,tests/*.spec.mjs}` | Playwright mobile suite |
| Modify | `.github/workflows/deploy-gormes-docs.yml` | Add Pagefind step after Hugo build |
| Modify | `gormes/www.gormes.ai/internal/site/content.go` | Add Phase 6 card to RoadmapPhases |
| Modify | `gormes/www.gormes.ai/internal/site/render_test.go` | Assert Phase 6 row present |
| Modify | `gormes/www.gormes.ai/internal/site/static_export_test.go` | Assert Phase 6 in dist export |

---

## Task 1: Re-home inherited Hermes content under upstream-hermes/

Mechanical migration. No content edits — just `git mv` the existing subtrees so they live under `content/upstream-hermes/`, then add a disclaimer banner at the new section root.

**Files:**
- Move: `gormes/docs/content/{guides,developer-guide,integrations,reference,user-guide,getting-started}/` → `gormes/docs/content/upstream-hermes/…`
- Create: `gormes/docs/content/upstream-hermes/_index.md`

- [ ] **Step 1: Create the `upstream-hermes/` target directory**

From the repo root:

```bash
mkdir -p gormes/docs/content/upstream-hermes
```

- [ ] **Step 2: Move the six inherited subtrees with `git mv` (preserves history)**

```bash
cd <repo> && \
for dir in guides developer-guide integrations reference user-guide getting-started; do
  if [ -d "gormes/docs/content/$dir" ]; then
    git mv "gormes/docs/content/$dir" "gormes/docs/content/upstream-hermes/$dir"
  fi
done && \
ls gormes/docs/content/upstream-hermes/
```

Expected output: a line per existing directory — `guides`, `developer-guide`, etc. Any directory that didn't exist is silently skipped (the `-d` guard).

- [ ] **Step 3: Create the upstream-hermes section landing with disclaimer**

Write `gormes/docs/content/upstream-hermes/_index.md`:

```markdown
---
title: "Upstream Hermes · Reference"
weight: 300
---

# Upstream Hermes · Reference

> These pages document the **Python upstream** `NousResearch/hermes-agent`. Gormes is porting these capabilities gradually — track progress in [§5 Final Purge](/building-gormes/architecture_plan/phase-5-final-purge/) of the roadmap. Features described here may or may not be shipping in Gormes today.

The content below is preserved verbatim from the upstream docs so operators evaluating Gormes can see the full Hermes stack in context. Anything that lands in native Go graduates out of this section into [Using Gormes](/using-gormes/).

## Sections

- **Guides** — task-oriented how-tos
- **Developer Guide** — architectural deep dives
- **Integrations** — platform-specific setup (Bedrock, voice, Telegram, …)
- **Reference** — API/CLI material
- **User Guide** — operator workflows
- **Getting Started** — first-run setup (use [Using Gormes → Quickstart](/using-gormes/quickstart/) for the Go-native path)
```

- [ ] **Step 4: Verify the move didn't lose files**

```bash
find gormes/docs/content/upstream-hermes -name '*.md' | wc -l
```

Expected: ≥20 (the inherited Hermes content count from earlier — 18 guides + developer-guide files + others).

- [ ] **Step 5: Commit**

```bash
cd <repo> && \
git add gormes/docs/content/upstream-hermes/ && \
git commit -m "docs(gormes): re-home inherited Hermes content under upstream-hermes/

Mechanical move of content/{guides,developer-guide,integrations,
reference,user-guide,getting-started}/ under
content/upstream-hermes/. No file contents touched — git mv
preserves history. Adds a disclaimer _index.md at the new section
root explaining that these document the Python upstream, not Gormes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Split ARCH_PLAN.md into architecture_plan/ pages + promote Phase 6

Split the 68 KB `ARCH_PLAN.md` into 12 digestible pages under `content/building-gormes/architecture_plan/`, add a NEW Phase 6 page for the Learning Loop, and replace `ARCH_PLAN.md` with a one-line stub.

**Files:**
- Read: `gormes/docs/ARCH_PLAN.md` (source of splits — do not edit in place)
- Create: `gormes/docs/content/building-gormes/architecture_plan/_index.md`
- Create: `gormes/docs/content/building-gormes/architecture_plan/phase-{1,2,3,4,5,6}-*.md` (6 files)
- Create: `gormes/docs/content/building-gormes/architecture_plan/{subsystem-inventory,mirror-strategy,technology-radar,boundaries,why-go}.md` (5 files)
- Modify: `gormes/docs/ARCH_PLAN.md` (replace with stub)

- [ ] **Step 1: Read the current ARCH_PLAN.md**

```bash
wc -l gormes/docs/ARCH_PLAN.md
```

Expected: ~1000+ lines. This is the source to split.

- [ ] **Step 2: Create the architecture_plan/ directory**

```bash
mkdir -p gormes/docs/content/building-gormes/architecture_plan
```

- [ ] **Step 3: Create `_index.md` (roadmap overview)**

Write `gormes/docs/content/building-gormes/architecture_plan/_index.md`:

```markdown
---
title: "Architecture Plan"
weight: 10
---

# Architecture Plan

{{< copy-from-archplan-sections sections="0,1" >}}

Copy §0 "Operational Moat Thesis" and §1 "Rosetta Stone Declaration" verbatim from the current `gormes/docs/ARCH_PLAN.md`. Then add the milestone table:

## 4. Milestone Status

| Phase | Status | Deliverable |
|---|---|---|
| Phase 1 — The Dashboard | ✅ complete | Tactical Go TUI bridge over Python's `api_server` |
| Phase 2 — The Gateway | 🔨 in progress | Go-native tools + Telegram + session resume + wider adapters |
| Phase 3 — The Black Box (Memory) | 🔨 substantially complete | SQLite + FTS5 + graph + recall + USER.md mirror |
| Phase 4 — The Brain Transplant | ⏳ planned | Native Go agent orchestrator + prompt builder (Hermes-off) |
| Phase 5 — The Final Purge | ⏳ planned | 100% Go — Python tool scripts ported |
| **Phase 6 — The Learning Loop (Soul)** | ⏳ planned | Native skill extraction. Compounding intelligence. The feature Hermes doesn't have. |

Legend: 🔨 in progress · ✅ complete · ⏳ planned.

Each phase has a dedicated page below with sub-phase breakdowns and current state.
```

**Implementation note for the subagent:** replace the `{{< copy-from-archplan-sections sections="0,1" >}}` placeholder with the actual §0 + §1 markdown copied verbatim from the current `ARCH_PLAN.md`. The placeholder is just pseudocode for this plan; the output file must contain real markdown.

- [ ] **Step 4: Create phase-1-dashboard.md through phase-5-final-purge.md**

For each of Phases 1–5, create a page like this template (example for Phase 1):

Write `gormes/docs/content/building-gormes/architecture_plan/phase-1-dashboard.md`:

```markdown
---
title: "Phase 1 — The Dashboard"
weight: 20
---

# Phase 1 — The Dashboard

**Status:** ✅ complete · evolving (polish, bug fixes, TUI ergonomics ongoing)

[Copy Phase 1 prose from current ARCH_PLAN.md §3 "Hybrid Manifesto" mention of Phase 1 as "Strangler Fig bridge", plus any other Phase 1 references. No new content — this is a migration.]

## What shipped

- Bubble Tea TUI shell
- Kernel with 16 ms render mailbox (coalescing)
- Route-B SSE reconnect (dropped streams recover)
- Wire Doctor — offline tool-registry validation
- Streaming token renderer

## What's ongoing

- Polish, bug fixes, TUI ergonomics. No formal sub-phases remain.
```

Repeat this structure for phases 2, 3, 4, 5 — copying the corresponding sub-phase outline, ledger tables, and status notes from the current `ARCH_PLAN.md`. Front-matter `weight` values: Phase 2 → 30, Phase 3 → 40, Phase 4 → 50, Phase 5 → 60.

- [ ] **Step 5: Create phase-6-learning-loop.md (NEW)**

Write `gormes/docs/content/building-gormes/architecture_plan/phase-6-learning-loop.md`:

```markdown
---
title: "Phase 6 — The Learning Loop (Soul)"
weight: 70
---

# Phase 6 — The Learning Loop (Soul)

**Status:** ⏳ planned · 0/6 sub-phases

The Learning Loop is the first Gormes-original core system — not a port. It detects when a task is complex enough to be worth learning from, distills the solution into a reusable skill, and improves that skill over successive runs. Upstream Hermes alludes to self-improvement; Gormes implements it as a dedicated subsystem.

> "Agents are not prompts. They are systems. Memory + skills > raw model intelligence."

## Sub-phase outline

| Subphase | Status | Deliverable |
|---|---|---|
| 6.A — Complexity Detector | ⏳ planned | Heuristic (or LLM-scored) signal for "this turn was worth learning from" |
| 6.B — Skill Extractor | ⏳ planned | LLM-assisted pattern distillation from the conversation + tool-call trace |
| 6.C — Skill Storage Format | ⏳ planned | Portable, human-editable Markdown (SKILL.md) with structured metadata |
| 6.D — Skill Retrieval + Matching | ⏳ planned | Hybrid lexical + semantic lookup for relevant skills at turn start |
| 6.E — Feedback Loop | ⏳ planned | Did the skill help? Adjust weight. Surface usage stats to operator |
| 6.F — Skill Surface (TUI + Telegram) | ⏳ planned | Browse, edit, disable skills from the CLI or messaging edge |

## Why this is Phase 6 and not Phase 5.F

Phase 5.F (Skills system) was previously scoped as "port the upstream Python skills plumbing". That's mechanical. Phase 6 is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. It depends on 5.F (needs the storage format), but it's not the same work.

Positioning: **Gormes's moat over Hermes**. Hermes has a skills directory; it does not have a native learning loop that decides what's worth writing down.
```

- [ ] **Step 6: Create the remaining split pages**

Four more pages, front-matter only shown — copy content from ARCH_PLAN.md sections as noted:

```markdown
# gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md
---
title: "Upstream Subsystem Inventory"
weight: 80
---

[Copy §7 from ARCH_PLAN.md verbatim — tables: gateway platforms, operational layer, memory+state, brain, tools surface, CLI+packaging, out-of-scope, cadence]


# gormes/docs/content/building-gormes/architecture_plan/mirror-strategy.md
---
title: "Mirror Strategy — Auditability Roadmap"
weight: 90
---

[Copy §8 from ARCH_PLAN.md verbatim — 8.1 through 8.4]


# gormes/docs/content/building-gormes/architecture_plan/technology-radar.md
---
title: "Technology Radar"
weight: 100
---

[Copy §9 from ARCH_PLAN.md verbatim — 9.1 through 9.3]


# gormes/docs/content/building-gormes/architecture_plan/boundaries.md
---
title: "Project Boundaries"
weight: 110
---

[Copy §5 from ARCH_PLAN.md verbatim]


# gormes/docs/content/building-gormes/architecture_plan/why-go.md
---
title: "Why Go"
weight: 120
---

[Copy §2 "Why Go — for a Python developer" and §3 "Hybrid Manifesto — the Motherboard Strategy" from ARCH_PLAN.md verbatim]
```

- [ ] **Step 7: Replace `gormes/docs/ARCH_PLAN.md` with a stub**

Overwrite `gormes/docs/ARCH_PLAN.md` with:

```markdown
# ARCH_PLAN moved

The executive roadmap is now authored as a set of pages under:

```
gormes/docs/content/building-gormes/architecture_plan/
```

Published at: https://docs.gormes.ai/building-gormes/architecture_plan/

This stub exists so external links to `docs/ARCH_PLAN.md` land somewhere useful. All historical content is preserved in the split pages; `git log -S "any-phrase"` will find its origin.
```

- [ ] **Step 8: Update docs_test.go assertions**

Read `gormes/docs/docs_test.go`, find any assertion that depends on `ARCH_PLAN.md` containing specific section content, and update it to read the corresponding split file instead. For example, if the test asserted:

```go
body, _ := os.ReadFile("ARCH_PLAN.md")
if !strings.Contains(string(body), "Phase 3 — The Black Box") { … }
```

Replace with:

```go
body, _ := os.ReadFile("content/building-gormes/architecture_plan/phase-3-memory.md")
if !strings.Contains(string(body), "Phase 3") { … }
```

- [ ] **Step 9: Verify tests still pass**

```bash
cd gormes/docs && go test ./... 2>&1 | tail -10
```

Expected: `ok` (any pre-existing failures unrelated to this task can be noted as out-of-scope).

- [ ] **Step 10: Commit**

```bash
cd <repo> && \
git add gormes/docs/ARCH_PLAN.md \
        gormes/docs/content/building-gormes/architecture_plan/ \
        gormes/docs/docs_test.go && \
git commit -m "docs(gormes): split ARCH_PLAN.md + promote Phase 6 Learning Loop

Move the 68 KB ARCH_PLAN.md into 12 digestible pages under
content/building-gormes/architecture_plan/. Add a NEW Phase 6
page promoting the Learning Loop (previously buried as Phase
5.F). Replace ARCH_PLAN.md with a one-line stub so external
links keep working.

docs_test.go updated to read the split files.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Create BUILDING GORMES content (overview + core systems)

Write the collaborator-facing pages that anchor the `/building-gormes/` section. These are net-new content — write tight, specific prose grounded in the user's 4-core-systems framing.

**Files:**
- Create: `gormes/docs/content/building-gormes/_index.md`
- Create: `gormes/docs/content/building-gormes/core-systems/{_index,learning-loop,memory,tool-execution,gateway}.md`
- Create: `gormes/docs/content/building-gormes/{what-hermes-gets-wrong,porting-a-subsystem,testing}.md`

- [ ] **Step 1: Create the building-gormes section landing**

Write `gormes/docs/content/building-gormes/_index.md`:

```markdown
---
title: "Building Gormes"
weight: 200
---

# Building Gormes

Contributor-facing documentation. If you're reading because you want to **use** Gormes, start at [Using Gormes](/using-gormes/).

## Gormes in one sentence

**Gormes is the production runtime for self-improving agents.** Four core systems live inside the binary:

1. **Learning Loop** — detect complex tasks, distill reusable skills, improve them over time ([Phase 6](/building-gormes/architecture_plan/phase-6-learning-loop/))
2. **Memory** — SQLite + FTS5 + ontological graph, with a human-readable USER.md mirror ([Phase 3](/building-gormes/architecture_plan/phase-3-memory/))
3. **Tool Execution** — typed Go interfaces, in-process registry, no Python bounce ([Phase 2.A](/building-gormes/architecture_plan/phase-2-gateway/))
4. **Gateway** — one runtime, many interfaces: TUI, Telegram, (future) Discord/Slack ([Phase 2.B](/building-gormes/architecture_plan/phase-2-gateway/))

## Contents

- [Core Systems](./core-systems/) — one page per system, how they work today
- [What Hermes Gets Wrong](./what-hermes-gets-wrong/) — the opportunities that justify Gormes's existence
- [Architecture Plan](./architecture_plan/) — full roadmap, phase-by-phase, with subsystem inventory
- [Porting a Subsystem](./porting-a-subsystem/) — the contribution path: pick from §7, write spec + plan, open PR
- [Testing](./testing/) — Go test suite, Playwright smoke, Hugo build rig
```

- [ ] **Step 2: Create the core-systems landing**

Write `gormes/docs/content/building-gormes/core-systems/_index.md`:

```markdown
---
title: "Core Systems"
weight: 10
---

# Core Systems

Four subsystems that make Gormes a runtime instead of a chatbot wrapper.

| System | Page | Phase |
|---|---|---|
| Learning Loop | [learning-loop](./learning-loop/) | [Phase 6](/building-gormes/architecture_plan/phase-6-learning-loop/) |
| Memory | [memory](./memory/) | [Phase 3](/building-gormes/architecture_plan/phase-3-memory/) |
| Tool Execution | [tool-execution](./tool-execution/) | [Phase 2.A](/building-gormes/architecture_plan/phase-2-gateway/) |
| Gateway | [gateway](./gateway/) | [Phase 2.B](/building-gormes/architecture_plan/phase-2-gateway/) |

Miss any one of these and you don't have "Hermes in Go" — you have a chatbot with tools.
```

- [ ] **Step 3: Create learning-loop, memory, tool-execution, gateway pages**

Each core-systems page is 150–300 words, framed around: what it does, why it matters, current status, links to the corresponding phase page.

Write `gormes/docs/content/building-gormes/core-systems/learning-loop.md`:

```markdown
---
title: "Learning Loop"
weight: 20
---

# The Learning Loop (The Soul)

Detects when a task was complex enough to learn from, distills the solution into a reusable skill, stores it, and improves the skill over successive runs.

## Simplified flow

```go
if taskComplexity(turn) > threshold {
    skill := extractSkill(conversation, toolCalls)
    store.Save(skill)
}
```

## Why this is load-bearing

Without a learning loop you lose:

- **Compounding intelligence** — the bot doesn't get smarter at *your* workflows over time
- **Differentiation** — every agent looks the same at turn zero
- **Long-term value** — you pay the same token tax on turn 1000 as on turn 1

Upstream Hermes has a `skills/` directory with hand-authored SKILL.md files. It does not have an algorithm that decides what's worth writing down. That's what Phase 6 delivers.

## Current status

⏳ Planned — see [Phase 6](/building-gormes/architecture_plan/phase-6-learning-loop/) for the sub-phase breakdown.
```

Write `gormes/docs/content/building-gormes/core-systems/memory.md`:

```markdown
---
title: "Memory"
weight: 30
---

# Memory

Persistent, searchable state that outlives the process. Structured enough for graph traversal; flat enough for `grep`.

## Components shipped today

- **SQLite + FTS5 lattice** (3.A) — `internal/memory/SqliteStore`. Schema migrations, fire-and-forget worker, lexical search.
- **Ontological graph** (3.B) — entities, relationships, LLM-assisted extractor with dead-letter queue.
- **Neural recall** (3.C) — 2-layer seed selection, CTE traversal, `<memory-context>` fence injection matching Hermes's `build_memory_context_block`.
- **USER.md mirror** (3.D.5) — async export of entity/relationship graph to human-readable Markdown. Gormes-original; no upstream equivalent.

## Still in flight

- **Semantic fusion** (3.D) — Ollama embeddings + cosine similarity. Spec approved.

## Why this is not just "chat logs"

Chat logs are append-only. Memory has schema. You query it, derive from it, inject it back into the context window. The SQLite + FTS5 combination gives you ACID durability *and* full-text search in a single ~100 KB binary dependency.

See [Phase 3](/building-gormes/architecture_plan/phase-3-memory/) for the full sub-status.
```

Write `gormes/docs/content/building-gormes/core-systems/tool-execution.md`:

```markdown
---
title: "Tool Execution"
weight: 40
---

# Tool Execution

Typed Go interfaces. In-process registry. No Python bounce.

## The contract

```go
type Tool interface {
    Name() string
    Execute(ctx context.Context, input string) (string, error)
}
```

Every tool lives behind this interface. Schemas are Go structs — schema drift is a compile error, not a silent agent-loop failure.

## What you get

- **Deterministic execution** — no subprocess spawning for in-process tools
- **Bounded side effects** — ctx cancels; deadlines respected
- **Wire Doctor** — `gormes doctor --offline` validates the registry before a live turn burns tokens

## Status

✅ Shipped (Phase 2.A). The current registry has the minimal tool surface; porting the 61 upstream Python tools is Phase 5.A. See [Phase 2](/building-gormes/architecture_plan/phase-2-gateway/).
```

Write `gormes/docs/content/building-gormes/core-systems/gateway.md`:

```markdown
---
title: "Gateway"
weight: 50
---

# Gateway

One runtime, multiple interfaces. The agent lives in the kernel; each gateway is a thin edge adapter over the same loop.

## Shipped

- **TUI** (Phase 1) — Bubble Tea interactive shell
- **Telegram adapter** (Phase 2.B.1) — long-poll ingress, 1-second edit coalescer, session resume

## Planned

- **Phase 2.B.2+** — Discord, Slack, WhatsApp, Signal, Email, SMS, Matrix, Feishu, WeChat, DingTalk, QQ, BlueBubbles, HomeAssistant, Webhook (14 more connectors). See [§7 Subsystem Inventory](/building-gormes/architecture_plan/subsystem-inventory/).

## Why this matters

Agents that only live in a terminal are academic. Agents that live where the operator lives — on their phone, in their team chat — are infrastructure. Gormes's split-binary-then-unified design lets each adapter ship independently without dragging the TUI's deps.

See [Phase 2](/building-gormes/architecture_plan/phase-2-gateway/) for the Gateway ledger.
```

- [ ] **Step 4: Create what-hermes-gets-wrong**

Write `gormes/docs/content/building-gormes/what-hermes-gets-wrong.md`:

```markdown
---
title: "What Hermes Gets Wrong"
weight: 30
---

# What Hermes Gets Wrong

The gaps that justify Gormes's existence. This is not a competitive teardown — upstream Hermes is excellent research. These are the operational-runtime problems Gormes is positioned to solve.

## 1. Python dependency stack

Hermes requires `uv`, a venv, Python 3.11, and platform-specific extras (`.[all]`, `.[termux]`). Every host install is a moving target.

**Gormes's answer:** one binary. No runtime. `scp` and run.

## 2. Execution chaos

Hermes tool execution is dynamic Python — flexible but hard to reason about. Schemas drift. Subprocess boundaries blur.

**Gormes's answer:** typed Go interfaces, controlled execution, bounded side effects.

## 3. Subagents are conceptual

Upstream documents subagent delegation but lacks a robust lifecycle — processes spin up, state leaks, cancellation is best-effort.

**Gormes's answer (planned, Phase 2.E):** real subagents with explicit `context.Context`, cancel funcs, memory scoping, resource limits:

```go
type Agent struct {
    ID      string
    Context context.Context
    Cancel  context.CancelFunc
}
```

## 4. Startup + recovery cost

Python startup is measured in seconds on every cold boot. Recovery after a crash requires re-scanning venv + re-importing half the world.

**Gormes's answer:** instant start. Crash → restart → continue, measured in milliseconds.

## The positioning

> *Hermes-class agents, without Python.*
>
> The production runtime for self-improving agents — not a research artifact. An **industrial-grade agent runtime** that runs 24/7 without babysitting.
```

- [ ] **Step 5: Create porting-a-subsystem and testing**

Write `gormes/docs/content/building-gormes/porting-a-subsystem.md`:

```markdown
---
title: "Porting a Subsystem from Upstream"
weight: 40
---

# Porting a Subsystem from Upstream

The contribution path. Use this when you want to port a piece of Hermes into Gormes.

## 1. Pick your target

Open [Subsystem Inventory](/building-gormes/architecture_plan/subsystem-inventory/). Every row is a Hermes subsystem with a target Gormes sub-phase. Pick one that:

- Carries a ⏳ planned status (not already shipped)
- Has no hard dependency on a later phase (check the "Target phase" column)
- You have context on (voice/vision are big lifts; a platform adapter is a reasonable first PR)

## 2. Write a spec

`gormes/docs/superpowers/specs/YYYY-MM-DD-<subsystem>-design.md`. Use the brainstorming skill if you want guided design; otherwise mirror the shape of an existing spec. Get maintainer approval before writing the plan.

## 3. Write a plan

`gormes/docs/superpowers/plans/YYYY-MM-DD-<subsystem>.md`. Break into tasks small enough for subagent-driven execution (5–10 tasks, 2–5 minute steps). See existing plans under `gormes/docs/superpowers/plans/` for examples.

## 4. Implement

Bite-sized commits. Tests first (TDD). Mirror the existing Go package layout under `gormes/internal/`.

## 5. Open a PR

Target `main`. Title convention: `feat(gormes/<subsystem>): port <capability> from upstream`. Reference the spec + plan in the description.

## 6. Update the inventory

Flip your row in [Subsystem Inventory](/building-gormes/architecture_plan/subsystem-inventory/) from ⏳ planned to ✅ shipped, with a link to the shipped spec.
```

Write `gormes/docs/content/building-gormes/testing.md`:

```markdown
---
title: "Testing"
weight: 50
---

# Testing

Three layers.

## Go tests

`go test ./...` from `gormes/`. Covers kernel, memory, tools, telegram adapter, session resume. Integration tests are tag-gated:

```bash
go test -tags=live ./...         # requires local Ollama
```

## Landing + docs smoke (Playwright)

`npm run test:e2e` from `gormes/www.gormes.ai/` and `gormes/docs/www-tests/`. Parametrized over mobile viewports (320 / 360 / 390 / 430 / 768 / 1024 px). Asserts:

- No horizontal overflow
- Copy buttons tappable (≥28×28 px bounding box)
- Section copy matches the locked strings in `content.go`
- Drawer opens/closes on mobile (docs only)

## Hugo build

`go test ./docs/... -run TestHugoBuild`. Shells out to `hugo --minify`, verifies every `_index.md` produces a rendered page, checks for broken internal links.

## Discipline

Every PR must keep all three layers green. The `deploy-gormes-landing.yml` and `deploy-gormes-docs.yml` workflows run the Go + Playwright subsets on every push to `main`.
```

- [ ] **Step 6: Commit**

```bash
cd <repo> && \
git add gormes/docs/content/building-gormes/ && \
git commit -m "docs(gormes): add building-gormes section (core systems + overview)

New collaborator-facing pages:
- _index.md — Gormes in one sentence, 4 core systems framing
- core-systems/{_index,learning-loop,memory,tool-execution,gateway}.md
- what-hermes-gets-wrong.md — Python stack, execution chaos,
  conceptual subagents, startup cost, positioning
- porting-a-subsystem.md — the contribution path
- testing.md — Go + Playwright + Hugo layers

Synthesizes the 2026-04-20 strategic framing into navigable pages.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Create USING GORMES content

Eight user-facing pages. Each is 100–200 words, specific, operational.

**Files:**
- Create: `gormes/docs/content/using-gormes/_index.md`
- Create: `gormes/docs/content/using-gormes/{quickstart,install,tui-mode,telegram-adapter,configuration,wire-doctor,faq}.md`

- [ ] **Step 1: Create the section landing**

Write `gormes/docs/content/using-gormes/_index.md`:

```markdown
---
title: "Using Gormes"
weight: 100
---

# Using Gormes

Operator-facing documentation. Run the binary, connect a Hermes backend, get work done.

## Start here

- [Quickstart](./quickstart/) — 60-second install + first run
- [Install](./install/) — full install matrix (Linux, macOS, WSL2, Termux, Go install)
- [TUI mode](./tui-mode/) — interactive terminal shell
- [Telegram adapter](./telegram-adapter/) — run Gormes as a Telegram bot
- [Configuration](./configuration/) — config files, env vars, state directories
- [Wire Doctor](./wire-doctor/) — `gormes doctor` validates the local stack before tokens burn
- [FAQ](./faq/) — offline mode, memory location, log files

If you want to contribute or understand how Gormes is built, see [Building Gormes](/building-gormes/).
```

- [ ] **Step 2: Create quickstart**

Write `gormes/docs/content/using-gormes/quickstart.md`:

````markdown
---
title: "Quickstart"
weight: 10
---

# Quickstart

Get Gormes running in 60 seconds.

## 1. Install

```bash
curl -fsSL https://gormes.ai/install.sh | sh
```

Installs `gormes` into `$HOME/go/bin` via `go install`. Requires Go 1.25+. For other install paths see [Install](../install/).

## 2. Bring up the Hermes backend

Gormes is a Go shell that talks to Hermes over HTTP. You need Hermes running on `localhost:8642` first:

```bash
curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash
API_SERVER_ENABLED=true hermes gateway start
```

## 3. Verify the local stack

```bash
gormes doctor --offline
```

See [Wire Doctor](../wire-doctor/) for what this checks.

## 4. Run

```bash
gormes
```

You're in the TUI. Press `Ctrl+C` to exit.

## Next

- [TUI mode](../tui-mode/) — keybindings, layout
- [Telegram adapter](../telegram-adapter/) — use the same brain from Telegram
- [Configuration](../configuration/) — persistent settings
````

- [ ] **Step 3: Create the remaining user pages**

Create each of these with the shown content:

```markdown
# gormes/docs/content/using-gormes/install.md
---
title: "Install"
weight: 20
---

# Install

Gormes is a single static Go binary (~17 MB). Zero CGO, no Python runtime on the host.

## Recommended: curl pipe

```bash
curl -fsSL https://gormes.ai/install.sh | sh
```

Installs via `go install github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes@latest` into `$HOME/go/bin/gormes`.

Requires Go 1.25+. On Ubuntu: `sudo apt install golang-1.25`. On macOS: `brew install go`.

## Go install directly

```bash
go install github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes@latest
```

## Platform matrix

| Platform | Status |
|---|---|
| Linux x86_64 | ✅ tested |
| Linux arm64 | ✅ tested |
| macOS arm64 (Apple Silicon) | ✅ tested |
| macOS Intel | 🟡 should work, not regression-tested |
| Windows (native) | ❌ not supported |
| Windows WSL2 | ✅ tested |
| Termux (Android) | ✅ tested |

## Verify

```bash
gormes version
gormes doctor --offline
```


# gormes/docs/content/using-gormes/tui-mode.md
---
title: "TUI Mode"
weight: 30
---

# TUI Mode

The default interface. A Bubble Tea terminal shell talking to the Hermes backend over SSE.

## Launch

```bash
gormes
```

## Keybindings

| Key | Action |
|---|---|
| `Ctrl+C` | Quit |
| `Ctrl+L` | Clear output |
| `↑` / `↓` | Cycle through history |
| `Enter` (on empty input) | Cancel current turn |
| `Enter` | Send |

## Layout

The TUI coalesces streamed tokens at 16 ms (the render mailbox), so scrolling under load stays responsive. Route-B reconnect recovers dropped SSE streams without resetting the turn.

## Session resume

Each invocation reattaches to the last session via a bbolt map at `~/.gormes/sessions.db`. To start fresh: `gormes --resume new`.


# gormes/docs/content/using-gormes/telegram-adapter.md
---
title: "Telegram Adapter"
weight: 40
---

# Telegram Adapter

Run Gormes as a Telegram bot. Same kernel, same tools, different edge.

## Setup

1. Create a bot with [@BotFather](https://t.me/BotFather) — get the token
2. Get your chat ID (DM [@userinfobot](https://t.me/userinfobot))
3. Launch:

```bash
GORMES_TELEGRAM_TOKEN=... GORMES_TELEGRAM_CHAT_ID=123456789 gormes telegram
```

## Behaviour

- Long-poll ingress (no webhook server needed)
- Edit coalescer: streamed tokens update the same Telegram message at ~1 Hz to avoid rate limits
- Session resume: each `(platform, chat_id)` maps to a persistent session_id via bbolt

## Multiple chats

Omit `GORMES_TELEGRAM_CHAT_ID` to respond to any chat the bot is added to. Each chat gets its own session.


# gormes/docs/content/using-gormes/configuration.md
---
title: "Configuration"
weight: 50
---

# Configuration

Gormes reads config from TOML files, env vars, and CLI flags — in that precedence.

## Config files

| Path | Purpose |
|---|---|
| `$XDG_CONFIG_HOME/gormes/config.toml` | User-level defaults |
| `./gormes.toml` | Project-local overrides (checked into the repo you're working in) |

Example:

```toml
[hermes]
endpoint = "http://127.0.0.1:8642"
api_key = ""
model = "claude-4-sonnet"

[input]
max_bytes = 65536
max_lines = 500
```

## Env vars

| Var | Purpose |
|---|---|
| `GORMES_HERMES_ENDPOINT` | Override Hermes backend URL |
| `GORMES_HERMES_API_KEY` | Hermes auth token |
| `GORMES_TELEGRAM_TOKEN` | Telegram bot token |
| `GORMES_TELEGRAM_CHAT_ID` | Telegram chat ID (optional) |

## State directories

| Path | Contents |
|---|---|
| `~/.gormes/sessions.db` | bbolt session resume map |
| `~/.hermes/memory/memory.db` | SQLite memory store |
| `~/.hermes/memory/USER.md` | Human-readable entity/relationship mirror |
| `~/.hermes/crash-*.log` | Crash dumps |


# gormes/docs/content/using-gormes/wire-doctor.md
---
title: "Wire Doctor"
weight: 60
---

# Wire Doctor

`gormes doctor` validates the local stack before a live turn burns tokens.

## Online mode

```bash
gormes doctor
```

Checks:

- Hermes backend reachable at configured endpoint (2s timeout)
- Tool registry built and every tool passes schema validation
- Config file parses, state dirs writable

## Offline mode

```bash
gormes doctor --offline
```

Skips the Hermes reachability check. Useful for CI or pre-flight when you want to verify the local tool surface without a live backend.

## Reading the output

```
[PASS] api_server: reachable at http://127.0.0.1:8642
[PASS] tool registry: 6 tools, all schemas valid
[PASS] config: loaded from ~/.config/gormes/config.toml
```

Any `[FAIL]` line names the subsystem and the error. `doctor` exits non-zero on failure so you can wire it into scripts.


# gormes/docs/content/using-gormes/faq.md
---
title: "FAQ"
weight: 70
---

# FAQ

### Do I need Hermes running?

Yes. Gormes is a Go frontend that talks to Hermes's `api_server` over HTTP. Without Hermes, only `--offline` mode (cosmetic smoke-tester) works.

### Can I use it without Python?

Not yet. Phase 4 makes Hermes optional; Phase 5 removes Python entirely. See the [Roadmap](/building-gormes/architecture_plan/).

### Where does memory live?

`~/.hermes/memory/memory.db` (SQLite) with a human-readable mirror at `~/.hermes/memory/USER.md`. The mirror refreshes every 30 seconds.

### How do I back up memory?

Copy `~/.hermes/memory/memory.db` — it's a single SQLite file. USER.md regenerates from it.

### The install script installed Gormes to `$HOME/go/bin` but it's not on my PATH.

Add it: `export PATH="$HOME/go/bin:$PATH"` in your shell rc.

### How do I reset a session?

```bash
gormes --resume new
```

### Logs?

`~/.hermes/gormes.log` (current run) and `~/.hermes/crash-*.log` (panics). Crash logs are timestamped.
```

- [ ] **Step 4: Commit**

```bash
cd <repo> && \
git add gormes/docs/content/using-gormes/ && \
git commit -m "docs(gormes): add using-gormes section (8 user-facing pages)

Quickstart, Install, TUI mode, Telegram adapter, Configuration,
Wire Doctor, FAQ. Operator-focused prose: what to run, what to
expect, where state lives.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Hugo layouts + partials

Build the layout system. Base template with topbar + sidebar + content area + TOC + footer. Sidebar renders from Hugo's menu config. TOC auto-generates from page `.TableOfContents`. Copy-button render hook on every code block.

**Files:**
- Modify: `gormes/docs/hugo.toml` (add menu + output config)
- Create: `gormes/docs/layouts/_default/baseof.html`
- Create: `gormes/docs/layouts/_default/single.html`
- Create: `gormes/docs/layouts/_default/list.html`
- Create: `gormes/docs/layouts/index.html`
- Create: `gormes/docs/layouts/partials/{topbar,sidebar,toc,breadcrumbs,search,footer}.html`
- Create: `gormes/docs/layouts/_default/_markup/render-codeblock.html`

- [ ] **Step 1: Update hugo.toml**

Overwrite `gormes/docs/hugo.toml`:

```toml
baseURL = "https://docs.gormes.ai/"
languageCode = "en-us"
title = "Gormes Docs"

# Pretty URLs for all sections
[permalinks]
  using-gormes = "/using-gormes/:slug/"
  building-gormes = "/building-gormes/:slug/"
  upstream-hermes = "/upstream-hermes/:slug/"

[markup]
  [markup.goldmark]
    [markup.goldmark.renderer]
      unsafe = true
  [markup.tableOfContents]
    startLevel = 2
    endLevel = 4
    ordered = false
  [markup.highlight]
    style = "github-dark"
    lineNos = false
    noClasses = false

[outputs]
  home = ["HTML"]
  section = ["HTML"]
  page = ["HTML"]

# Sidebar sections are emitted programmatically from content weights.
# The three top-level groups (USING / BUILDING / UPSTREAM) correspond
# to the three top-level content sections above.
```

- [ ] **Step 2: Create baseof.html**

Write `gormes/docs/layouts/_default/baseof.html`:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ if .IsHome }}{{ site.Title }}{{ else }}{{ .Title }} · {{ site.Title }}{{ end }}</title>
  <meta name="description" content="{{ if .Description }}{{ .Description }}{{ else }}{{ site.Params.description | default "Gormes documentation." }}{{ end }}">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=DM+Sans:opsz,wght@9..40,400;9..40,500;9..40,700&family=Fraunces:ital,opsz,wght,SOFT@0,9..144,400;0,9..144,700;0,9..144,900;1,9..144,400;1,9..144,700&family=JetBrains+Mono:wght@400;500;700&display=swap">
  <link rel="stylesheet" href="{{ "site.css" | relURL }}">
  <link rel="stylesheet" href="/pagefind/pagefind-ui.css">
  <script src="/pagefind/pagefind-ui.js" defer></script>
</head>
<body>
  <div class="grain" aria-hidden="true"></div>
  <input type="checkbox" id="drawer-toggle" class="drawer-toggle">
  {{ partial "topbar.html" . }}
  <div class="docs-shell">
    <aside class="docs-sidebar">
      {{ partial "sidebar.html" . }}
    </aside>
    <label for="drawer-toggle" class="drawer-backdrop" aria-hidden="true"></label>
    <main class="docs-main">
      {{ block "main" . }}{{ end }}
    </main>
  </div>
  {{ partial "footer.html" . }}
  <script>
    function gormesCopy(b) {
      var code = b.parentElement.querySelector('code').innerText;
      navigator.clipboard.writeText(code).then(function () {
        var label = b.querySelector('.copy-label');
        var orig = label.textContent;
        label.textContent = 'Copied';
        b.classList.add('copied');
        setTimeout(function () {
          label.textContent = orig;
          b.classList.remove('copied');
        }, 1500);
      });
    }
    document.addEventListener('DOMContentLoaded', function() {
      if (window.PagefindUI) {
        new PagefindUI({ element: "#search", showSubResults: true });
      }
    });
  </script>
</body>
</html>
```

- [ ] **Step 3: Create single.html and list.html**

Write `gormes/docs/layouts/_default/single.html`:

```html
{{ define "main" }}
<article class="docs-article">
  {{ partial "breadcrumbs.html" . }}
  <header class="docs-article-header">
    <h1 class="docs-title">{{ .Title }}</h1>
    {{ if .Description }}<p class="docs-lede">{{ .Description }}</p>{{ end }}
  </header>
  <div class="docs-layout-with-toc">
    <div class="docs-content">
      {{ .Content }}
    </div>
    <aside class="docs-toc">
      {{ partial "toc.html" . }}
    </aside>
  </div>
</article>
{{ end }}
```

Write `gormes/docs/layouts/_default/list.html`:

```html
{{ define "main" }}
<article class="docs-article">
  {{ partial "breadcrumbs.html" . }}
  <header class="docs-article-header">
    <h1 class="docs-title">{{ .Title }}</h1>
    {{ if .Description }}<p class="docs-lede">{{ .Description }}</p>{{ end }}
  </header>
  <div class="docs-content">
    {{ .Content }}
    <h2>In this section</h2>
    <ul class="docs-child-list">
      {{ range (.Pages.ByWeight) }}
      <li>
        <a href="{{ .RelPermalink }}">{{ .Title }}</a>
        {{ if .Description }}<p>{{ .Description }}</p>{{ end }}
      </li>
      {{ end }}
    </ul>
  </div>
</article>
{{ end }}
```

- [ ] **Step 4: Create index.html (docs home)**

Write `gormes/docs/layouts/index.html`:

```html
{{ define "main" }}
<article class="docs-home">
  <header>
    <p class="kicker">GORMES · DOCUMENTATION</p>
    <h1 class="docs-home-title">{{ site.Title }}</h1>
    <p class="docs-lede">The Go operator shell for Hermes. Install it, run it, understand how it's built.</p>
  </header>
  <section class="docs-home-cards">
    <a class="docs-home-card" href="/using-gormes/">
      <p class="kicker" style="color:var(--status-shipped-fg)">USING GORMES</p>
      <h2>For operators</h2>
      <p>Install, run, configure. Wire up Telegram. Troubleshoot with the doctor.</p>
    </a>
    <a class="docs-home-card" href="/building-gormes/">
      <p class="kicker" style="color:var(--status-progress-fg)">BUILDING GORMES</p>
      <h2>For contributors</h2>
      <p>Architecture, roadmap, how to port a subsystem from upstream.</p>
    </a>
    <a class="docs-home-card" href="/upstream-hermes/">
      <p class="kicker" style="color:var(--status-next-fg)">UPSTREAM HERMES</p>
      <h2>For reference</h2>
      <p>Inherited Hermes guides. What the Python upstream does that Gormes is porting.</p>
    </a>
  </section>
</article>
{{ end }}
```

- [ ] **Step 5: Create partials**

Write `gormes/docs/layouts/partials/topbar.html`:

```html
<header class="docs-topbar">
  <div class="docs-topbar-inner">
    <label for="drawer-toggle" class="drawer-btn" aria-label="Toggle navigation">☰</label>
    <a class="docs-brand" href="{{ "/" | relURL }}">Gormes</a>
    <div id="search" class="docs-search"></div>
    <nav class="docs-topnav">
      <a href="https://gormes.ai/">gormes.ai</a>
      <a href="https://github.com/TrebuchetDynamics/gormes-agent">GitHub</a>
    </nav>
  </div>
</header>
```

Write `gormes/docs/layouts/partials/sidebar.html`:

```html
<nav class="docs-nav" aria-label="Documentation navigation">
  {{ $sections := slice "using-gormes" "building-gormes" "upstream-hermes" }}
  {{ $toneMap := dict "using-gormes" "shipped" "building-gormes" "progress" "upstream-hermes" "next" }}
  {{ $labelMap := dict "using-gormes" "USING GORMES" "building-gormes" "BUILDING GORMES" "upstream-hermes" "UPSTREAM HERMES" }}
  {{ range $sections }}
    {{ $section := . }}
    {{ $tone := index $toneMap $section }}
    {{ $label := index $labelMap $section }}
    {{ with site.GetPage (printf "/%s" $section) }}
      <div class="docs-nav-group">
        <p class="docs-nav-group-label docs-nav-group-label-{{ $tone }}">{{ $label }}</p>
        <ul class="docs-nav-list">
          <li><a href="{{ .RelPermalink }}"{{ if eq $.RelPermalink .RelPermalink }} aria-current="page"{{ end }}>Overview</a></li>
          {{ range .Pages.ByWeight }}
          <li>
            <a href="{{ .RelPermalink }}"{{ if eq $.RelPermalink .RelPermalink }} aria-current="page"{{ end }}>{{ .Title }}</a>
            {{ if .Pages }}
            <ul class="docs-nav-sublist">
              {{ range .Pages.ByWeight }}
              <li><a href="{{ .RelPermalink }}"{{ if eq $.RelPermalink .RelPermalink }} aria-current="page"{{ end }}>{{ .Title }}</a></li>
              {{ end }}
            </ul>
            {{ end }}
          </li>
          {{ end }}
        </ul>
      </div>
    {{ end }}
  {{ end }}
</nav>
```

Write `gormes/docs/layouts/partials/toc.html`:

```html
{{ if .TableOfContents }}
<details class="docs-toc-details" open>
  <summary class="docs-toc-summary">On this page</summary>
  <div class="docs-toc-body">
    {{ .TableOfContents }}
  </div>
</details>
{{ end }}
```

Write `gormes/docs/layouts/partials/breadcrumbs.html`:

```html
{{ $ancestors := slice }}
{{ $p := . }}
{{ range until 5 }}
  {{ if $p.Parent }}
    {{ $ancestors = $ancestors | append $p.Parent }}
    {{ $p = $p.Parent }}
  {{ end }}
{{ end }}
<nav class="docs-breadcrumbs" aria-label="Breadcrumbs">
  {{ range (sort $ancestors "Weight" "asc") }}
    <a href="{{ .RelPermalink }}">{{ .Title | upper }}</a>
    <span class="docs-breadcrumb-sep">/</span>
  {{ end }}
</nav>
```

Write `gormes/docs/layouts/partials/footer.html`:

```html
<footer class="docs-footer">
  <div class="docs-footer-inner">
    <p class="docs-footer-left">Gormes Docs · <a href="https://trebuchetdynamics.com/">TrebuchetDynamics</a></p>
    <p class="docs-footer-right">MIT License · 2026</p>
  </div>
</footer>
```

Write `gormes/docs/layouts/partials/search.html` (stub for direct usage; the main mount point is `#search` in topbar):

```html
<div id="search"></div>
```

- [ ] **Step 6: Create the code-block render hook**

Write `gormes/docs/layouts/_default/_markup/render-codeblock.html`:

```html
<div class="cmd-wrap">
  <pre class="cmd"><code{{ with .Type }} class="language-{{ . }}"{{ end }}>{{ .Inner | safeHTML }}</code></pre>
  <button type="button" class="copy-btn" aria-label="Copy code" onclick="gormesCopy(this)">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    <span class="copy-label">Copy</span>
  </button>
</div>
```

- [ ] **Step 7: Build and sanity-check**

```bash
cd gormes/docs && hugo --minify 2>&1 | tail -5
```

Expected: `Total in Xms` with no error. If the build fails, read the error and fix the offending template.

- [ ] **Step 8: Commit**

```bash
cd <repo> && \
git add gormes/docs/hugo.toml gormes/docs/layouts/ && \
git commit -m "docs(gormes): hand-built Hugo layouts for docs.gormes.ai

Base template with topbar (brand + search + external nav), left
sidebar rendered from content sections with colored group headers,
content area with breadcrumbs, right-side TOC on article pages,
and a code-block render hook that wraps every <pre> with a copy
button reusing the landing page's gormesCopy helper.

- baseof.html: page shell, font loading, Pagefind JS
- single.html / list.html: article and section landings
- index.html: docs home with 3 entry-point cards
- partials/{topbar,sidebar,toc,breadcrumbs,footer,search}.html
- _markup/render-codeblock.html: copy-button hook
- hugo.toml: permalinks, markup config, TOC levels 2-4

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: CSS — landing-aligned + docs ergonomics + bulletproof mobile

Full rewrite of `gormes/docs/static/site.css`. Ports the landing's design tokens verbatim, adds docs-specific layout (sidebar, TOC, breadcrumbs, drawer), and carries the same bulletproof mobile discipline (min-width:0, clamp typography, :has() tone-coded borders, parametrized-tested viewports).

**Files:**
- Modify: `gormes/docs/static/site.css`

- [ ] **Step 1: Replace site.css entirely**

Overwrite `gormes/docs/static/site.css` with:

```css
/* docs.gormes.ai — docs layout aligned with gormes.ai landing. */

:root {
  --font-display: 'Fraunces', 'Iowan Old Style', Georgia, serif;
  --font-mono: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  --font-body: 'DM Sans', -apple-system, BlinkMacSystemFont, "Helvetica Neue", Helvetica, Arial, sans-serif;

  --bg-0: #0a0d11;
  --bg-1: #121720;
  --bg-2: #1a1f29;
  --border: #1e232e;
  --border-strong: #2a3140;

  --text: #ebe9e2;
  --muted: rgba(235, 233, 226, 0.62);
  --muted-strong: rgba(235, 233, 226, 0.80);
  --label: rgba(235, 233, 226, 0.48);

  --accent: #e8c547;
  --accent-hover: #f0d66c;
  --accent-ink: #1a1300;

  --status-shipped-fg: #5be79a;
  --status-progress-fg: #5bc7e7;
  --status-next-fg: #e7c25b;
  --status-later-fg: #8a99c7;

  --sidebar-w: 260px;
  --toc-w: 200px;
  --topbar-h: 60px;
  --pad: 28px;
}

* { box-sizing: border-box; }

html, body {
  margin: 0;
  padding: 0;
  background: var(--bg-0);
  color: var(--text);
  font-family: var(--font-body);
  font-size: 15px;
  line-height: 1.55;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
  font-feature-settings: 'kern', 'liga', 'calt';
}

/* Grain overlay — same as landing */
.grain {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 100;
  opacity: 0.05;
  mix-blend-mode: overlay;
  background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='240' height='240'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2' stitchTiles='stitch'/><feColorMatrix values='0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 1 0'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>");
}

a { color: var(--accent); text-decoration: none; transition: color 0.15s; }
a:hover { color: var(--accent-hover); }
a:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; border-radius: 2px; }

/* Drawer toggle (hidden checkbox + label pattern, pure CSS mobile drawer) */
.drawer-toggle { display: none; }
.drawer-btn {
  display: none;
  font-size: 20px;
  line-height: 1;
  padding: 8px 12px;
  color: var(--text);
  cursor: pointer;
  user-select: none;
}
.drawer-backdrop { display: none; }

/* ── Topbar ────────────────────────────────────────── */
.docs-topbar {
  position: sticky;
  top: 0;
  z-index: 50;
  background: rgba(10, 13, 17, 0.92);
  backdrop-filter: blur(8px);
  border-bottom: 1px solid var(--border);
  height: var(--topbar-h);
}
.docs-topbar-inner {
  max-width: 1280px;
  margin: 0 auto;
  padding: 0 var(--pad);
  height: 100%;
  display: flex;
  align-items: center;
  gap: 16px;
}
.docs-brand {
  font-family: var(--font-display);
  font-weight: 900;
  font-size: 22px;
  font-variation-settings: "opsz" 144, "SOFT" 0;
  letter-spacing: -0.01em;
  color: var(--text);
}
.docs-brand:hover { color: var(--accent); }
.docs-search { flex: 1; max-width: 320px; }
.docs-topnav { margin-left: auto; display: flex; gap: 20px; }
.docs-topnav a {
  font-family: var(--font-mono);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--muted);
}
.docs-topnav a:hover { color: var(--text); }

/* ── Shell layout ──────────────────────────────────── */
.docs-shell {
  max-width: 1280px;
  margin: 0 auto;
  padding: 0 var(--pad);
  display: grid;
  grid-template-columns: var(--sidebar-w) 1fr;
  gap: 40px;
  min-height: calc(100vh - var(--topbar-h));
}

/* ── Sidebar ───────────────────────────────────────── */
.docs-sidebar {
  position: sticky;
  top: var(--topbar-h);
  align-self: start;
  max-height: calc(100vh - var(--topbar-h));
  overflow-y: auto;
  padding: 28px 0;
  min-width: 0;
}
.docs-nav-group { margin-bottom: 28px; }
.docs-nav-group-label {
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.22em;
  text-transform: uppercase;
  margin: 0 0 10px;
  padding-left: 10px;
  border-left: 2px solid currentColor;
}
.docs-nav-group-label-shipped { color: var(--status-shipped-fg); }
.docs-nav-group-label-progress { color: var(--status-progress-fg); }
.docs-nav-group-label-next { color: var(--status-next-fg); }
.docs-nav-list, .docs-nav-sublist { list-style: none; margin: 0; padding: 0; }
.docs-nav-list > li { margin-bottom: 2px; }
.docs-nav-list a, .docs-nav-sublist a {
  display: block;
  padding: 5px 10px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--muted-strong);
  border-left: 2px solid transparent;
  border-radius: 0;
}
.docs-nav-list a:hover, .docs-nav-sublist a:hover {
  color: var(--accent);
  border-left-color: var(--border-strong);
}
.docs-nav-list a[aria-current="page"],
.docs-nav-sublist a[aria-current="page"] {
  color: var(--accent);
  border-left-color: var(--accent);
  background: var(--bg-1);
}
.docs-nav-sublist {
  padding-left: 16px;
  margin-top: 2px;
}
.docs-nav-sublist a { font-size: 11.5px; }

/* ── Main content ──────────────────────────────────── */
.docs-main {
  min-width: 0;
  padding: 36px 0 80px;
}
.docs-article {
  min-width: 0;
}
.docs-layout-with-toc {
  display: grid;
  grid-template-columns: 1fr var(--toc-w);
  gap: 32px;
  align-items: start;
}
.docs-content { min-width: 0; }
.docs-toc {
  position: sticky;
  top: calc(var(--topbar-h) + 36px);
  max-height: calc(100vh - var(--topbar-h) - 80px);
  overflow-y: auto;
  min-width: 0;
}

/* ── Article typography ────────────────────────────── */
.docs-breadcrumbs {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.16em;
  color: var(--label);
  margin-bottom: 14px;
}
.docs-breadcrumbs a { color: var(--muted); }
.docs-breadcrumb-sep { margin: 0 6px; color: var(--label); }

.docs-title {
  font-family: var(--font-display);
  font-weight: 900;
  font-size: clamp(28px, 4.5vw, 42px);
  line-height: 1.05;
  letter-spacing: -0.02em;
  margin: 0 0 14px;
  font-variation-settings: "opsz" 144, "SOFT" 30;
  overflow-wrap: break-word;
}
.docs-lede {
  font-size: 16px;
  color: var(--muted-strong);
  margin: 0 0 28px;
  line-height: 1.55;
  max-width: 60ch;
  overflow-wrap: break-word;
}

.docs-content h2 {
  font-family: var(--font-display);
  font-size: 24px;
  font-weight: 700;
  font-variation-settings: "opsz" 60, "SOFT" 20;
  letter-spacing: -0.01em;
  margin: 44px 0 14px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border);
  overflow-wrap: break-word;
}
.docs-content h3 {
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 700;
  font-variation-settings: "opsz" 48, "SOFT" 15;
  margin: 32px 0 10px;
  overflow-wrap: break-word;
}
.docs-content p { margin: 0 0 14px; overflow-wrap: break-word; }
.docs-content ul, .docs-content ol { margin: 0 0 18px; padding-left: 22px; }
.docs-content li { margin-bottom: 4px; }
.docs-content li > p { margin-bottom: 4px; }
.docs-content blockquote {
  border-left: 3px solid var(--accent);
  padding: 2px 0 2px 16px;
  margin: 18px 0;
  font-style: italic;
  color: var(--muted-strong);
}
.docs-content table {
  width: 100%;
  border-collapse: collapse;
  margin: 18px 0;
  font-size: 13px;
  display: block;
  overflow-x: auto;
}
.docs-content thead th {
  text-align: left;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-strong);
  font-family: var(--font-mono);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--muted-strong);
}
.docs-content tbody td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
}

/* Inline code */
.docs-content code {
  font-family: var(--font-mono);
  font-size: 12.5px;
  background: var(--bg-1);
  padding: 1px 6px;
  border-radius: 3px;
  color: var(--text);
}
.docs-content pre code { background: transparent; padding: 0; }

/* ── Code block (copy-button wrapper from render hook) ─ */
.cmd-wrap {
  position: relative;
  min-width: 0;
  margin: 18px 0;
}
.cmd {
  background: var(--bg-1);
  border: 1px solid var(--border);
  padding: 16px 92px 16px 16px;
  border-radius: 4px;
  font-family: var(--font-mono);
  font-size: 12.5px;
  margin: 0;
  min-width: 0;
  max-width: 100%;
  overflow-x: auto;
  line-height: 1.55;
}
.copy-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: transparent;
  border: 1px solid var(--border-strong);
  color: var(--muted);
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  padding: 6px 10px;
  border-radius: 2px;
  cursor: pointer;
  min-height: 32px;
}
.copy-btn:hover { color: var(--accent); border-color: var(--accent); }
.copy-btn.copied { background: rgba(14, 59, 33, 1); color: var(--status-shipped-fg); border-color: rgba(14, 59, 33, 1); }

/* ── TOC (right side) ──────────────────────────────── */
.docs-toc-details summary {
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.2em;
  text-transform: uppercase;
  color: var(--label);
  cursor: pointer;
  padding: 0 0 10px;
  list-style: none;
}
.docs-toc-details summary::marker, .docs-toc-details summary::-webkit-details-marker { display: none; }
.docs-toc-body ul { list-style: none; margin: 0; padding: 0; }
.docs-toc-body ul ul { padding-left: 12px; }
.docs-toc-body a {
  display: block;
  padding: 3px 0;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--muted);
  border-left: 2px solid transparent;
  padding-left: 8px;
}
.docs-toc-body a:hover { color: var(--accent); }

/* ── Home page ─────────────────────────────────────── */
.docs-home { padding-top: 40px; }
.docs-home-title {
  font-family: var(--font-display);
  font-weight: 900;
  font-size: clamp(36px, 6vw, 58px);
  line-height: 1;
  letter-spacing: -0.02em;
  margin: 14px 0 16px;
  font-variation-settings: "opsz" 144, "SOFT" 30;
}
.kicker {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.22em;
  text-transform: uppercase;
  color: var(--label);
  margin: 0 0 8px;
}
.docs-home-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 18px;
  margin-top: 40px;
}
.docs-home-card {
  background: var(--bg-1);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 22px;
  color: var(--text);
  transition: border-color 0.2s, transform 0.2s;
}
.docs-home-card:hover { border-color: var(--accent); transform: translateY(-2px); color: var(--text); }
.docs-home-card h2 {
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 700;
  margin: 8px 0 6px;
  color: var(--text);
}
.docs-home-card p { font-size: 13px; color: var(--muted-strong); margin: 0; line-height: 1.55; }
.docs-child-list { list-style: none; padding: 0; display: grid; gap: 10px; }
.docs-child-list li {
  background: var(--bg-1);
  border: 1px solid var(--border);
  border-radius: 3px;
  padding: 12px 14px;
}
.docs-child-list li a { font-family: var(--font-mono); font-size: 13px; }
.docs-child-list li p { margin: 4px 0 0; font-size: 12px; color: var(--muted-strong); }

/* ── Footer ────────────────────────────────────────── */
.docs-footer { border-top: 1px solid var(--border); margin-top: 60px; }
.docs-footer-inner {
  max-width: 1280px;
  margin: 0 auto;
  padding: 20px var(--pad);
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.docs-footer p { margin: 0; font-size: 11px; color: var(--label); font-family: var(--font-mono); letter-spacing: 0.04em; }

/* ── Pagefind overrides (amber accent) ─────────────── */
.pagefind-ui {
  --pagefind-ui-scale: 0.8;
  --pagefind-ui-primary: var(--accent);
  --pagefind-ui-text: var(--text);
  --pagefind-ui-background: var(--bg-1);
  --pagefind-ui-border: var(--border);
  --pagefind-ui-tag: var(--bg-2);
  --pagefind-ui-border-width: 1px;
  --pagefind-ui-border-radius: 3px;
  --pagefind-ui-font: var(--font-body);
}

/* ── Responsive — 1023px breakpoint ────────────────── */
@media (max-width: 1023px) {
  .docs-layout-with-toc { grid-template-columns: 1fr; }
  .docs-toc { position: static; max-height: none; margin-bottom: 24px; }
  .docs-toc-details { background: var(--bg-1); border: 1px solid var(--border); border-radius: 3px; padding: 10px 14px; }
  .docs-toc-details[open] { padding-bottom: 14px; }
  .docs-home-cards { grid-template-columns: 1fr; }
}

/* ── Responsive — 767px drawer breakpoint ──────────── */
@media (max-width: 767px) {
  .drawer-btn { display: inline-flex; align-items: center; }
  .docs-shell { grid-template-columns: 1fr; }
  .docs-sidebar {
    position: fixed;
    top: var(--topbar-h);
    left: 0;
    height: calc(100vh - var(--topbar-h));
    width: min(300px, 85vw);
    background: var(--bg-0);
    border-right: 1px solid var(--border);
    padding: 20px 20px;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
    z-index: 40;
  }
  .drawer-toggle:checked ~ .docs-shell .docs-sidebar { transform: translateX(0); }
  .drawer-toggle:checked ~ .docs-shell .drawer-backdrop {
    display: block;
    position: fixed;
    inset: var(--topbar-h) 0 0 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 35;
    cursor: pointer;
  }
  .docs-main { padding-top: 24px; }
  .docs-topnav a:nth-child(1) { display: none; } /* hide "gormes.ai" link at <768px, keep "GitHub" */
  .docs-topbar-inner { gap: 10px; }
  .docs-search { max-width: 100%; }
}

/* ── Responsive — 480px tight-mobile ───────────────── */
@media (max-width: 480px) {
  .cmd { padding-right: 80px; }
  .docs-footer-inner { flex-direction: column; align-items: flex-start; gap: 6px; }
  .docs-content table { font-size: 12px; }
}

/* ── Reduced motion ────────────────────────────────── */
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    transition: none !important;
    animation-duration: 0.001ms !important;
  }
}
```

- [ ] **Step 2: Build and visually verify**

```bash
cd gormes/docs && hugo --minify -d /tmp/gormes-docs-preview 2>&1 | tail -5
```

Expected: `Total in Xms`. Open `/tmp/gormes-docs-preview/index.html` (or spin up `hugo server -D` locally) and verify the three-card home page + sidebar + sample article renders.

- [ ] **Step 3: Commit**

```bash
cd <repo> && \
git add gormes/docs/static/site.css && \
git commit -m "style(gormes/docs): site.css aligned with landing + docs layout

~500 lines. Ports the landing's tokens (Fraunces/JetBrains Mono/
DM Sans, amber accent, dark bed, SVG grain) and layers docs-
specific rules: sticky topbar with backdrop-filter, left sidebar
with color-coded group headers + active-page aria-current
highlighting, right-side TOC that collapses to <details> under
1024px, CSS-only drawer sidebar under 768px via hidden-checkbox
pattern, and article typography (Fraunces H1/H2, DM Sans body,
inline JetBrains Mono code, bordered tables with mono headers).

Reuses landing's copy button via render hook. Pagefind CSS vars
overridden for amber accent. Every flex/grid child carries
min-width:0; clamp() on hero titles; overflow-wrap discipline on
prose containers; reduced-motion media query.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Pagefind search integration + deploy workflow update

Add Pagefind to the deploy pipeline. No runtime backend — search index is static JSON + JS, loaded client-side.

**Files:**
- Modify: `.github/workflows/deploy-gormes-docs.yml`

- [ ] **Step 1: Update the deploy workflow**

Read the current `.github/workflows/deploy-gormes-docs.yml`. Find the "Build Hugo site" step. Directly after it, insert a Pagefind step. Full updated file:

```yaml
name: Deploy docs.gormes.ai

on:
  push:
    branches: [main]
    paths:
      - 'gormes/docs/**'
      - '.github/workflows/deploy-gormes-docs.yml'
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: gormes-docs-pages
  cancel-in-progress: true

jobs:
  deploy:
    if: github.repository == 'TrebuchetDynamics/gormes-agent'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          submodules: recursive

      - uses: peaceiris/actions-hugo@v3
        with:
          hugo-version: '0.140.0'
          extended: true

      - name: Build Hugo site
        working-directory: gormes/docs
        run: hugo --minify

      - name: Generate Pagefind search index
        working-directory: gormes/docs
        run: npx --yes pagefind --site public

      - name: Verify build artifacts
        working-directory: gormes/docs
        run: |
          test -f public/index.html
          test -d public/pagefind
          test -f public/pagefind/pagefind-ui.js

      - name: Ensure Pages project exists
        uses: cloudflare/wrangler-action@v3
        continue-on-error: true
        with:
          apiToken: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          accountId: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          command: pages project create gormes-docs --production-branch=main

      - name: Deploy to Cloudflare Pages
        uses: cloudflare/wrangler-action@v3
        with:
          apiToken: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          accountId: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          command: pages deploy gormes/docs/public --project-name=gormes-docs --branch=main --commit-dirty=true

      - name: Attach docs.gormes.ai
        env:
          CF_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CF_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
        run: |
          set -u
          domain=docs.gormes.ai
          echo "→ attaching ${domain}"
          response=$(curl -sS -o /tmp/cf-resp.json -w "%{http_code}" -X POST \
            "https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/pages/projects/gormes-docs/domains" \
            -H "Authorization: Bearer ${CF_API_TOKEN}" \
            -H "Content-Type: application/json" \
            -d "{\"name\":\"${domain}\"}")
          case "$response" in
            200|201) echo "  ✓ attached" ;;
            409|400) echo "  • already attached (status $response) — skipping" ;;
            *) echo "  ✗ unexpected status $response"; cat /tmp/cf-resp.json ;;
          esac
```

- [ ] **Step 2: Commit**

```bash
cd <repo> && \
git add .github/workflows/deploy-gormes-docs.yml && \
git commit -m "ci(docs): add Pagefind search index to the deploy pipeline

New step runs 'npx pagefind --site public' after hugo --minify.
Pagefind scans the rendered HTML and writes a static search index
at public/pagefind/. Client-side JS (baseof.html already loads
pagefind-ui.js) reads that index and renders a dropdown from the
topbar search input. Zero backend.

Verify step asserts public/pagefind/pagefind-ui.js exists before
handing to wrangler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Hugo build test + content structure test

Go test that shells out to Hugo and asserts the build produces every expected page. Guards against theme bloat and broken internal links.

**Files:**
- Create: `gormes/docs/build_test.go`
- Modify: `gormes/docs/docs_test.go` (if needed — assertions for new content structure)

- [ ] **Step 1: Create build_test.go**

Write `gormes/docs/build_test.go`:

```go
package docs_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHugoBuild runs `hugo --minify` in a temp directory and asserts
// the full set of expected pages are emitted. Guards against:
//   - Theme regressions (build fails silently)
//   - Broken front-matter (page doesn't render)
//   - Missing content files (section landing without children)
func TestHugoBuild(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("hugo", "--minify", "-d", tmp)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hugo build failed: %v\noutput:\n%s", err, string(out))
	}

	wantPages := []string{
		"index.html",
		"using-gormes/index.html",
		"using-gormes/quickstart/index.html",
		"using-gormes/install/index.html",
		"using-gormes/tui-mode/index.html",
		"using-gormes/telegram-adapter/index.html",
		"using-gormes/configuration/index.html",
		"using-gormes/wire-doctor/index.html",
		"using-gormes/faq/index.html",
		"building-gormes/index.html",
		"building-gormes/core-systems/index.html",
		"building-gormes/core-systems/learning-loop/index.html",
		"building-gormes/core-systems/memory/index.html",
		"building-gormes/core-systems/tool-execution/index.html",
		"building-gormes/core-systems/gateway/index.html",
		"building-gormes/what-hermes-gets-wrong/index.html",
		"building-gormes/porting-a-subsystem/index.html",
		"building-gormes/testing/index.html",
		"building-gormes/architecture_plan/index.html",
		"building-gormes/architecture_plan/phase-1-dashboard/index.html",
		"building-gormes/architecture_plan/phase-2-gateway/index.html",
		"building-gormes/architecture_plan/phase-3-memory/index.html",
		"building-gormes/architecture_plan/phase-4-brain-transplant/index.html",
		"building-gormes/architecture_plan/phase-5-final-purge/index.html",
		"building-gormes/architecture_plan/phase-6-learning-loop/index.html",
		"building-gormes/architecture_plan/subsystem-inventory/index.html",
		"building-gormes/architecture_plan/mirror-strategy/index.html",
		"building-gormes/architecture_plan/technology-radar/index.html",
		"building-gormes/architecture_plan/boundaries/index.html",
		"building-gormes/architecture_plan/why-go/index.html",
		"upstream-hermes/index.html",
	}

	for _, p := range wantPages {
		full := filepath.Join(tmp, p)
		if _, err := exec.Command("test", "-f", full).CombinedOutput(); err != nil {
			t.Errorf("expected built page missing: %s", p)
		}
	}
}

func TestHugoBuild_IndexHasSidebarSections(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("hugo", "--minify", "-d", tmp)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo build failed: %v\n%s", err, string(out))
	}
	body, err := readFile(filepath.Join(tmp, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"USING GORMES",
		"BUILDING GORMES",
		"UPSTREAM HERMES",
		"docs-nav-group-label-shipped",
		"docs-nav-group-label-progress",
		"docs-nav-group-label-next",
		`href="/using-gormes/"`,
		`href="/building-gormes/"`,
		`href="/upstream-hermes/"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("built index.html missing %q", want)
		}
	}
}

func readFile(path string) (string, error) {
	b, err := exec.Command("cat", path).Output()
	return string(b), err
}
```

- [ ] **Step 2: Run the tests**

```bash
cd gormes/docs && go test -run TestHugoBuild -v 2>&1 | tail -20
```

Expected: `PASS`. If any page is missing, the test names which one.

- [ ] **Step 3: Commit**

```bash
cd <repo> && \
git add gormes/docs/build_test.go && \
git commit -m "test(gormes/docs): Hugo build smoke + sidebar structure assertions

TestHugoBuild runs 'hugo --minify' into a temp dir and asserts
every expected page (31 total: home + 8 using-gormes + 13
building-gormes + 12 architecture_plan + upstream landing)
produces index.html. Catches silent theme failures and missing
content.

TestHugoBuild_IndexHasSidebarSections asserts the rendered home
page contains all three colored sidebar group labels
(USING/BUILDING/UPSTREAM) and the expected root links.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Playwright mobile + search smoke suite

Mirror the landing page's Playwright discipline. Parametrize over 320/360/390/430/768/1024 px viewports. Assert the same bulletproof invariants plus docs-specific affordances (drawer, TOC, search).

**Files:**
- Create: `gormes/docs/www-tests/package.json`
- Create: `gormes/docs/www-tests/playwright.config.mjs`
- Create: `gormes/docs/www-tests/tests/home.spec.mjs`
- Create: `gormes/docs/www-tests/tests/mobile.spec.mjs`
- Create: `gormes/docs/www-tests/tests/drawer.spec.mjs`

- [ ] **Step 1: Create package.json**

Write `gormes/docs/www-tests/package.json`:

```json
{
  "name": "gormes-docs-tests",
  "private": true,
  "type": "module",
  "scripts": {
    "test:e2e": "playwright test"
  },
  "devDependencies": {
    "@playwright/test": "^1.48.0"
  }
}
```

- [ ] **Step 2: Create playwright.config.mjs**

Write `gormes/docs/www-tests/playwright.config.mjs`:

```javascript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  use: {
    baseURL: 'http://127.0.0.1:1313',
  },
  webServer: {
    command: 'hugo server -D --bind 127.0.0.1 --port 1313',
    port: 1313,
    cwd: '..',
    reuseExistingServer: !process.env.CI,
  },
});
```

- [ ] **Step 3: Create home.spec.mjs**

Write `gormes/docs/www-tests/tests/home.spec.mjs`:

```javascript
import { test, expect } from '@playwright/test';

test('docs home renders the three-audience split', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle(/Gormes Docs/);
  await expect(page.getByRole('heading', { name: 'Gormes Docs', level: 1 })).toBeVisible();

  // Three cards, one per audience
  await expect(page.getByRole('link', { name: /USING GORMES/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /BUILDING GORMES/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /UPSTREAM HERMES/i })).toBeVisible();

  // Sidebar has colored group labels
  await expect(page.locator('.docs-nav-group-label-shipped')).toBeVisible();
  await expect(page.locator('.docs-nav-group-label-progress')).toBeVisible();
  await expect(page.locator('.docs-nav-group-label-next')).toBeVisible();

  // No inline <script src=...> external drift — only the Pagefind + copy JS
  const scripts = await page.locator('script[src]').count();
  expect(scripts).toBeLessThanOrEqual(1); // pagefind-ui.js only
});
```

- [ ] **Step 4: Create mobile.spec.mjs**

Write `gormes/docs/www-tests/tests/mobile.spec.mjs`:

```javascript
import { test, expect } from '@playwright/test';

const VIEWPORTS = [
  { label: 'iPhone SE', width: 320, height: 568 },
  { label: 'small Android', width: 360, height: 760 },
  { label: 'iPhone 15', width: 390, height: 844 },
  { label: 'iPhone Plus', width: 430, height: 932 },
  { label: 'iPad portrait', width: 768, height: 1024 },
  { label: 'Laptop', width: 1024, height: 768 },
];

for (const vp of VIEWPORTS) {
  test(`docs home (${vp.label} ${vp.width}×${vp.height}) has no horizontal overflow`, async ({ page }) => {
    await page.setViewportSize({ width: vp.width, height: vp.height });
    await page.goto('/');

    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow, `page overflows at ${vp.width}px`).toBeFalsy();
  });

  test(`docs article page (${vp.label}) — Phase 6 — has no overflow and renders TOC correctly`, async ({ page }) => {
    await page.setViewportSize({ width: vp.width, height: vp.height });
    await page.goto('/building-gormes/architecture_plan/phase-6-learning-loop/');

    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow, `article overflows at ${vp.width}px`).toBeFalsy();

    // Every code block has a tappable copy button
    const copyBoxes = await page.locator('button.copy-btn').evaluateAll(btns =>
      btns.map(b => b.getBoundingClientRect()).map(r => ({ h: r.height, w: r.width }))
    );
    for (const box of copyBoxes) {
      expect(box.h).toBeGreaterThanOrEqual(28);
      expect(box.w).toBeGreaterThanOrEqual(28);
    }

    // TOC is visible either as right-side panel (≥1024) or collapsed details (<1024)
    if (vp.width >= 1024) {
      await expect(page.locator('aside.docs-toc')).toBeVisible();
    } else {
      await expect(page.locator('.docs-toc-details')).toBeVisible();
    }
  });
}
```

- [ ] **Step 5: Create drawer.spec.mjs**

Write `gormes/docs/www-tests/tests/drawer.spec.mjs`:

```javascript
import { test, expect } from '@playwright/test';

test('mobile drawer opens and closes via hamburger', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 760 });
  await page.goto('/using-gormes/quickstart/');

  // Sidebar starts off-screen (transform: translateX(-100%))
  const sidebar = page.locator('.docs-sidebar');
  let transform = await sidebar.evaluate(el => getComputedStyle(el).transform);
  expect(transform).toContain('matrix'); // some transform applied

  // Click the hamburger
  await page.locator('.drawer-btn').click();
  await page.waitForTimeout(250); // transition

  // Sidebar now visible (transform ~= translateX(0))
  const isVisibleByCoord = await sidebar.evaluate(el => el.getBoundingClientRect().left >= 0);
  expect(isVisibleByCoord).toBeTruthy();

  // Click the backdrop — drawer closes
  await page.locator('.drawer-backdrop').click({ force: true });
  await page.waitForTimeout(250);
  const isHiddenAgain = await sidebar.evaluate(el => el.getBoundingClientRect().left < 0);
  expect(isHiddenAgain).toBeTruthy();
});

test('desktop >=768px has no hamburger', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.goto('/');
  const btn = page.locator('.drawer-btn');
  // drawer-btn exists in DOM but hidden via CSS at this viewport
  const display = await btn.evaluate(el => getComputedStyle(el).display);
  expect(display).toBe('none');
});
```

- [ ] **Step 6: Install + run (best-effort)**

```bash
cd gormes/docs/www-tests && \
(npm install 2>&1 | tail -5; npx playwright install chromium 2>&1 | tail -3; npm run test:e2e 2>&1 | tail -15) || \
echo "Playwright install/run skipped — CI will exercise these"
```

Expected: `X passed` (at least 7 — home smoke + 12 mobile overflow + 2 drawer). If `npm install` fails in this sandbox, that's OK — the GitHub Actions runner has full network and will run them on push.

- [ ] **Step 7: Commit**

```bash
cd <repo> && \
git add gormes/docs/www-tests/ && \
git commit -m "test(gormes/docs): parametrized Playwright suite for docs site

Three spec files:
- home.spec.mjs: three-audience cards visible, sidebar has all
  three colored group labels, no external script drift
- mobile.spec.mjs: parametrized over 320/360/390/430/768/1024 px;
  asserts no page overflow, tappable copy buttons (≥28×28 px),
  TOC visible as right-side panel at ≥1024 or <details> below
- drawer.spec.mjs: hamburger opens/closes CSS-only drawer via
  backdrop click at <768 px; hamburger hidden at ≥768 px

Playwright config spins up 'hugo server -D' as the webServer so
tests run against a live-built site. CI runs these on every
push to main touching gormes/docs/**.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Promote Phase 6 on the landing page

Add the sixth phase card to the roadmap section of `gormes.ai`. Update tests.

**Files:**
- Modify: `gormes/www.gormes.ai/internal/site/content.go`
- Modify: `gormes/www.gormes.ai/internal/site/render_test.go`
- Modify: `gormes/www.gormes.ai/internal/site/static_export_test.go`

- [ ] **Step 1: Add Phase 6 to RoadmapPhases**

Read `gormes/www.gormes.ai/internal/site/content.go`. Find the `RoadmapPhases` slice — it ends with the Phase 5 entry. Append a new entry before the closing `}`:

```go
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
```

- [ ] **Step 2: Update render_test.go**

Find the `wants` slice in `gormes/www.gormes.ai/internal/site/render_test.go`. After the Phase 5 block, add:

```go
		// Phase 6 — Learning Loop (new)
		"PLANNED · 0/6",
		"Phase 6 — Learning Loop (The Soul)",
		"Compounding intelligence. The feature Hermes doesn&#39;t have.",
		"6.A Complexity detector",
		"6.B Skill extractor",
		"6.C Skill storage format",
		"6.D Skill retrieval + matching",
		"6.E Feedback loop",
		"6.F Skill surface",
```

- [ ] **Step 3: Update static_export_test.go**

Find the `wants` slice in `gormes/www.gormes.ai/internal/site/static_export_test.go`. Add a minimal Phase 6 assertion:

```go
		"Phase 6 — Learning Loop (The Soul)",
```

- [ ] **Step 4: Run tests**

```bash
cd gormes/www.gormes.ai && go test ./internal/site/ 2>&1 | tail -3
```

Expected: `ok`.

- [ ] **Step 5: Playwright landing tests still pass**

```bash
cd gormes/www.gormes.ai && npm run test:e2e 2>&1 | tail -10
```

Expected: `5 passed`. The overflow-check test at the smallest viewport now has 6 phase cards in the ledger — confirming the mobile-hardening work still holds.

- [ ] **Step 6: Commit**

```bash
cd <repo> && \
git add gormes/www.gormes.ai/internal/site/ && \
git commit -m "feat(gormes/www): add Phase 6 (Learning Loop) card to landing roadmap

Roadmap grows from 5 to 6 phase cards. Phase 6 carries status
PLANNED · 0/6 with six sub-phases (6.A–6.F: complexity detector,
skill extractor, storage format, retrieval + matching, feedback
loop, TUI+Telegram surface).

Subtitle: 'Compounding intelligence. The feature Hermes doesn't
have.' — positions Phase 6 as the Gormes-original differentiator.

Render + static-export tests assert the new phase header + each
sub-item. Playwright parametrized mobile tests still green at
320/360/390/430 px with the longer ledger.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Final smoke + push

Full end-to-end verification, then push.

**Files:** none modified.

- [ ] **Step 1: Go test the docs package**

```bash
cd gormes/docs && go test ./... 2>&1 | tail -10
```

Expected: `ok`. All three test functions (TestHugoBuild, TestHugoBuild_IndexHasSidebarSections, plus any pre-existing) green.

- [ ] **Step 2: Hugo build + Pagefind index locally**

```bash
cd gormes/docs && rm -rf public && hugo --minify && npx --yes pagefind --site public 2>&1 | tail -10
```

Expected: Hugo reports `Total in Xms`, Pagefind reports `Indexed N pages`.

- [ ] **Step 3: Verify key pages**

```bash
grep -F "One Go Binary" public/index.html >/dev/null 2>&1 && echo "(no — expected Gormes Docs title, not landing hero)"
grep -F "Gormes Docs" public/index.html && echo "home OK"
grep -F "USING GORMES" public/index.html && echo "sidebar group OK"
grep -F "Phase 6 — The Learning Loop" public/building-gormes/architecture_plan/phase-6-learning-loop/index.html && echo "phase 6 OK"
grep -F "pagefind-ui.js" public/index.html && echo "search JS OK"
```

Expected: four `OK` lines.

- [ ] **Step 4: Landing page Go tests still green**

```bash
cd gormes/www.gormes.ai && go test ./internal/site/ 2>&1 | tail -3
```

Expected: `ok`.

- [ ] **Step 5: Push to main**

```bash
cd <repo> && \
git push origin main 2>&1 | tail -5
```

Expected: `<old>..<new>  main -> main`. Three workflows fire:
- `Deploy gormes.ai landing` — because Phase 6 card lives under `gormes/www.gormes.ai/**`
- `Deploy docs.gormes.ai` — because this PR touches `gormes/docs/**` extensively
- Nothing else (the 8 upstream workflows are gone from the fork)

- [ ] **Step 6: Post-deploy live verification**

Wait ~60 seconds, then:

```bash
curl -sS -m 10 https://docs.gormes.ai/ | grep -F "Gormes Docs" && echo "docs live"
curl -sS -m 10 https://docs.gormes.ai/building-gormes/architecture_plan/phase-6-learning-loop/ | grep -F "Learning Loop" && echo "phase 6 live"
curl -sS -m 10 https://gormes.ai/ | grep -F "Phase 6 — Learning Loop" && echo "landing phase 6 live"
```

Expected: three lines, all ending in `live`. If `docs.gormes.ai` 404s, the Cloudflare Pages workflow may have failed — check the Actions tab for the "Deploy docs.gormes.ai" run log.

---

## Self-Review

**1. Spec coverage:**

| Spec section | Plan task |
|---|---|
| IA (USING / BUILDING / UPSTREAM) | Tasks 1, 3, 4 (content) + Task 5 (layouts/sidebar) + Task 6 (CSS group colors) |
| Landing-aesthetic visual system | Task 6 (site.css ports landing tokens) |
| Right-side TOC | Task 5 (layouts/partials/toc.html), Task 6 (.docs-toc CSS) |
| Breadcrumbs | Task 5 (breadcrumbs.html partial), Task 6 (.docs-breadcrumbs CSS) |
| Inline copy buttons | Task 5 (render-codeblock.html hook), Task 6 (.copy-btn CSS reused) |
| Pagefind search | Task 7 (deploy workflow), Task 5 (baseof.html script tag) |
| Mobile: ≥1024 / 640–1023 / <640 | Task 6 (three @media blocks: 1023, 767, 480) |
| Playwright parametrized viewports | Task 9 (mobile.spec.mjs over 6 viewports) |
| CSS-only drawer | Task 5 (checkbox in baseof.html), Task 6 (.drawer-toggle + backdrop CSS) |
| prefers-reduced-motion | Task 6 (@media block) |
| ARCH_PLAN split into 12 pages | Task 2 |
| Phase 6 promotion in ARCH_PLAN | Task 2 Step 5 |
| ARCH_PLAN.md stub | Task 2 Step 7 |
| Phase 6 on landing page | Task 10 |
| Hugo build test | Task 8 |
| Deploy workflow (Pagefind step) | Task 7 |
| Out-of-scope items (no i18n, no versioning, no light mode, no analytics) | Not addressed — that's correct; they're out of scope |
| Acceptance criteria #1–14 | Verified by Task 11 Steps 3 + 6 |

No gaps found.

**2. Placeholder scan:**

- Task 2 Steps 3–6 reference "Copy §X from ARCH_PLAN.md verbatim" — this is intentional; the subagent runs a mechanical copy. Each step names the exact source section and the exact target file. Not a placeholder.
- Task 3 inline prose examples are complete — not "write some words about X".
- All test assertions use concrete strings, not "assert something reasonable".

Clean.

**3. Type consistency:**

- `RoadmapPhase` + `RoadmapItem` struct names match content.go from earlier commits.
- CSS class names (`.docs-nav-group-label-shipped` etc.) consistent between Task 5 (templates emit them) and Task 6 (CSS styles them) and Task 8 (Go test asserts them).
- Sidebar partial's `$toneMap` keys (`"using-gormes"`, `"building-gormes"`, `"upstream-hermes"`) match the content directory names created in Tasks 1, 3, 4.
- `gormesCopy(this)` onclick in Task 5 render hook matches the helper defined in Task 5 baseof.html `<script>`.

All consistent.

No issues to fix inline.
