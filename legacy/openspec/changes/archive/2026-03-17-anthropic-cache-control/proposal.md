## Why

The Anthropic provider does not support `cache_control` annotations on any request element (system prompts, content blocks, tool definitions). Without prompt caching, every request re-processes the full system prompt, tool definitions, and conversation history -- resulting in 2-5x cost increase and measurably higher TTFT latency for production workloads. The upstream Vercel AI SDK already supports this as a first-class Anthropic provider feature via `providerOptions`, and the Go port's `ProviderOptions` plumbing is already in place on all relevant types but unused for caching.

## What Changes

- Add `cache_control` extraction from `ProviderOptions["anthropic"]` on system messages, user/assistant/tool content parts, and tool definitions in the Anthropic provider
- Add a cache control validator that enforces Anthropic's max 4 breakpoints per request and prevents cache_control on non-cacheable contexts (e.g., thinking blocks)
- Implement the last-part cascade: when the last content part in a message has no cache_control, fall back to the message-level providerOptions
- Accept both `cacheControl` (camelCase) and `cache_control` (snake_case) key names, matching upstream flexibility
- Support TTL configuration (`ephemeral` type with optional `ttl` of `5m` or `1h`)
- Expose cache usage metrics (`cacheCreationInputTokens`, `cacheReadInputTokens`) in response provider metadata

## Capabilities

### New Capabilities

- `anthropic-prompt-caching`: Cache control annotation support in the Anthropic provider -- extraction from providerOptions, validation (breakpoint limits, context rules), plumbing to all Anthropic API block types, and response-side cache usage metrics

### Modified Capabilities

_(none -- no existing specs)_

## Impact

- **Code**: `anthropic/` module only -- `convert_request.go` (request building), response handling, new helper/validator files
- **APIs**: No changes to the `provider/` package interface. Callers opt in by setting `ProviderOptions["anthropic"]["cacheControl"]` on messages, content parts, or tools they already construct
- **Dependencies**: The Anthropic Go SDK (`anthropic-sdk-go` v1.17.0) already has `CacheControl` fields on all block param types -- no dependency changes needed
- **Wire compatibility**: No impact on SSE/UIMessageChunk format. Cache usage metrics surface through existing provider metadata passthrough
