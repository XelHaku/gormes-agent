---
title: "Agent Queue"
weight: 20
aliases:
  - /building-gormes/agent-queue/
---

# Agent Queue

This page is generated from the canonical progress file:
`docs/content/building-gormes/architecture_plan/progress.json`.

It lists unblocked, non-umbrella contract rows that are ready for a focused
autonomous implementation attempt. Each card carries the execution owner,
slice size, contract, trust class, degraded-mode requirement, fixture target,
write scope, test commands, done signal, acceptance checks, and source
references.

Shared unattended-loop facts live in [Builder Loop Handoff](../builder-loop-handoff/):
the main entrypoint, orchestrator plan, candidate source, generated docs,
tests, and candidate policy. Keep those control-plane facts in
`meta.builder_loop`, and keep row-specific execution facts in `progress.json`.

<!-- PROGRESS:START kind=agent-queue -->
## 1. Durable worker execution loop

- Phase: 2 / 2.E.3
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gormes durable jobs have a Go-native fake-handler execution seam that claims one waiting job from the SQLite ledger, invokes an injected handler with context.Context, records progress/result/failure evidence, and exits cleanly when no job is claimable
- Trust class: operator, system
- Ready when: Durable job backpressure + timeout audit, Durable worker supervisor status seam, and Durable replay and inbox message contract are validated on main., The first implementation can be tested with injected fake handlers, fake clock, and SQLite ledger only; no shell executor, external worker daemon, or GBrain TypeScript runtime is required.
- Not ready when: The slice starts subprocess workers, creates a systemd service, implements shell execution, changes live delegate_task or cron semantics, or imports GBrain TypeScript code., The worker needs polling goroutines, real sleeps, network services, Postgres/PGLite, or claim loops beyond a single deterministic RunOne-style helper.
- Degraded mode: Durable-worker status reports idle, claim_unavailable, handler_failed, or heartbeat_unavailable without starting a background daemon or shell executor.
- Fixture: `internal/subagent/durable_worker_test.go`
- Write scope: `internal/subagent/durable_worker.go`, `internal/subagent/durable_worker_test.go`, `internal/doctor/durable_ledger.go`, `internal/doctor/durable_ledger_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/subagent -run '^TestDurableWorkerRunOne_' -count=1`, `go test ./internal/subagent ./internal/doctor -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Durable worker fake-handler fixtures prove claim, progress, completion, failure, idle return, and heartbeat/status evidence without external worker processes.
- Acceptance: TestDurableWorkerRunOne_ClaimsWaitingJobAndCompletesResult seeds a waiting durable job, runs a fake handler that returns JSON, and asserts the ledger records completed status, result_json, lock owner, and worker heartbeat evidence., TestDurableWorkerRunOne_HandlerProgressPersists runs a fake handler that emits progress through the worker seam and asserts the ledger stores the progress_json before completion., TestDurableWorkerRunOne_HandlerErrorFailsJob runs a fake handler that returns an error and asserts the ledger records failed status with the error text preserved., TestDurableWorkerRunOne_NoJobReturnsIdle proves an empty ledger returns idle evidence, records no fake heartbeat, and does not mutate existing terminal jobs., Doctor/status fixtures expose the last durable-worker heartbeat or heartbeat_unavailable evidence without requiring operators to inspect raw ledger rows.
- Source refs: ../gbrain/src/core/minions/worker.ts@c78c3d0:launchJob/executeJob, ../gbrain/src/core/minions/types.ts@c78c3d0:MinionWorkerOpts, ../gbrain/test/minions.test.ts@c78c3d0, internal/subagent/durable_ledger.go:Claim/UpdateProgress/Complete/Fail/RecordWorkerHeartbeat, internal/subagent/durable_ledger_test.go, internal/subagent/durable_backpressure_test.go, internal/subagent/durable_supervisor_status_test.go, internal/doctor/durable_ledger.go
- Unblocks: Durable worker abort-slot recovery safety net
- Why now: Unblocks Durable worker abort-slot recovery safety net.

## 2. BlueBubbles iMessage bubble formatting parity

- Phase: 7 / 7.E
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes
- Trust class: gateway, system
- Ready when: The first-pass BlueBubbles adapter already owns Send, markdown stripping, cached GUID resolution, and home-channel fallback in internal/channels/bluebubbles.
- Not ready when: The slice attempts to add live BlueBubbles HTTP/webhook registration, attachment download, reactions, typing indicators, or edit-message support.
- Degraded mode: BlueBubbles remains a usable first-pass adapter, but long replies may still arrive as one stripped text send until paragraph splitting and suffix-free chunking are fixture-locked.
- Fixture: `internal/channels/bluebubbles/bot_test.go`
- Write scope: `internal/channels/bluebubbles/bot.go`, `internal/channels/bluebubbles/bot_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/bluebubbles -count=1`
- Done signal: BlueBubbles adapter tests prove paragraph-to-bubble sends, suffix-free chunking, and no edit/placeholder capability.
- Acceptance: Send splits blank-line-separated paragraphs into separate SendText calls while preserving existing chat GUID resolution and home-channel fallback., Long paragraph chunks omit `(n/m)` pagination suffixes and concatenate back to the stripped original text., Bot does not implement gateway.MessageEditor or gateway.PlaceholderCapable, preserving non-editable iMessage semantics.
- Source refs: ../hermes-agent/gateway/platforms/bluebubbles.py@f731c2c2, ../hermes-agent/tests/gateway/test_bluebubbles.py@f731c2c2, internal/channels/bluebubbles/bot.go, internal/gateway/channel.go
- Unblocks: BlueBubbles iMessage session-context prompt guidance
- Why now: Unblocks BlueBubbles iMessage session-context prompt guidance.

<!-- PROGRESS:END -->
