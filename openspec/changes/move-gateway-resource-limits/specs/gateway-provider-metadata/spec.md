## ADDED Requirements

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

## MODIFIED Requirements

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
- **THEN** finish and its metadata SHALL remain final, later output SHALL be suppressed and existing bounded cleanup SHALL occur

### Requirement: Metadata processing retains delivered stream lifecycle
IDs, non-terminal provider errors, cancellation, deadlines, authoritative finish and bounded drain SHALL retain their delivered semantics.

#### Scenario: Metadata processing retains delivered stream lifecycle
- **WHEN** a valid metadata-bearing finish is followed by a provider error or later content
- **THEN** finish SHALL remain authoritative with existing IDs, non-terminal error behavior, cancellation/deadlines and bounded drain

### Requirement: Evidence provenance and licensing
Gateway codecs/schemas/contract/native-command test sources SHALL remain under the AGPL ai-gateway boundary; independent client and generic core/UI evidence SHALL remain Apache. Available authentic recorded/imported simple-text/tool/reasoning inputs SHALL replay against unchanged direct UI/native-request expectations where supported. Provider inputs SHALL NOT be invented, rewritten or normalized to hide metadata loss. Synthetic malformed/large-output/transport witnesses SHALL remain focused tests.

#### Scenario: Authentic replay retains the direct contract
- **WHEN** an available authentic fixture exercises supported metadata through the Gateway
- **THEN** its direct expectations SHALL remain the comparison contract without modifying provider input or normalizing missing metadata away

#### Scenario: Synthetic cases remain focused tests
- **WHEN** a malformed namespace, transport fault or large output is locally constructed
- **THEN** it SHALL be tested outside recorded/upstream provider input directories and SHALL NOT support a live-provider parity claim

## REMOVED Requirements

### Requirement: Aggregate bounded metadata processing
**Reason**: Metadata is no longer preflighted against a unary or event budget.
**Migration**: UTF-8 and object-shape validation moves to "Metadata validity on original bytes". Size is bounded by gateway-resource-policy.

### Requirement: Shared metadata budgets and bounded temporary work
**Reason**: The shared `UnaryResponseBytes` and `StreamFrameBytes` budgets are removed, and the client limits are removed by #479.
**Migration**: None for callers. Large metadata now succeeds.
