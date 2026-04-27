---
title: "Upstream Hermes · Reference"
weight: 300
---

# Upstream Hermes · Reference

> These pages document the **Python upstream** `NousResearch/hermes-agent`. Gormes is porting these capabilities gradually — track progress in [§5 Final Purge](../building-gormes/architecture_plan/phase-5-final-purge/) of the roadmap. Features described here may or may not be shipping in Gormes today.

The content below is preserved verbatim from the upstream docs so operators evaluating Gormes can see the full Hermes stack in context. Anything that lands in native Go graduates out of this section into [Using Gormes](../using-gormes/).

## Study Snapshot

- Upstream studied: `/home/xel/git/sages-openclaw/workspace-mineru/hermes-agent`
- Upstream commit: `b288934d`
- Gormes repo studied: `/home/xel/git/sages-openclaw/workspace-mineru/gormes-agent`
- Date: 2026-04-27

## 2026-04-27 Security And Tooling Drift Check

Hermes `b288934d` is current in the synchronized sibling repo. Honcho remains
at `e659b6b` and GBrain moved to `891c28b`; no new Goncho/Honcho memory row is
needed from this Hermes sync, and Gormes keeps the internal `goncho` package
name while preserving public `honcho_*` compatibility surfaces.

- WhatsApp identity drift: `91512b82` and `6993e566` reject traversal-like and
  non-ASCII identifiers before alias expansion. Gormes already avoids Hermes'
  filesystem `lid-mapping-*` files, but now tracks a Phase 2.B.4
  `WhatsApp ASCII identifier guard` row so the Go alias graph cannot promote
  unsafe raw peers into session keys or outbound pairing state.
- Hook consent drift: `e19854d8` parses `hooks_auto_accept` strictly so quoted
  false-like strings and non-bool scalars do not authorize executable hooks.
  Gormes now tracks this as a parser-only Phase 5.J approval/security row
  before any native auto-accept config or CLI wiring is exposed.
- Provider timeout drift: `16e243e0` and `366351b9` make Hermes provider timeout
  lookup fail closed when config loading raises. Gormes has no equivalent
  provider timeout config surface yet, so this is recorded as a future provider
  config hardening lesson rather than a runtime row in this pass.
- Discord tool drift: `b288934d` coerces model-provided Discord `limit`
  arguments to integers before clamping. Gormes only has descriptor-level
  Discord tools today, so the roadmap adds a pure Phase 5.A
  `Discord tool limit coercion helper` before REST handlers land.

## 2026-04-27 MCP And Gateway Drift Check

Hermes `cb51baec` was current in the synchronized sibling repo. Honcho remained
at `e659b6b` and GBrain remained at `c78c3d0`; no new Goncho/Honcho memory row
is needed from this sync, and Gormes keeps the internal `goncho` package name
while preserving public `honcho_*` compatibility surfaces.

- MCP/cron drift: `930494d6` reaps orphaned MCP stdio subprocesses only after
  a cron tick has joined all siblings. Gormes now tracks this as a small
  Phase 5.G row over native MCP stdio and cron seams.
- Browser security drift: `7317d69f` treats quoted false-like config values as
  false in SSRF guards. Gormes now tracks this as a pure Phase 5.C browser
  safety helper before native browser provider wiring.
- Busy steer drift: `635253b9` adds `steer` as a busy input mode. Gormes
  already had a steering row; it is now refined to exact source refs and a
  smaller registry/fallback write scope.
- Telegram streaming drift: `b16f9d43` ports openclaw#72038 so long-running
  Telegram streamed previews finalize as a fresh message after
  `fresh_final_after_seconds` (default 60s) and best-effort delete the stale
  preview. Gormes now tracks this as three small Phase 2.B.5 rows: a shared
  gateway eligibility helper, a shared send/delete fallback, then Telegram
  config + deleteMessage wiring.
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
