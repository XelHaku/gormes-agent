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
## 1. Gateway message deduplicator bounded eviction

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Shared gateway deduplication caps tracked message IDs at max_size with deterministic oldest-entry eviction and visible dropped-duplicate evidence
- Trust class: gateway, system
- Ready when: Shared gateway inbound event normalization is validated and platform adapters already pass stable message IDs where available., The helper can be tested as a pure in-memory structure with synthetic message IDs; no live platform SDK or manager run loop is required., Adapters without message IDs remain accepted and produce dedup_unavailable evidence rather than being dropped.
- Not ready when: The slice changes channel session keys, authorization/pairing policy, message rendering, or outbound coalescing., The slice adds platform-specific Telegram/Slack/Discord dedup state instead of a shared helper., The slice stores dedup history on disk; this row is an in-memory bounded guard only.
- Degraded mode: Gateway status reports deduplicator_disabled or deduplicator_evicted evidence instead of growing unbounded in long-running platform adapters.
- Fixture: `internal/gateway/message_deduplicator_test.go`
- Write scope: `internal/gateway/message_deduplicator.go`, `internal/gateway/message_deduplicator_test.go`, `internal/gateway/event.go`, `internal/gateway/manager.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run 'TestMessageDeduplicator\|TestGatewayInbound_Dedup' -count=1`, `go test ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway deduplicator fixtures prove max_size eviction, duplicate rejection, disabled mode, and missing-ID degraded evidence without platform SDKs or disk state.
- Acceptance: TestMessageDeduplicator_MaxSizeEvictsOldest adds max_size+1 distinct IDs and proves the first ID is no longer considered duplicate while the newest IDs remain tracked., TestMessageDeduplicator_DuplicateReturnsSeen proves repeated IDs are rejected before eviction and emits duplicate evidence., TestMessageDeduplicator_ZeroMaxSizeDisabled treats max_size=0 as disabled and never rejects messages., TestGatewayInbound_DedupMissingMessageIDDegrades proves empty message IDs do not drop messages but expose dedup_unavailable evidence.
- Source refs: ../hermes-agent/gateway/platforms/helpers.py@cebf9585:MessageDeduplicator, ../hermes-agent/tests/gateway/test_message_deduplicator.py@cebf9585, internal/gateway/event.go, internal/gateway/manager.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 2. Checkpoint shadow-repo GC policy

- Phase: 5 / 5.L
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Native checkpoint manager prunes orphan and stale shadow repositories at startup using a deterministic policy before any write-capable file tools depend on rollback state
- Trust class: operator, child-agent, system
- Ready when: The row can be implemented as pure filesystem fixtures under t.TempDir with fake timestamps and no model/tool execution., internal/tools exists and can own the checkpoint manager contract without exposing write_file or patch tools yet., Rollback state paths are Gormes-owned XDG paths, not upstream ~/.hermes paths.
- Not ready when: The slice exposes write_file, patch, or checkpoint restore tools before the cleanup/read-model contract is fixture-locked., The slice shells out to git or deletes real repositories outside t.TempDir in tests., The slice copies Hermes home layout instead of documenting the Gormes XDG rollback directory decision.
- Degraded mode: Checkpoint status reports shadow_gc_unavailable, orphan_shadow_repo, or stale_shadow_repo evidence instead of silently leaving rollback directories to accumulate.
- Fixture: `internal/tools/checkpoint_manager_test.go`
- Write scope: `internal/tools/checkpoint_manager.go`, `internal/tools/checkpoint_manager_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestCheckpointManager' -count=1`, `go test ./internal/tools -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Checkpoint manager fixtures prove startup orphan/stale shadow cleanup, dry-run reporting, fake-clock TTL behavior, and redacted status evidence under t.TempDir only.
- Acceptance: TestCheckpointManagerPrunesOrphanShadowRepos seeds an active shadow repo and an orphan repo under t.TempDir; startup cleanup removes only the orphan and records evidence., TestCheckpointManagerPrunesStaleShadowRepos uses a fake clock to remove stale shadows older than the configured TTL while preserving fresh active shadows., TestCheckpointManagerDryRunReportsCandidates returns the same orphan/stale candidates without deleting them., Status evidence names counts and paths relative to the checkpoint root, with no absolute home-directory leakage.
- Source refs: ../hermes-agent/tools/checkpoint_manager.py@478444c2, ../hermes-agent/tests/tools/test_checkpoint_manager.py@478444c2, ../hermes-agent/cli.py@478444c2:startup checkpoint cleanup, ../hermes-agent/gateway/run.py@478444c2:startup checkpoint cleanup, docs/content/building-gormes/architecture_plan/phase-5-final-purge.md
- Unblocks: File read dedup cache invalidation and wrapper guard
- Why now: Unblocks File read dedup cache invalidation and wrapper guard.

## 3. BlueBubbles iMessage bubble formatting parity

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

## 4. Yuanbao protocol envelope + markdown fixtures

- Phase: 7 / 7.E
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P4`
- Contract: Gormes parses Yuanbao websocket/protobuf-style envelopes and Markdown message fragments into gateway-neutral events using fixture data only
- Trust class: gateway, system
- Ready when: The Phase 2 shared gateway event shape and Regional + Device Adapter Backlog are available; this row does not need a live Yuanbao account., Workers can start with captured JSON/proto/markdown testdata under internal/channels/yuanbao/testdata copied or minimized from upstream fixtures., No send loop, login flow, tool registration, media download, or sticker parsing is required for this first slice.
- Not ready when: The slice opens a websocket, performs login, calls Tencent/Yuanbao endpoints, downloads media, or registers user-visible tools., The slice stores credentials or changes shared gateway session policy., The slice combines protocol parsing with send/reply runtime behavior.
- Degraded mode: Yuanbao adapter status reports protocol_unavailable or markdown_parse_failed evidence instead of starting a live session with unparsed payloads.
- Fixture: `internal/channels/yuanbao/proto_test.go`
- Write scope: `internal/channels/yuanbao/proto.go`, `internal/channels/yuanbao/proto_test.go`, `internal/channels/yuanbao/markdown.go`, `internal/channels/yuanbao/markdown_test.go`, `internal/channels/yuanbao/testdata/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/yuanbao -run 'TestYuanbao(Proto\|Markdown)' -count=1`, `go test ./internal/channels/yuanbao -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Yuanbao protocol/markdown fixtures prove inbound text event normalization and degraded parse evidence with no live Yuanbao network call.
- Acceptance: TestYuanbaoProto_DecodesInboundTextFixture loads a captured fixture and returns source, conversation id, message id, author role, and text content., TestYuanbaoMarkdown_RendersCodeAndLinks proves code blocks, links, mentions, and list fragments are normalized into plain prompt-safe text without losing URLs., Malformed/unknown envelope fixtures return typed degraded evidence and do not panic., No test imports a generated protobuf runtime unless a local generated fixture file is checked in under internal/channels/yuanbao.
- Source refs: ../hermes-agent/gateway/platforms/yuanbao_proto.py@ab687963, ../hermes-agent/gateway/platforms/yuanbao.py@ab687963, ../hermes-agent/tests/test_yuanbao_proto.py@ab687963, ../hermes-agent/tests/test_yuanbao_markdown.py@ab687963, ../hermes-agent/website/docs/user-guide/messaging/yuanbao.md@ab687963
- Unblocks: Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration
- Why now: Unblocks Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration.

<!-- PROGRESS:END -->
