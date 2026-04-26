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
## 1. Bedrock stream event decoding (SSE fixtures)

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: internal/hermes/bedrock_stream.go decodes synthetic Bedrock ConverseStream event maps into the shared hermes.Event model without AWS SDK clients: text deltas emit EventToken, reasoningContent.text deltas emit EventReasoning, contentBlockStart/contentBlockDelta/contentBlockStop toolUse chunks accumulate one ToolCall, messageStop maps stopReason to FinishReason, and metadata.usage maps inputTokens/outputTokens to the final EventDone
- Trust class: system
- Ready when: Bedrock Converse payload mapping (no AWS SDK) is complete and internal/hermes/bedrock_converse.go defines the request-side Bedrock types., The row can use literal []map[string]any or small typed fixture structs in tests; no boto3, AWS SDK, network, SigV4, credential, or provider registration code is needed., The first edit should be internal/hermes/bedrock_stream_test.go with RED tests for text stream, reasoning delta, tool-use split chunks, empty stream, interrupt stop, and usage propagation.
- Not ready when: The slice imports an AWS SDK, signs requests, opens HTTP, registers a Bedrock provider, or changes buildBedrockConversePayload., The slice rewrites the shared Stream interface or kernel event loop instead of returning []Event from a pure decoder helper., The slice handles stale client eviction or credential discovery; those belong to the dependent rows.
- Degraded mode: Provider status reports bedrock_stream_unavailable until fixtures prove text, reasoning, tool-use, interrupt, and usage event decoding; Bedrock remains request-mapping-only before this row lands.
- Fixture: `internal/hermes/bedrock_stream_test.go`
- Write scope: `internal/hermes/bedrock_stream.go`, `internal/hermes/bedrock_stream_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'TestDecodeBedrockStream_' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/bedrock_stream_test.go proves text, reasoning, tool-use chunking, empty stream, interrupt stop, and usage propagation through shared hermes.Event values with no AWS SDK import.
- Acceptance: TestDecodeBedrockStream_TextAndUsage feeds messageStart, contentBlockDelta{text:'Hello'}, contentBlockStop, messageStop{end_turn}, metadata{inputTokens:5,outputTokens:3}; it returns EventToken('Hello') followed by EventDone{FinishReason:'stop', TokensIn:5, TokensOut:3}., TestDecodeBedrockStream_ReasoningDelta emits EventReasoning with Reasoning='Let me think...' and does not include the reasoning text in EventToken., TestDecodeBedrockStream_ToolUseChunks accumulates two toolUse input chunks into ToolCall{ID:'call_1', Name:'read_file', Arguments:{"path":"/tmp/f"}} and final EventDone{FinishReason:'tool_calls'}., TestDecodeBedrockStream_MixedTextAndToolPreservesText emits the pre-tool text token and still returns the tool call in final EventDone., TestDecodeBedrockStream_EmptyStreamReturnsStop returns exactly one EventDone with FinishReason='stop' and zero tokens., TestDecodeBedrockStream_InterruptStopsBeforeRemainingEvents accepts a callback or limit hook and proves later deltas are not emitted after interruption., go test ./internal/hermes -run 'TestDecodeBedrockStream_' -count=1 passes without network, AWS SDK, or credential imports.
- Source refs: ../hermes-agent/agent/bedrock_adapter.py:651:normalize_converse_stream_events, ../hermes-agent/agent/bedrock_adapter.py:673:stream_converse_with_callbacks, ../hermes-agent/tests/agent/test_bedrock_adapter.py:457:TestNormalizeConverseStreamEvents, ../hermes-agent/tests/agent/test_bedrock_adapter.py:868:TestStreamConverseWithCallbacks, internal/hermes/client.go:Event,EventToken,EventReasoning,EventDone, internal/hermes/bedrock_converse.go
- Unblocks: Bedrock SigV4 + credential seam
- Why now: Unblocks Bedrock SigV4 + credential seam.

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
