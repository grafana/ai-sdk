# gateway-sources Specification

## Purpose
Define bounded native Gateway source projection, independent client decoding, and separate lifecycle and metadata-only observation contracts for URL and document citations.

## Requirements

### Requirement: Registered source output
The Gateway SHALL emit atomic flat source parts in unary content and streams. URL sources SHALL contain type, sourceType, id and url; nonempty title MAY be included. Document sources SHALL contain type, sourceType, id, mediaType and title even when title is empty; nonempty filename MAY be included. Optional empty URL titles and document filenames SHALL be omitted as the documented Go absent/empty normalization.

#### Scenario: Title compatibility
- **WHEN** a unary document contains Title "current" and Text "legacy"
- **THEN** its wire title SHALL be "current"
- **AND** Title absent/empty with Text "legacy" SHALL retain "legacy"

#### Scenario: Required empty title
- **WHEN** a document has no title or legacy text
- **THEN** its wire title SHALL be the empty string
- **AND** optional empty filename SHALL be absent

#### Scenario: Native file-path display
- **WHEN** an OpenAI or Azure file_path source uses the native file ID as title/filename
- **THEN** both display values SHALL survive without Document substitution or filename removal
- **AND** native source identity SHALL remain the adapter-provided ID, not be derived from file metadata

### Requirement: Native source display and unary legacy title precedence
Nonempty unary Title SHALL take precedence over legacy Text; empty Title SHALL fall back to Text because the string API cannot distinguish absence and explicit empty Title. Native title/filename values SHALL remain unchanged, including file_path display.

#### Scenario: Native source display and unary legacy title precedence
- **WHEN** a unary source supplies both Title and legacy Text, or native file_path display
- **THEN** nonempty Title SHALL win, empty Title SHALL fall back to Text and native title/filename values SHALL remain unchanged

### Requirement: Source validation and inactive-field boundaries
Unknown discriminators, nil stream sources and invalid represented UTF-8 SHALL fail through the existing safe error. Inactive fields SHALL NOT be serialized. URLs SHALL NOT be fetched or canonicalized.

#### Scenario: Source validation and inactive-field boundaries
- **WHEN** a stream source is nil, has an unknown discriminator or contains invalid represented UTF-8
- **THEN** the existing safe error SHALL apply without inactive-field serialization, URL fetching or canonicalization

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

### Requirement: Opaque public source metadata
Source providerMetadata SHALL preserve all supplied native object-valued namespaces and nested JSON under gateway-provider-metadata, with no synthetic citation namespace, numeric-only projection, recognized-namespace byte cap or key inventory. Absence SHALL remain absent and an explicit empty object SHALL remain present.

#### Scenario: Native file path
- **WHEN** file_path supplies native source ID, title, filename and index
- **THEN** native identity/title/filename and opaque metadata SHALL survive unchanged, including index in its original namespace

#### Scenario: Malformed unknown namespace fails explicitly
- **WHEN** a source has a previously unknown namespace with malformed or non-object JSON
- **THEN** the unary response SHALL fail before HTTP success or the committed stream SHALL reject the event through its existing terminal adaptation path, without numeric-only or metadata-free success

### Requirement: Source metadata display and protection separation
Useful native IDs, cited text and unknown fields inside ordinary provider metadata SHALL NOT be discarded merely because operator capture excludes them. Concrete credential and tenant-source protections SHALL remain independent from ordinary metadata transport. Recognized file_path metadata SHALL NOT alter native source identity, title or filename.

#### Scenario: Source metadata display and protection separation
- **WHEN** file_path metadata carries native IDs/cited text/unknown fields while operator capture excludes payloads
- **THEN** ordinary metadata SHALL survive without changing identity/title/filename or discarding fields, with concrete credential/tenant-source protections remaining independent

### Requirement: Atomic source lifecycle and observation
Sources SHALL preserve relative output order and SHALL NOT close active text or tool blocks. A source SHALL NOT prevent a later response-metadata part once no block is open, and SHALL reset idle activity through the existing loop. Provider error parts SHALL remain non-terminal; a source after a finish without intervening response-metadata SHALL fail safely. Sources SHALL commit fallback candidates without replay. Existing cancellation, timeouts and cleanup ownership SHALL remain effective.

#### Scenario: Source interleaving
- **WHEN** a source appears between text-start and text-end
- **THEN** the text block SHALL remain active and subsequent text deltas SHALL be accepted
- **AND** a response-metadata part before that text-end SHALL fail safely

#### Scenario: Source-only observation
- **WHEN** a source is the only payload before finish
- **THEN** reusable observers SHALL record first-output timing without recording its content in metadata-only output

#### Scenario: Metadata after a source
- **WHEN** response-metadata follows a source with no block open
- **THEN** the metadata SHALL be written and open a new response segment

#### Scenario: Source after a finish
- **WHEN** a source follows a valid finish without intervening response-metadata
- **THEN** the source SHALL NOT be written, the finish SHALL remain written and one synthetic terminal internal error SHALL follow it

### Requirement: Source observers preserve payload without unsupported recordings
Reusable logging, Prometheus and Agent Observability SHALL treat source as payload-bearing first output and pass it through unchanged. Gateway metadata-only telemetry SHALL omit source IDs, URLs, titles, filenames and metadata. Agent Observability SHALL NOT manufacture an unsupported source recording representation.

#### Scenario: Source observers preserve payload without unsupported recordings
- **WHEN** a source is the first and only payload before finish
- **THEN** reusable logging/Prometheus/Agent Observability SHALL pass it unchanged and mark first output, while Gateway metadata-only output omits source payloads and no unsupported recording representation is manufactured

### Requirement: Independent consumption and evidence
The Apache Go client SHALL decode both registered variants without Gateway implementation imports, preserve native ID/display/order and current object-valued metadata, require document title presence while accepting an empty title, and normalize optional strings. Its bounded HTTP/SSE consumption SHALL apply. Exact-pinned Vercel/Go differential tests SHALL exercise unary/stream sources through the handler and authenticated command.

#### Scenario: Both clients
- **WHEN** both clients consume bounded URL/document output
- **THEN** native IDs/display/order SHALL agree after declared optional-empty normalization
- **AND** missing document title SHALL fail on the strict Go path while explicit empty title SHALL succeed

#### Scenario: Frontend assembly
- **WHEN** schema-parsed UI chunks contain repeated native IDs, equal IDs across source variants and native file-path display
- **THEN** assembled message parts SHALL preserve order, sourceId and display without deduplication or renaming

### Requirement: Independent strict source schema and UI evidence
Raw schemas SHALL independently verify supported fields, required empties, inactive-field absence and containing-document bounds because TS parses permissively. Provider-independent schema-parsed UI tests SHALL prove native source assembly, repeated/equal-cross-variant IDs and display values with WithUIMessageStreamSources(true).

#### Scenario: Independent strict source schema and UI evidence
- **WHEN** source wire output contains required empty document titles and corresponding UI chunks contain repeated equal IDs across URL/document variants
- **THEN** raw schemas SHALL independently verify fields/bounds and schema-parsed provider-independent UI tests with WithUIMessageStreamSources(true) SHALL prove native assembly/order/display

### Requirement: Test-only explicit UI response identity projection
Any response identity in assembled message metadata SHALL be copied explicitly by a test-only WithUIMessageStreamMessageMetadata callback from supplied StreamFinishStep.Response values; it SHALL NOT be represented as automatic UI identity/warning projection or proof of Gateway absent-value semantics.

#### Scenario: Test-only explicit UI response identity projection
- **WHEN** an assembled UI message includes supplied StreamFinishStep.Response identity
- **THEN** a test-only WithUIMessageStreamMessageMetadata callback SHALL explicitly copy it without claiming automatic identity/warning projection or Gateway absent-value semantics

### Requirement: Source transport provenance and delivery evidence separation
Synthetic transports SHALL NOT be presented as provider recordings. Published dependency, same-revision source and plugin package evidence SHALL be reported separately.

#### Scenario: Source transport provenance and delivery evidence separation
- **WHEN** source acceptance uses a synthetic transport and a same-revision source build
- **THEN** the report SHALL NOT label the transport as a recording and SHALL separate published-dependency, same-revision source and plugin package evidence

### Requirement: Source metadata validity and explicit failure
Each source metadata namespace SHALL be validated on its original bytes as a UTF-8 name with a UTF-8 JSON object value. Malformed JSON, invalid UTF-8 and null or non-object namespaces SHALL fail the response or event explicitly rather than selectively omit fields. Source strings and metadata SHALL NOT be budgeted by size or cardinality.

#### Scenario: Large source metadata
- **WHEN** whitespace-heavy or numerous source metadata namespaces exceed the former unary or frame budget
- **THEN** the source SHALL be returned with all namespaces unchanged

#### Scenario: Invalid source metadata
- **WHEN** a source metadata namespace is malformed, invalid UTF-8, null or not an object
- **THEN** the response or event SHALL fail through the existing safe error path rather than omit the namespace
