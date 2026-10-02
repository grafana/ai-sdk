## ADDED Requirements

### Requirement: Opaque metadata at supported registered scopes
The Gateway SHALL preserve ordinary providerMetadata at every registered metadata position of its currently supported output families: unary result and text/reasoning/reasoning-file/source/function-call content; streaming text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result and finish. Both the registered TypeScript client and independent Go client SHALL retain unknown/future namespace objects and nested JSON semantics without IncludeRawChunks. The service SHALL NOT rename namespaces, select keys by inventory, relocate metadata to another event, or expand unsupported output unions. Response-metadata, stream-start, error and raw SHALL NOT acquire an unregistered providerMetadata field.

#### Scenario: All supported scopes retain opaque values
- **WHEN** supported unary content/results and stream parts carry unknown namespaces with nested arrays, objects, null, false, zero and empty strings
- **THEN** raw schema-valid server output and both clients SHALL retain those values at their original scopes without a raw-event option or native-key inventory

#### Scenario: Unsupported family is not activated
- **WHEN** an otherwise unsupported provider-executed tool or custom/generated-content variant supplies metadata
- **THEN** its existing explicit unsupported-family behavior SHALL remain in force rather than admitting the content because a metadata codec exists

### Requirement: Object shape and metadata presence
Metadata SHALL follow the registered Record<string, JSONObject> contract. Absent metadata SHALL remain absent and an explicitly supplied empty metadata object or namespace object SHALL remain present. Nested JSON null and empty scalar/collection values SHALL survive. Strict server encoding and independent client decoding SHALL reject present top-level null and namespace null/scalar/array values, malformed JSON and invalid original UTF-8 explicitly, not reinterpret them as absent or selectively omit them. Pinned TS permissive malformed-response parsing SHALL NOT be presented as proof of strict output shape.

#### Scenario: Absence and empty objects differ
- **WHEN** otherwise equivalent supported output supplies no metadata, an empty metadata object, or an object containing an empty unknown namespace
- **THEN** both clients SHALL preserve those three represented states in low-level results/events and affected assembled content

#### Scenario: Nested null is not namespace null
- **WHEN** an unknown namespace contains nested null alongside empty/false/zero values
- **THEN** those values SHALL survive
- **WHEN** the entire namespace value is null or a non-object
- **THEN** server mapping or independent Go decoding SHALL fail explicitly without selective omission

### Requirement: Aggregate bounded metadata processing
AGPL-owned service DTOs SHALL transport provider metadata directly through standard JSON encoding with nil-sensitive omission, and Apache client decoding SHALL remain independent of Gateway implementation imports. Server preflight SHALL account for namespace key bytes, original namespace JSON bytes including whitespace, collection cardinality and other content/event values using overflow-safe aggregate accounting before scanning, decoding or copying metadata. Preflight SHALL charge one budget unit per namespace alongside original namespace key/value bytes; JSON punctuation SHALL be bounded by the final encoded-size check rather than estimated in preflight. Unary result and all content scopes SHALL share one UnaryResponseBytes budget; each stream event SHALL share its complete StreamFrameBytes budget. Metadata validation/temporary allocation SHALL remain bounded by accepted original bytes/cardinality. Complete standard-encoded unary bytes and SSE bytes including data prefix and suffix SHALL fit their final configured limits before any success document/event is written. Existing client response/event/cumulative/count bounds SHALL apply independently. Existing usage.raw limits SHALL remain unchanged.

#### Scenario: Small objects collectively overflow
- **WHEN** multiple namespace/content metadata objects fit individually but their original bytes or cardinality collectively cannot fit the unary or event budget
- **THEN** preflight SHALL reject them before JSON parsing or mapped allocation rather than using a separate allowance for each metadata object

#### Scenario: Whitespace and escaping cannot evade limits
- **WHEN** whitespace-heavy original metadata exceeds preflight or valid metadata escaping pushes final unary/SSE bytes above the complete limit
- **THEN** only complete fitting documents/events SHALL be committed, without compacting input first to evade preflight

#### Scenario: Exact resource boundaries
- **WHEN** original-byte/cardinality or complete encoded responses/frames are below, at or one unit above their accepted bounds
- **THEN** focused tests SHALL prove in-limit acceptance and explicit over-limit failure without partial writes

#### Scenario: Invalid original UTF-8 is not repaired
- **WHEN** an in-limit namespace name or raw JSON key/value contains invalid UTF-8
- **THEN** validation SHALL fail before Go decoding/encoding can replace it
- **WHEN** a namespace object instead contains valid JSON lone or paired surrogate escapes
- **THEN** it SHALL remain valid under the same aggregate and final-byte limits

### Requirement: Metadata adaptation preserves commitment and ownership
Invalid or oversized unary metadata SHALL fail the whole response before HTTP 200 through the existing adaptation error. Invalid committed stream metadata SHALL prevent writing that event, cancel provider work, and use the existing bounded terminal error when the writer is usable. The service SHALL NOT silently omit metadata, return normalized-only success, read ahead for selection, restart fallback after a selected result/part, or introduce another cleanup owner. Part budgets, IDs, non-terminal provider errors, cancellation, deadlines, authoritative finish and bounded drain SHALL retain their delivered semantics.

#### Scenario: Selected result cannot replay after adaptation
- **WHEN** a selected direct/fallback unary result or committed stream has invalid or oversized metadata
- **THEN** the existing failure path SHALL apply and no later candidate SHALL be invoked to hide metadata loss or replay generation

#### Scenario: Finish remains authoritative
- **WHEN** an in-limit finish carrying metadata is written and a later provider part is available
- **THEN** finish and its metadata SHALL remain final, later output SHALL be suppressed and existing bounded cleanup SHALL occur

### Requirement: Actual two-client native continuation
Both clients' real first-output-derived unary and streaming history SHALL produce correct subsequent native requests for supported Anthropic/OpenAI/compatible continuation through direct and configured-fallback routes. Acceptance SHALL distinguish TS high-level unary/streaming, Go direct unary DoGenerate with a labeled independently tested consumer adaptation using only actual returned content/metadata and public ToResponseMessages, and Go high-level streaming assembly. Go GenerateText SHALL be counted as DoStream/streaming HTTP, not unary evidence. Tests SHALL verify actual mode selectors/flags and reuse output-derived history without injecting expected metadata, rewriting namespaces or inventing a public Go orchestration API. Scope witnesses SHALL include supported Anthropic tool-call caller and reasoning signature/redacted values, OpenAI text item/phase and final encrypted reasoning, and compatible providerName google tool-call thought signatures from unary output or the initial streaming tool-call delta. The compatible witness SHALL retain the native google namespace unchanged and produce extra_content.google.thought_signature in the second native request; arbitrary configured-name consumer and late-signature support SHALL NOT be inferred or fixed by metadata rewriting. Latest non-nullish object replacement SHALL be retained rather than deep merge; an explicit empty object SHALL replace earlier metadata. Sources SHALL remain response-only and families not represented in the native request SHALL NOT be echoed solely to claim continuation.

#### Scenario: Returned caller and continuation values reach second request
- **WHEN** either client's first call receives supported native metadata and reuses actual output-derived history through its declared high-level assembly or labeled Go unary consumer adaptation, with local tool execution consumer-owned
- **THEN** a fake native endpoint SHALL observe the required continuation values in the second request on direct and selected-fallback paths, with local execution remaining consumer-owned

#### Scenario: Replacement arrives late
- **WHEN** text/reasoning metadata is initially supplied and later omitted, replaced with a disjoint object, or explicitly cleared by an empty object
- **THEN** assembled history SHALL respectively retain, replace or clear the earlier object without recursive merge or test-injected providerOptions

### Requirement: Observer and authorization separation
Caller metadata SHALL be preserved independently from service-operator capture and consumer middleware configuration. Operator metadata-only telemetry SHALL continue to exclude native metadata/continuation payloads. Independently configured consumer middleware SHALL be able to observe supported returned metadata without enabling server capture. Concrete credential-source and tenant boundaries SHALL remain protected, while arbitrary token-shaped application strings SHALL NOT trigger generic censorship. The Gateway SHALL NOT synthesize credential/configuration/transport dumps into providerMetadata or use untrusted returned metadata to authorize accounts or routing.

#### Scenario: Response remains stable across observation policies
- **WHEN** an identical supported native response is processed with differing operator capture settings and independently enabled consumer observation
- **THEN** response metadata SHALL be unchanged, operator metadata-only exports SHALL omit payloads and the consumer SHALL observe the returned metadata under its own configuration

#### Scenario: Application token-shaped strings are not credentials
- **WHEN** ordinary metadata contains token-shaped or secret-looking application strings, including names resembling routing controls
- **THEN** those values SHALL remain opaque response data and SHALL NOT select accounts/destinations or alter authorization
- **AND** configured real authentication credentials and other requests' tenant state SHALL remain absent through their concrete source protections

### Requirement: Evidence provenance and licensing
Gateway codecs/schemas/contract/native-command test sources SHALL remain under the AGPL ai-gateway boundary; independent client and generic core/UI evidence SHALL remain Apache. Available authentic recorded/imported simple-text/tool/reasoning inputs SHALL replay against unchanged direct UI/native-request expectations where supported. Provider inputs SHALL NOT be invented, rewritten or normalized to hide metadata loss. Synthetic malformed/resource/transport witnesses SHALL remain focused tests. Any affected frontend wire behavior SHALL include schema-parsed cross-language assembly evidence. Remaining native-adapter gaps and #201 full-matrix coverage SHALL be reported separately from delivered metadata behavior.

#### Scenario: Authentic replay retains the direct contract
- **WHEN** an available authentic fixture exercises supported metadata through the Gateway
- **THEN** its direct expectations SHALL remain the comparison contract without modifying provider input or normalizing missing metadata away

#### Scenario: Synthetic cases remain focused tests
- **WHEN** a malformed namespace, transport fault or extreme resource boundary is locally constructed
- **THEN** it SHALL be tested outside recorded/upstream provider input directories and SHALL NOT support a live-provider parity claim
