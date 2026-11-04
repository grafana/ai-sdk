## Context

The registered baseline is @ai-sdk/openai 4.0.102 at 612be2311464804d1e4897afacbf8f3fc2f6d9e3. Its request options schema validates ttl against 30m/12h and accepts optional prewarm. OpenAIResponsesPromptCacheBreakpoint exposes the same TTL union, but getPromptCacheBreakpoint casts directly without runtime validation. The existing Go converter already forwards a shared breakpoint object and handles empty outputs and cache-write usage. PARITY.md classifies provider implementations and Mantle coverage as mixed.

## Goals / Non-Goals

**Goals:** preserve upstream request semantics with idiomatic Go options, validate request TTL before transport, and prove empty prewarm results retain usage.

**Non-Goals:** new runtime breakpoint validation, cache-mode validation, provider recording, frontend changes or exposing dedicated TTL usage fields.

## Decisions

Use PromptCacheTTL with typed 30m/12h constants per repository conventions. Keep its zero value omitted. As with existing optional Go string options, raw JSON empty/null TTL also decodes to the omitted zero value; unlike the upstream Zod schema, this does not distinguish explicitly empty/null TTL from absence. This is an intentional Go decoding boundary, not strict raw-option parsing parity. Prewarm is *bool so false remains distinguishable from omission. Add TTL to PromptCacheBreakpoint; all existing converter sites then carry it without new conversion paths. Extend the existing request-option validator, shared by unary, stream, Azure and Mantle. Breakpoint values retain upstream passthrough behavior; issue acceptance concerning rejection applies to request TTLs, matching upstream runtime evidence.

Use focused synthetic HTTP tests based on the registered upstream unit examples. They establish emitted request bodies, rejection before transport, empty terminal results and raw TTL usage retention, not live-provider acceptance. No recorded fixture is fabricated or changed.

## Risks / Trade-offs

PromptCacheOptions.TTL becomes a named string type. Callers assigning a string variable must write PromptCacheTTL(value); literals continue to compile. This is a source API change and must be highlighted in release review. Arbitrary values can still be constructed, so request validation remains necessary. Part TTLs intentionally preserve upstream runtime passthrough behavior.

Empty prewarm coverage is scoped to provider DoGenerate/DoStream. Core text orchestration may require generated content; changing that behavior is outside the OpenAI provider issue.
