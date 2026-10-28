## MODIFIED Requirements

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
