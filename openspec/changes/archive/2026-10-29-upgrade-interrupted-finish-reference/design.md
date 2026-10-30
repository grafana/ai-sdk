# Design

## Context

The selected coherent npm package set is `ai@7.0.153`, provider `4.0.34`, Anthropic `4.0.88`, Bedrock `5.0.124`, OpenAI `4.0.102`, compatible `3.0.78`, Gateway `4.0.126`, provider-utils `5.0.68` and React `4.0.157`, all sourced at `612be2311464804d1e4897afacbf8f3fc2f6d9e3`. Selection provenance remains in external automation artifacts.

## Goals / Non-Goals

**Goals:** keep required checks and supported integrations working against the target with authentic inputs and unweakened comparisons.

**Non-Goals:** implement other assessed parity packages, emit `interrupted` from OpenAI Responses or Bedrock Converse (recorded intentional deviations, reassessment registered separately), relax the ProviderWire open-block lifecycle, or change Go module requirements.

## Decisions

- Add `interrupted` as a typed unified constant. Every exhaustive consumer (ProviderWire unary/stream validation and JSON schemas, Grafana client decoding, Gateway log allowlist and agent stop reason, Prometheus label, Agent Observability stop string `other`) handles it explicitly; unknown values keep existing rejection or fallback.
- Implement upstream's Anthropic rule as an observer on the forwarded part stream: output is a non-empty text, reasoning or tool-input delta or a tool call since the last response-metadata or finish; any error part suppresses the interrupted finish; it repeats the previous finish's usage and metadata. Placing it in the forwarding loop also observes panic-recovery error parts, and checking the context avoids emitting after cancellation.
- Core skips required/named tool-choice enforcement and stops continuation for interrupted steps; local execution already requires stop or tool-calls. Output parsing and partial-stream handling are unchanged, matching upstream.
- Shell controls use ordered structs so tool input JSON matches upstream key order; null values are omitted; a non-numeric `maxOutputLength` in a result fails conversion like other shell output fields.
- Update the conformance generator for the removed `experimental_lowMemory` option (now a separate function) without casts or weakened comparisons.

## Exact upstream evidence

At the fixed commit above: provider `language-model-v4-finish-reason.ts`; core `stream-language-model-call.ts`, `stream-text.ts`, `generate-text.ts`, `ui-message-chunks.ts` and `ui/chat.ts` with their tests (10026d1); Anthropic `anthropic-language-model.ts` and tests (43ad966, aac6026); OpenAI `openai-responses-language-model.ts`, `openai-responses-api.ts`, `convert-to-openai-responses-input.ts`, `tool/shell.ts` and tests (3cb1dae); `stream-text.ts` low-memory split (0ed59e2).

## Risks / Trade-offs

- Through the Gateway, an interrupted finish that arrives with an open block still fails the ProviderWire lifecycle as before; a registered package owns that protocol decision.
- OpenAI Responses and Bedrock Converse keep their recorded EOF deviations, now described against the target.
- Synthetic inputs prove mappings, not live acceptance; provider recordings are byte-identical.

## Migration Plan

Apply only the frozen record, regenerate expectations and UI generators, validate candidate source and then record verification. Roll back pins, lockfile, expectations and corrections together.
