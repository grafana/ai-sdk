# Design

## Context

See proposal.md for the reported failure. `ProviderOptions.MarshalJSON` serializes each concrete option; decoding produces a `RawProviderOption`. The Anthropic cache parser accepts nested `cacheControl` and `cache_control`, whereas `AnthropicCacheControl` currently emits exported Go field names.

The registered reference is `@ai-sdk/anthropic` 4.0.71 at Vercel commit `eb77f09e3c06c28e860d92e0de941b143c2eecec`. Its source has the matching package version. `anthropic-language-model-options.ts` defines nested `cacheControl`; `get-cache-control.ts` reads the camel-case and snake-case keys. `convert-to-anthropic-prompt.test.ts` covers system, message, and content annotations. This is provider-implementation work in the coverage map. No upstream behavior deviation is intended.

## Goals / Non-Goals

Preserve helper values across the existing JSON representation without changing the struct's public fields. Do not broaden cache validation or export a new resolver.

## Decisions

Implement `MarshalJSON` with a nested `CacheControlType`, using its existing tags so an absent TTL is omitted. Adding flat tags alone would still be unreadable by the raw parser. Implement matching `UnmarshalJSON` so `provider.ResolveOption[AnthropicCacheControl]` can recover the documented typed value. Accept the same camel-case/snake-case keys and precedence as cache extraction.

Add table-driven tests for helper JSON, typed resolution, and native request conversion after a `provider.CallOptions` JSON round trip. Run requests in unary and streaming conversion modes at each supported annotation location. These tests exercise actual provider conversion using synthetic local options; they are not recorded provider conformance evidence. Existing recorded inputs and expectations remain unchanged.

## Risks / Trade-offs

- Existing callers may have persisted the previously ineffective exported-field representation. Preserve decoding of that legacy shape so typed resolution remains compatible while new serialization uses the canonical nested form.
- Local request tests do not prove live provider acceptance. The wire shape is grounded in the registered upstream source and existing cache-control conversion tests; no live-provider claim is made.

## Migration Plan

No caller migration or module dependency update is required. Archive the completed specification change after regression tests and parity checks. Rollback is a revert of the provider serialization methods and added requirement.
