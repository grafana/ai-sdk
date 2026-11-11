# gateway-provider-metadata Specification

## Purpose
Define bounded opaque Gateway provider-metadata transport, independent client consumption and output-derived continuation evidence.

## Requirements

### Requirement: Opaque metadata at supported registered scopes
The Gateway SHALL preserve ordinary providerMetadata at every registered metadata position of its currently supported output families: unary result and text/reasoning/reasoning-file/source/function-call/custom content; streaming text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result, custom and finish.

#### Scenario: All supported scopes retain opaque values
- **WHEN** supported unary content/results and stream parts carry unknown namespaces with nested arrays, objects, null, false, zero and empty strings
- **THEN** raw schema-valid server output and both clients SHALL retain those values at their original scopes without a raw-event option or native-key inventory

#### Scenario: Custom metadata presence
- **WHEN** unary or streamed custom content supplies no metadata, an empty metadata object, or an unknown namespace with nested values
- **THEN** raw server output and both clients SHALL respectively omit metadata, retain `{}`, or retain the namespace unchanged

#### Scenario: Unsupported family is not activated
- **WHEN** an otherwise unsupported provider-executed tool or generated-content variant supplies metadata
- **THEN** its existing explicit unsupported-family behavior SHALL remain in force rather than admitting the content because a metadata codec exists

### Requirement: Two-client opaque metadata preservation without raw chunks

Both the registered TypeScript client and independent Go client SHALL retain unknown/future namespace objects and nested JSON semantics without IncludeRawChunks.

#### Scenario: Two-client opaque metadata preservation without raw chunks policy
- **WHEN** the runtime adapts native metadata and considers optional enrichment
- **THEN** Except for optional result/finish gateway.execution enrichment and fitting native gateway relocation at the runtime feature boundary, the service SHALL NOT rename namespaces, select keys by inventory, relocate metadata to another event, or expand unsupported output unions. Response-metadata, stream-start, error and raw SHALL NOT acquire an unregistered providerMetadata field.

#### Scenario: Two-client opaque metadata preservation without raw chunks
- **WHEN** an ordinary supported event includes an unknown object-valued namespace and no raw-chunk option
- **THEN** both clients SHALL retain it at its original scope without inventory filtering or unsupported/unregistered event fields; optional result/finish gateway.execution enrichment and fitting native gateway relocation SHALL remain separately governed

### Requirement: Object shape and metadata presence
Metadata SHALL follow the registered Record<string, JSONObject> contract. Absent metadata SHALL remain absent and an explicitly supplied empty metadata object or namespace object SHALL remain present. Nested JSON null and empty scalar/collection values SHALL survive.

#### Scenario: Absence and empty objects differ
- **WHEN** otherwise equivalent supported output supplies no metadata, an empty metadata object, or an object containing an empty unknown namespace
- **THEN** both clients SHALL preserve those three represented states in low-level results/events and affected assembled content

#### Scenario: Nested null is not namespace null
- **WHEN** an unknown namespace contains nested null alongside empty/false/zero values
- **THEN** those values SHALL survive
- **WHEN** the entire namespace value is null or a non-object
- **THEN** server mapping or independent Go decoding SHALL fail explicitly without selective omission

### Requirement: Strict malformed metadata rejection evidence
Strict server encoding and independent client decoding SHALL reject present top-level null and namespace null/scalar/array values, malformed JSON and invalid original UTF-8 explicitly, not reinterpret them as absent or selectively omit them. Pinned TS permissive malformed-response parsing SHALL NOT be presented as proof of strict output shape.

#### Scenario: Strict malformed metadata rejection evidence
- **WHEN** metadata is present as top-level null, a namespace array, malformed JSON or invalid original UTF-8
- **THEN** strict server/Go decoding SHALL reject it explicitly without omission; permissive TS parsing SHALL NOT establish strict output-shape proof

### Requirement: Metadata codec licensing and nil-sensitive JSON transport
AGPL-owned service DTOs SHALL transport provider metadata directly through standard JSON encoding with nil-sensitive omission, and Apache client decoding SHALL remain independent of Gateway implementation imports.

#### Scenario: Metadata codec licensing and nil-sensitive JSON transport
- **WHEN** service encoding receives absent metadata and an explicit empty object
- **THEN** standard service JSON SHALL retain nil-sensitive presence and Apache decoding SHALL remain independent of AGPL implementation imports

### Requirement: Complete encoded metadata envelopes before commitment
Complete standard-encoded unary bytes and SSE bytes including data prefix and suffix SHALL be fully encoded, with their metadata, before any success document/event is written. Escaping expansion SHALL NOT cause a valid document or event to fail.

#### Scenario: Complete encoded metadata envelopes before commitment
- **WHEN** valid raw metadata expands through escaping beyond the former unary or frame limit
- **THEN** the complete document or event SHALL be written with the metadata, and no partial bytes SHALL be written if encoding fails

### Requirement: Metadata adaptation preserves commitment and ownership

Invalid original unary metadata SHALL fail the whole response before HTTP 200 through the existing adaptation error.

#### Scenario: Metadata adaptation preserves commitment and ownership policy
- **WHEN** the runtime adapts native metadata and considers optional enrichment
- **THEN** Invalid committed stream metadata SHALL prevent writing that event, cancel provider work, and use the existing bounded terminal error when the writer is usable. Optional Gateway overview enrichment SHALL be encoded together with the response/event. Native namespace relocation SHALL NOT repair primary-invalid output. The service SHALL NOT silently omit native metadata, return normalized-only success, read ahead for selection, restart fallback after a selected result/part, or introduce another cleanup owner.

#### Scenario: Selected result cannot replay after adaptation
- **WHEN** a selected direct/fallback unary result or committed stream has invalid metadata
- **THEN** the existing failure path SHALL apply and no later candidate SHALL be invoked to hide metadata loss or replay generation

#### Scenario: Finish remains authoritative
- **WHEN** a valid finish carrying metadata is written and a later provider part is available
- **THEN** the finish and its metadata SHALL remain written unchanged in their position; a later response-metadata, raw or provider error part SHALL follow the sequential response segment rules, any other later part SHALL fail safely without rewriting the finish, and existing bounded cleanup SHALL occur

#### Scenario: Each finish keeps its own metadata
- **WHEN** two response segments end with finish parts carrying different metadata namespaces, including an explicit empty object
- **THEN** each finish SHALL carry exactly its own metadata and presence, with no merge or carry-over between them

### Requirement: Metadata processing retains delivered stream lifecycle
IDs, non-terminal provider errors, cancellation, deadlines, the sequential response segment rules, per-segment finish authority and bounded drain SHALL retain their delivered semantics.

#### Scenario: Metadata processing retains delivered stream lifecycle
- **WHEN** a valid metadata-bearing finish is followed by a provider error or later content
- **THEN** the finish SHALL remain written with its metadata, the provider error SHALL be written in place, content without intervening response-metadata SHALL fail safely, and existing IDs, cancellation/deadlines and bounded drain SHALL apply

### Requirement: Actual two-client native continuation
Both clients' real first-output-derived unary and streaming history SHALL produce correct subsequent native requests for supported Anthropic/OpenAI/compatible continuation through direct and configured-fallback routes. Acceptance SHALL distinguish TS high-level unary/streaming, Go direct unary DoGenerate with a labeled independently tested consumer adaptation using only actual returned content/metadata and public ToResponseMessages, and Go high-level streaming assembly.

#### Scenario: Returned caller and continuation values reach second request
- **WHEN** either client's first call receives supported native metadata and reuses actual output-derived history through its declared high-level assembly or labeled Go unary consumer adaptation, with local tool execution consumer-owned
- **THEN** a fake native endpoint SHALL observe the required continuation values in the second request on direct and selected-fallback paths, with local execution remaining consumer-owned

#### Scenario: Replacement arrives late
- **WHEN** text/reasoning metadata is initially supplied and later omitted, replaced with a disjoint object, or explicitly cleared by an empty object
- **THEN** assembled history SHALL respectively retain, replace or clear the earlier object without recursive merge or test-injected providerOptions

### Requirement: Actual invocation mode and unmodified output-derived history
Go GenerateText SHALL be counted as DoStream/streaming HTTP, not unary evidence. Tests SHALL verify actual mode selectors/flags and reuse output-derived history without injecting expected metadata, rewriting namespaces or inventing a public Go orchestration API.

#### Scenario: Actual invocation mode and unmodified output-derived history
- **WHEN** Go GenerateText is used for a continuation witness
- **THEN** it SHALL count as streaming HTTP evidence and tests SHALL verify selectors/flags and reuse actual output history without injection, namespace rewriting or an invented Go API

### Requirement: Native continuation witness value families
Scope witnesses SHALL include supported Anthropic tool-call caller and reasoning signature/redacted values, OpenAI text item/phase and final encrypted reasoning, and compatible providerName google tool-call thought signatures from unary output or the initial streaming tool-call delta.

#### Scenario: Native continuation witness value families
- **WHEN** first outputs include Anthropic caller/signature/redacted values, OpenAI text item/phase/encrypted reasoning or google tool-call thought signatures
- **THEN** continuation tests SHALL cover those supported native values, with compatible google tool-call signatures sourced from unary output or the initial streaming tool-call delta

### Requirement: Compatible google continuation namespace and timing boundary
The compatible witness SHALL retain the native google namespace unchanged and produce extra_content.google.thought_signature in the second native request; arbitrary configured-name consumer and late-signature support SHALL NOT be inferred or fixed by metadata rewriting.

#### Scenario: Compatible google continuation namespace and timing boundary
- **WHEN** a configured providerName google returns a tool-call thought signature in unary output or the initial streaming delta
- **THEN** the second native request SHALL carry extra_content.google.thought_signature under the unchanged google namespace without inferring arbitrary-name or late-signature support

### Requirement: Continuation metadata replacement and response-only families
Latest non-nullish object replacement SHALL be retained rather than deep merge; an explicit empty object SHALL replace earlier metadata. Sources SHALL remain response-only and families not represented in the native request SHALL NOT be echoed solely to claim continuation.

#### Scenario: Continuation metadata replacement and response-only families
- **WHEN** later metadata is an empty object and the first response also contains sources
- **THEN** the empty object SHALL replace prior metadata without deep merge and sources/unrepresented families SHALL NOT be echoed to fabricate continuation

### Requirement: Observer and authorization separation
Caller metadata SHALL be preserved independently from service-operator capture and consumer middleware configuration. Operator metadata-only telemetry SHALL continue to exclude native metadata/continuation payloads. Independently configured consumer middleware SHALL be able to observe supported returned metadata without enabling server capture.

#### Scenario: Response remains stable across observation policies
- **WHEN** an identical supported native response is processed with differing operator capture settings and independently enabled consumer observation
- **THEN** response metadata SHALL be unchanged, operator metadata-only exports SHALL omit payloads and the consumer SHALL observe the returned metadata under its own configuration

#### Scenario: Application token-shaped strings are not credentials
- **WHEN** ordinary metadata contains token-shaped or secret-looking application strings, including names resembling routing controls
- **THEN** those values SHALL remain opaque response data and SHALL NOT select accounts/destinations or alter authorization
- **AND** configured real authentication credentials and other requests' tenant state SHALL remain absent through their concrete source protections

### Requirement: Metadata credential protections without generic censorship
Concrete credential-source and tenant boundaries SHALL remain protected, while arbitrary token-shaped application strings SHALL NOT trigger generic censorship. The Gateway SHALL NOT synthesize credential/configuration/transport dumps into providerMetadata or use untrusted returned metadata to authorize accounts or routing.

#### Scenario: Metadata credential protections without generic censorship
- **WHEN** ordinary returned metadata contains token-shaped application strings and routing-like names
- **THEN** it SHALL remain opaque without authorization/routing effects or synthesized credential/configuration/transport dumps, while concrete credentials and tenant boundaries remain protected

### Requirement: Evidence provenance and licensing
Gateway codecs/schemas/contract/native-command test sources SHALL remain under the AGPL ai-gateway boundary; independent client and generic core/UI evidence SHALL remain Apache. Available authentic recorded/imported simple-text/tool/reasoning inputs SHALL replay against unchanged direct UI/native-request expectations where supported. Provider inputs SHALL NOT be invented, rewritten or normalized to hide metadata loss.

#### Scenario: Authentic replay retains the direct contract
- **WHEN** an available authentic fixture exercises supported metadata through the Gateway
- **THEN** its direct expectations SHALL remain the comparison contract without modifying provider input or normalizing missing metadata away

#### Scenario: Synthetic cases remain focused tests
- **WHEN** a malformed namespace, transport fault or large output is locally constructed
- **THEN** it SHALL be tested outside recorded/upstream provider input directories and SHALL NOT support a live-provider parity claim

### Requirement: Metadata frontend evidence and remaining native coverage gaps
Any affected frontend wire behavior SHALL include schema-parsed cross-language assembly evidence. Remaining native-adapter gaps and #201 full-matrix coverage SHALL be reported separately from delivered metadata behavior.

#### Scenario: Metadata frontend evidence and remaining native coverage gaps
- **WHEN** metadata behavior affects frontend assembly but a native adapter or #201 matrix case remains uncovered
- **THEN** schema-parsed cross-language evidence SHALL cover affected behavior and remaining gaps SHALL be reported separately

### Requirement: Metadata validity on original bytes
The server SHALL validate each supplied namespace name as UTF-8 and each namespace value as a UTF-8 JSON object on its original bytes, before Go decoding or encoding could replace invalid sequences. It SHALL NOT budget metadata by namespace count, key bytes or value bytes, alone or together with other content.

#### Scenario: Many or large namespaces
- **WHEN** a unary result or a stream event carries many namespaces or whitespace-heavy objects whose original bytes exceed the former unary or frame budget
- **THEN** every namespace SHALL be returned unchanged in its registered scope

#### Scenario: Invalid original UTF-8 is not repaired
- **WHEN** a namespace name or raw JSON key/value contains invalid UTF-8
- **THEN** validation SHALL fail before Go decoding/encoding can replace it
- **WHEN** a namespace object instead contains valid JSON lone or paired surrogate escapes
- **THEN** it SHALL remain valid and its escapes SHALL be preserved
