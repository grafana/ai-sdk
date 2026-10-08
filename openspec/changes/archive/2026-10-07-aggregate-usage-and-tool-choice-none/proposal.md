## Why

`aggregateUsage` summed only `InputTokens.Total` and `OutputTokens.Total`, so `TotalUsage()`, `AggregateUsage()` and the finish usage lost the cache, text and reasoning counts. Upstream `addLanguageModelUsage` sums every field.

The Anthropic converter removed every tool for tool choice `none`. Tools are at the start of the prompt cache prefix, so a `none` call invalidated the whole cache. The Messages API accepts `tool_choice: {"type":"none"}` with tools on the direct and Vertex endpoints.

## What Changes

- Sum every usage field across steps. Totals stay set (zero when unreported); breakdown fields stay nil when no step reported them.
- For tool choice `none` with tools, keep the tools and send `tool_choice: none`. Without tools, omit `tool_choice`.
- With the JSON response tool fallback, `none` still sends only the `json` tool, because the fallback forces tool use.
- Record the difference from upstream `@ai-sdk/anthropic` in `test/conformance/upstream.yaml`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `provider-v4-core-types`: aggregate every usage field.
- `anthropic-structured-output`: `none` with the fallback sends only the `json` tool.
- `anthropic-tool-options`: `none` keeps tools and sends `tool_choice: none`.

## Impact

`streamtext.go`, `providers/anthropic/convert_request.go`, their tests, one `upstream.yaml` entry and the specs. No public signatures, pins or Bedrock behavior change.
