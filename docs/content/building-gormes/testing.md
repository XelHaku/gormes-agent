---
title: "Testing"
weight: 50
---

# Testing

Three layers.

## Go tests

`go test ./...` from the repository root. Covers kernel, memory, tools, telegram adapter, session resume. Integration tests are tag-gated:

```bash
go test -tags=live ./...         # requires local Ollama
```

## Landing + docs smoke (Playwright)

`npm run test:e2e` from `www.gormes.ai/` and `docs/www-tests/`. Parametrized over mobile viewports (320 / 360 / 390 / 430 / 768 / 1024 px). Asserts:

- No horizontal overflow
- Copy buttons tappable (≥28×28 px bounding box)
- Section copy matches the locked strings in `content.go`
- Drawer opens/closes on mobile (docs only)

## Hugo build

`go test ./docs/... -run TestHugoBuild`. Shells out to `hugo --minify`, verifies every `_index.md` produces a rendered page, checks for broken internal links.

## Architecture fixtures

Subsystem ports need tests that freeze contracts, not just happy-path code.
Borrow these fixture classes from the Hermes and GBrain studies:

- **Command registry parity:** one registry drives parsing, help text, platform
  command exposure, aliases, and active-turn policy.
- **Provider transcripts:** replay request/stream fixtures for text,
  reasoning, tool-call deltas, finish reasons, usage, and retryable errors.
- **Tool schema parity:** compare upstream tool names, toolsets, JSON schemas,
  result envelopes, trust classes, and availability/degraded status.
- **Memory scope negatives:** prove same-chat default recall, opt-in user-scope
  widening, source allowlists, deny paths, and no cross-chat leakage.
- **Compression replay:** prove head/tail preservation, tool call/result pair
  integrity, summary lineage, and JSON-valid shrunken tool arguments.
- **Durable job replay:** prove cron/subagent jobs can be claimed, renewed,
  completed, failed, cancelled, retried, and inspected after restart.
- **Skill resolver conformance:** prove triggers, exclusions, disabled state,
  review state, and confusing phrases route to the expected skill or no skill.

## Discipline

Every PR must keep all three layers green. The `deploy-gormes-landing.yml` and `deploy-gormes-docs.yml` workflows run the Go + Playwright subsets on every push to `main`.
