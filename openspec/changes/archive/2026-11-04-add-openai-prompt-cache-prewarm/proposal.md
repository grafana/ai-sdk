## Why

Issue #606 tracks missing prompt-cache prewarming and breakpoint TTLs in the registered OpenAI Responses baseline, @ai-sdk/openai 4.0.102 (612be2311464804d1e4897afacbf8f3fc2f6d9e3). Invalid request TTLs currently reach the provider.

## What Changes

- Add typed 30m/12h TTL values, optional prewarm, and per-part breakpoint TTL.
- Reject unsupported request TTLs before HTTP and preserve upstream breakpoint passthrough semantics.
- Verify empty unary/stream prewarm results and raw cache-write usage through OpenAI and the shared Mantle adapter.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: prompt-cache request controls, breakpoint TTLs and prewarm results.

## Impact

OpenAI public options and shared request validation; focused synthetic HTTP tests cover OpenAI and Mantle. Changing PromptCacheOptions.TTL from string to PromptCacheTTL requires callers assigning string variables to convert them explicitly; untyped string literals remain compatible. No dependency or frontend protocol changes.
