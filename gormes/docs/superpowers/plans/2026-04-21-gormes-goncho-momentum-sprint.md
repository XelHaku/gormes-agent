# Gormes GONCHO Momentum Sprint (72h)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stabilize the live adapter path, then make GONCHO usable across all active Gormes run modes, and finally glue it to the bounded subagent and static-skills slices without breaking the Phase 2 gateway path.

**Architecture:** This is a coordination plan for a 72-hour sprint, not a second architecture spec. It sequences four real subsystems that already exist in the repo: `internal/telegram`, `internal/goncho`, `internal/subagent`, and the future static `internal/skills` runtime. The sprint stays in-binary, reuses the SQLite memory store, preserves external `honcho_*` compatibility, and lands the gateway chassis only after the runtime surfaces are green.

**Tech Stack:** Go 1.25+, existing `internal/goncho`, `internal/memory`, `internal/tools`, `internal/subagent`, `internal/telegram`, Cobra in `cmd/gormes`, `go test`

---

## Naming Lock

- Internal subsystem name: `goncho`
- Product/runtime name in docs: **GONCHO**
- External compatibility surface stays `honcho_*` for drop-in parity
- Optional aliases in this sprint:
  - `goncho_profile`
  - `goncho_search`
  - `goncho_context`
  - `goncho_reasoning`
  - `goncho_conclude`

## Source Documents

- Sprint coordinator: `gormes/docs/superpowers/plans/2026-04-21-gormes-goncho-momentum-sprint.md`
- Goncho architecture: `gormes/docs/superpowers/specs/2026-04-21-goncho-architecture-design.md`
- Goncho base slice: `gormes/docs/superpowers/plans/2026-04-21-goncho-immediate-slice.md`
- Subagent runtime slice: `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2e0-runtime-execution.md`
- Skills runtime slice: `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2g0-skills-execution.md`

## Baseline Snapshot

- `internal/goncho/*` already exists and has service/contract tests.
- `internal/tools/honcho_tools.go` already exposes `honcho_*` tools backed by Goncho.
- Telegram currently wires Goncho into a live-memory runtime in `cmd/gormes/telegram.go`.
- TUI still runs on `store.NewNoop()` and therefore does not expose Goncho on the live path.
- Current full-suite failure:
  - `go test ./... -count=1`
  - `internal/telegram: TestBot_StreamsAssistantDraft`

## Dependency Order

1. Stop the bleeding in `internal/telegram`
2. Move Goncho from Telegram-only wiring to shared live runtime wiring
3. Add `goncho_*` aliases over the compatibility surface
4. Land the minimal static skills runtime
5. Glue bounded subagent delegation to explicit skill injection
6. Extract the gateway chassis and add the second adapter skeleton

The order is strict. Do not start skills or chassis work while the live Telegram path is red.

---

### Slice 0: Stop The Bleeding In Telegram

**Files:**
- Modify: `gormes/internal/telegram/bot_test.go`
- Modify: `gormes/internal/telegram/bot.go` only if the runtime bug is confirmed and the test fix alone is insufficient

**Acceptance criteria:**
- `TestBot_StreamsAssistantDraft` fails for the right reason before the fix.
- The test no longer depends on `lastSentText()` as the only observation point for streamed edits.
- `go test ./internal/telegram -count=1` passes after the fix.
- No Goncho or subagent code changes are mixed into this slice.

- [ ] **Step 1: Reproduce the exact failure**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/telegram -run TestBot_StreamsAssistantDraft -count=1 -v
```

Expected: FAIL with the empty final bot draft symptom.

- [ ] **Step 2: Tighten the test first**

Change `gormes/internal/telegram/bot_test.go` so the test asserts against emitted edit/send history rather than only the last snapshot. Keep the test focused on one property: at least one outbound event must contain `"hello"`.

- [ ] **Step 3: Re-run the failing test**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/telegram -run TestBot_StreamsAssistantDraft -count=1 -v
```

Expected: still FAIL, but now from a deterministic assertion.

- [ ] **Step 4: Apply the minimal runtime fix only if needed**

If the deterministic test still proves a runtime bug, patch `gormes/internal/telegram/bot.go` so the final streamed draft is flushed before turn completion.

- [ ] **Step 5: Verify the package**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/telegram -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add gormes/internal/telegram/bot_test.go gormes/internal/telegram/bot.go
git commit -m "test(telegram): make streamed draft assertion deterministic"
```

Use `fix(telegram): flush final streamed draft before done` only if `bot.go` changed.

**Risks:** masking a real runtime bug by weakening the assertion.

**Rollback:** revert this commit only. It is intentionally isolated from Goncho work.

---

### Slice 1: Put GONCHO In Every Live Runtime

#### Task 1.1 Shared live-memory registry wiring

**Files:**
- Modify: `gormes/cmd/gormes/main.go`
- Modify: `gormes/cmd/gormes/registry.go`
- Modify: `gormes/cmd/gormes/telegram.go`
- Create: `gormes/cmd/gormes/registry_test.go`

**Acceptance criteria:**
- TUI path stops using `store.NewNoop()` for the Goncho-capable runtime path.
- A shared registry-builder can register Goncho tools against a real SQLite-backed memory store.
- Telegram keeps its current behavior after the refactor.
- Shutdown ordering remains bounded and explicit.

- [ ] **Step 1: Write the failing registry tests**

Add tests in `gormes/cmd/gormes/registry_test.go` that prove:

- shared live registry includes `honcho_*` tools when a real DB-backed Goncho service is provided
- the same builder can be used by both TUI and Telegram paths

- [ ] **Step 2: Run the red tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./cmd/gormes -run 'Registry|Honcho|Goncho' -count=1 -v
```

Expected: FAIL because the shared live builder does not exist yet.

- [ ] **Step 3: Implement the shared builder**

Move the Telegram-only Goncho wiring into a reusable registry-building path in `gormes/cmd/gormes/registry.go`, then consume it from both `main.go` and `telegram.go`.

- [ ] **Step 4: Re-run the package tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./cmd/gormes -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/cmd/gormes/main.go gormes/cmd/gormes/registry.go gormes/cmd/gormes/telegram.go gormes/cmd/gormes/registry_test.go
git commit -m "feat(goncho): share live registry wiring across runtimes"
```

**Risks:** TUI now owns a real memory store lifecycle and can leak handles if close ordering is wrong.

**Rollback:** revert this commit; Telegram keeps the old local wiring.

#### Task 1.2 Alias `goncho_*` tool names

**Files:**
- Modify: `gormes/internal/tools/honcho_tools.go`
- Modify: `gormes/internal/tools/honcho_tools_test.go`

**Acceptance criteria:**
- `goncho_profile`, `goncho_search`, `goncho_context`, `goncho_reasoning`, and `goncho_conclude` register alongside the `honcho_*` compatibility names.
- Aliases resolve to the same handler logic and JSON schema as the compatibility names.
- Existing `honcho_*` contract tests continue to pass.

- [ ] **Step 1: Write the failing alias tests**

Extend `gormes/internal/tools/honcho_tools_test.go` to assert the alias names are registered and callable.

- [ ] **Step 2: Run the red tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/tools -run 'Honcho|Goncho' -count=1 -v
```

Expected: FAIL because only `honcho_*` names exist today.

- [ ] **Step 3: Implement the aliases**

Register alias tool wrappers in `gormes/internal/tools/honcho_tools.go` with identical behavior and schema.

- [ ] **Step 4: Verify the package**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/tools -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/tools/honcho_tools.go gormes/internal/tools/honcho_tools_test.go
git commit -m "feat(goncho): add goncho alias tools"
```

**Risks:** duplicate registration names or drift between alias and compatibility surfaces.

**Rollback:** revert this commit only; the compatibility surface remains intact.

---

### Slice 2: Static Skills Runtime

This slice must stay aligned with the deeper task breakdown in `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2g0-skills-execution.md`. The momentum sprint only locks the integration order.

**Files:**
- Create: `gormes/internal/skills/catalog.go`
- Create: `gormes/internal/skills/catalog_test.go`
- Create: `gormes/internal/tools/skills_tools.go`
- Create: `gormes/internal/tools/skills_tools_test.go`
- Modify: `gormes/cmd/gormes/registry.go`
- Modify: `gormes/internal/config/config.go` if a skills root is required

**Acceptance criteria:**
- Static markdown skills can be loaded from disk.
- Two minimal tools exist: `skills_list` and `skill_view`.
- The runtime does not mutate active skills during a live run.
- No candidate-drafting logic lands in this slice.

- [ ] **Step 1: Write the red catalog tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/skills -run 'Catalog|Skill' -count=1 -v
```

Expected: FAIL because the package does not exist yet.

- [ ] **Step 2: Implement static loading and view/list tools**

Follow the detailed 2.G0 plan for the implementation order, but keep this sprint slice limited to:

- static catalog load
- deterministic list/view
- runtime registration

- [ ] **Step 3: Verify the packages**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/skills ./internal/tools ./cmd/gormes -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add gormes/internal/skills gormes/internal/tools/skills_tools.go gormes/internal/tools/skills_tools_test.go gormes/cmd/gormes/registry.go gormes/internal/config/config.go
git commit -m "feat(skills): add static catalog and runtime tools"
```

**Risks:** leaking future candidate-promotion complexity into the static runtime.

**Rollback:** revert the commit; no migration should be required in this slice.

---

### Slice 3: GONCHO + Subagent + Skills Glue

This slice depends on the existing bounded runtime work in `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2e0-runtime-execution.md`.

**Files:**
- Modify: `gormes/internal/subagent/delegate_tool.go`
- Modify: `gormes/internal/subagent/delegate_tool_test.go`
- Create: `gormes/internal/kernel/integration_subagent_skills_test.go`
- Modify: `gormes/internal/kernel/kernel.go` only if an explicit skill block injection seam is required

**Acceptance criteria:**
- Delegate payload can carry an explicit `skills` list or equivalent bounded selector input.
- Depth limits, cancellation, timeout, and concurrency invariants stay unchanged.
- One end-to-end integration test proves delegated work can consume a static skill without mutating the active catalog.

- [ ] **Step 1: Write the red integration tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/subagent ./internal/kernel -run 'Delegate|Skill|Integration' -count=1 -v
```

Expected: FAIL because delegate flow is not skill-aware yet.

- [ ] **Step 2: Implement bounded skill-aware delegation**

Patch `gormes/internal/subagent/delegate_tool.go` to accept explicit skill input and pass it through without widening the runtime policy surface.

- [ ] **Step 3: Verify the runtime packages**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/subagent ./internal/kernel -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add gormes/internal/subagent/delegate_tool.go gormes/internal/subagent/delegate_tool_test.go gormes/internal/kernel/integration_subagent_skills_test.go gormes/internal/kernel/kernel.go
git commit -m "feat(subagent): inject static skills into delegate flow"
```

**Risks:** coupling subagent runtime to skill storage internals.

**Rollback:** revert this commit only; subagent runtime remains deterministic and skill runtime remains standalone.

---

### Slice 4: Gateway Chassis Kickoff

**Files:**
- Create: `gormes/internal/gateway/adapter.go`
- Create: `gormes/internal/gateway/runtime.go`
- Create: `gormes/internal/gateway/runtime_test.go`
- Modify: `gormes/internal/telegram/bot.go`
- Create: `gormes/internal/discord/adapter.go`
- Create: `gormes/cmd/gormes/discord.go`
- Modify: `gormes/cmd/gormes/main.go`

**Acceptance criteria:**
- Telegram conforms to a shared adapter lifecycle without changing user-visible behavior.
- A Discord skeleton exists and compiles against the same runtime contract.
- This slice does not attempt full Discord parity; it only proves the chassis seam.

- [ ] **Step 1: Write the red gateway runtime tests**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/gateway ./internal/telegram ./cmd/gormes -run 'Gateway|Adapter|Telegram' -count=1 -v
```

Expected: FAIL because the shared gateway runtime does not exist yet.

- [ ] **Step 2: Extract the shared adapter contract**

Implement the shared `Start`, `Send`, `Receive`, and `Close` runtime contract in `internal/gateway`, then adapt Telegram onto it.

- [ ] **Step 3: Add the Discord skeleton**

Create a compile-checked second adapter that satisfies the same contract, plus a `gormes discord` command surface.

- [ ] **Step 4: Verify the gateway packages**

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./internal/gateway ./internal/telegram ./internal/discord ./cmd/gormes -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/gateway gormes/internal/telegram/bot.go gormes/internal/discord gormes/cmd/gormes/discord.go gormes/cmd/gormes/main.go
git commit -m "refactor(gateway): extract shared adapter runtime"
```

Then:

```bash
git add gormes/internal/discord gormes/cmd/gormes/discord.go gormes/cmd/gormes/main.go
git commit -m "feat(discord): add shared-chassis adapter skeleton"
```

**Risks:** mixing runtime extraction with adapter feature work and losing the behavioral baseline for Telegram.

**Rollback:** revert the Discord commit first, then revert the gateway runtime extraction if Telegram behavior changed.

---

## Hard Gate After Every Slice

Run:

```bash
cd /home/xel/git/sages-openclaw/workspace-mineru/golang-hermes-agent/gormes
go test ./... -count=1
go test ./docs -count=1
```

No slice is done until both commands are green.

## Operator Rule

If a slice breaks global tests, stop feature work and open the next atomic commit as:

```text
fix(regression): <what broke>
```

Do not stack new feature work on top of a known red global suite.
