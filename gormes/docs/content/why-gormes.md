---
title: "The Gormes Philosophy: Why Go-Native Matters"
date: 2026-04-19
draft: false
description: "A 7.9 MB agent with a 16 ms coalescer, a chaos-proof reconnect loop, and no runtime dependencies. The Operational Moat, explained."
tags: ["manifesto", "architecture", "go", "agents", "gormes"]
---

# The Gormes Philosophy

Most AI-agent projects ship as a 500 MB virtual environment and a README that starts with "just pip-install this… and that… and compile this C extension… and set this env var…"

Gormes is **7.9 MB**. Static. CGO-free. `scp` to a $5 VPS and it runs. Same binary on Ubuntu 22.04 LTS. Same binary in Termux on your phone. Same behavior, same race-detector-clean concurrency, same tests.

This is not an accident. It is the **Operational Moat** — a set of engineering decisions that compound into something a Python-first ecosystem cannot match on its own ground: **relentless availability at scale-of-one.**

We're [**@GormesAI**](https://x.com/GormesAI) on X. This document is why.

---

## Why Go-native matters

The prevailing move in 2024–2026 was to treat Go as "the scripting glue around a Python brain." Write your orchestrator in Go for concurrency, then shell out to Python for anything that requires an LLM. Many excellent systems live at that boundary.

Gormes takes the other direction. We treat the **agent** itself — the state machine that receives user input, calls a provider, streams tokens back, invokes tools, handles failures — as a first-class Go program. Python remains a peripheral library during the transition (we're in a Ship-of-Theseus rewrite of the upstream [Hermes Agent](https://github.com/NousResearch/hermes-agent)), but every phase ships a little more Go and a little less Python, until the pipeline is 100% Go.

Three engineering disciplines make this practical rather than romantic. Each one comes from a specific failure mode that Gormes has seen, tested, and locked down.

---

## 1. Taming the Thundering Herd: the 16 ms coalescer

A fast LLM emits tokens at 100–200 per second. A terminal UI can redraw at most 60 times per second. If you push every token straight into the UI's redraw cycle, one of two things happens:

1. **The UI blocks the producer.** Back-pressure propagates up the stack; the agent stalls mid-turn; the user watches a spinner that shouldn't exist.
2. **The producer drops tokens.** The final assistant message is truncated or out of order; the demo goes viral; the bug never gets fixed because no one can reproduce it.

Gormes solves this at the kernel. The `Kernel.Run` goroutine coalesces tokens into an **in-memory draft buffer** and emits a `RenderFrame` at most every **16 ms** — a natural 60 Hz ceiling that the UI consumes at its own pace. Semantic edges (`PhaseStreaming`, `PhaseIdle`, `PhaseReconnecting`) flush immediately so the user sees transitions the moment they happen.

The render channel itself is **capacity-1 with replace-latest semantics**: a slow consumer never blocks the producer; it just drops stale frames. The mailbox is a test-enforced invariant, proven by `TestKernel_NonBlockingUnderTUIStall`:

> 2 000 tokens, intentionally-stalled consumer for 2 seconds, kernel finishes the turn and the final `RenderFrame` is the latest state — never a stale mid-stream frame.

Same discipline re-emerges in our Telegram adapter: a **1-second coalescer** turns 100 token updates per turn into roughly one `editMessageText` call per second, staying comfortably under Telegram's rate limit while still feeling "live."

This is what a Thundering Herd killer looks like in 150 lines of Go. No external queue. No debouncer library. No cron job. Just the right state machine.

---

## 2. Chaos-proofing with Route-B reconnect

Networks fail. WebSockets close. The user's train enters a tunnel. The Python `api_server` restarts during deploy. Most agents treat the in-flight turn as lost and ask the user to retry.

Gormes treats every turn as **visual-continuity resilient**. When the SSE stream drops mid-response, the kernel:

1. Transitions to `PhaseReconnecting`.
2. **Preserves the partial draft** — the tokens already rendered stay on screen.
3. Retries with **exponential backoff plus jitter** (1 s → 2 s → 4 s → 8 s → 16 s, ±20 %).
4. On successful reconnect, clears the partial draft and starts fresh — a single clean assistant message, no Frankenstein concatenation.

The test for this is the kind of test we wish more agents shipped with. It stands up an `httptest.Server`, streams five tokens, **closes the client connection mid-stream**, swaps in a fresh server, and asserts the full five-assertion contract: transition to `PhaseReconnecting`, draft preservation, transition back to `PhaseStreaming`, single-message final history, no goroutine leaks.

That test is `TestKernel_HandlesMidStreamNetworkDrop` in `internal/kernel/reconnect_test.go`. It was written as a **red test** before the implementation existed. Flipping it from `SKIP` to `PASS` was how we knew Route-B was done — and in the process, it caught a header-phase context bug in the HTTP client that would have broken every real slow-streaming deployment. Invisible in the happy-path tests because `httptest` buffers the whole body before `Do` returns; exposed the moment we asked the stream to survive a chaos-monkey drop.

This is the value of test-first engineering on the resilience layer: the bug never reached production because the test demanded the behavior before the feature existed.

---

## 3. The Surgical Strike: 7.9 MB vs 500 MB

Binary size is the dimension no one talks about until they need it.

Here's what Gormes measures:

| Artifact | Size | Dependencies |
|---|---|---|
| `gormes` (TUI) | **7.9 MB** stripped static | zero runtime deps |
| `gormes-telegram` (Phase 2.B.1, in progress) | ≤ 12 MB | zero runtime deps |
| `gormes doctor --offline` | same binary | zero runtime deps |

Compare to a typical Python-based agent runtime:
- ~200 MB Python virtual environment.
- Transitively 50–200 third-party packages.
- Compile-time C dependencies for NumPy / Pydantic / asyncpg, depending on stack.
- A 5-10 second cold start.

We enforce the size discipline with a **build-isolation test** in `internal/`:

```go
// TestTUIBinaryHasNoTelegramDep: cmd/gormes must never transitively
// import telegram-bot-api. If it does, the binary size jumps and the
// per-binary-per-platform promise breaks.
```

`go list -deps ./cmd/gormes` is scanned on every CI run. If a reviewer accidentally pulls the Telegram SDK into the TUI's dependency graph, the test fails the build with the exact offender named.

This is deliberately boring. It's the kind of test that nobody notices until it saves you six weeks of "why did my binary just double in size?" debugging during a version bump. We write those tests on purpose.

**Two binaries, one kernel.** The TUI and the Telegram bot share the exact same `internal/kernel` + `internal/tools` + `internal/hermes` code paths. Platform adapters are siblings: `internal/tui/` and `internal/telegram/`. The kernel itself has never imported an HTTP client, a terminal library, or a Telegram SDK. It processes `PlatformEvent`s and emits `RenderFrame`s. Every adapter is a thin translator.

When Phase 2.B.2 adds Discord, the pattern doesn't change. `cmd/gormes-discord` imports `internal/kernel` + a new `internal/discord/` package. The existing 7.9 MB TUI binary stays 7.9 MB. The Telegram bot stays ≤ 12 MB. The Discord bot ships ≤ 12 MB. Nothing cross-contaminates.

---

## What's already shipped

We're not speculating here. As of today:

- **Phase 1 (Ignition)** — 7.9 MB static binary, Bubble Tea TUI, OpenAI-compatible HTTP+SSE client, zero local state. 20 tasks, all green under `-race`.
- **Phase 1.5 (Hardening)** — Route-B reconnect resilience, SSG-portable docs, `--offline` dev mode, 10/10 discipline test scorecard. The red chaos test exposed a production HTTP bug before it shipped.
- **Phase 2.A (Tool Registry)** — Go-native tool interface (`tools.Tool`), in-process registry, three built-in tools (`echo`, `now`, `rand_int`), full tool-call round-trip through the kernel's `runTurn`. Nineteen kernel tests green; race-clean with tool execution in play.
- **Phase 2.B.1 (Telegram Scout)** — in flight right now. `telegramClient` interface + mock-driven tests landed today; 10-task plan converts the spec into a shipping binary by end of week.

Read the docs, read the tests, read the commits. The engineering receipts are all there.

---

## Why this matters for you

Gormes isn't trying to be the agent framework with the most features. It's trying to be the one that **doesn't break** when you put it on an always-on $5 VPS and walk away for six months.

- **Deploy it:** `scp gormes-telegram user@server:/usr/local/bin/ && systemctl start`. No container orchestration needed.
- **Inspect it:** `gormes doctor --offline` lists every registered tool with its JSON schema validity. No network required.
- **Extend it:** a domain-specific tool (scientific, business, research) is a 40-line Go file implementing `tools.Tool`. Register it in your fork's `cmd/gormes/main.go`. Your fork inherits every Phase-1/1.5/2.A resilience test for free.
- **Fork it:** the repository is MIT. The spec → plan → implementation discipline is documented at every phase. If you want to audit why something is the way it is, the commit log tells you.

The factory is producing trucks. Good trucks. One truck per platform, each under 12 MB, each hardened to keep going when the network has a bad day.

---

## Follow the build

- **X:** [@GormesAI](https://x.com/GormesAI)
- **Source:** [github.com/XelHaku/golang-hermes-agent](https://github.com/XelHaku/golang-hermes-agent)
- **Upstream reference:** [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent)
- **Docs:** this site, built from Markdown that doubles as our test fixtures — [goldmark](https://github.com/yuin/goldmark) validates every page on every CI run, matching what Hugo renders in production.

The philosophy is the file on disk. The Operational Moat is measurable. The binary size is the scoreboard.

Go-native matters because someone, eventually, has to answer the pager at 3 a.m. Let's make sure it's a small, well-tested, statically-linked binary with a clean exit code.

🦞🏛️🚀
