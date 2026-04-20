# Gormes.ai Operator Console Redesign Spec

**Date:** 2026-04-20
**Author:** Xel (via Codex brainstorm)
**Status:** Approved design direction; ready for planning
**Scope:** Redesign `gormes/www.gormes.ai` from a generic dark landing page into an aggressive operator-console page aimed at existing Hermes users who should try Gormes locally now.

---

## 1. Purpose

The current public site works technically, but it presents Gormes like a generic dark SaaS page instead of a hardened Go control surface. That undercuts the product story.

This redesign must change that.

The page should make an existing Hermes user think:

> This is the Go shell I can try right now, not another vague rewrite promise.

The page is not trying to win broad non-technical traffic first. It is trying to convert existing Hermes users into active Gormes evaluators.

The primary action is:

- try Gormes locally now.

Everything else on the page should support that action.

---

## 2. Locked Decisions

### 2.1 Audience lock

Primary audience:

- existing Hermes users who may switch, experiment, or validate the port.

Secondary audience:

- technical buyers evaluating whether Gormes is real.

The page should optimize first for people who already understand Hermes and need proof that Gormes is tangible, usable, and worth a local run.

### 2.2 Conversion lock

The page should push one main action:

- run Gormes locally now.

The site may still expose roadmap and contribution paths, but they are support actions, not the headline conversion.

### 2.3 Tone lock

Tone should be:

- aggressive;
- direct;
- technical;
- grounded in shipped behavior;
- anti-fluff.

The copy should feel like an operator talking to another operator, not marketing speaking to a buyer committee.

### 2.4 Visual lock

The chosen design direction is:

- `Operator Console`.

This means the page must feel like a control surface, field terminal, or command rack, not a premium startup homepage.

### 2.5 Truthfulness lock

The redesign may sharpen the claims, but it must not lie.

It must remain clear that:

- Gormes is real and usable now;
- the Go-native shell and related moat layers ship today;
- the memory lattice and full brain cutover are still later phases.

The redesign must not imply:

- that the entire Hermes stack is already fully ported to Go;
- that all future roadmap work is already shipped;
- fake telemetry, fake runtime state, or fake live metrics.

### 2.6 Proof-claim lock

Numeric proof claims must be current.

This is an explicit correction to stale copy such as:

- `7.9 MB static binary`

when the current build no longer matches it.

For this page, any claim about binary size, shipped surface, or phase status must come from current local reality, not inherited marketing copy. If a binary-size number is shown, it must be sourced from a fresh local build artifact and updated when that artifact changes.

Current local reference point as of 2026-04-20:

- `bin/gormes` built via `make build` is approximately `8.2M`
- `bin/gormes-telegram` in the current local tree is approximately `15M`

The redesign may use these figures if they are still verified at implementation time, but it must not freeze them as timeless claims. Hardcoded legacy size lines are explicitly disallowed.

### 2.7 Mobile lock

The redesign must work on mobile as a first-class surface, not as a desktop page that merely collapses.

That means:

- the hero must remain readable and forceful on narrow screens;
- the primary run-now action must stay visible without layout breakage;
- command blocks must remain legible without horizontal chaos where avoidable;
- proof rails and status panels must stack cleanly;
- the nav must stay usable without overlapping, clipping, or creating fake-app chrome.

The operator-console aesthetic must survive down to mobile widths instead of degrading into a pile of broken panels.

---

## 3. Positioning

### 3.1 Core message

The page should communicate, in order:

1. Gormes is the Go operator shell for Hermes users.
2. It is available now, not just roadmap copy.
3. It adds operational hardening, not branding paint.
4. You can run it immediately and judge it yourself.

### 3.2 Reframing

The existing headline, tone, and sectioning read as a soft product intro. The redesign should instead frame Gormes as:

- the hardened execution surface around Hermes;
- the tool-capable Go console layer;
- the honest first slice of a larger port.

### 3.3 Copy stance

The copy may be rewritten aggressively.

It should favor:

- short punches over polite paragraphs;
- explicit proof over generic adjectives;
- verbs like `run`, `verify`, `ship`, `switch`, `cut`, `harden`, `replace`.

It should avoid:

- “platform transformation” language;
- investor-demo language;
- generic “modern, fast, scalable” filler.

---

## 4. Visual System

### 4.1 Design language

The page should feel like:

- oxidized steel;
- rack gear;
- terminal glass;
- warning labels;
- command channels;
- deployment status panels.

It should not feel like:

- a Notion clone;
- a B2B SaaS template;
- a crypto dashboard;
- a soft “premium dark mode” product page.

### 4.2 Color system

Base palette:

- graphite and near-black for the frame;
- cold gray-metal neutrals for panel depth;
- warning amber as the dominant signal accent;
- cold cyan for system/route/tool accents;
- small hits of signal green for shipped/live states.

Color should communicate machine state, not decoration.

### 4.3 Typography

Typography should do real hierarchy work:

- hard display face or condensed industrial-feeling headline treatment for major claims;
- strong monospace for labels, stats, commands, and microcopy;
- body copy kept tight and readable, not airy.

The current serif-led hero should be removed. It softens the page too much.

### 4.4 Motion

Motion should be sparse and mechanical:

- short boot-up reveals;
- light panel rise or scan-line entry;
- restrained status flickers or separator sweeps.

No floating-card softness, no decorative parallax, no fake data activity.

### 4.5 Texture and chrome

The page should gain structural texture:

- grid overlays;
- seam lines;
- panel headers;
- micro-labels;
- status chips;
- alignment marks;
- tactical dividers.

These elements should make the page feel engineered, not ornamental.

### 4.6 Mobile behavior

On mobile, the visual system should simplify rather than collapse.

Expected behavior:

- the command-deck hero becomes a strong single-column stack;
- proof items compress into compact status rows or tiles;
- chrome density is reduced where needed to protect readability;
- commands and labels stay crisp and scannable;
- CTA hierarchy remains obvious.

The page should feel like a mobile field console, not a shrunk desktop mockup.

---

## 5. Content Strategy

### 5.1 What changes from the current page

The redesign should not just repaint the existing stacked sections.

It should change the information emphasis:

- less “what is this project” explanation;
- more “why run it now” pressure;
- more visible proof up front;
- tighter route from claim to command.

### 5.2 Main narrative

The page should tell this story:

1. Hermes users do not need another future promise.
2. Gormes already ships real Go-native moat layers.
3. The right next step is to run it locally and verify the surface yourself.

### 5.3 Proof style

Proof should be presented as operational facts, not marketing bullets.

Examples of proof framing:

- binary size;
- zero-CGO;
- Go-native tool loop;
- split Telegram binary;
- current shipped phase boundaries.

These should appear as status, inventory, or deployment-state information rather than generic feature cards.

---

## 6. Information Architecture

The redesigned page remains a single route at `/`, but its sections should be reorganized into a stronger conversion flow.

Required high-level blocks:

1. Header / nav
2. Hero command deck
3. Proof strip / status rail
4. Run-now activation block
5. Operational advantage section
6. Roadmap / shipped-state section
7. Contribute / source block
8. Footer

The order matters. The page must not bury the run-now action beneath long explanatory copy.

---

## 7. Hero Command Deck

### 7.1 Job

The hero must do three things immediately:

- tell Hermes users what Gormes is;
- make it feel real and already shipping;
- point to a concrete local action.

### 7.2 Structure

The hero should become a command deck rather than a centered marketing intro.

Expected ingredients:

- a harder headline than the current poetic line alone;
- one short supporting argument;
- visible status labels;
- one dominant CTA linked to local trial;
- one adjacent terminal or system panel;
- one concise honesty note about current scope.

### 7.3 Headline direction

The page may retain “The Agent That GOes With You” as a secondary line if it still works, but it should not carry the whole hero by itself.

The real headline should read more like an operator verdict, for example:

- Gormes is the Go shell Hermes users can run now.
- Run Hermes through a hardened Go control surface.
- Stop waiting for the rewrite. Start the shell that already ships.

Exact wording can change during implementation, but the hero must be more forceful than the current version.

---

## 8. Proof Strip / Status Rail

### 8.1 Purpose

Immediately below or beside the hero, the page should show a compact strip of proof that answers:

- what exists now;
- what is shipped;
- what is still pending.

### 8.2 Content

The proof rail should include facts like:

- zero-CGO;
- current measured binary size, if shown;
- Go-native tools;
- Telegram split binary;
- current phase status.

### 8.3 Presentation

These should appear as:

- status chips;
- compact telemetry tiles;
- inventory rows;
- deployment state labels.

They should not appear as soft marketing cards.

If a binary-size number is shown, it must be validated against a current local build during implementation. Using stale historical numbers is a design failure, not just a copy nit.

---

## 9. Run-Now Activation Block

### 9.1 Purpose

This is the primary conversion section.

It should sit high on the page and make the next step obvious to a Hermes user who wants to test the surface immediately.

### 9.2 Structure

The activation block should include:

- the shortest realistic command path;
- brief annotations explaining each step;
- a direct explanation of what the user gets after running it;
- a strong CTA tone, not a tutorial tone.

### 9.3 Interaction rule

The commands must look like a real operator sequence, not decorative code.

The current quick-start stack can be reorganized, merged, or reframed as a single run-path if that improves urgency and clarity.

On mobile, this block must remain the strongest conversion surface on the page. The commands may reflow, annotate differently, or condense, but they must still feel executable and immediate.

---

## 10. Operational Advantage Section

### 10.1 Purpose

This section answers:

> Why use Gormes instead of just staying on Hermes alone?

### 10.2 Content

The answer should focus on operational hardening:

- shell responsiveness;
- typed Go host;
- tool loop surface;
- platform isolation;
- reconnection and route resilience;
- honest phase boundaries.

### 10.3 Presentation

This should read like a systems advantage panel, not a consumer feature grid.

The current feature cards can be replaced with denser modules or labeled system blocks if that better supports the operator-console look.

---

## 11. Roadmap as Shipping State

### 11.1 Purpose

The roadmap should stop reading like generic aspirational phases and start reading like a shipping ledger.

### 11.2 Presentation

The roadmap should distinguish:

- shipped now;
- in progress;
- planned next.

It should feel like release-state reporting, not brand storytelling.

### 11.3 Honesty rule

This section is where the page earns trust.

It must be explicit about what is already real versus what is still on deck, especially around memory and full Go cutover.

---

## 12. Contribute / Source Block

### 12.1 Purpose

This section is secondary, but it matters once a Hermes user decides the page is real.

### 12.2 Content

It should point to:

- source;
- architecture docs;
- proof documents;
- contribution path.

### 12.3 Tone

This should feel like:

- “inspect the machine,”

not:

- “join our community journey.”

---

## 13. Implementation Boundaries

The redesign should preserve the current architectural constraints:

- Go templates;
- embedded static assets;
- static export compatibility for Cloudflare Pages;
- no dependency on client-side JavaScript.

The operator-console feel must come from HTML structure, copy, and CSS, not from introducing a frontend framework or dynamic runtime.

The current exporter and server must continue to share the same rendering truth.

---

## 14. Testing and Verification

The redesign should keep or extend page-level verification so that:

- the rendered homepage still contains the critical product truths;
- the stylesheet link still exists;
- the exported static site still includes `index.html` and `static/site.css`;
- the page remains script-free unless a later spec changes that constraint.
- mobile layouts at narrow widths preserve CTA visibility, readable commands, and non-broken panel stacking;
- truth-bearing numeric claims are updated to current verified values or removed if they cannot be kept current.

If copy is rewritten heavily, tests should assert the new truth-bearing phrases rather than cling to obsolete phrasing.

---

## 15. Acceptance Criteria

The redesign is successful when:

- the page no longer reads like a generic dark SaaS landing page;
- the hero feels like a Go operator console, not a soft brand intro;
- the primary action to run Gormes locally is obvious and dominant;
- existing Hermes users can understand what is shipped now in seconds;
- the roadmap reads as honest shipping state;
- the implementation stays static-export friendly for Pages;
- the page remains usable and convincing on mobile widths;
- stale proof claims such as the old `7.9 MB` line are replaced with current verified claims or removed;
- the visual system is aggressive without becoming fake or noisy.

---

## 16. Non-Goals

This redesign does not include:

- multiple routes;
- a docs portal redesign;
- Pages Functions;
- fake live dashboards;
- animations that depend on JavaScript;
- buyer-oriented enterprise messaging as the primary voice.

The target is a harder, truer, more convincing single-page operator surface.
