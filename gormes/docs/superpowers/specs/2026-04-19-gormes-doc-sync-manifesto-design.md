# Gormes Documentation Sync and Public Manifesto Design Spec

**Date:** 2026-04-19
**Author:** Xel (via Codex brainstorm)
**Status:** Approved design direction; ready for planning
**Scope:** Realign the Gormes README, public Hugo docs surface, and Phase 2 spec index around the current Go-native architecture without publishing unsupported benchmark claims.

---

## 1. Purpose

Gormes has shipped enough real architecture that the documentation is now lagging the code:

- Phase 2.A landed the Go-native Tool Registry and doctor checks;
- Route-B reconnect and the 16 ms coalescing mailbox are now part of the resilience story;
- Phase 2.B Telegram has a concrete design that explains why separate binaries are intentional, not missing features.

Right now those facts are scattered across roadmap docs, specs, and recent commits. The public surface does not explain the engineering moat cleanly enough, and the repository README is still too small to serve as the operator-facing front door.

This work creates a documentation split with one clear rule:

- `README.md` explains **how to run Gormes today**.
- the Hugo docs site explains **why Gormes is architecturally better**.

The result should make the documentation look like it belongs to the same project as the code: precise, technical, and disciplined.

---

## 2. Locked Decisions

### 2.1 Public manifesto, not an internal scroll

The manifesto belongs on the public Hugo surface, not only in repository-root docs.

The canonical "why" page is:

- `docs/content/why-gormes.md`

That page is the public home of the manifesto. If a root-level `docs/MANIFESTO.md` ever exists, it is only a pointer or internal note, not the canonical copy. Duplicating the full manifesto in two places is explicitly rejected because it invites drift.

### 2.2 README is the operator-facing "how", not the full philosophy

`README.md` stays short and practical. Its job is:

- explain the operational thesis in one paragraph;
- show build/run steps immediately;
- explain `gormes doctor`, including offline/local tool validation;
- call out the architectural edge in a few proof-linked bullets.

It must not become a second long-form manifesto page.

### 2.3 Proof-first claims only

All "brag" copy must be traceable to one of:

- shipped code in `gormes/`;
- approved specs and plans in `docs/superpowers/`;
- existing repository docs such as `docs/ARCH_PLAN.md` or `docs/THEORETICAL_ADVANTAGES_GORMES_HERMES.md`.

The docs must not publish unmeasured claims such as:

- exact startup latency;
- exact memory-at-scale claims;
- universal token-cost reduction claims.

If a number is used, it must already be documented or directly reproducible in-tree. For example, the current "7.9 MB" binary figure is acceptable only as a current documented fact, not a timeless guarantee.

### 2.4 Public message architecture is fixed

The manifesto page uses these four sections, in this order:

1. **Operational Moat**
2. **Wire Doctor**
3. **Chaos Resilience**
4. **Surgical Architecture**

Those sections are the core public argument. A short closing paragraph may mention Trebuchet Dynamics' engineering posture, but the page must not drift into generic company-marketing copy.

### 2.5 Phase 2 specs become an indexable proof set

The specs directory needs a real index plus explicit reciprocal links for the two most relevant public-proof docs:

- Phase 2.A Tool Registry
- Phase 2.B Telegram Scout

The implementation must create a local `docs/superpowers/specs/README.md` and add "Related Documents" blocks to both Phase 2 specs.

### 2.6 Hugo mirror constraint is real and must be handled explicitly

`docs/content/` is currently validated by `docs/docs_test.go` as a mirror of `../../website/docs`.

That means a brand-new public page such as `docs/content/why-gormes.md` cannot simply be added without also handling the mirror test. Because the active repository policy limits edits to `README.md` and files under `gormes/`, the implementation must stay inside `gormes/`.

Therefore the locked implementation strategy is:

- keep mirrored coverage strict for existing imported docs;
- explicitly allow a narrow set of **Gormes-native Hugo pages** in `docs/content/`;
- scope that allowlist to the new manifesto page instead of weakening the mirror rule broadly.

Silently ignoring the test harness or adding the page without adjusting the local rules is not acceptable.

### 2.7 No broad site rewrite

This is a documentation synchronization pass, not a full Hugo redesign.

The Hugo homepage should be updated enough to:

- brand the surface as Gormes;
- link to the manifesto;
- point readers to quick start and roadmap;

but it should not become a sprawling marketing landing page in this task.

---

## 3. Scope

### 3.1 In scope

- Refresh `README.md` as the operator-facing quick-start and architecture-summary page.
- Create a public manifesto page under `docs/content/why-gormes.md`.
- Rewrite `docs/content/_index.md` so the public docs surface starts from Gormes rather than stale Hermes branding.
- Create `docs/superpowers/specs/README.md` as the engineering-spec index.
- Add reciprocal "Related Documents" blocks to the Phase 2.A and Phase 2.B specs.
- Adjust local docs tests so the new manifesto page is validated without breaking the existing mirror discipline.
- Add or extend targeted docs tests for the new public manifesto and updated homepage copy.

### 3.2 Out of scope

- Editing Python code or legacy Python docs outside `gormes/`.
- Editing `../../website/docs` or any other path outside the allowed `gormes/` scope.
- Generating new benchmark data.
- Claiming new performance numbers not already proven in the repository.
- Rebuilding Hugo templates or CSS beyond what is needed for copy and link placement.
- Creating company, hiring, pricing, or non-technical marketing pages.

---

## 4. Documentation Architecture

### 4.1 Reader split

The documentation surface should split by user intent:

| Surface | Primary question | Style |
|---|---|---|
| `README.md` | "How do I run Gormes today?" | Short, executable, operator-facing |
| `docs/content/why-gormes.md` | "Why is this architecture better?" | Technical argument, public-facing |
| `docs/ARCH_PLAN.md` | "Where is the rewrite going?" | Strategic roadmap |
| `docs/superpowers/specs/README.md` | "Where is the proof and design detail?" | Internal engineering index |

This removes the current muddle where roadmap, proof, and quick-start content are mixed together.

### 4.2 README structure

`README.md` should contain, in order:

1. one-paragraph thesis;
2. install/build/run commands;
3. a `gormes doctor` section that explains local/offline tool-schema validation before token spend;
4. a short "Architectural Edge" section with bullets for:
   - current documented static-binary posture;
   - Route-B reconnect;
   - 16 ms coalescing mailbox;
5. links to:
   - `docs/content/why-gormes.md` as public "why";
   - `docs/ARCH_PLAN.md`;
   - Phase 2.A and Phase 2.B specs.

The README should not repeat the full manifesto prose.

### 4.3 Public manifesto structure

`docs/content/why-gormes.md` should contain:

- Hugo front matter with a Gormes-specific title and description;
- a short framing introduction;
- the four approved manifesto sections;
- a "Further Reading" or equivalent closing link block pointing at:
  - `../ARCH_PLAN.md` only if linked via repo-relative prose is appropriate in context, or the nearest public equivalent;
  - the spec index;
  - Phase 2.A and Phase 2.B.

The copy should contrast Gormes with wrapper-style architectures by naming concrete failure classes:

- tool-schema drift caught too late;
- reconnect behavior under dropped streams;
- render/event thundering-herd pressure;
- binary bloat from one-size-fits-all platform packaging.

The tone should remain engineering-forward, not chest-thumping.

### 4.4 Hugo homepage role

`docs/content/_index.md` is not the manifesto itself. Its job is to:

- correct the stale Hermes identity;
- establish Gormes as the public docs subject;
- route readers quickly to:
  - quick start;
  - Why Gormes;
  - roadmap.

The homepage should stay concise so it does not compete with the manifesto page.

### 4.5 Spec index role

`docs/superpowers/specs/README.md` should:

- list current specs in chronological order;
- give each one a one-sentence summary;
- call out the active milestone cluster;
- include an explicit "Phase 2 cross-links" section connecting:
  - `2026-04-19-gormes-phase2-tools-design.md`;
  - `2026-04-19-gormes-phase2b-telegram.md`;
  - `docs/ARCH_PLAN.md`.

This file is the internal map for contributors who want the underlying proof set behind the public docs.

### 4.6 Phase 2 reciprocal links

Both Phase 2 specs should gain a short "Related Documents" block near the top.

Each block links to:

- `docs/ARCH_PLAN.md`;
- `docs/superpowers/specs/README.md`;
- the sibling Phase 2 spec.

This makes the Tool Registry and Telegram Scout docs navigable as one architectural slice instead of isolated markdown islands.

### 4.7 Claim sourcing rules

The public docs should draw from existing in-repo proof sources:

- `docs/ARCH_PLAN.md` for the operational-moat thesis and roadmap framing;
- `docs/THEORETICAL_ADVANTAGES_GORMES_HERMES.md` for the concurrency and systems argument;
- `docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md` for tool registry and doctor framing;
- `docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md` for surgical-binary isolation and moat language;
- recent commits around `doctor`, built-in tools, and tool-loop plumbing for recency checks during implementation.

The manifesto may synthesize these sources, but it must not invent a new claim class unsupported by them.

---

## 5. Exact Files

### 5.1 Files to modify

- `README.md`
- `docs/content/_index.md`
- `docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md`
- `docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md`
- `docs/docs_test.go`
- `docs/landing_page_docs_test.go`

### 5.2 Files to create

- `docs/content/why-gormes.md`
- `docs/superpowers/specs/README.md`

No other new public doc page is required for this task.

---

## 6. Testing Strategy

This is documentation work, but it still needs test discipline because the Hugo content surface has a local harness.

### 6.1 Test-first areas

Before editing the public docs, the implementation should update tests that define the local contract:

- `docs/docs_test.go` for the mirror-coverage rule and any narrowly-scoped allowlist for Gormes-native Hugo pages;
- `docs/landing_page_docs_test.go` for public-surface assertions such as:
  - the presence of `why-gormes.md`;
  - expected manifesto section headings;
  - the updated Gormes branding on `_index.md`;
  - absence of stale Hermes-first framing where the Gormes public surface should now lead.

### 6.2 Verification command

Primary verification:

```bash
cd gormes
go test ./docs
```

If the docs are later rendered in a separate publish step, that step is outside this spec unless it already exists under `gormes/`.

---

## 7. Acceptance Criteria

The task is complete when all of the following are true:

1. `README.md` functions as a concise Gormes quick start and explicitly mentions `gormes doctor`, Route-B reconnect, and 16 ms coalescing as architectural differentiators.
2. `docs/content/why-gormes.md` exists as the public manifesto page and contains the four approved sections:
   - Operational Moat
   - Wire Doctor
   - Chaos Resilience
   - Surgical Architecture
3. `docs/content/_index.md` is Gormes-branded and links readers to the manifesto, quick start, and roadmap.
4. `docs/superpowers/specs/README.md` exists and cross-links the active Phase 2 proof docs.
5. The Phase 2.A and Phase 2.B specs each include a "Related Documents" block linking back to the roadmap, the index, and each other.
6. The docs test harness explicitly handles the new manifesto page without broadly weakening the mirror rule.
7. `go test ./docs` passes.
8. No new unsupported benchmark or capacity claims are introduced.

---

## 8. Risks and Controls

### 8.1 Overclaim risk

The easiest way to make the docs look stronger is also the fastest way to make them look unserious: publish impressive numbers with no proof.

Control:

- every numeric claim must come from an existing repository proof source;
- when proof is architectural rather than benchmarked, say so in engineering language instead of implying measured certainty.

### 8.2 Duplicate-copy drift

If the manifesto is written both in `README.md` and a root `docs/MANIFESTO.md`, the two will drift.

Control:

- keep one canonical public manifesto page;
- make README a concise operational summary with links outward.

### 8.3 Mirror-harness breakage

Adding `docs/content/why-gormes.md` without adjusting `docs/docs_test.go` will break the current mirror-coverage test.

Control:

- make the allowlist explicit, narrow, and documented;
- keep the rest of the mirrored-doc contract intact.

### 8.4 Brand confusion

The current Hugo homepage still reads as Hermes docs. If the manifesto page says "Why Gormes" but the homepage says "Hermes Agent Documentation", the public surface looks stitched together.

Control:

- update `_index.md` in the same change set as the manifesto page.

### 8.5 Documentation sprawl

The temptation is to turn this into a large site rewrite because the public-surface gap is real.

Control:

- constrain the work to README, homepage, manifesto, spec index, and reciprocal links;
- defer broader information-architecture work.

---

## 9. Summary

This documentation pass is not about making Gormes sound impressive. It is about making the docs finally tell the truth about what the project has already built.

The README should make it easy to run Gormes.
The public manifesto should explain why the Go-native architecture matters.
The spec index should make the proof set navigable.

If those three surfaces agree, the documentation stops trailing the code and starts acting like part of the moat.
