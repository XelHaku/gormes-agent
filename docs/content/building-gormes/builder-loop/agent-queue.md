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
## 1. WhatsApp unsafe sender/chat inbound evidence

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: WhatsApp inbound normalization in internal/channels/whatsapp/inbound.go uses the validated NormalizeSafeWhatsAppIdentifier helper to reject unsafe raw sender, chat, and reply target identifiers before Event.UserID, Event.ChatID, Reply.ChatID, or outbound pairing targets are created
- Trust class: gateway, operator, system
- Ready when: WhatsApp identifier safety predicate is validated on main; identity.go is a read-only dependency for this slice., The worker edits only inbound.go and a new inbound safety fixture; no platform runtime, bridge process, native WhatsApp client, send/reconnect path, or gateway manager dispatch is needed., Safe identity_contract.json fixtures must remain unchanged.
- Not ready when: The slice edits identity.go, identity_test.go, send.go, reconnect code, gateway manager code, or live WhatsApp client setup., The slice accepts unsafe IDs after stripping dangerous characters instead of returning whatsapp_identifier_unsafe evidence., The slice rewrites the existing identity_contract.json safe-case fixture.
- Degraded mode: Unsafe inbound WhatsApp sender/chat/reply IDs produce whatsapp_identifier_unsafe evidence and an unresolved event shape instead of a session key or pairing target.
- Fixture: `internal/channels/whatsapp/identity_inbound_safety_test.go::TestNormalizeInboundWithIdentity_UnsafeRawIDs`
- Write scope: `internal/channels/whatsapp/inbound.go`, `internal/channels/whatsapp/identity_inbound_safety_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/whatsapp -run '^TestNormalizeInboundWithIdentity_UnsafeRaw\|^TestNormalizeInboundWithIdentity_UnsafeChat\|^TestNormalizeInboundWithIdentity_UnsafeReply' -count=1`, `go test ./internal/channels/whatsapp -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Inbound safety fixtures prove unsafe sender, chat, and reply identifiers are rejected with whatsapp_identifier_unsafe evidence while safe identity contracts remain green.
- Acceptance: TestNormalizeInboundWithIdentity_UnsafeRawSenderRejected proves unsafe sender/user IDs return whatsapp_identifier_unsafe evidence and leave Event.UserID empty., TestNormalizeInboundWithIdentity_UnsafeChatRejected proves unsafe chat IDs do not produce Event.ChatID, Reply.ChatID, or an outbound pairing target., TestNormalizeInboundWithIdentity_UnsafeReplyTargetRejected proves unsafe reply targets are dropped with evidence while safe sender/chat IDs still normalize., Existing identity_contract.json fixtures still pass unchanged for safe bridge/native DM and group cases.
- Source refs: ../hermes-agent/gateway/whatsapp_identity.py@91512b82:expand_whatsapp_aliases path traversal guard, ../hermes-agent/gateway/whatsapp_identity.py@6993e566:_SAFE_IDENTIFIER_RE ASCII guard, internal/channels/whatsapp/inbound.go:NormalizeInboundWithIdentity, internal/channels/whatsapp/identity.go:NormalizeSafeWhatsAppIdentifier
- Unblocks: WhatsApp unsafe alias endpoint inbound evidence
- Why now: Unblocks WhatsApp unsafe alias endpoint inbound evidence.

## 2. ContextEngine compression-boundary callback vocabulary

- Phase: 4 / 4.B
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/hermes defines a compression-boundary callback vocabulary on ContextEngine with stable lineage evidence and status fields, without binding kernel compression execution yet
- Trust class: operator, system
- Ready when: ContextEngine interface + status tool contract is validated on main., Compression token-budget and single-prompt threshold fixtures are validated, so this slice only adds callback vocabulary and status evidence., The worker edits only internal/hermes context-engine files; no kernel, transcript storage, summarizer, or Goncho/Honcho memory behavior is in scope.
- Not ready when: The slice edits internal/kernel/kernel.go, creates internal/kernel/context_engine.go, implements summarization, mutates transcript history, or binds live compression execution., The slice hides boundary failures or lets status imply a boundary callback ran before the kernel binding row is complete., The slice changes Goncho/Honcho memory extraction semantics; memory pre/post-compression observation remains a separate Phase 3/4 concern.
- Degraded mode: Context status reports compression_boundary_unavailable or last_boundary_missing evidence until kernel compression execution binds the callback.
- Fixture: `internal/hermes/context_engine_boundary_test.go::TestContextEngineCompressionBoundaryVocabulary`
- Write scope: `internal/hermes/context_engine.go`, `internal/hermes/context_engine_test.go`, `internal/hermes/context_engine_boundary_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'Test.*ContextEngine.*Boundary\|TestDisabledContextEngine_StatusToolFixture' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Hermes package fixtures prove boundary vocabulary, status evidence, unavailable/missing degraded modes, and no kernel or memory side effects.
- Acceptance: A new CompressionBoundary value carries old_session_id, new_session_id, reason, and compressed_at or equivalent stable lineage evidence., DisabledContextEngine or an in-package fake can record one boundary callback and expose last boundary evidence through Status without contacting a provider., Status reports compression_boundary_unavailable or last_boundary_missing when no boundary has been recorded., Existing context_status tool fixtures remain stable except for the added boundary evidence fields.
- Source refs: ../hermes-agent/run_agent.py@e85b7525, ../hermes-agent/tests/run_agent/test_compression_boundary_hook.py@e85b7525, internal/hermes/context_engine.go:ContextEngine, internal/hermes/context_engine_test.go
- Unblocks: ContextEngine compression-boundary notification
- Why now: Unblocks ContextEngine compression-boundary notification.

## 3. Custom provider model-switch key_env write guard

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

## 4. API server detailed health endpoint

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Native API server binds the validated DetailedHealthSnapshot value model to unauthenticated GET /health/detailed and /v1/health/detailed routes while preserving existing flat /health and /v1/health behavior
- Trust class: operator, gateway, system
- Ready when: API server detailed health snapshot contract is validated on main; detailed_health.go is a read-only dependency for this row., This row only binds GET /health/detailed and GET /v1/health/detailed routes and intentionally follows Hermes' unauthenticated /health/detailed behavior., The slice can use httptest against Server.Handler with fake snapshot inputs; no provider, live gateway, scheduler goroutine, cron admin endpoint, or cron mutation endpoint is required., Existing /health and /v1/health fixtures remain byte-compatible except for explicitly allowed internal refactoring.
- Not ready when: The slice edits detailed_health.go, creates /api/jobs endpoints, mutates cron jobs, starts a scheduler, adds auth requirements to health routes, changes OpenAI-style error envelopes, or weakens auth/body-size checks on non-health routes., The slice changes dashboard client behavior or imports Hermes Python.
- Degraded mode: HTTP detailed health returns per-section degraded evidence through stable JSON without requiring an API key, while flat /health remains backward-compatible.
- Fixture: `internal/apiserver/detailed_health_endpoint_test.go`
- Write scope: `internal/apiserver/detailed_health_endpoint_test.go`, `internal/apiserver/server.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/apiserver -run '^TestAPIServerDetailedHealthEndpoint_' -count=1`, `go test ./internal/apiserver -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: API server endpoint fixtures prove /health/detailed and /v1/health/detailed route binding, unauthenticated health access, error-envelope compatibility, redaction, and unchanged flat health routes.
- Acceptance: TestAPIServerDetailedHealthEndpoint_OK GETs /health/detailed and /v1/health/detailed and decodes provider, response_store, run_events, gateway, and cron sections., TestAPIServerDetailedHealthEndpoint_NoAuthRequired configures APIKey="sk-test" and proves both detailed health routes return 200 without Authorization or X-API-Key headers., TestAPIServerDetailedHealthEndpoint_MethodNotAllowed reuses the shared OpenAI-style method-not-allowed error envelope., TestAPIServerDetailedHealthEndpoint_RedactsSecrets proves the HTTP response body omits provider keys, cron script bodies, gateway tokens, and raw request payloads., Existing /health and /v1/health tests continue to report status/platform/responses/runs as before.
- Source refs: ../hermes-agent/gateway/platforms/api_server.py@ee1a07f9:_handle_health_detailed no authentication required, internal/apiserver/detailed_health.go:DetailedHealthSnapshot, internal/apiserver/server.go:NewServer,handleHealth, internal/apiserver/chat_completions_test.go:health fixtures
- Unblocks: API server cron admin read-only endpoints
- Why now: Unblocks API server cron admin read-only endpoints.

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
