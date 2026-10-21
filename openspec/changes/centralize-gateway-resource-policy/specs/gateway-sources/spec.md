# Spec Delta

## MODIFIED Requirements

### Requirement: Bounded response-local source identity
The Gateway SHALL preserve each native source ID exactly in its response rather than assigning a sequential, hashed or generated replacement. Source ID SHALL be a required valid-UTF-8 string, including an empty value permitted by the registered contract. Repeated IDs and equal IDs across URL/document variants SHALL remain unchanged and SHALL NOT cause deduplication or collision repair. Identity-map state and its 1024-byte key cap SHALL be removed; source identity is not public route identity.

#### Scenario: Repeated source
- **WHEN** the same URL ID appears twice and a document has the same ID
- **THEN** all three SHALL keep their native ID and original order without deduplication or variant-specific renaming

#### Scenario: Empty or long native ID
- **WHEN** the source ID is empty or exceeds the old 1024-byte cap while the complete document remains valid
- **THEN** it SHALL be emitted unchanged as a required string

#### Scenario: Source crosses complete bounds
- **WHEN** a source ID, URL, title, media type or filename contains invalid UTF-8, or source strings exceed the former aggregate or frame budget
- **THEN** invalid UTF-8 SHALL use the existing bounded unary or streaming safe error path without partial source output, while valid large sources SHALL be emitted complete

### Requirement: No standalone source ID byte cap
No source-specific ID, string or metadata byte cap SHALL reject a valid source.

#### Scenario: No standalone source ID byte cap
- **WHEN** a valid source ID exceeds 1024 bytes
- **THEN** it SHALL remain accepted without a source-specific identity cap

### Requirement: Atomic source lifecycle and observation
Sources SHALL preserve relative output order and SHALL NOT close active text or tool blocks. Accepted sources SHALL mark output as started, disallow subsequent response metadata and reset idle activity through the existing loop. Provider error parts SHALL remain non-terminal; authoritative finish SHALL suppress later sources. Sources SHALL commit fallback candidates without replay. Existing cancellation, timeouts and cleanup ownership SHALL remain effective.

#### Scenario: Source interleaving
- **WHEN** a source appears between text-start and text-end
- **THEN** the text block SHALL remain active and subsequent text deltas SHALL be accepted
- **AND** a later response-metadata part SHALL fail safely

#### Scenario: Source-only observation
- **WHEN** a source is the only payload before finish
- **THEN** reusable observers SHALL record first-output timing without recording its content in metadata-only output

## ADDED Requirements

### Requirement: Source metadata validity and explicit failure
Source metadata SHALL follow gateway-provider-metadata validity rules. Malformed JSON, invalid UTF-8 or null/non-object namespaces SHALL fail the response or event explicitly rather than selectively omitting fields. Source strings and metadata SHALL NOT be counted against an adapter byte or cardinality budget.

#### Scenario: Malformed source metadata
- **WHEN** source metadata has a malformed or non-object namespace
- **THEN** the unary response SHALL fail before HTTP 200 or the committed event SHALL use the terminal adaptation path, without omitting fields

#### Scenario: Many large sources
- **WHEN** a valid result contains sources and metadata whose aggregate size exceeds the former unary or frame budget
- **THEN** all sources SHALL be emitted in order with their metadata

## REMOVED Requirements

### Requirement: Aggregate source envelope and mapping-work bounds
**Reason**: Unary preflight, `StreamParts` and complete-frame source bounds are removed by gateway-resource-policy.
**Migration**: Source volume is bounded by the shared outbound response bound and execution deadlines.

### Requirement: Original source metadata preflight and explicit failure
**Reason**: Source metadata preflight against aggregate budgets is removed; explicit failure for invalid metadata is retained.
**Migration**: Use "Source metadata validity and explicit failure".
