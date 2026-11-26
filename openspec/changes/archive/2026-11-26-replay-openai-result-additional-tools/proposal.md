## Why

OpenAI Responses can load tool definitions immediately after an ordinary tool result. The Go adapter drops these result options, so a request cannot replay the original insertion points of tools discovered during a conversation.

## What Changes

- Expose a typed OpenAI result-part option; accept the same definitions from raw JSON options.
- Emit ordered developer `additional_tools` items next to ordinary function results and grouped parallel wrapper results.
- Reject malformed definitions before either unary or streaming HTTP requests.

## Non-goals

Do not change the discovery matcher, application tool registration or execution, request-level tool declarations, or system-message controls.

## Capabilities

### Modified Capabilities

- `openai-responses-provider`: Replay additional tools from function result history.

## Impact

The OpenAI provider module gains an additive option and changes request conversion only when a participating result carries that option. Core forwarding is verified, not changed.
