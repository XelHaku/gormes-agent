---
title: "Upstream Hermes · Reference"
weight: 300
---

# Upstream Hermes · Reference

> These pages document the **Python upstream** `NousResearch/hermes-agent`. Gormes is porting these capabilities gradually — track progress in [§5 Final Purge](../building-gormes/architecture_plan/phase-5-final-purge/) of the roadmap. Features described here may or may not be shipping in Gormes today.

The content below is preserved verbatim from the upstream docs so operators evaluating Gormes can see the full Hermes stack in context. Anything that lands in native Go graduates out of this section into [Using Gormes](../using-gormes/).

## Porting Lens

Because Gormes is porting Hermes to Go, this section is also the upstream capability ledger:

- [Features Overview](./user-guide/features/overview/) now enumerates the full upstream feature surface and the primary method Hermes uses to implement each feature.
- [Messaging Gateway](./user-guide/messaging/) now enumerates each adapter and the transport or SDK pattern it uses upstream.
- [Developer Guide](./developer-guide/) now enumerates the implementation subsystems and the primary runtime method Hermes uses for each one.
- [Reference](./reference/) now enumerates the operator and configuration surfaces that a Go port still has to expose.

**Honcho is an explicit porting target in this section.** It is not just a single feature page: upstream Hermes treats Honcho as a memory-provider plugin, a dedicated `hermes honcho` command family, a provider-specific env/config surface, and a provider-owned tool surface (`honcho_profile`, `honcho_search`, `honcho_context`, `honcho_reasoning`, `honcho_conclude`).

In both pages, **method used** means the dominant upstream implementation mechanism or integration pattern. It is there to help Go port planning, not to force a line-by-line Python clone.

## Sections

- **Guides** — task-oriented how-tos
- **Developer Guide** — architectural deep dives
- **Integrations** — platform-specific setup (Bedrock, voice, Telegram, …)
- **Reference** — API/CLI material
- **User Guide** — operator workflows
- **Getting Started** — first-run setup (use [Using Gormes → Quickstart](../using-gormes/quickstart/) for the Go-native path)
