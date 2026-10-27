# Design

## Context

See the proposal for motivation. The selected coherent npm package set is `ai@7.0.143`, provider `4.0.29`, Anthropic `4.0.82`, Bedrock `5.0.118`, OpenAI `4.0.96`, compatible `3.0.71`, Gateway `4.0.116`, provider-utils `5.0.62` and React `4.0.147`, all sourced at `b7f6255acaae7724adc0fd37aac3dae3f3466578`. Selection provenance remains in external automation artifacts, not a second canonical manifest.

## Goals / Non-Goals

**Goals:** preserve supported observable behavior through the entire source-workspace integration, with authentic provider inputs and strict output validation.

**Non-Goals:** implement optional parity packages, change independent module requirements mechanically, claim live-provider acceptance, or infer Vercel private-service semantics.

## Decisions

- Represent citations as a typed provider value reusing source data, with optional offsets/cited text. A dedicated JSON projection adds the nested source discriminator and required empty document fields without changing existing standalone source serialization. This avoids opaque citation JSON or an unrelated source-API redesign. Existing optional scalar source display fields retain the established Go absence/empty normalization.
- Keep citations on generated text and UI text, separate from prompt content. Native annotation metadata continues to carry provider-specific continuation information. Clone mutable citation data during persisted UI and root content assembly.
- Model input encoding as a typed enum. Partial text is a JSON-encoded string in the existing Go opaque input field; final calls keep their established JSON contract. Reuse existing UI reader state and transport projection rather than introduce a second tool state machine.
- Apply existing shared Gateway unary/frame budgets cumulatively to citations before encoding. Do not add per-citation quotas, truncation, diagnostic capture fields or schema exceptions. Keep the independent Apache client separate from Gateway validators.
- Preserve message-start custom parts independently of raw selection. Existing custom content transport now supports their exact kind; native replay tests verify actual producer output and both-client handler tests verify delivery through finish.
- Mark unavailable tool history only in copied in-process parts, analogous to upstream's nonserialized marker. Do not rewrite persisted caller output or classify arbitrary dynamic tools as unavailable.
- Stored OpenAI text references require preceding reasoning and deduplicate within one assistant message. The existing shared Mantle adapter receives the same correction without changing endpoint ownership or Go dependency pins.
- Use explicit normal-retention TypeScript overload selection in the conformance generator. Use `responseMessages` rather than final-step response messages when the pinned high-level client continues approvals across steps.

## Exact upstream evidence

At the fixed commit above:

- [Citation contract](https://github.com/vercel/ai/blob/b7f6255acaae7724adc0fd37aac3dae3f3466578/packages/provider/src/language-model/v4/language-model-v4-citation.ts), [UI citation schema](https://github.com/vercel/ai/blob/b7f6255acaae7724adc0fd37aac3dae3f3466578/packages/ai/src/types/citation.ts), and core `citations.test.ts`.
- Anthropic `map-anthropic-citation.ts`, `anthropic-language-model.ts` and adjacent tests establish text attachment, message-start custom output and retrieval fallback ordering.
- OpenAI `map-openai-responses-annotation.ts`, `openai-responses-language-model.ts`, `convert-to-openai-responses-input.ts` and their adjacent tests establish citations, retrieved sources, text input encoding and stored-reference eligibility.
- Core `process-ui-message-stream.ts`, `validate-ui-messages.ts`, `unavailable-tool.ts` and `convert-to-model-messages.ts` establish persistence and unavailable-tool output semantics.
- Provider-utils `map-reasoning-to-provider.ts` and native provider tests establish maximum effort and 95 percent budget mapping.

## Risks / Trade-offs

- Imported/mock inputs prove mappings, not live acceptance. Provider recordings remain byte-identical; synthetic transport failures remain focused tests.
- New root producer fields require coordinated candidate-source tests. Standalone producer-before-consumer publication remains separate and must precede independent module releases.
- Shape checks cover discriminators, not every nested field. Schema-parsed frontend tests, both-client Gateway tests and native provider-part snapshots provide the complementary evidence.
- Optional parity work and ambiguous historical issue dispositions remain external issues/upgrade-PR decisions, not permanent deviations or new APIs approved by this design.

## Migration Plan

Apply only the frozen selected record, regenerate existing expectations, validate candidate source and refresh reviewed baseline evidence only afterward. Roll back the coherent baseline, lockfile, expectations and compatibility corrections together rather than mixing references.
