## Why

OpenAI-compatible unary responses currently place text before reasoning. The
registered upstream emits reasoning before text and tool calls, so consumers of
`DoGenerate` receive a different content order (grafana/ai-sdk#607).

## What Changes

- Emit existing reasoning before existing text, followed by tool calls.
- Test unary HTTP responses and response messages through simulated streaming.
- Preserve reasoning-field selection, text decoding, tool metadata and native streaming.

## Capabilities

### New Capabilities

- `openai-compatible-unary-order`: ordering of supported unary response content.

### Modified Capabilities

None.

## Impact

Only the OpenAI-compatible unary response converter changes. No new API, provider
fixture, dependency, request behavior or baseline upgrade is required.
