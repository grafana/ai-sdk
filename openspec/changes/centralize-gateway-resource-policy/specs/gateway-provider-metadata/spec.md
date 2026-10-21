# Spec Delta

## MODIFIED Requirements

### Requirement: Two-client opaque metadata preservation without raw chunks

Both the registered TypeScript client and independent Go client SHALL retain unknown/future namespace objects and nested JSON semantics without IncludeRawChunks.

#### Scenario: Two-client opaque metadata preservation without raw chunks policy
- **WHEN** the runtime adapts native metadata and considers optional enrichment
- **THEN** Except for optional result/finish gateway.execution enrichment and native gateway relocation at the runtime feature boundary, the service SHALL NOT rename namespaces, select keys by inventory, relocate metadata to another event, or expand unsupported output unions. Response-metadata, stream-start, error and raw SHALL NOT acquire an unregistered providerMetadata field.

#### Scenario: Two-client opaque metadata preservation without raw chunks
- **WHEN** an ordinary supported event includes an unknown object-valued namespace and no raw-chunk option
- **THEN** both clients SHALL retain it at its original scope without inventory filtering or unsupported/unregistered event fields; optional result/finish gateway.execution enrichment and native gateway relocation SHALL remain separately governed

### Requirement: Metadata adaptation preserves commitment and ownership

Invalid original unary metadata SHALL fail the whole response before HTTP 200 through the existing adaptation error.

#### Scenario: Metadata adaptation preserves commitment and ownership policy
- **WHEN** the runtime adapts native metadata and considers optional enrichment
- **THEN** Invalid committed stream metadata SHALL prevent writing that event, cancel provider work, and use the existing bounded terminal error when the writer is usable. Optional Gateway overview enrichment SHALL be encoded together with the original response/event; only an encoding failure of the optional data SHALL preserve the original response/event. Native namespace relocation SHALL NOT repair primary-invalid output. The service SHALL NOT silently omit native metadata, return normalized-only success, read ahead for selection, restart fallback after a selected result/part, or introduce another cleanup owner.

#### Scenario: Selected result cannot replay after adaptation
- **WHEN** a selected direct/fallback unary result or committed stream has invalid metadata
- **THEN** the existing failure path SHALL apply and no later candidate SHALL be invoked to hide metadata loss or replay generation

#### Scenario: Finish remains authoritative
- **WHEN** a finish carrying metadata is written and a later provider part is available
- **THEN** finish and its metadata SHALL remain final, later output SHALL be suppressed and existing bounded cleanup SHALL occur

### Requirement: Metadata processing retains delivered stream lifecycle
IDs, non-terminal provider errors, cancellation, deadlines, authoritative finish and bounded drain SHALL retain their delivered semantics.

#### Scenario: Metadata processing retains delivered stream lifecycle
- **WHEN** a metadata-bearing finish is followed by a provider error or later content
- **THEN** finish SHALL remain authoritative with existing IDs, non-terminal error behavior, cancellation/deadlines and bounded drain

## ADDED Requirements

### Requirement: Metadata validity without adapter budgets
The server SHALL validate every supplied metadata namespace as a valid UTF-8 name mapping to one valid UTF-8 JSON object before encoding. It SHALL NOT account namespace bytes, whitespace or cardinality against a unary or event budget. Metadata size SHALL be bounded only by the shared resource policy, and both clients SHALL accept every metadata document the server emits under their default limits.

#### Scenario: Many small namespaces
- **WHEN** many individually small metadata objects across content parts and the result exceed the former unary budget in aggregate
- **THEN** the complete result SHALL be returned with every namespace at its original scope

#### Scenario: Invalid original UTF-8 is not repaired
- **WHEN** a namespace name or raw JSON key/value contains invalid UTF-8
- **THEN** validation SHALL fail before Go decoding/encoding can replace it
- **WHEN** a namespace object instead contains valid JSON lone or paired surrogate escapes
- **THEN** it SHALL remain valid

#### Scenario: Whitespace-heavy metadata
- **WHEN** valid metadata contains substantial insignificant whitespace
- **THEN** it SHALL be accepted and encoded through standard JSON without a whitespace-inclusive size failure

## REMOVED Requirements

### Requirement: Aggregate bounded metadata processing
**Reason**: Overflow-safe aggregate preflight of namespace bytes and cardinality is adapter-local budget machinery removed by gateway-resource-policy.
**Migration**: Use "Metadata validity without adapter budgets"; resource bounds come from the shared route request-body and outbound limits.

### Requirement: Shared metadata budgets and bounded temporary work
**Reason**: `UnaryResponseBytes` and `StreamFrameBytes` are removed, and client per-event, count and raw-usage caps are removed with them.
**Migration**: Server output follows gateway-resource-policy; client reads follow the grafana-gateway-client per-response bound.

### Requirement: Complete encoded metadata envelopes before commitment
**Reason**: Complete-envelope byte limits are response-writer budgets that cannot prevent allocation.
**Migration**: Documents and events are encoded once and written only after encoding succeeds, under the unary and streaming runtime specs.
