# Gormes Phase 2.G0 Skills Execution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the static skills runtime that can load approved `SKILL.md` artifacts from disk, validate them, select a bounded active subset deterministically, inject them into parent or child turns, and record immutable usage events.

**Architecture:** Keep the skills runtime filesystem-first and reviewable. `internal/skills` owns parsing, validation, storage, selection, and rendering. The kernel consumes only a rendered prompt block and a usage sink; it does not parse markdown or mutate skills live. Candidate drafting stays inactive and separate until the later `2.E1 / 2.G1-lite` slice.

**Tech Stack:** Go stdlib (`os`, `path/filepath`, `strings`, `sort`, `time`), existing `gormes/internal/kernel`, existing `gormes/internal/config`, new `gormes/internal/skills` package, docs test harness.

---

## File Map

- Create: `gormes/internal/skills/types.go`
- Create: `gormes/internal/skills/parser.go`
- Create: `gormes/internal/skills/store.go`
- Create: `gormes/internal/skills/selector.go`
- Create: `gormes/internal/skills/render.go`
- Create: `gormes/internal/skills/usage.go`
- Create: `gormes/internal/skills/*_test.go`
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`
- Modify: `gormes/internal/kernel/kernel.go`
- Modify: `gormes/internal/kernel/kernel_test.go`
- Modify later vertical proof: `gormes/internal/subagent/*` for child-skill injection

## Task 1: Skill artifact schema and parser

**Acceptance criteria**

- `SKILL.md` files parse into a typed Go structure.
- Missing required header fields fail cleanly.
- Oversized or malformed documents are rejected before selection.

- [ ] **Step 1: Write the failing tests**

Create:

- `gormes/internal/skills/parser_test.go`
- `gormes/internal/skills/types_test.go`

Test intent:

- Parse a valid `SKILL.md` fixture with header, description, and body.
- Reject a document missing `name` or `description`.
- Reject a document larger than the configured cap.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/skills -run 'TestParse|TestSkill' -count=1 -v
```

Expected: FAIL because the package does not exist yet.

- [ ] **Step 3: Implement the schema and parser**

Create:

- `gormes/internal/skills/types.go`
- `gormes/internal/skills/parser.go`

Keep parsing deterministic and markdown-structure-aware; do not embed LLM calls in this layer.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/skills -run 'TestParse|TestSkill' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/skills/types.go gormes/internal/skills/parser.go gormes/internal/skills/types_test.go gormes/internal/skills/parser_test.go
git commit -m "feat(skills): add skill schema and parser"
```

**Risk:** parser drift if future skill templates widen too early.

**Rollback:** revert the commit; no external state created.

## Task 2: Active store and immutable snapshot loading

**Acceptance criteria**

- Active skills load from a single filesystem root.
- Candidate and active stores are separate directories.
- A run sees an immutable snapshot; mid-run file changes do not mutate the active set.

- [ ] **Step 1: Write the failing tests**

Create:

- `gormes/internal/skills/store_test.go`
- `gormes/internal/config/config_test.go` additions for skills-root defaults

Test intent:

- Load active skills from `<root>/active`.
- Ensure `<root>/candidates` is ignored by the active loader.
- Snapshot once, mutate disk, assert the in-memory snapshot does not change.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/skills ./internal/config -run 'TestSkillStore|TestSkillsRoot' -count=1 -v
```

Expected: FAIL because the store and config surface do not exist yet.

- [ ] **Step 3: Implement store + config wiring**

Create or modify:

- `gormes/internal/skills/store.go`
- `gormes/internal/config/config.go`
- `gormes/internal/config/config_test.go`

Expose filesystem roots only; candidate promotion is deferred.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/skills ./internal/config -run 'TestSkillStore|TestSkillsRoot' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/skills/store.go gormes/internal/skills/store_test.go gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "feat(skills): add active skill store and config roots"
```

**Risk:** path defaults can conflict with later XDG policy if invented loosely.

**Rollback:** revert the commit and remove the new config keys.

## Task 3: Deterministic selection and rendering

**Acceptance criteria**

- Selection is deterministic for identical input.
- Selection is capped to a small bounded set.
- Rendering produces a stable prompt block suitable for kernel injection.

- [ ] **Step 1: Write the failing tests**

Create:

- `gormes/internal/skills/selector_test.go`
- `gormes/internal/skills/render_test.go`

Test intent:

- Repeated selection over the same skill set yields the same order.
- Selection cap is enforced.
- Rendered output includes the chosen skills in stable order and nothing else.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/skills -run 'TestSelect|TestRender' -count=1 -v
```

Expected: FAIL because selector/renderer do not exist yet.

- [ ] **Step 3: Implement selector + renderer**

Create:

- `gormes/internal/skills/selector.go`
- `gormes/internal/skills/render.go`

Use deterministic scoring and stable sort keys. Do not use randomization or wall-clock data.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/skills -run 'TestSelect|TestRender' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/skills/selector.go gormes/internal/skills/render.go gormes/internal/skills/selector_test.go gormes/internal/skills/render_test.go
git commit -m "feat(skills): add deterministic selection and rendering"
```

**Risk:** overfitting selection rules to one prompt shape.

**Rollback:** revert selector/renderer only; store/parser stay intact.

## Task 4: Kernel injection and usage logging

**Acceptance criteria**

- Kernel can prepend a rendered skills block to the request path without parsing markdown itself.
- Usage events are append-only and do not mutate the active skill set.
- Invalid or absent skills degrade cleanly to “no skills injected”.

- [ ] **Step 1: Write the failing tests**

Add:

- `gormes/internal/kernel/kernel_test.go` coverage for skill-block injection
- `gormes/internal/skills/usage_test.go`

Test intent:

- Build a kernel with a stub skills provider and assert the rendered block is prepended exactly once.
- Record usage for a selected skill and assert one append-only event is written.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/kernel ./internal/skills -run 'TestKernel.*Skill|TestUsage' -count=1 -v
```

Expected: FAIL because the kernel has no skills provider seam yet.

- [ ] **Step 3: Implement kernel seam + usage log**

Create or modify:

- `gormes/internal/skills/usage.go`
- `gormes/internal/kernel/kernel.go`
- `gormes/internal/kernel/kernel_test.go`

Add a narrow provider interface so the kernel sees only a rendered block and a usage recorder.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/kernel ./internal/skills -run 'TestKernel.*Skill|TestUsage' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/skills/usage.go gormes/internal/skills/usage_test.go gormes/internal/kernel/kernel.go gormes/internal/kernel/kernel_test.go
git commit -m "feat(skills): inject active skills into kernel turns"
```

**Risk:** kernel prompt ordering can drift if the skills block is inserted in the wrong place.

**Rollback:** revert the commit; skills package remains dormant until reintegration.

## Validation Gate After Each Task

Run:

```bash
cd <repo>/gormes
go test ./internal/skills/... -count=1
go test ./internal/kernel -count=1
go test ./docs -count=1
```

For the full `2.G0` branch gate, run:

```bash
cd <repo>/gormes
go test ./... -count=1
go test ./docs -count=1
```

If full-suite verification is deferred during an intermediate slice, the report must say exactly which scoped packages were exercised and why.
