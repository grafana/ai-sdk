## Context

Registered baseline: `@ai-sdk/openai@4.0.106`, commit `028a025398782f4938303f48fbeee45c79af544b`. The issue originally cites 4.0.103; the current registered source preserves the same whole-object selection in `openai-responses-prepare-tools.ts` and derives the namespace from model identity in `openai-responses-language-model.ts`. Matching Azure source tests cover generation and stream deferred loading; Azure is not a registered package and those tests are semantic evidence only.

`PARITY.md` classifies provider adapters as mixed provider-implementation coverage. No matching recorded/imported input demonstrates this boundary. Evidence here is synthetic HTTP request tests, not provider recordings.

## Decisions

Capture the identity-derived tool namespace before the existing default call-level fallback resolves an Azure-only options map. Pass it explicitly to function-tool preparation and input-history context. Use `provider.ResolveOption` for typed and JSON-decoded values, preserving its null handling and contextual errors. Missing/null selected objects fall back; empty objects and explicit false values do not merge fields. Custom provider tools retain their existing OpenAI-only option handling.

An `OpenAIToolOptions` value can already be stored directly in `provider.ProviderOptions{"azure": options}`. The map key controls lookup and JSON serialization; `ProviderKey()` only controls `BuildProviderOptions`. Document and test that existing idiom instead of adding an Azure wrapper or generic namespace API.

## Risks / Trade-offs

Default model call options have a compatibility fallback to Azure, unlike upstream. Preserve that existing contract but do not extend it to function-tool options. Synthetic requests prove adapter selection and encoding, not Azure deployment acceptance. No frontend wire behavior changes and no new recorded fixture is claimed.
