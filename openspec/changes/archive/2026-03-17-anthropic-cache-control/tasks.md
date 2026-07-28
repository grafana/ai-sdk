## 1. Cache control extraction and validation

- [x] 1.1 Create `anthropic/cache_control.go` with `cacheControlValidator` struct (breakpoint counter, warnings slice) and `getCacheControl(opts map[string]json.RawMessage, canCache bool)` method that extracts from `"anthropic"` namespace, checks both `cacheControl` and `cache_control` keys, maps TTL, and enforces the 4-breakpoint limit
- [x] 1.2 Write unit tests for the extraction helper: dual key names, camelCase precedence, TTL mapping (`5m`, `1h`, absent), missing/empty providerOptions, invalid JSON graceful handling
- [x] 1.3 Write unit tests for the validator: exactly 4 breakpoints pass, 5th+ dropped with warning, non-cacheable context (`canCache: false`) dropped with warning, breakpoint counting across multiple calls

## 2. Request-side plumbing in convert_request.go

- [x] 2.1 Update `buildParams()` to create a `cacheControlValidator` instance and thread it through all conversion calls. Reorder to process tools before messages (so breakpoint count matches upstream tool-first ordering). Collect validator warnings into the returned warnings slice.
- [x] 2.2 Update system message handling in `buildParams()` to call `getCacheControl(m.ProviderOptions, true)` and set `CacheControl` on `BetaTextBlockParam`
- [x] 2.3 Update `convertUserContent` signature to accept `*cacheControlValidator` and message-level `ProviderOptions`. For each content part, extract part-level cache_control; on the last part, fall back to message-level cache_control if part-level is absent.
- [x] 2.4 Update `convertAssistantContent` signature to accept `*cacheControlValidator` and message-level `ProviderOptions`. Apply cache_control to `TextContentPart` and `ToolCallContentPart` blocks. Pass `canCache: false` for `ReasoningContentPart`. Implement last-part cascade.
- [x] 2.5 Update `convertToolContent` signature to accept `*cacheControlValidator` and message-level `ProviderOptions`. Apply cache_control to `BetaToolResultBlockParam`. Implement last-part cascade.
- [x] 2.6 Update `convertTools` to accept `*cacheControlValidator` and apply cache_control from each `Tool.ProviderOptions` to `BetaToolParam`

## 3. Request-side tests

- [x] 3.1 Write `buildParams` integration tests: system message with cache_control, user message with part-level cache_control, tool definitions with cache_control
- [x] 3.2 Write last-part cascade tests: message-level fallback on last part only, part-level overrides message-level, non-last parts unaffected
- [x] 3.3 Write breakpoint limit integration test: 6 cache_control annotations across tools + messages, verify first 4 applied and last 2 dropped with warnings
- [x] 3.4 Write non-cacheable context test: ReasoningContentPart with cache_control, verify dropped with warning

## 4. Streaming cache usage metrics

- [x] 4.1 Update `handleEvent` for `BetaRawMessageStartEvent` in `convert_stream.go`: check `CacheReadInputTokens` and `CacheCreationInputTokens` on usage, populate `InputTokenDetails` when non-zero
- [x] 4.2 Update `handleEvent` for `BetaRawMessageDeltaEvent`: same cache metric extraction as above
- [x] 4.3 Write streaming tests: verify `InputTokenDetails` populated when cache metrics are present, absent when zero

## 5. Verification

- [x] 5.1 Run full test suite (`make test`) and verify no regressions
- [x] 5.2 Run `make lint` and fix any issues
