# Gormes Gateway Donor Map Design Spec

**Date:** 2026-04-20
**Author:** Xel (via Codex brainstorm)
**Status:** Approved design direction; ready for writing
**Scope:** A documentation-only research deliverable that mines `picoclaw` for reusable Go channel-adapter code and patterns while keeping Hermes/Gormes architecture authoritative.

---

## 1. Purpose

Gormes needs a faster path through the Phase 2 gateway backlog without diluting the larger objective: **Hermes is still the brain and system architecture for the Operative System AI.**

`picoclaw` is useful here because it is already a completed all-Go assistant with a broad set of working messaging adapters. The goal is not to copy PicoClaw's product architecture. The goal is to extract practical leverage from its channel-edge implementations:

- transport setup;
- SDK choices;
- inbound event handling;
- send/edit/reply/media flows;
- rate-limit and reconnect behavior;
- channel-specific quirks that are already solved in Go.

This spec defines a documentation set that turns that donor repo into a porting surface for Gormes contributors.

---

## 2. Locked Boundary

These rules are final for this documentation effort.

### 2.1 Hermes/Gormes architecture stays authoritative

The donor analysis must not propose replacing Gormes kernel, session, or agent architecture with PicoClaw's runtime model.

The approved boundary is:

- **Gormes/Hermes owns** kernel structure, session model, agent behavior, and the Operative System AI product direction.
- **PicoClaw donates** transport-edge code, adapter mechanics, SDK decisions, and channel-specific Go solutions where reuse has positive ROI.

### 2.2 The research scope is channel-first

The first pass focuses on channel adapters because that is where PicoClaw has the highest immediate reuse value against Gormes Phase 2.

This effort is explicitly **not** a broad "copy PicoClaw in general" initiative.

### 2.3 The deliverable must be prescriptive

The output is not a descriptive survey. It must be hard documentation that makes reuse easier to execute inside Gormes.

Every channel page must end in an explicit recommendation:

- `copy candidate`
- `adapt pattern only`
- `not worth reusing`

---

## 3. Deliverable Shape

The deliverable is a new documentation section under:

`gormes/docs/content/building-gormes/gateway-donor-map/`

Planned file set:

- `_index.md`
- `shared-adapter-patterns.md`
- `telegram.md`
- `discord.md`
- `slack.md`
- `whatsapp.md`
- `matrix.md`
- `irc.md`
- `line.md`
- `onebot.md`
- `qq.md`
- `wecom.md`
- `weixin.md`
- `feishu.md`
- `dingtalk.md`
- `vk.md`
- `webhook.md`

If the hub page becomes too dense, it may also include:

- `priority-order.md`

### 3.1 Hub page responsibilities

`_index.md` must:

- explain why PicoClaw is being mined as a donor repo;
- restate that Hermes/Gormes architecture remains the source of truth;
- summarize which shared adapter patterns are worth reusing;
- list every channel dossier and link it directly;
- give a quick triage view of donor value across the channel surface.

### 3.2 Shared patterns page responsibilities

`shared-adapter-patterns.md` must capture cross-channel mechanics from PicoClaw that are reusable independent of any one SDK, such as:

- channel manager behavior;
- message splitting and outbound size limits;
- typing, reaction, and placeholder flows;
- send-vs-edit behavior;
- media plumbing expectations;
- retry, backoff, reconnect, and rate-limit patterns;
- webhook vs polling vs socket transport trade-offs.

This page exists to prevent repeating the same transport analysis inside every channel dossier.

---

## 4. Channel Dossier Template

Each per-channel page must follow the same structure so contributors can use them as implementation checklists.

Required sections:

- `Status`
- `Why This Adapter Is Reusable`
- `Picoclaw Donor Files`
- `What To Copy vs What To Rebuild`
- `Gormes Mapping`
- `Implementation Notes`
- `Risks / Mismatches`
- `Port Order Recommendation`
- `Code References`

Each dossier must be written from the point of view of a future Gormes porter, not from the point of view of a historian documenting PicoClaw.

---

## 5. Recommendation Rules

Each channel recommendation must be derived from a repeatable evaluation, not taste.

Required evaluation criteria:

1. Protocol fit with Hermes-style gateway boundaries
2. SDK/runtime maturity in Go
3. Amount of channel-specific code versus genuinely reusable transport code
4. Session and messaging model compatibility with Gormes
5. Hidden coupling to PicoClaw-only systems

Each channel page must make the decision explicit:

- **`copy candidate`** when the donor code is close to Gormes needs and the main work is integration
- **`adapt pattern only`** when the code teaches the right shape but carries too much PicoClaw coupling to lift directly
- **`not worth reusing`** when the channel exists but its implementation is too specific, too thin, or too mismatched to justify imitation

---

## 6. Analytical Layers

To avoid contaminating Gormes with PicoClaw-specific assumptions, every channel dossier must separate three layers.

### 6.1 Transport edge

What the adapter has to do at the protocol boundary:

- auth and startup;
- inbound event intake;
- outbound send/edit/reply;
- media upload/download;
- thread or reply semantics;
- webhook, long-poll, websocket, or socket mode behavior.

### 6.2 Shared adapter mechanics

Patterns that may generalize beyond the channel:

- splitting long responses;
- placeholder/edit loops;
- typing indicators;
- reactions/acks;
- retries and reconnects;
- rate limiting;
- message identity mapping.

### 6.3 PicoClaw-specific coupling

Anything that should not be imported blindly into Gormes, including:

- assumptions tied to PicoClaw's agent loop;
- PicoClaw-only bus or manager contracts;
- config or auth flows that conflict with Gormes direction;
- runtime integrations that belong to PicoClaw rather than the channel edge itself.

---

## 7. Research Scope

Included:

- implemented adapters under `picoclaw/pkg/channels/*`;
- PicoClaw channel docs under `picoclaw/docs/channels/*` where they clarify runtime shape or setup expectations;
- shared runtime code that directly supports channel adapter reuse;
- references back to existing Gormes gateway docs and the shipped Telegram adapter where comparison is useful.

Excluded:

- replacing Hermes/Gormes architecture with PicoClaw's architecture;
- a general architecture study of the full PicoClaw stack;
- non-channel subsystems except where they directly affect channel reuse;
- Python edits or upstream Hermes changes.

---

## 8. Initial Channel Set

The first donor-map pass covers the implemented channel surface visible in PicoClaw that intersects or informs Gormes gateway planning:

- Telegram
- Discord
- Slack
- WhatsApp
- Matrix
- IRC
- LINE
- OneBot
- QQ
- WeCom
- WeiXin
- Feishu
- DingTalk
- VK
- Webhook

Where a PicoClaw implementation exists but Gormes has no direct roadmap row yet, the dossier should still note whether the code suggests a future adapter path or is merely informative.

---

## 9. Existing Gormes Context To Anchor Against

The donor-map docs must align with, and link back to, the current Gormes documentation and shipped work:

- `gormes/docs/content/building-gormes/core-systems/gateway.md`
- `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`
- `gormes/docs/content/using-gormes/telegram-adapter.md`
- `gormes/docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md`

The donor-map section is a porting aid layered onto the existing roadmap, not a replacement for it.

---

## 10. Success Criteria

This design is successful when a contributor can open a single channel dossier and answer all of these questions quickly:

- Which PicoClaw files are the best donor references?
- What parts are safe to copy or imitate?
- What parts must be rebuilt for Gormes?
- What Hermes/Gormes constraints matter for this port?
- Is this adapter worth doing now, later, or not at all?

If the docs do not materially lower the cost of porting a Gormes channel adapter, the deliverable failed even if the research is technically accurate.
