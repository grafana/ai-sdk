## Why

The streaming feature prototype fails for two inputs exercised by integration:
a flush-capable plugin writer without socket write deadlines, and the
Anthropic provider's start-metadata custom part. Both are representation and
transport compatibility fixes, distinct from adding Chat streaming itself.

## What Changes

- Accept unsupported write deadlines while retaining response-controller
  flushing and the host's cancellation ownership.
- Discard `anthropic.message_start` custom metadata while preserving canonical
  response metadata; continue rejecting other custom kinds.
- Add focused regressions for both compatibility boundaries.

## Non-goals

This change does not register a plugin route, implement shared execution,
permit arbitrary custom content, or change provider lifecycle semantics. The
restack already uses #555/#579/#581; #483 lifecycle validation remains pending
before readiness. It does not establish deployment readiness.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-openai-chat-completions-adapter`: support flush-capable writers
  without socket deadlines and discard the known Anthropic metadata part.

## Impact

Only the AGPL streaming handler and its tests change. The feature draft stays
separately reviewable; this draft follows it. No provider recordings or native
provider implementations change.
