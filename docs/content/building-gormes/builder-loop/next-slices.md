---
title: "Next Slices"
weight: 30
aliases:
  - /building-gormes/next-slices/
---

# Next Slices

This page is generated from the canonical progress file and lists the highest
leverage contract-bearing roadmap rows to execute next.

The ordering is:

1. unblocked `P0` handoffs;
2. active `in_progress` rows;
3. `fixture_ready` rows;
4. unblocked rows that unblock other slices;
5. remaining `draft` contract rows.

Use this page when choosing implementation work. If a row is too broad, split
the row in `progress.json` before assigning it.

<!-- PROGRESS:START kind=next-slices -->
| Phase | Slice | Contract | Trust class | Fixture | Why now |
|---|---|---|---|---|---|
| 4 / 4.A | Bedrock stream event decoding (SSE fixtures) | internal/hermes/bedrock_stream.go decodes synthetic Bedrock ConverseStream event maps into the shared hermes.Event model without AWS SDK clients: text deltas emit EventToken, reasoningContent.text deltas emit EventReasoning, contentBlockStart/contentBlockDelta/contentBlockStop toolUse chunks accumulate one ToolCall, messageStop maps stopReason to FinishReason, and metadata.usage maps inputTokens/outputTokens to the final EventDone | system | `internal/hermes/bedrock_stream_test.go` | Unblocks Bedrock SigV4 + credential seam. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
<!-- PROGRESS:END -->
