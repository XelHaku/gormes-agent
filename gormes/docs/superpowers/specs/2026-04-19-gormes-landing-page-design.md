# Gormes.io Landing Page Design Spec

**Date:** 2026-04-19
**Author:** Xel (via Codex brainstorm)
**Status:** Approved design direction; ready for planning
**Scope:** A simple, high-performance public landing page for `www.gormes.io`, built in Go and targeted at current Hermes users evaluating the Phase 1 Go port.

---

## 1. Purpose

`www.gormes.io` must explain Gormes in one screenful to the right audience:

- existing Hermes users who want a faster terminal experience now;
- contributors and early adopters who want to help finish the Go port.

The page is not a general-purpose marketing site for non-technical newcomers. It is an honest upgrade page for people who already understand Hermes.

The promise is:

> You already love Hermes. Now run it through a lightning-fast Go terminal.

Working tagline:

> **The Agent That GOes With You.**

---

## 2. Locked Decisions

### 2.1 Audience lock

Primary audience:

- existing Hermes users.

Secondary audience:

- contributors and early adopters, especially Go developers.

The site must optimize first for trust and adoption inside the existing Hermes community, not broad top-of-funnel onboarding.

### 2.2 Product framing lock

Gormes must be framed as:

- the Go frontend for Hermes Agent in Phase 1;
- a real upgrade available now;
- the first delivered slice of a larger pure-Go migration.

The page must not frame Gormes as a fully complete Go replacement today.

### 2.3 Truthfulness lock

The page must state or strongly imply all of the following:

- Phase 1 still depends on the existing Python Hermes backend;
- Gormes currently upgrades the terminal and UI layer first;
- the pure single-binary Go future is roadmap work, not a present-tense fact.

The page must not claim:

- that Hermes is already fully rewritten in Go;
- that current platform integrations are Go-native end to end;
- that Phase 1 removes the Python backend requirement;
- absolute performance guarantees such as "zero lag."

### 2.4 Implementation lock

The landing page must be built in Go, not JavaScript.

The default implementation shape is:

- `net/http`;
- `html/template`;
- `embed` for templates, CSS, and static assets;
- plain CSS for layout, typography, and motion.

No client-side framework, no hydration, and no JavaScript dependency are required for the initial release.

### 2.5 Product goal lock

The page must be:

- simple to scan;
- fast to load;
- credible to technical users;
- easy to extend as the roadmap progresses.

The site is a landing page, not a docs portal, blog, or showcase directory in Phase 1.5.

---

## 3. Positioning

### 3.1 Core message

The page should communicate:

1. Gormes is the Go-powered terminal face of Hermes available right now.
2. You keep the Hermes brain you already trust.
3. The migration path to a pure-Go stack is active, visible, and worth joining.

### 3.2 Tone

Tone should be:

- direct;
- technical;
- confident;
- slightly playful;
- never breathless.

It should borrow the fast rhythm and clear sectioning of `openclaw.ai` without borrowing its claim density, social-proof volume, or "does everything" sprawl.

### 3.3 Copy rule

Every major section should answer one question:

- Hero: Why should an existing Hermes user care?
- Quick start: How do I try it right now?
- Demo: What changes in practice?
- Features: What is actually better today?
- Roadmap: Where is the port going next?
- Contributor callout: How do I help?

---

## 4. Visual Direction

### 4.1 Design language

The page should feel like:

- an upgraded terminal product page;
- editorial, not startup-generic;
- minimal, but not sterile.

Recommended visual language:

- deep graphite background;
- warm off-white surfaces for code and content panels;
- brass or gold accents inherited from Hermes;
- a Go-leaning cool accent only as secondary support, not as the main brand color.

### 4.2 Typography

Use a deliberate pairing:

- expressive but readable display/sans font for headlines;
- strong monospace for command snippets and terminal demos.

Performance matters more than novelty. One text family plus one mono family is enough.

### 4.3 Motion

Motion must be CSS-only and restrained:

- subtle page-load fade/slide on hero blocks;
- at most a light stagger on feature cards;
- no scroll-jacking, carousels, or animated counters.

### 4.4 Simplicity rule

If a section needs JavaScript to feel alive, the section is too complicated for this version.

---

## 5. Information Architecture

The landing page is a single long-form route at `/`.

Required sections, in order:

1. Header / nav
2. Hero
3. Quick start
4. Demo panel
5. Feature grid
6. Roadmap
7. Contributor callout
8. Footer

No testimonial wall, no newsletter form, no showcase feed, and no press carousel in the first release.

---

## 6. Hero Section

### 6.1 Job of the hero

The hero must immediately tell an existing Hermes user:

- this is for you;
- this is real now;
- this is not a fake "future Go rewrite" teaser.

### 6.2 Hero content

Proposed structure:

- product lockup: `Hermes` / `Agent`
- product badge: `Open Source • MIT License • Phase 1 Go Port`
- headline: `The Agent That GOes With You.`
- supporting copy:

> You already love Hermes. Now run it through a faster, lighter Go terminal.
>
> Gormes is the Phase 1 Go frontend for Hermes Agent: a Bubble Tea dashboard and CLI facade that connects to your existing Python Hermes backend. Same agent. Same memory. Same workflows. A sharper terminal today, with the rewrite underway.

- primary CTA: `Run Gormes`
- secondary CTA: `Read the Roadmap`
- tertiary text link: `View on GitHub`

### 6.3 Hero honesty note

The hero must include a small but visible clarifier near the quick start or below the support copy:

> Phase 1 uses your existing Hermes backend. Pure single-binary Go arrives later in the roadmap.

---

## 7. Quick Start Block

### 7.1 Job of the quick start

The quick start must make adoption feel frictionless for current Hermes users.

It is not a generic install matrix. It is a short "already run Hermes? do this next" block.

### 7.2 Content

Required sequence:

`1. Start your Hermes backend`

```bash
API_SERVER_ENABLED=true hermes gateway start
```

`2. Build and run Gormes`

```bash
cd gormes
make build
./bin/gormes
```

### 7.3 Supporting line

Short support copy beneath the commands:

> Gormes is a drop-in Go terminal for the Hermes stack you already run.

---

## 8. Demo Panel

### 8.1 Job of the demo

The demo should prove that Gormes preserves Hermes capability while changing the feel of the interface.

### 8.2 Demo rule

The demo must be realistic and present-tense accurate.

It must not imply:

- Go-native tool execution;
- Go-native memory;
- a finished pure-Go backend.

### 8.3 Demo direction

The panel should be a styled terminal transcript showing:

- backend start;
- Gormes launch;
- one Hermes task streaming through the Go terminal.

Example direction:

```text
$ API_SERVER_ENABLED=true hermes gateway start
$ ./bin/gormes

Gormes
❯ Review the open PR and summarize the risks

  status   connected to Hermes backend
  tool     git diff main...feature-branch
  tool     scripts/run_tests.sh tests/gateway/
  tool     write_file ./notes/pr-review.md

Found 2 risks and saved a review summary.
```

The point is continuity:

- same Hermes workflows;
- cleaner, faster-feeling terminal surface.

---

## 9. Feature Grid

The feature grid should stay tight: five cards total.

Recommended cards:

### 9.1 Same Hermes brain

Copy focus:

- keeps the existing Python backend in Phase 1;
- preserves agent behavior, memory, and workflows.

### 9.2 Faster Go terminal

Copy focus:

- Bubble Tea dashboard;
- lower-latency interaction feel;
- tighter terminal rendering.

### 9.3 Drop-in path

Copy focus:

- current Hermes users do not need to relearn the system;
- Gormes is an upgrade path, not a separate product universe.

### 9.4 Honest migration

Copy focus:

- public five-phase plan;
- visible progress from UI to pure-Go core.

### 9.5 Built for contributors

Copy focus:

- clear scope;
- clear boundaries;
- attractive entry point for Go developers.

---

## 10. Roadmap Block

### 10.1 Job of the roadmap

The roadmap is not filler. It is the trust mechanism that makes the landing page honest.

It explains why Phase 1 is valuable now and why the pure-Go future is credible later.

### 10.2 Section header

Recommended header:

> **The Port Is Already Moving**

### 10.3 Intro copy

Recommended intro:

> Gormes is not a mockup and not a futureware landing page. Phase 1 ships the Go user interface first, then each layer of Hermes moves across until the stack is pure Go.

### 10.4 Phase copy

Required phases:

- `Phase 1 — The Dashboard`
  - A Go Bubble Tea interface over the existing Hermes backend. Faster terminal rendering, cleaner interaction loop, minimal migration risk.
- `Phase 2 — The Gateway`
  - Platform adapters move into Go.
- `Phase 3 — Memory`
  - Persistence and recall layers move into Go.
- `Phase 4 — The Brain`
  - Agent orchestration and prompt-building move into Go.
- `Phase 5 — The Final Purge`
  - Remaining Python dependencies are removed.

### 10.5 Closing line

Recommended close:

> Today: the fastest way to use Hermes in a terminal. Tomorrow: Hermes, fully rewritten in Go.

---

## 11. Contributor Callout

### 11.1 Job of the contributor block

The contributor block should convert the secondary audience without diluting the primary message.

### 11.2 Content

Suggested structure:

- headline: `Help Finish the Port`
- support copy:

> Phase 1 is the user-facing proof. The next phases move the gateway, memory, and agent core into Go. If you want to help build a serious Go-native agent stack, this is the seam to join.

- CTA options:
  - `Read ARCH_PLAN.md`
  - `Browse the Gormes source`
  - `Open the implementation docs`

This section should feel like an invitation to serious builders, not a vague "community" block.

---

## 12. Footer

The footer should stay compact.

Recommended links:

- GitHub
- Gormes architecture plan
- Hermes upstream reference
- License

Footer line:

> Gormes is the Go port of Hermes Agent, shipping in public.

---

## 13. Technical Design

### 13.1 Route surface

Phase 1.5 only needs:

- `GET /`
- `GET /static/*`

Health endpoints are out of scope for the initial landing-page release.

### 13.2 Rendering model

Server-rendered HTML using Go templates.

Recommended template split:

- layout template
- page template
- small reusable partials for hero, code block, feature card, and roadmap phase

### 13.3 Assets

Static CSS and any images should be embedded into the binary using `embed`.

The first version should avoid:

- external runtime CDNs;
- client-side data fetching;
- analytics scripts by default.

### 13.4 Content ownership

Most landing-page content can live as structured Go data passed into templates.

This keeps:

- copy easy to update;
- tests deterministic;
- roadmap states explicit.

---

## 14. Error Handling

The landing page has simple needs, but the design should still define failure behavior.

### 14.1 Asset failure

If CSS or non-critical assets fail, the page must remain readable HTML.

### 14.2 Template failure

Template execution errors should fail fast on startup if possible, not at request time.

### 14.3 Unknown routes

Unknown routes should return a clean plain 404 response, not an application panic or directory listing.

---

## 15. Testing

The implementation plan must cover at least:

- HTTP handler tests for `/` returning `200 OK`;
- assertions for key copy being present in rendered HTML;
- tests proving the roadmap and Phase 1 dependency note render correctly;
- tests that static assets are served;
- a guard against accidental JavaScript dependency by asserting the rendered page contains no `<script` tags and that no JavaScript assets are required to render the page.

Manual verification should also include:

- page works with JavaScript disabled;
- page loads cleanly on desktop and mobile widths;
- the first viewport clearly communicates the Phase 1 positioning.

---

## 16. Out of Scope

Not part of this landing-page release:

- blog;
- showcase feed;
- automated testimonial ingestion;
- CMS;
- newsletter signup;
- interactive playground;
- client-side animations or scroll effects;
- claims tailored to non-Hermes-first audiences.

---

## 17. Acceptance Criteria

This design is complete when the implemented page:

1. clearly targets current Hermes users first;
2. states the Phase 1 Go frontend reality without hedging or lying;
3. gives a concrete two-step way to try Gormes;
4. presents the five-phase roadmap cleanly;
5. invites contributors without overwhelming the page;
6. ships as a Go-rendered landing page with no JavaScript requirement;
7. feels simpler, faster, and more focused than a generic marketing site.

---

## 18. Final Direction

Use the structural rhythm of the current Hermes page and the clarity/tempo of `openclaw.ai`, but tighten everything around one message:

> **Gormes is the terminal upgrade for Hermes users today, and the public path to a pure-Go Hermes tomorrow.**
