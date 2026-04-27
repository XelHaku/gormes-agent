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
## 1. ContextEngine compression-boundary callback vocabulary

- Phase: 4 / 4.B
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/hermes defines a compression-boundary callback vocabulary on ContextEngine with stable lineage evidence and status fields, without binding kernel compression execution yet
- Trust class: operator, system
- Ready when: ContextEngine interface + status tool contract is validated on main., Compression token-budget and single-prompt threshold fixtures are validated, so this slice only adds callback vocabulary and status evidence., The worker edits only internal/hermes context-engine files and the declared context_status JSON fixture; no kernel, transcript storage, summarizer, or Goncho/Honcho memory behavior is in scope.
- Not ready when: The slice edits internal/kernel/kernel.go, creates internal/kernel/context_engine.go, implements summarization, mutates transcript history, or binds live compression execution., The slice hides boundary failures or lets status imply a boundary callback ran before the kernel binding row is complete., The slice changes Goncho/Honcho memory extraction semantics; memory pre/post-compression observation remains a separate Phase 3/4 concern., The slice edits any testdata fixture except internal/hermes/testdata/context_status/disabled_pressure_unknown_tool.json.
- Degraded mode: Context status reports compression_boundary_unavailable or last_boundary_missing evidence until kernel compression execution binds the callback.
- Fixture: `internal/hermes/context_engine_boundary_test.go::TestContextEngineCompressionBoundaryVocabulary`
- Write scope: `internal/hermes/context_engine.go`, `internal/hermes/context_engine_test.go`, `internal/hermes/context_engine_boundary_test.go`, `internal/hermes/testdata/context_status/disabled_pressure_unknown_tool.json`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'Test.*ContextEngine.*Boundary\|TestDisabledContextEngine_StatusToolFixture\|TestContextStatusFixtures' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Hermes package fixtures and the disabled_pressure_unknown_tool.json status fixture prove boundary vocabulary, status evidence, unavailable/missing degraded modes, and no kernel or memory side effects.
- Acceptance: A new CompressionBoundary value carries old_session_id, new_session_id, reason, and compressed_at or equivalent stable lineage evidence., DisabledContextEngine or an in-package fake can record one boundary callback and expose last boundary evidence through Status without contacting a provider., Status reports compression_boundary_unavailable or last_boundary_missing when no boundary has been recorded., Existing context_status tool fixtures remain stable except for the added boundary evidence fields., The disabled_pressure_unknown_tool.json fixture is updated only as needed to include explicit missing-boundary or unavailable-boundary evidence.
- Source refs: ../hermes-agent/run_agent.py@e85b7525, ../hermes-agent/tests/run_agent/test_compression_boundary_hook.py@e85b7525, internal/hermes/context_engine.go:ContextEngine, internal/hermes/context_engine_test.go, internal/hermes/testdata/context_status/disabled_pressure_unknown_tool.json
- Unblocks: ContextEngine compression-boundary notification
- Why now: Unblocks ContextEngine compression-boundary notification.

## 2. Image input mode router + native content parts

- Phase: 5 / 5.D
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/hermes exposes a pure image input routing helper that resolves agent.image_input_mode auto/native/text from model vision capability and auxiliary vision override, then builds native provider content parts with text plus data-url image_url entries without invoking a live provider
- Trust class: operator, system
- Ready when: The worker can add a pure helper under internal/hermes with injected model capability and auxiliary-vision config values; no run_agent, kernel, gateway, or config-file binding is required., Tests create temp fixture image bytes and inspect generated data URLs; no provider request, image resizing, OCR, or external binary is required., Auto mode must choose native only when the active model is known to support vision and no auxiliary vision provider/model/base_url override is configured.
- Not ready when: The slice changes provider HTTP request builders, kernel message history, TUI file-drop behavior, gateway media ingestion, or image generation tools., The slice implements text OCR/vision-tool fallback, image resizing, or shrink retry., The slice treats unknown model capability as native vision support in auto mode.
- Degraded mode: Multimodal status reports image_input_text_fallback, image_input_native_forced, image_input_native_unavailable, or image_input_auxiliary_vision_override instead of silently dropping images.
- Fixture: `internal/hermes/image_routing_test.go::TestImageInputRouting_*`
- Write scope: `internal/hermes/image_routing.go`, `internal/hermes/image_routing_test.go`, `internal/hermes/client.go`, `internal/hermes/model_registry.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run '^TestImageInputRouting_\|^TestBuildNativeImageContentParts_' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Image routing fixtures prove auto/native/text selection, auxiliary-vision fallback, model capability handling, native data-url content part construction, unreadable-image evidence, and default prompt behavior without live providers.
- Acceptance: TestImageInputRouting_AutoNativeWhenModelSupportsVision proves auto mode returns native for a vision-capable active model with no auxiliary vision override., TestImageInputRouting_AutoTextWhenAuxVisionConfigured proves auxiliary vision provider/model/base_url config forces text fallback even when the active model supports vision., TestImageInputRouting_AutoTextForUnknownOrNonVisionModel proves unknown and non-vision model capabilities choose text fallback., TestBuildNativeImageContentParts_TextAndImages emits one text part plus image_url data-url parts in input order, skipping unreadable paths with evidence., TestBuildNativeImageContentParts_DefaultPrompt inserts a short default prompt when user text is empty and at least one image is present.
- Source refs: ../hermes-agent/agent/image_routing.py@ec671c41, ../hermes-agent/tests/agent/test_image_routing.py@ec671c41, ../hermes-agent/run_agent.py@ec671c41:_model_supports_vision, ../hermes-agent/run_agent.py@ec671c41:vision-aware preprocessing, internal/hermes/client.go:MessageContentPart, internal/hermes/model_registry.go
- Unblocks: Image-too-large shrink retry helper
- Why now: Unblocks Image-too-large shrink retry helper.

## 3. Backup/update opt-in and exclusion policy

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: CLI backup/update policy defaults pre-update backups off unless explicitly requested, honors --no-backup over --backup, and excludes checkpoints plus SQLite WAL/SHM/journal sidecars from backup manifests
- Trust class: operator, system
- Ready when: Diagnostics, backup, logs, and status CLI remains an umbrella; this row is the first pure backup policy helper and does not require a real update command., Tests use synthetic flag values and temp path lists; no archive writer, git pull, network, package manager, or real Gormes home is required.
- Not ready when: The slice implements update execution, writes archives, contacts git remotes, changes installer scripts, or scans the real operator home directory., The slice includes checkpoints/, *.db-wal, *.db-shm, or *.db-journal files in a default backup manifest., The slice changes log redaction or support-upload behavior.
- Degraded mode: Update status reports backup_skipped_default, backup_forced, backup_disabled_by_flag, or backup_manifest_excluded_paths instead of silently archiving large or unsafe runtime files.
- Fixture: `internal/cli/backup_policy_test.go::TestBackupPolicy_*`
- Write scope: `internal/cli/backup_policy.go`, `internal/cli/backup_policy_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run '^TestBackupPolicy_\|^TestBackupManifestExclusions_' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Backup policy fixtures prove pre-update backups are opt-in, --no-backup wins, and checkpoints plus SQLite WAL/SHM/journal sidecars are excluded from manifests without archive or network side effects.
- Acceptance: TestBackupPolicy_DefaultSkipsPreUpdateBackup proves no backup is requested when neither --backup nor --no-backup is set., TestBackupPolicy_ExplicitBackupEnables proves --backup requests a backup and emits backup_forced evidence., TestBackupPolicy_NoBackupWins proves --no-backup suppresses backup even when --backup is also true., TestBackupManifestExclusions_SkipsCheckpointsAndSQLiteSidecars proves checkpoints/, *.db-wal, *.db-shm, and *.db-journal are excluded while ordinary .db files remain eligible., Tests use synthetic paths/temp dirs only and do not create archives or invoke git.
- Source refs: ../hermes-agent/hermes_cli/main.py@ea3c5a14:update backup flags, ../hermes-agent/hermes_cli/backup.py@a9033c92:exclude checkpoints, ../hermes-agent/hermes_cli/backup.py@817633bc:exclude SQLite sidecars, ../hermes-agent/tests/hermes_cli/test_backup.py@817633bc, internal/cli/log_snapshot.go, cmd/gormes/doctor.go
- Unblocks: Backup manifest dry-run contract
- Why now: Unblocks Backup manifest dry-run contract.

## 4. Custom provider model-switch key_env write guard

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/cli exposes a pure model-switch patch helper that accepts an in-memory custom provider ref plus a target model and returns the config patch/evidence for default_model changes while preserving original credential storage: providers that relied on key_env and had no inline api_key/api_key_ref must not gain an api_key entry, while providers that already had inline plaintext or `${VAR}` api_key may keep that existing value without writing resolved plaintext
- Trust class: operator, system
- Ready when: Custom provider model-switch credential preservation is validated on main and provides the resolver vocabulary for env-template, plaintext, key_env, unset, and missing credentials., This slice only adds a pure patch/model-switch helper under internal/cli/custom_provider_model_switch.go; no config reader, /model command handler, TUI picker, fake /v1/models server, provider routing, or cmd/gormes wiring is required., Table tests should construct input provider maps/structs in memory and assert the planned write shape; no process environment, filesystem, or network access is needed.
- Not ready when: The slice changes internal/config, internal/hermes, provider catalog probing, TUI model picker behavior, command wiring, or the existing custom_provider_secret resolver semantics., The helper writes an api_key field for a provider whose original config relied only on key_env., The helper writes resolved plaintext when the original provider used `${VAR}` or key_env references.
- Degraded mode: Model-switch planning returns credential_write_skipped_key_env, credential_ref_preserved, plaintext_preserved, or credential_missing evidence so setup/status surfaces can explain why api_key was not written. The credential-preservation prerequisite and backend health bypass are now validated, so this row should run as a pure internal/cli patch-helper fixture.
- Fixture: `internal/cli/custom_provider_model_switch_test.go::TestCustomProviderModelSwitchPatch_*`
- Write scope: `internal/cli/custom_provider_model_switch.go`, `internal/cli/custom_provider_model_switch_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestCustomProviderModelSwitchPatch_\|TestResolveCustomProviderSecret_' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/cli custom-provider model-switch fixtures prove key_env-backed providers update default_model without adding api_key, existing inline references/plaintext are preserved without resolution, and resolver tests still pass.
- Acceptance: TestCustomProviderModelSwitchPatch_KeyEnvDoesNotSynthesizeAPIKey starts with {default_model:'old', key_env:'ACME_KEY'} and proves the patch sets default_model='new', preserves key_env, omits api_key, and returns credential_write_skipped_key_env evidence., TestCustomProviderModelSwitchPatch_InlineEnvRefPreserved starts with {api_key:'${ACME_KEY}'} and proves the patch keeps api_key='${ACME_KEY}' without resolving or overwriting it., TestCustomProviderModelSwitchPatch_PlaintextPreserved starts with {api_key:'sk-plain'} and proves plaintext is preserved only because it was already present., TestCustomProviderModelSwitchPatch_MissingCredentialStillUpdatesModelWithEvidence proves model changes remain possible while credential_missing evidence is returned for setup/status guidance., Existing TestResolveCustomProviderSecret_* fixtures remain green; this row does not redefine resolver semantics.
- Source refs: ../hermes-agent/hermes_cli/main.py@8258f4dc:_model_flow_named_custom, ../hermes-agent/tests/hermes_cli/test_custom_provider_model_switch.py@8258f4dc, ../hermes-agent/hermes_cli/main.py@8bbeaea6:_named_custom_provider_map, internal/cli/custom_provider_secret.go:CustomProviderRef,ResolveCustomProviderSecret, internal/cli/custom_provider_secret_test.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 5. BlueBubbles iMessage bubble formatting parity

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

## 6. Yuanbao protocol envelope + markdown fixtures

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
