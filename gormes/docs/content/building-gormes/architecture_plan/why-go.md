---
title: "Why Go + Hybrid Manifesto"
weight: 120
---

## 2. Why Go — what the end user actually gets

Go isn't a better Python. It's a different **shape of thing to live with.** The benefits list is long, but most of it isn't about performance — performance is invisible on a modern PC. The real story is what the user experiences over months of use, under load, across machines, when things go wrong. This section is ordered by how the user actually notices.

### 2.1 The obvious wins (install-time)

These matter most on low-end hardware; on a fast PC they're table stakes.

- **Single static binary.** ~17 MB (CGO-free, `make build` post-Phase 2.D). `scp` and run. No `uv`, `pip`, venv, system Python, brew Python, pyenv, or platform-wheel roulette.
- **Cold start in milliseconds.** Go is ready instantly; Python agents with tokenizer/transformers-adjacent imports take seconds every session. Felt every TUI open, every cron fire, every shell one-shot.
- **Idle footprint.** Measured ~10 MB RSS vs. ≈ 80+ MB for Python Hermes. Matters on VPS, Pi, always-on homelab.
- **Cross-compile.** One build → `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`. User doesn't audit their platform.

### 2.2 The real wins (over time)

On a high-end PC you don't feel install latency. You feel these.

1. **It still works in a year.** No pip drift, no CUDA/torch realignment, no Python 3.X bump breaking imports, no wheel that stopped building on your OS. The binary you copied in April runs identically in November. Python long-running agents rot — that's why users babysit them. Go genuinely closes this failure mode.

2. **No mystery stalls.** Python agents under concurrent load (gateway + cron + TUI + tools) hit GIL edges and asyncio-vs-threading seams that manifest as "it froze for a sec." On a fast PC, startup is invisible — stutters aren't. Go's scheduler + the bounded-mailbox kernel (see [Core Systems → Gateway](../core-systems/gateway/)) removes that class of bug.

3. **No segfault class.** Python with native deps (torch, numpy, llama.cpp bindings) can take the whole process down with a C-level crash. Go crashes are recoverable panics with stack traces. Users experience this as "it never disappears on me."

4. **Kill and restart cleanly.** `pkill gormes` + relaunch → done. Python agents with asyncio tasks, multiprocessing workers, file locks, and a loaded model leak child processes and orphan sockets; sometimes the fix is a reboot. Every user who has ever had to `lsof` a stuck Python daemon at 2am feels this.

5. **Resumes from a reboot.** OOM kill, laptop close, power blip. bbolt + SQLite means state is flushed; Go restart is sub-second; the next cron fires on time. Python agents with in-memory SQLAlchemy sessions and loaded models pay a cold start every time, sometimes fail to restart at all. Users experience this as "it remembered."

6. **Graceful degradation.** Embedding model offline → semantic fusion skipped, lexical recall still answers. Tool unavailable → agent says so, keeps going. Python agents with hard top-of-module imports die entirely when one dep is misbehaving. Users feel this as "it keeps working when the network is weird" vs "crashed and told me to reinstall."

### 2.3 The portability wins

Go goes places Python won't.

7. **Phone, air-gapped, corp laptop.** A single binary runs under Termux on Android, on a machine with no internet, inside a locked-down corporate laptop without admin rights, in a rootless container that's 20 MB not 800 MB. A user who wants the agent on a second device just copies the file.

8. **Local model becomes a sibling file.** When the wrapper is 17 MB, bundling a 2 GB local model next to it is plausible — `gormes` + `gemma.bin` → works on a plane. With Python + torch, the wrapper is already 2 GB, so "add a local model" is a different product.

9. **Upgrade is `cp`.** Not "dependency resolution error, please file issue." One failed pip upgrade and a user's trust is gone for the year.

### 2.4 The integration wins (power user)

10. **Scriptability.** `gormes ask "..."` from a shell alias, Raycast, editor macro, keyboard shortcut, cron one-liner, git hook. Python pays import cost every invocation, so users give up wiring the agent *into* their tools. Go makes it a thing you call from other things. The agent becomes part of the keyboard, not an app you open.

11. **State is one directory they can back up.** Config + SQLite + bbolt under `$XDG_STATE_HOME/gormes/`. A user rsyncs that directory to a new laptop and their agent has their memory, their tools, their skills. Python agents smear state across `site-packages`, import-time globals, `~/.cache/huggingface`, `/tmp`, and CWD — "move to a new machine" is a research project.

12. **When it breaks, it's fixable.** Structured logs with request IDs traceable across goroutines. `pprof` on a live binary. Clean stack traces. A user filing a bug pastes something a maintainer can act on. Python async logs interleave tasks, traces go through coroutine magic, and the support thread dies after two round-trips because nobody can tell what happened.

### 2.5 The trust wins

13. **Supply-chain audit surface.** This agent reads your messages, your calendar, your files. Python's pip graph is hundreds of transitive deps, regular typosquatting, malicious post-install wheels. `go mod graph` is a finite checksummed list. Self-hosters who chose this *over* ChatGPT precisely because they don't want a black box feel this directly.

14. **Real skill sandboxing.** Phase 2.G skills in Python means `exec()` and hope. In Go there is a straight path to WASM via [wazero](https://github.com/tetratelabs/wazero) — user-written or third-party skills run in a box that can't exfil data or escape. This is the only way "community skills" ever becomes safe to install. Python can't get there without bolting on another runtime.

15. **Subagent resource bounds are real now.** Phase 2.E now ships subagent-scoped `context.Context` deadlines, deterministic cancellation, max-depth guards, bounded batch concurrency, Go-native `delegate_task`, runner-enforced tool policy, typed child tool-call audit, append-only run logging, and real child LLM execution. Python subagents under a shared GIL + shared heap can *claim* isolation, but a runaway one still starves the others and OOMs the parent. For a *learning loop* that runs user-written skills, this is the difference between **safe** and **pretending to be safe**.

### 2.6 The developer wins (included for completeness)

These are real, but they're why *you* ship features faster — not why the user stays.

- **Static types and compile-time contracts.** Tool schemas, Provider envelopes, and MCP payloads are typed structs. Schema drift is a compile error, not a silent agent-loop failure.
- **True concurrency.** Goroutines over channels replace `asyncio`. The gateway scales to 10+ platform connections without event-loop starvation.
- **Race detector, `go vet`, `staticcheck`.** A class of Heisenbugs caught before ship that Python leaves for users to find.
- **Reproducible builds.** Two users on the same version get the same thing. Users feel this indirectly when they compare notes in a support channel.

### 2.7 The pattern

Every item above is about the agent being **a thing in the world the user operates**, rather than a Python project the user maintains. They stop being the mechanic and become the driver. That is the real product difference, and it is invisible until you have lived with both.

### 2.8 Explicit trade-off

The Python AI-library moat — `litellm`, `instructor`, heavyweight ML frameworks, research-tier skills — stays in Python until Phase 4–5. Gormes pays this in adapter code, not in user-facing latency.

---

## 3. Hybrid Manifesto — the Motherboard Strategy

The hybrid is **temporary**. The long-term state is 100% Go.

During Phases 1–4, Go is the chassis (orchestrator, state, persistence, platform I/O, agent cognition) and Python is the peripheral library (research tools, legacy skills, ML heavy lifting). Each phase shrinks Python's footprint. Phase 5 deletes the last Python dependency.

Phase 3 (The Black Box) is substantially delivered as of 2026-04-23: the SQLite + FTS5 lattice (3.A), ontological graph with async LLM extraction (3.B), lexical/FTS5 recall with `<memory-context>` fence injection (3.C), semantic fusion via Ollama embeddings with cosine similarity recall (3.D), and the operator-facing memory mirror (3.D.5) are all implemented. Phase 3.E is mixed closeout now: session index, tool audit, transcript export, extraction status, and insights logging are shipped; decay freshness, cross-chat deny-path/tool evidence, and parent-session lineage remain.

Phase 1 should be read correctly: it is a tactical Strangler Fig bridge, not a philosophical compromise. It exists to deliver immediate value to existing Hermes users while preserving a clean migration path toward a pure Go runtime that owns the entire lifecycle end to end.

---

## 3.5 Build Priority Framework — The Four Systems That Matter

Based on analysis of Hermes architecture and Gormes current state, here is the build priority order. **Skip even one of these and you don't have "Hermes in Go"—you have a chatbot with tools.**

### P0: Skills System — The Learning Loop (THE SOUL)

**Why first:** This is the only truly unique thing in Hermes. Without it, Gormes is undifferentiated from any other agent framework.

**What it does:**
- Detects "this task was complex" (heuristic or LLM-based)
- Extracts a reusable pattern from conversations and actions
- Saves it as a skill (structured, versioned, improvable)
- Improves that skill over time through feedback

**Minimum viable implementation:**

```go
type SkillExtractor interface {
    IsComplex(task Task) bool                    // Detect complex work
    ExtractPattern(conv Conversation) Skill      // LLM extraction
    Save(skill Skill) error                      // SQLite storage
    Improve(skillID string, feedback Feedback)   // Iterative refinement
}
```

**Without this:** You lose compounding intelligence, differentiation, and long-term value. Gormes becomes a stateless chat interface.

**Status:** ✅ **Phase 2.G runtime and the reviewed candidate flow are now in-tree.** Gormes can parse approved `SKILL.md` files from disk, snapshot the active store, select a bounded deterministic subset, inject that prompt block into live turns, append immutable usage events, draft inactive candidate skills from successful delegated runs, and explicitly promote reviewed candidates into the active store. Phase 5.F is still the broader upstream skills-plumbing port.

---

### P1: Subagent System — Execution Isolation Model

**Why second:** Enables parallel workstreams with real isolation—a Gormes **advantage over Hermes**, which has loosely-defined subagent lifecycles.

**What it does:**
- Spawns isolated subagents for parallel tasks
- Provides resource boundaries (memory limits, timeouts)
- Maintains context isolation (no cross-contamination)
- Implements scoped cancellation (parent cancels children, but not vice versa)
- Contains failures (subagent crashes don't cascade)

**Minimum viable implementation:**

```go
type Subagent struct {
    ID       string
    Context  context.Context    // Isolated conversation context
    Cancel   context.CancelFunc // Scoped cancellation
    MemoryMB int                // Soft memory limit
    Tools    []Tool             // Restricted tool subset
    ParentID string               // For lineage tracking
}
```

**Why this beats Hermes:** Python's "isolated subagents" are loosely-defined processes. Gormes can provide **process-adjacent isolation within a single binary**—strict logical boundaries with resource accounting and deterministic cleanup.

**Status:** 🔨 Runtime core implemented in `internal/subagent` and exposed through Go-native `delegate_task`. Runner-enforced tool policy, typed child tool-call audit, append-only run logging, and real child LLM execution are landed.

---

### P2: Multi-Platform Gateway

**Why third:** Telegram proves the pattern. Scale to Discord/Slack/WhatsApp/Signal/Email. Platform breadth matters for adoption, but it doesn't differentiate architecturally.

**Status:** 🔨 Shared gateway chassis + `gormes gateway` landed; Telegram (2.B.1) and Discord (2.B.2) are shipped. Slack has a private Socket Mode bot but still needs shared runtime registration, WhatsApp has only ingress normalization, and the long-tail regional adapters are advancing as contract-first seams before real transports.

---

### P3: Native Agent Loop (Phase 4)

**Why last:** The Python bridge works. Replace it only after Skills and Subagents prove the architecture is correct. Phase 4 is **optimization**, not **differentiation**.

**Status:** ⏳ Phase 4.A–4.H (Provider adapters, context engine, prompt builder, smart routing, insights, etc.)

---

### Summary: What to Build and When

| Priority | System | Differentiation | Risk if Skipped |
|----------|--------|----------------|-----------------|
| **P0** | Skills System | Compounding intelligence | Undifferentiated chatbot |
| **P1** | Subagent System | Execution isolation | Unreliable parallel work |
| **P2** | Multi-Platform Gateway | Reach | Limited user access |
| **P3** | Native Agent Loop | Performance optimization | Bridge dependency continues |

**Current dependency chain:** 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → Phase 4 native agent loop.
