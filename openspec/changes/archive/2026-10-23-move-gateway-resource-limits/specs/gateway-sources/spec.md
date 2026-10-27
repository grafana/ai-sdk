## ADDED Requirements

### Requirement: Source metadata validity and explicit failure
Each source metadata namespace SHALL be validated on its original bytes as a UTF-8 name with a UTF-8 JSON object value. Malformed JSON, invalid UTF-8 and null or non-object namespaces SHALL fail the response or event explicitly rather than selectively omit fields. Source strings and metadata SHALL NOT be budgeted by size or cardinality.

#### Scenario: Large source metadata
- **WHEN** whitespace-heavy or numerous source metadata namespaces exceed the former unary or frame budget
- **THEN** the source SHALL be returned with all namespaces unchanged

#### Scenario: Invalid source metadata
- **WHEN** a source metadata namespace is malformed, invalid UTF-8, null or not an object
- **THEN** the response or event SHALL fail through the existing safe error path rather than omit the namespace

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
- **WHEN** a valid source's strings and metadata exceed the former aggregate preflight or complete frame limit
- **THEN** the complete source SHALL be emitted in order without a size failure

#### Scenario: Source is invalid
- **WHEN** a source string contains invalid UTF-8 or the source has an unknown discriminator
- **THEN** the existing unary or streaming safe error path SHALL apply without partial source output

### Requirement: No standalone source ID byte cap
No source-specific or containing-document size cap SHALL reject a valid source.

#### Scenario: No standalone source ID byte cap
- **WHEN** a valid source ID exceeds 1024 bytes, or a source's strings exceed the former unary or frame budget
- **THEN** it SHALL remain accepted without a size cap

### Requirement: Atomic source lifecycle and observation
Sources SHALL preserve relative output order and SHALL NOT close active text or tool blocks. Accepted sources SHALL mark output as started, disallow subsequent response metadata and reset idle activity through the existing loop. Provider error parts SHALL remain non-terminal; authoritative finish SHALL suppress later sources. Sources SHALL commit fallback candidates without replay. Existing cancellation, timeouts and cleanup ownership SHALL remain effective.

#### Scenario: Source interleaving
- **WHEN** a source appears between text-start and text-end
- **THEN** the text block SHALL remain active and subsequent text deltas SHALL be accepted
- **AND** a later response-metadata part SHALL fail safely

#### Scenario: Source-only observation
- **WHEN** a source is the only payload before finish
- **THEN** reusable observers SHALL record first-output timing without recording its content in metadata-only output

## REMOVED Requirements

### Requirement: Aggregate source envelope and mapping-work bounds
**Reason**: Source strings and metadata are no longer budgeted against `UnaryResponseBytes`, `StreamParts` or the frame limit.
**Migration**: Validity remains under "Source validation and inactive-field boundaries" and "Source metadata validity and explicit failure". Size is bounded by gateway-resource-policy.

### Requirement: Original source metadata preflight and explicit failure
**Reason**: Source metadata is no longer preflighted by size.
**Migration**: Replaced by "Source metadata validity and explicit failure".
