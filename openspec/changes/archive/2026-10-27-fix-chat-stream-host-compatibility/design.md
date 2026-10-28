## Context

This change extracts the fixes originally committed as `cb5e0e40` and
`0148f06a` into their own draft after feature/OpenSpec head `4bfb3105`.
They now appear as `d6dffb5a` and `94e16d24` before this change is archived.

The registered Vercel baseline is `b7f6255acaae7724adc0fd37aac3dae3f3466578`,
including `@ai-sdk/anthropic@4.0.82` and provider `4.0.29`. Its Anthropic
`message_start` path emits canonical `response-metadata` and then a custom
`anthropic.message_start` part. This source was inspected at the registered
commit. Synthetic tests exercise that shape without claiming a live recording.

## Goals / Non-Goals

Accept supported host writers and one known unrepresented metadata kind.
Do not widen Chat's content subset, publish a route, or implement the runtime
and lifecycle prerequisites. Full plugin-process and TypeScript integration
remain separate evidence gates.

## Decisions

### Unsupported socket deadlines are not a stream failure

The response controller may report http.ErrNotSupported for SetWriteDeadline
on plugin writers even though flushing works. Accept that result and continue
flushing. Apply a request deadline only when one is present, never clear a
host-installed deadline and never add a Chat-visible idle timer. On caller
cancellation the writer watcher requests an immediate write deadline to unblock
native HTTP writes; plugin writers rely on host transport cancellation.
This is writer/framing handling, not a selection or invocation worker.
It cannot interrupt an arbitrary custom writer that blocks forever.

### Discard only represented Anthropic metadata

`anthropic.message_start` carries provider metadata, while canonical response
metadata already carries id/model. Discard the known custom part instead of
failing the response; do not copy its raw payload into Chat or count it as
visible progress. Continue failing for every other custom kind. Identity
continues to follow the streaming feature's early/late rules.

### Architectural dependency remains mandatory

The 2026-10-28 restack already uses #555 runtime, #579 attempt visibility and
#581 host contract. Private setup/drain ownership is removed. #483's shared
lifecycle validator remains pending; these compatibility fixes do not implement
it or finish #394. Preserve Chat's single-segment refusal when integrating it.

## Risks / Trade-offs

Tests with `httptest.ResponseRecorder` establish flush-capable/no-deadline
behavior, not real plugin transport cancellation. The Anthropic case is a
narrow metadata exception, not arbitrary custom-content support. The prior
prototype's post-finish rule and other #394 migration work remain unchanged.

## Migration Plan

Review this draft after the runtime-based streaming feature. Revalidate both
fixes through the protocol test host, and do not mark the adapter ready before
#483 and its integration checks.
