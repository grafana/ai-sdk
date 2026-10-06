## Context

The Anthropic provider in `anthropic/` converts `provider.CallOptions` into Anthropic API requests via `buildParams()` in `convert_request.go`. Every provider type (`SystemMessage`, `UserMessage`, content parts, `Tool`) already carries a `ProviderOptions map[string]json.RawMessage` field -- but only two extraction points exist today: top-level thinking config (`applyProviderOptions`) and signature on reasoning blocks (`extractSignature`). The Anthropic Go SDK (v1.26.0) already has `CacheControl BetaCacheControlEphemeralParam` fields on all relevant block param types. Response-side cache metrics (`CacheReadInputTokens`, `CacheCreationInputTokens`) are already handled in `convert_response.go` for the non-streaming path, but not in `convert_stream.go`.

## Goals / Non-Goals

**Goals:**
- Enable callers to annotate any message, content part, or tool definition with `cache_control` via the existing `ProviderOptions` mechanism
- Enforce Anthropic's 4-breakpoint limit per request with warnings (not errors) for excess breakpoints
- Prevent cache_control on non-cacheable contexts (thinking blocks)
- Match upstream Vercel AI SDK behavior: last-part cascade, dual key names, TTL support
- Surface cache usage metrics in streaming responses (currently missing)

**Non-Goals:**
- Automatic cache placement heuristics or middleware -- callers decide what to cache
- Changes to the `provider/` package types -- ProviderOptions plumbing already exists
- Request-level `cache_control` on the API body top-level (Anthropic's docs don't clearly specify this for the messages API; upstream supports it but it's low priority and can be added later)
- Cache metrics in `ProviderMetadata` as a separate field -- the existing `InputTokenDetails` on `Usage` already covers this

## Decisions

### 1. New file `anthropic/cache_control.go` for extraction and validation

**Decision**: Create a dedicated file rather than inlining into `convert_request.go`.

**Rationale**: The validator is stateful (tracks breakpoint count across the full request) and the extraction logic needs to be called from multiple sites. A separate file keeps `convert_request.go` focused on conversion and makes the validator testable in isolation.

**What goes in this file**:
- `cacheControlValidator` struct with breakpoint counter and warnings slice
- `getCacheControl(opts map[string]json.RawMessage, canCache bool) BetaCacheControlEphemeralParam` method that extracts, validates, and tracks
- Internal `extractCacheControl(opts)` helper that reads `cacheControl` or `cache_control` from the `"anthropic"` namespace

### 2. Stateful validator shared across the entire request

**Decision**: A single `cacheControlValidator` instance is created in `buildParams()` and threaded through all conversion functions.

**Rationale**: Anthropic enforces a max of 4 cache breakpoints per request across all block types (system, messages, tools). The validator must track the running count globally. Upstream uses the same pattern -- one `CacheControlValidator` instance per request.

**Alternatives considered**:
- Stateless per-block extraction (simpler, but can't enforce the 4-breakpoint limit)
- Post-processing pass that counts breakpoints and trims excess (adds complexity, harder to determine which to keep)

### 3. Warnings for excess breakpoints, not errors

**Decision**: When a 5th+ cache_control annotation is encountered, drop it silently and append a `provider.Warning`. Do not fail the request.

**Rationale**: Matches upstream behavior. The caller may not know how many breakpoints are set across tools + messages + system prompt. Failing the request would be a bad UX.

### 4. Last-part cascade via message-level ProviderOptions fallback

**Decision**: Change content conversion function signatures to accept the parent message's `ProviderOptions`. On the last content part in a message, if the part has no `cache_control`, fall back to the message-level `cache_control`.

**Current signatures**:
- `convertUserContent(parts []provider.UserContentPart)`
- `convertAssistantContent(parts []provider.AssistantContentPart)`
- `convertToolContent(parts []provider.ToolResultContentPart)`

**New signatures** (add validator + message opts):
- `convertUserContent(v *cacheControlValidator, parts []provider.UserContentPart, msgOpts map[string]json.RawMessage)`
- `convertAssistantContent(v *cacheControlValidator, parts []provider.AssistantContentPart, msgOpts map[string]json.RawMessage)`
- `convertToolContent(v *cacheControlValidator, parts []provider.ToolResultContentPart, msgOpts map[string]json.RawMessage)`

**Rationale**: Matches upstream cascade behavior exactly. The common case is setting `cache_control` once on a message rather than on every individual part.

**Alternatives considered**:
- Pre-processing step that copies message-level cache_control to the last part before conversion (mutates input, awkward)
- Only supporting part-level cache_control (simpler but diverges from upstream)

### 5. Dual key names: `cacheControl` and `cache_control`

**Decision**: The extraction helper checks both `cacheControl` (camelCase) and `cache_control` (snake_case) in the `"anthropic"` provider options namespace. `cacheControl` takes precedence.

**Rationale**: Upstream accepts both. Go callers may prefer either depending on whether they're constructing JSON manually or via struct tags.

### 6. Thinking blocks are non-cacheable

**Decision**: When `getCacheControl` is called for a `ReasoningContentPart`, pass `canCache: false`. The validator drops the annotation and emits a warning.

**Rationale**: Anthropic caches thinking blocks implicitly by position. Explicit `cache_control` on them is invalid and the upstream SDK prevents it.

### 7. Streaming cache metrics

**Decision**: In `convert_stream.go`, check for `CacheReadInputTokens` and `CacheCreationInputTokens` on usage objects in both `BetaRawMessageStartEvent` and `BetaRawMessageDeltaEvent`, populating `InputTokenDetails` the same way `convert_response.go` does.

**Rationale**: The non-streaming path already does this. The streaming path should be consistent. Without this, streaming callers would never see cache metrics.

## Risks / Trade-offs

**[Signature changes to conversion functions]** Adding `validator` and `msgOpts` parameters to `convertUserContent`, `convertAssistantContent`, `convertToolContent`, and `convertTools` is a moderately invasive change to `convert_request.go`. -> These are all unexported functions within the `anthropic` package, so no external API impact. The change is mechanical.

**[Breakpoint count accuracy]** Tools are converted before messages in `buildParams()` (tools at line 47-49, messages at line 20-45). The validator must process tools first to get the correct running count. -> Reorder `buildParams()` to convert tools before iterating messages, or accept the current order and ensure the validator is passed through both. Current order already processes messages first, then tools -- we should process tools first to match upstream order (tools consume breakpoints before messages).

**[TTL field availability]** The `BetaCacheControlEphemeralParam` in the Go SDK has a `TTL` field of type `BetaCacheControlEphemeralTTL` with values `"5m"` and `"1h"`. We need to map from the JSON input to this typed field. -> Straightforward string mapping in the extraction helper.
