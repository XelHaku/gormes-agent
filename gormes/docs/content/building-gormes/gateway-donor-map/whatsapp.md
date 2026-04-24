---
title: "WhatsApp"
weight: 40
---

# WhatsApp

WhatsApp is partially ported for Phase 2.B.4, but PicoClaw actually offers two distinct donors: a bridge-based adapter and an optional build-tagged native adapter. A future Gormes porter needs to treat those as different decisions, not one blended implementation.

## Status

Gormes does not yet ship a runnable WhatsApp adapter. The current Go surface is a transport-neutral `internal/channels/whatsapp.NormalizeInbound` contract that normalizes direct/group peer IDs and passes generic slash commands through `gateway.ParseInboundText`; runtime selection, pairing, reconnect, and outbound send lifecycle are still planned. The upstream Hermes docs currently describe a built-in Baileys bridge flow, while PicoClaw supports:

Evidence level:

- Donor code for this dossier was verified against the external sibling repo at `<picoclaw donor repo>`.
- The donor commit inspected for this research was `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- The upstream donor repo is `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.
- Current Gormes status and operator-facing behavior were verified in-tree against `gormes/internal/channels/whatsapp/inbound.go`, `gormes/internal/channels/whatsapp/inbound_test.go`, `gormes/docs/content/building-gormes/architecture_plan/subsystem-inventory.md`, and `gormes/docs/content/upstream-hermes/user-guide/messaging/whatsapp.md`.

- a thin bridge-based WebSocket adapter in `picoclaw/pkg/channels/whatsapp/whatsapp.go`
- an optional in-process `whatsmeow` adapter in `picoclaw/pkg/channels/whatsapp_native/whatsapp_native.go`, compiled behind the `whatsapp_native` build tag

That means the donor question is not just "how do we port WhatsApp?" but also "which operational model does Gormes want to own?"

Keep the boundary explicit: PicoClaw is donor input for channel-edge WhatsApp mechanics. Gormes architecture, release model, and deployment ergonomics remain authoritative.

## Why This Adapter Is Reusable

The bridge and native donors are useful for different reasons.

- The bridge path is reusable as the thinnest possible WhatsApp edge: websocket connect, JSON message pump, simple send path, and straightforward inbound shaping.
- The native path is reusable as an operational blueprint for QR login, SQLite-backed device state, reconnect logic, and direct `whatsmeow` message handling without an external sidecar.
- The command tests in both variants are valuable because they prove an important policy: generic commands such as `/help` and `/new` are forwarded upward, not consumed locally by the adapter.

The donor becomes less reusable where it hardcodes bridge payload shapes or relies on build tags and library-specific lifecycle details.

## Picoclaw Donor Files

- Provenance note: the following `pkg/...` and `docs/...` paths are relative to the external donor root `<picoclaw donor repo>` at commit `6421f146a99df1bebcd4b1ca8de2a289dfca3622`, not relative to the Gormes repo.
- `picoclaw/pkg/channels/whatsapp/whatsapp.go`
- `picoclaw/pkg/channels/whatsapp/whatsapp_command_test.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_native.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_command_test.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_native_stub.go`
- `picoclaw/docs/guides/chat-apps.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/whatsapp.md`

## What To Copy vs What To Rebuild

Copy candidates:

- From the bridge path: the minimal inbound and outbound message shape, allow-list check placement, and "adapter does not execute generic commands" policy.
- From the native path: QR login flow, persistent device store setup, reconnect backoff, graceful stop sequencing, and direct JID parsing.
- From both command tests: preserve the contract that `/help`, `/new`, and similar commands go to the shared runtime.

Rebuild in Gormes-native form:

- Bridge protocol details. `picoclaw/pkg/channels/whatsapp/whatsapp.go` assumes a very specific websocket JSON schema with fields like `type`, `from`, `chat`, `content`, and `media`. That is donor input, not a stable Gormes contract.
- Build-tag packaging. `picoclaw/pkg/channels/whatsapp_native/whatsapp_native_stub.go` is useful only if Gormes chooses the same optional-compile strategy.
- Session storage and path layout. PicoClaw's native path writes a SQLite store under a configurable local directory; Gormes should align that with its own config and state conventions.
- Upstream Hermes product behavior such as unauthorized-DM handling, bridge lifecycle UX, and risk messaging should come from Gormes product docs, not from PicoClaw.

## Gormes Mapping

- The bridge donor maps to a future `internal/channels/whatsapp/bridge.go` if Gormes wants an external sidecar or embedded bridge process.
- The native donor maps to a future `internal/channels/whatsapp/native.go` if Gormes wants direct in-process ownership of WhatsApp connectivity.
- `handleIncomingMessage` in the bridge path and `handleIncoming` in the native path both map conceptually to the Gormes responsibility already started by `NormalizeInbound`: sanitize inbound content, attach sender/chat metadata, preserve message ID, then hand off to the kernel-facing gateway layer.
- `reconnectWithBackoff`, `Stop`, and the QR-path startup logic are the most reusable parts of the native donor if Gormes adopts `whatsmeow`.
- The stub file maps to a release decision, not a runtime design. If Gormes does not want build-tag fragmentation, do not port that pattern.

## Implementation Notes

- Decide the operating model first. If Gormes wants the simplest parity path with upstream Hermes docs, a bridge-based adapter is the lower-friction start. If Gormes wants one self-contained Go binary, the native path is the better donor.
- The bridge adapter is operationally light but only as good as the bridge contract behind it. Its code is mostly wiring, not durable WhatsApp domain knowledge.
- The native adapter contains the more valuable engineering: persistent pairing state, reconnect control, QR flow, stop-versus-reconnect race handling, and message send validation when pairing is incomplete.
- If Gormes adopts a native implementation, port the shutdown and reconnection safeguards, not just the happy path. `whatsapp_native.go` is strongest in lifecycle handling.
- If Gormes stays bridge-based first, still borrow the native tests' policy around command passthrough and sender/chat metadata.

## Risks / Mismatches

- The bridge donor is only reusable if the bridge exists and remains maintained. Its websocket payload schema is not portable by itself.
- The native donor depends on `whatsmeow`, QR pairing, and a local session store. That increases binary complexity, operational support burden, and test surface.
- The `whatsapp_native` build tag creates a split-brain release model. That may be acceptable for PicoClaw, but it may be the wrong trade-off for Gormes.
- Upstream Hermes currently documents a Baileys-based bridge experience. A future native Gormes implementation would need documentation and operator UX updates, not just code.

## Port Order Recommendation

1. Decide whether Gormes wants bridge-first or native-first ownership and freeze that in a runtime-selection contract.
2. Keep the existing `NormalizeInbound` command-passthrough tests as the invariant for both runtime paths.
3. If bridge-first, port only the thin adapter ideas from `picoclaw/pkg/channels/whatsapp/whatsapp.go` and keep the bridge contract explicitly external.
4. If native-first, port lifecycle pieces from `picoclaw/pkg/channels/whatsapp_native/whatsapp_native.go` before worrying about parity extras.
5. Treat build-tag strategy as a product and release decision, not an automatic code reuse choice.

## Code References

- `picoclaw/pkg/channels/whatsapp/whatsapp.go`: `Start`, `Stop`, `Send`, `listen`, `handleIncomingMessage`.
- `picoclaw/pkg/channels/whatsapp/whatsapp_command_test.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_native.go`: `Start`, `Stop`, `eventHandler`, `reconnectWithBackoff`, `handleIncoming`, `Send`, `parseJID`.
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_command_test.go`
- `picoclaw/pkg/channels/whatsapp_native/whatsapp_native_stub.go`
- `picoclaw/docs/guides/chat-apps.md`
- `gormes/docs/content/upstream-hermes/user-guide/messaging/whatsapp.md`

Recommendation: `adapt pattern only`.
