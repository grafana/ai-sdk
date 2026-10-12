## Context

Fresh canonical main bb0cacce registers ai 7.0.127, Gateway 4.0.103, Provider 4.0.21 at eb77f09e3c06c28e860d92e0de941b143c2eecec. Exact `git show` of that commit in the local upstream repository is the reference, not that checkout's different HEAD. V4 call options use optional string name/description and JSONSchema7 object schema. Gateway getArgs forwards the supplied options; OpenAI Responses uses `name ?? 'response'`, so empty is meaningful. This is public-client/native adapter evidence, not a private Vercel service oracle.

Gateway request schema already accepts JSON response formats with opaque object schema, optional name/description. `providerwire/v4/request.go` only decodes type and rejects JSON. Grafana request encoding already transports schema but drops empty name. OpenAI and compatible default empty Go strings; output Object/Array/Choice/JSON producers use string names. PARITY classifies Gateway runtime as mixed with established mapped fallback, metadata, native options and independent capture; reuse these foundations.

## Goals / Non-Goals

Goals: native-equivalent structured request semantics through both clients in unary and stream, including configured fallback and existing response/error/capture paths. Preserve object schemas semantically, including numbers and unknown keywords. Non-goals: a schema validator in the Gateway, a new output response codec, automatic JSON repair, routing/BYOK, general diagnostic framework or complete #322 delivery, release/deployment, boolean root schema expansion, or changes copied from stopped worktrees.

## Decisions

1. Change ResponseFormat.Name to *string with omitempty. Nil means absent and pointer to empty means explicitly supplied. Migrate every compile-time producer/consumer, including output WithName("") presence; use an internal optional-name representation in output options rather than testing nonempty. Native OpenAI/compatible default only nil. Anthropic/Bedrock keep existing native behavior; audit all consumers. Description stays string and absence/empty normalize to empty. This source-breaking API adjustment is explicit; a second boolean field would duplicate state and obscure optional semantics.
2. Add schema RawMessage, name *string, description string to private wire DTO. Preserve raw JSON object bytes through Gateway mapping and response-schema numeric semantics through native serialization; never decode response-schema numbers through float64. Use number-preserving decoding in the scoped OpenAI, Anthropic and Bedrock response-schema consumers, retaining rejection of trailing JSON/malformed input and intentional native schema normalization. This corrects accidental rounding, not native schema support or validation policy. Accept omitted schema as JSON mode and any object schema allowed by existing opaque schema boundary. Reject null/boolean/array/scalar roots and malformed wire JSON before invocation; do not introduce Gateway meta-schema validation. A structurally valid but provider-invalid schema reaches native validation and its ordinary error boundary.
3. Reuse text generation/result/stream codecs: JSON text remains text, while high-level Output parses and validates client-side. No Gateway output reparsing, buffering or new SSE parts. Preserve warnings/metadata/usage/finish through current codecs.
4. Newly mapped structured output follows the existing mapped fallback admission. Candidates independently interpret matching native options and schema support; no capability intersection or text-only refusal. Existing eligibility, first-part commitment (including stream-start/error), cancellation and no replay after selected output remain authoritative. The original canonical-only error codec lacked scoped attribution. The authorized continuation adds compact request-owned execution metadata following current #322 carriers, without claiming the full #322 workstream. A context-scoped fallback observer is independent of the operator observer, and configured candidate descriptors come from catalog resolution. Unary direct calls are observed on actual completion; direct streams select on the first part, without another reader. Incomplete, mismatched, late or oversized histories are omitted. Gateway classification, retryability and selection remain unchanged.
5. Test with deterministic native fake HTTP and exact pinned client execution, explicitly labelled synthetic; no new recorded inputs without real recording provenance. Operator capture settings must not alter responses, while opt-in consumer capture proves its own request/output visibility.

## Test Matrix

| Layer | Required evidence |
| --- | --- |
| Root/output/native names | absent, empty, nonempty names; WithName("") versus no option; OpenAI/compatible default only absent; all changed modules compile |
| Both clients × unary/stream | no schema JSON mode and schema mode; absent/empty/nonempty names; absent/empty descriptions normalize; nested schema unknown keywords, Unicode, arrays, false/null values and integer above 2^53 preserved |
| Validation | malformed JSON, root schema null/boolean/array/scalar and wrong name/description types reject before invocation; invalid native schema/provider HTTP failure uses existing safe error behavior |
| Native boundaries | fake Anthropic plus OpenAI Responses/compatible requests show native schema/options and no Gateway invention; raw native OpenAI/Anthropic/Bedrock requests prove exact response-schema numeric values and malformed/trailing-input rejection; native warnings/errors and output usage/finish/metadata survive |
| High-level output | pinned TS generateText/streamText with Output and Go GenerateText/StreamText with output.Object parse a deterministic valid object; invalid final object remains client validation error |
| Configured fallback | both clients, unary/stream eligible primary failure then native secondary success; no schema/options loss; noneligible failure; all failed attribution/bounds; first-part commitment prevents replay; cancel cleanup |
| Observation | independent operator/consumer capture modes; one logical operation, structured output unchanged under capture disabled/enabled, warnings/errors visible |

Numeric witnesses compare the client-submitted JSON semantics: use raw HTTP/Go for 9007199254740993 and an exactly representable value or captured emitted JSON for TypeScript; do not claim JS represents arbitrary unsafe integers. Number-preserving Go decoding is a parity-preserving Go adaptation: it retains the JSON values that Go callers can submit, while TypeScript comparisons remain limited to the values emitted by its numeric representation. Native numeric rounding is an implementation bug, not an accepted deviation.

## Risks / Trade-offs

Source API change → update all literals/tests/docs and test modules independently. Provider-specific schema transformation → assert end-to-end numeric semantic preservation while allowing intentional registered native normalization of schema structure. Synthetic transports → explicitly retain live/provider-recording coverage limits. Time limit → preserve unfinished checklist and concrete failed commands, never claim complete proof.

## Migration Plan

Implement tests first, migrate name API and mappers, run focused tests then affected module and mandatory parity/integration checks. Document name pointer migration in API guidance. Published module versions and release tags are managed separately. Rollback is a source revert.

## Open Questions

No design blockers. Both registered clients now prove configured primary rejection, exhaustion and secondary success through actual service construction; this closes the scoped #114 attribution gap, not the independent #322 delivery.


## Scoped execution metadata

`providerMetadata.gateway.execution` contains `requestedModelId`, `canonicalModelId` and ordered `attempts`. Each actual attempt carries configured `providerInstance`, `provider`, `modelId`, `outcome` and optional `nativeError` (`message`, `type`, string-or-number `code`, `statusCode`). There are no selected indices, fallback-intent flags, raw bodies, headers or aggregate cause prose. Native message/type/code are decoded from the candidate-local API error payload; wrapper `APICallError.Message` is deliberately not copied because native SDK wrappers can include URLs and complete response bodies. A missing/malformed payload leaves only available status.

Unary/setup failures use top-level `providerMetadata`; stream finish uses ordinary provider metadata. Committed native errors use `error.data.providerMetadata` and event-local `error.data.nativeError`; local adaptation failures retain only the selected execution metadata. Go reads API error `Data`, while pinned TypeScript normalized Gateway errors retain the original `APICallError` in `cause.data`. This is an additive service contract using upstream-supported extension carriers, not private Vercel service parity.

Enrichment first validates the original success/finish document and never repairs invalid native metadata. Existing complete response/frame limits decide fit; HTTP error enrichment also respects the published default Go client 64 KiB error-envelope limit. Overflow retains the original document, not a truncated diagnostic. Source payload parsing is bounded by the existing response/frame limit. When enrichment fits, an existing native gateway namespace is preserved intact under `gateway.nativeMetadata`. On omission native metadata remains unchanged; opaque native `gateway.execution` alone is not trusted provenance or proof that no attempts occurred. Provider-origin messages may contain caller/provider-echoed text: this is not an operator redaction or credential-filtering protocol.
