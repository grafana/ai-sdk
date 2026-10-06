## MODIFIED Requirements

### Requirement: Registered source output

The Gateway SHALL emit atomic flat source parts in unary content and streams. URL sources SHALL contain type, sourceType, id and url; nonempty title MAY be included. Document sources SHALL contain type, sourceType, id, mediaType and title even when title is empty; nonempty filename MAY be included. Optional empty URL titles and document filenames SHALL be omitted as the documented Go absent/empty normalization. Nonempty unary Title SHALL take precedence over legacy Text; empty Title SHALL fall back to Text because the string API cannot distinguish absence and explicit empty Title. Native title/filename values SHALL remain unchanged, including file_path display. Unknown discriminators, nil stream sources and invalid represented UTF-8 SHALL fail through the existing safe error. Inactive fields SHALL NOT be serialized. URLs SHALL NOT be fetched or canonicalized.

#### Scenario: Title compatibility
- **WHEN** unary Title is current and Text is legacy
- **THEN** title SHALL be current
- **AND** absent/empty Title with legacy Text SHALL retain legacy

#### Scenario: Required empty title
- **WHEN** a document has neither title nor legacy text
- **THEN** title SHALL be present as an empty string and optional empty filename SHALL be absent

#### Scenario: Native file-path display
- **WHEN** an OpenAI or Azure file_path source uses the native file ID as title/filename
- **THEN** both display values SHALL survive without Document substitution or filename removal
- **AND** native source identity SHALL remain the adapter-provided ID, not be derived from file metadata

### Requirement: Bounded response-local source identity

The Gateway SHALL preserve each native source ID exactly in its response rather than assigning a sequential, hashed or generated replacement. Source ID SHALL be a required valid-UTF-8 string, including an empty value permitted by the registered contract. Repeated IDs and equal IDs across URL/document variants SHALL remain unchanged and SHALL NOT cause deduplication or collision repair. Identity-map state and its 1024-byte key cap SHALL be removed; source identity is not public route identity.

Unary preflight SHALL bound aggregate source strings and currently inspected metadata with other represented values by UnaryResponseBytes; content cardinality SHALL bound mapping work. Streaming SHALL use StreamParts and complete-frame bounds, including source strings, current metadata and SSE framing. Size preflight SHALL precede UTF-8 scanning and encoding; exact final-byte checks SHALL prevent partial output. No source-specific ID cap SHALL reject a value that otherwise fits these containing-document bounds.

#### Scenario: Repeated source
- **WHEN** the same URL ID appears twice and a document has the same ID
- **THEN** all three SHALL keep their native ID and original order without deduplication or variant-specific renaming

#### Scenario: Empty or long native ID
- **WHEN** the source ID is empty or exceeds the old 1024-byte cap while the complete document remains valid and bounded
- **THEN** it SHALL be emitted unchanged as a required string

#### Scenario: Source crosses complete bounds
- **WHEN** raw strings exceed aggregate preflight, UTF-8 is invalid or escaping makes the complete response/frame one byte too large
- **THEN** the existing bounded unary or streaming safe error path SHALL apply without partial source output

### Requirement: Closed public source metadata

This scalar/display delivery SHALL leave the existing bounded numeric citation projection unexpanded. Its current citation fields are index, startPageNumber, endPageNumber, startCharIndex and endCharIndex, integers from 0 through 1000000000. index originates from openai/azure; page/character fields originate from anthropic. Invalid, null, fractional, negative or out-of-range values are omitted. Current inspected namespaces remain bounded to 8192 bytes and the response/frame budget before decoding; oversize fails safely and malformed namespace JSON is omitted. Empty projected metadata remains absent.

Loss of native namespaces, unknown fields and cited-text metadata SHALL be documented as an outstanding implementation gap separately owned by opaque metadata work, not an accepted privacy/concealment contract or complete parity. This change SHALL NOT introduce an opaque codec or general metadata helper. Recognized file_path metadata SHALL NOT influence display or identity. Credentials and another tenant's state remain protected independently; native IDs and ordinary display content SHALL NOT be classified as credentials merely by their appearance.

#### Scenario: Current metadata projection remains scoped
- **WHEN** a native source includes current supported citation positions and other native metadata
- **THEN** this delivery SHALL retain its existing numeric projection without adding a metadata codec
- **AND** docs/coverage SHALL identify remaining native metadata loss as a gap rather than a privacy feature

#### Scenario: Native file path
- **WHEN** file_path supplies native source ID, title, filename and a valid index
- **THEN** source ID/title/filename SHALL survive unchanged and index SHALL retain its current citation projection

### Requirement: Independent consumption and evidence

The Apache Go client SHALL decode both registered variants without Gateway implementation imports, preserve native ID/display/order and current object-valued metadata, require document title presence while accepting an empty title, and normalize optional strings. Its bounded HTTP/SSE consumption SHALL apply. Exact-pinned Vercel/Go differential tests SHALL exercise unary/stream sources through the handler and authenticated command. Raw schemas SHALL independently verify supported fields, required empties, inactive-field absence and containing-document bounds because TS parses permissively. Provider-independent schema-parsed UI tests SHALL prove native source assembly, repeated/equal-cross-variant IDs and display values with WithUIMessageStreamSources(true). Any response identity in assembled message metadata SHALL be copied explicitly by a test-only WithUIMessageStreamMessageMetadata callback from supplied StreamFinishStep.Response values; it SHALL NOT be represented as automatic UI identity/warning projection or proof of Gateway absent-value semantics. Synthetic transports SHALL NOT be presented as provider recordings. Published dependency, candidate-workspace and image evidence SHALL be reported separately.

#### Scenario: Both clients
- **WHEN** both clients consume bounded URL/document output
- **THEN** native IDs/display/order SHALL agree after declared optional-empty normalization
- **AND** missing document title SHALL fail on the strict Go path while explicit empty title SHALL succeed

#### Scenario: Frontend assembly
- **WHEN** schema-parsed UI chunks contain repeated native IDs, equal IDs across source variants and native file-path display
- **THEN** assembled message parts SHALL preserve order, sourceId and display without deduplication or renaming
