---
title: "Upstream Hermes · Reference"
weight: 300
---

# Upstream Hermes · Reference

> These pages document the **Python upstream** `NousResearch/hermes-agent`. Gormes is porting these capabilities gradually — track progress in [§5 Final Purge](../building-gormes/architecture_plan/phase-5-final-purge/) of the roadmap. Features described here may or may not be shipping in Gormes today.

The content below is preserved verbatim from the upstream docs so operators evaluating Gormes can see the full Hermes stack in context. Anything that lands in native Go graduates out of this section into [Using Gormes](../using-gormes/).

## Study Snapshot

- Upstream studied: `/home/xel/git/sages-openclaw/workspace-mineru/hermes-agent`
- Upstream commit: `b16f9d43`
- Gormes repo studied: `/home/xel/git/sages-openclaw/workspace-mineru/gormes-agent`
- Date: 2026-04-27

## 2026-04-27 Drift Check

Hermes `b16f9d43` is current in the synchronized sibling repo. Honcho remains
at `e659b6b` and GBrain remains at `c78c3d0`; no new Goncho/Honcho memory row
is needed from this sync, and Gormes keeps the internal `goncho` package name
while preserving public `honcho_*` compatibility surfaces.

- Telegram streaming drift: `b16f9d43` ports openclaw#72038 so long-running
  Telegram streamed previews finalize as a fresh message after
  `fresh_final_after_seconds` (default 60s) and best-effort delete the stale
  preview. Gormes now tracks this as two small Phase 2.B.5 rows: a shared
  gateway coalescer policy seam, then Telegram config + deleteMessage wiring.
- The upstream configuration mirror now includes the
  `streaming.fresh_final_after_seconds` field so the study docs match the
  synchronized sibling repo.

## 2026-04-26 Drift Check

Hermes `755a2804` is current in the synchronized sibling repo. The meaningful
post-roadmap drift is not the release-author-map commit itself; it is the
preceding gateway/tooling fixes now represented in the Gormes plan:

- Slack ingress drift: `6087e040`, `c0d25df3`, and `f414df3a` add readable
  rich_text quote/list extraction, link-unfurl preview text, bot-parent thread
  context, and team-scoped thread cache keys. Gormes now tracks these as small
  Phase 2.B.5 context/delivery rows over `internal/slack` and
  `internal/gateway` so the completed Slack chassis subphase stays closed.
- Home Assistant toolset drift: `4921b269` keeps `homeassistant` available for
  cron/CLI defaults when `HASS_TOKEN` is set while leaving other default-off
  toolsets disabled. Gormes now tracks this as a Phase 5.A toolset resolver row.
- Shutdown/memory cleanup drift: `18beb69b`, `bf05b8f4`, and `2d86e97a` reinforce
  the existing Gormes rule that background providers and gateway agents need
  explicit shutdown drains. The current Gormes memory/gateway closeout rows
  already cover the Go-native drain contract, so no new runtime row was added.

## Porting Lens

Because Gormes is porting Hermes to Go, this section is also the upstream capability ledger:

- [Source Study](./source-study/) adds a code-grounded reading of the local upstream tree studied for Gormes architecture work.
- [Good and Bad](./good-and-bad/) separates Hermes design moves worth keeping from coupling risks Gormes should avoid.
- [Gormes Takeaways](./gormes-takeaways/) translates the source study into Go-native architecture decisions.
- [Features Overview](./user-guide/features/overview/) now enumerates the full upstream feature surface and the primary method Hermes uses to implement each feature.
- [Messaging Gateway](./user-guide/messaging/) now enumerates each adapter and the transport or SDK pattern it uses upstream.
- [Developer Guide](./developer-guide/) now enumerates the implementation subsystems and the primary runtime method Hermes uses for each one.
- [Reference](./reference/) now enumerates the operator and configuration surfaces that a Go port still has to expose.

**Honcho is an explicit porting target in this section.** It is not just a single feature page: upstream Hermes treats Honcho as a memory-provider plugin, a dedicated `hermes honcho` command family, a provider-specific env/config surface, and a provider-owned tool surface (`honcho_profile`, `honcho_search`, `honcho_context`, `honcho_reasoning`, `honcho_conclude`).

In both pages, **method used** means the dominant upstream implementation mechanism or integration pattern. It is there to help Go port planning, not to force a line-by-line Python clone.

## Sections

- **Source Study** - local code study for Gormes architecture decisions
- **Guides** — task-oriented how-tos
- **Developer Guide** — architectural deep dives
- **Integrations** — platform-specific setup (Bedrock, voice, Telegram, …)
- **Reference** — API/CLI material
- **User Guide** — operator workflows
- **Getting Started** — first-run setup (use [Using Gormes → Quickstart](../using-gormes/quickstart/) for the Go-native path)
