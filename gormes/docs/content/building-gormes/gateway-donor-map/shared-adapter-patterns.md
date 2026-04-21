---
title: "Shared Adapter Patterns"
weight: 10
---

# Shared Adapter Patterns

This page captures the PicoClaw mechanics that are reusable across more than one channel adapter. Copy transport-edge ideas from here; do not import PicoClaw's product architecture.

## Provenance

- Donor inspected for this page: external sibling repo `/home/xel/git/sages-openclaw/workspace-mineru/picoclaw`.
- Donor commit pinned for this research: `6421f146a99df1bebcd4b1ca8de2a289dfca3622`.
- Upstream donor repo: `https://github.com/sipeed/picoclaw`.
- Any `pkg/...` or `docs/...` path listed below is relative to that donor root, not relative to the Gormes repo.

## Donor Files

- `picoclaw/pkg/channels/base.go`
- `picoclaw/pkg/channels/interfaces.go`
- `picoclaw/pkg/channels/manager.go`
- `picoclaw/pkg/channels/manager_channel.go`
- `picoclaw/pkg/channels/split.go`
- `picoclaw/pkg/channels/marker.go`
- `picoclaw/pkg/channels/dynamic_mux.go`
- `picoclaw/pkg/channels/media.go`
- `picoclaw/pkg/channels/registry.go`
- `picoclaw/pkg/channels/voice_capabilities.go`
- `picoclaw/pkg/channels/webhook.go`

If you are porting a new adapter, start with `picoclaw/pkg/channels/base.go`, `picoclaw/pkg/channels/interfaces.go`, `picoclaw/pkg/channels/manager.go`, and `picoclaw/pkg/channels/split.go` before drilling into the channel-specific donor files.

## The Reusable Shape

PicoClaw's reusable contribution is not a full gateway architecture. The reusable part is a small adapter surface plus a set of optional capabilities.

- Treat `picoclaw/pkg/channels/interfaces.go` as the donor for capability-style boundaries around typing, message edit, delete, reaction, placeholder, streaming, and placeholder recording instead of one giant adapter contract.
- Treat `picoclaw/pkg/channels/media.go`, `picoclaw/pkg/channels/webhook.go`, and `picoclaw/pkg/channels/voice_capabilities.go` as the donor files for media send, webhook or health endpoints, and voice capability reporting respectively.
- Treat `picoclaw/pkg/channels/base.go` as the donor for common adapter state: runtime name, running flag, allow-list evaluation, group-trigger policy, media store injection, placeholder-recorder injection, and a per-adapter outbound length declaration.
- Keep Gormes authoritative on message models and runtime ownership. PicoClaw's `Channel` interface is coupled to its own bus and config types, so only the shape should transfer cleanly.

`BaseChannel` exposes several options that generalize well to Gormes:

- `WithMaxMessageLength` in `picoclaw/pkg/channels/base.go` is the right abstraction for platform-specific outbound limits. Gormes adapters should declare limits in one place and let shared outbound code split around them.
- `WithGroupTrigger` is reusable as transport-edge policy when a platform mixes direct chats, mentions, and prefix-triggered group messages. The exact config type should remain Gormes-native.
- `WithReasoningChannelID` is only reusable as a narrow "send internal traces somewhere else" hook. Do not let it drag PicoClaw's reasoning-channel product behavior into Gormes unless Gormes explicitly wants that feature.
- The allow-list handling in `BaseChannel.IsAllowed` and `BaseChannel.IsAllowedSender` is worth copying as a pattern: normalize identity matching once, keep adapter-specific sender parsing at the edge, and keep authorization checks before publishing inbound events.

The useful `BaseChannel` pattern is "shared adapter helper with opt-in capabilities", not "embed this type and inherit the whole runtime."

## Message Splitting And Outbound Limits

`picoclaw/pkg/channels/split.go` should be treated as the source donor for per-channel outbound length policy.

- `SplitMessage` splits on rune count, not byte count, which matters for Unicode-heavy chat platforms.
- It prefers natural boundaries such as newlines and spaces before falling back to hard cuts.
- It preserves fenced code blocks when possible and, when necessary, closes and reopens fences so chunks still render sanely.
- The worker path in `picoclaw/pkg/channels/manager.go` applies splitting after reading the adapter's `MaxMessageLength`, which keeps the policy adapter-driven instead of scattering limits through call sites.

`picoclaw/pkg/channels/marker.go` adds a second layer: semantic split markers inserted upstream. That is useful only if Gormes wants model-authored chunk boundaries. It should stay optional and subordinate to the transport limit logic from `split.go`.

Recommended Gormes rule:

1. Let each adapter declare its outbound text ceiling.
2. Run shared splitting against that ceiling.
3. Preserve formatting, especially code fences.
4. Only add explicit split markers if Gormes has a real product need for model-directed chunking.

## Typing, Placeholder, And Reaction Mechanics

The strongest reusable interaction pattern is PicoClaw's placeholder lifecycle.

Inbound side:

- `BaseChannel.HandleMessageWithContext` in `picoclaw/pkg/channels/base.go` auto-starts typing, adds a reaction, and optionally sends a placeholder before publishing the inbound message.
- It records undo/cleanup functions through `PlaceholderRecorder` from `picoclaw/pkg/channels/interfaces.go` rather than baking those mechanics into every adapter.

Outbound side:

- `Manager.preSend` in `picoclaw/pkg/channels/manager.go` is the donor pattern Gormes should copy.
- First stop typing.
- Then undo the inbound reaction.
- Then check whether streaming already finalized delivery.
- If streaming already produced the final visible output, delete the placeholder and skip a second send.
- Otherwise try to edit the placeholder in place.
- If edit fails, fall back to a normal outbound send.

That placeholder-edit flow is the key reusable mechanic. It gives adapters a better UX without forcing all channels to support editing.

Two implementation details are worth keeping:

- The stop and undo functions are explicitly required to be idempotent in `picoclaw/pkg/channels/interfaces.go`. Gormes should keep that contract.
- `preSendMedia` in `picoclaw/pkg/channels/manager.go` handles media separately: stop typing, undo reactions, clear any placeholder, then send media without pretending an attachment can replace text by edit.

What not to copy blindly:

- PicoClaw triggers placeholder behavior directly from `BaseChannel.HandleMessageWithContext`, which assumes a particular bus publication path and lifecycle timing.
- In Gormes, keep the same state machine if it helps, but hang it off Gormes' own inbound/outbound orchestration points rather than copying the exact call graph.

## Manager / Worker / Rate-Limit Patterns

The reusable part of `picoclaw/pkg/channels/manager.go` is operational, not architectural.

- One worker queue per adapter is a good default when the platform needs ordered delivery per channel instance.
- A per-adapter rate limiter in front of outbound sends is a good shared safeguard.
- Retry classification is useful: permanent send failures should stop quickly, rate-limit failures should wait on a fixed delay, and transient failures can use exponential backoff.
- Separate text and media workers are reasonable when media delivery has different capabilities and cleanup rules.
- TTL cleanup for typing/placeholder/reaction state is worth copying whenever adapter-side UX state can outlive the request path.

What should not transfer as-is:

- PicoClaw's `Manager` is tightly coupled to `pkg/bus`, shared channel maps, config hashing in `picoclaw/pkg/channels/manager_channel.go`, and runtime channel reload behavior.
- Gormes should not adopt PicoClaw's manager as a central gateway authority unless Gormes independently decides to own delivery through the same kind of bus-centric runtime.
- `registry.go` is useful as a reminder that channel factories can self-register behind a narrow constructor surface, but the exact registration and config-decoding path should stay Gormes-native.

The safe takeaway is "shared delivery helpers around adapters", not "port PicoClaw's whole manager package."

## Webhook And Dynamic-Mux Patterns

`picoclaw/pkg/channels/webhook.go` and `picoclaw/pkg/channels/dynamic_mux.go` are reusable for webhook-family adapters only.

- `WebhookHandler` is a good narrow contract: provide a mount path and an `http.Handler`.
- `HealthChecker` is similarly narrow and can share the same HTTP surface.
- `dynamicServeMux` is useful when webhook-capable adapters are added, removed, or reloaded without rebuilding the entire HTTP server.
- Its routing model is simple and practical: exact match first, then longest matching subtree prefix.

Keep the boundary explicit: `dynamic_mux.go` is a webhook-family pattern, not a universal gateway model.

Do not turn every Gormes adapter into an HTTP-mounted unit just because PicoClaw supports dynamic webhook registration. Polling adapters, websocket adapters, and RPC-backed adapters do not benefit from that abstraction. Use it only where Gormes needs a shared inbound HTTP edge for multiple webhook-style transports.

## What Gormes Should Not Import Blindly

- Do not import PicoClaw's `Channel` or manager types. They are bound to PicoClaw's bus, config, health server, and reload assumptions.
- Do not copy `manager_channel.go` hashing and hidden-secret logic unless Gormes actually wants live channel reload with config-diff detection. That is runtime infrastructure, not adapter edge logic.
- Do not copy `WithReasoningChannelID` as a product requirement. It is only a hint that some systems may route internal output differently.
- Do not copy the deprecated fallback table in `picoclaw/pkg/channels/voice_capabilities.go`. If Gormes wants voice capability reporting, prefer explicit adapter-declared capabilities over hard-coded channel-name maps.
- Do not assume placeholder, reaction, typing, streaming, edit, delete, media, and webhook support all belong on every adapter. PicoClaw's design works because these are optional capabilities.
- Do not let manager/bus assumptions leak into contributor guidance. Gormes architecture remains authoritative; PicoClaw is donor input for transport-edge mechanics only.

## Code References

- `picoclaw/pkg/channels/base.go`: `WithMaxMessageLength`, `WithGroupTrigger`, `WithReasoningChannelID`, `ShouldRespondInGroup`, `IsAllowedSender`, `HandleMessageWithContext`.
- `picoclaw/pkg/channels/interfaces.go`: `TypingCapable`, `MessageEditor`, `MessageDeleter`, `ReactionCapable`, `PlaceholderCapable`, `StreamingCapable`, `PlaceholderRecorder`, `CommandRegistrarCapable`.
- `picoclaw/pkg/channels/manager.go`: `RecordPlaceholder`, `RecordTypingStop`, `RecordReactionUndo`, `preSend`, `preSendMedia`, `GetStreamer`, `newChannelWorker`, `runWorker`, `sendWithRetry`, `sendMediaWithRetry`, `runTTLJanitor`.
- `picoclaw/pkg/channels/manager_channel.go`: `toChannelHashes`, `compareChannels`, `toChannelConfig`.
- `picoclaw/pkg/channels/split.go`: `SplitMessage` and its code-fence-preserving helpers.
- `picoclaw/pkg/channels/marker.go`: `MessageSplitMarker`, `SplitByMarker`.
- `picoclaw/pkg/channels/dynamic_mux.go`: `dynamicServeMux`, `Handle`, `Unhandle`, `ServeHTTP`.
- `picoclaw/pkg/channels/media.go`: `MediaSender`.
- `picoclaw/pkg/channels/registry.go`: `ChannelFactory`, `RegisterFactory`, `RegisterSafeFactory`.
- `picoclaw/pkg/channels/voice_capabilities.go`: `VoiceCapabilityProvider`, `DetectVoiceCapabilities`.
- `picoclaw/pkg/channels/webhook.go`: `WebhookHandler`, `HealthChecker`.
