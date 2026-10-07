# gateway-sources Specification

## Purpose
Define bounded native Gateway source projection, independent client decoding, and separate lifecycle and metadata-only observation contracts for URL and document citations.
## Requirements
### Requirement: Registered source output

The Gateway SHALL emit atomic flat source parts in unary content and streams. URL sources SHALL contain type, sourceType, id and url; nonempty title MAY be included. Document sources SHALL contain type, sourceType, id, mediaType and title even when title is empty; nonempty filename MAY be included. Optional empty URL titles and document filenames SHALL be omitted as the documented Go absent/empty normalization. Nonempty unary Title SHALL take precedence over legacy Text; empty Title SHALL fall back to Text because the string API cannot distinguish absence and explicit empty Title. Native title/filename values SHALL remain unchanged, including file_path display. Unknown discriminators, nil stream sources and invalid represented UTF-8 SHALL fail through the existing safe error. Inactive fields SHALL NOT be serialized. URLs SHALL NOT be fetched or canonicalized.

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

### Requirement: Bounded response-local source identity

The Gateway SHALL preserve each native source ID exactly in its response rather than assigning a sequential, hashed or generated replacement. Source ID SHALL be a required valid-UTF-8 string, including an empty value permitted by the registered contract. Repeated IDs and equal IDs across URL/document variants SHALL remain unchanged and SHALL NOT cause deduplication or collision repair. Identity-map state and its 1024-byte key cap SHALL be removed; source identity is not public route identity.

Unary preflight SHALL bound aggregate source strings and all original metadata namespace key/value bytes with other represented values by UnaryResponseBytes; content/metadata cardinality SHALL bound mapping work. Streaming SHALL use StreamParts and complete-frame bounds, including source strings, current metadata and SSE framing. Size preflight SHALL precede UTF-8 scanning and encoding; exact final-byte checks SHALL prevent partial output. No source-specific ID cap SHALL reject a value that otherwise fits these containing-document bounds.

#### Scenario: Repeated source
- **WHEN** the same URL ID appears twice and a document has the same ID
- **THEN** all three SHALL keep their native ID and original order without deduplication or variant-specific renaming

#### Scenario: Empty or long native ID
- **WHEN** the source ID is empty or exceeds the old 1024-byte cap while the complete document remains valid and bounded
- **THEN** it SHALL be emitted unchanged as a required string

#### Scenario: Source crosses complete bounds
- **WHEN** raw strings exceed aggregate preflight, UTF-8 is invalid or escaping makes the complete response/frame one byte too large
- **THEN** the existing bounded unary or streaming safe error path SHALL apply without partial source output

### Requirement: Opaque public source metadata

Source providerMetadata SHALL preserve all supplied native object-valued namespaces and nested JSON under gateway-provider-metadata, with no synthetic citation namespace, numeric-only projection, recognized-namespace byte cap or key inventory. Absence SHALL remain absent and an explicit empty object SHALL remain present. All original namespace bytes including whitespace, namespace key bytes and cardinality SHALL participate in aggregate unary or complete-frame preflight before validation and encoding. Malformed JSON, invalid UTF-8, null/non-object namespaces or excess budget SHALL fail the response/event explicitly rather than selectively omitting fields. Useful native IDs, cited text and unknown fields inside ordinary provider metadata SHALL NOT be discarded merely because operator capture excludes them. Concrete credential and tenant-source protections SHALL remain independent from ordinary metadata transport.

Recognized file_path metadata SHALL NOT alter native source identity, title or filename.

#### Scenario: Native file path
- **WHEN** file_path supplies native source ID, title, filename and index
- **THEN** native identity/title/filename and opaque metadata SHALL survive unchanged, including index in its original namespace

#### Scenario: Malformed unknown namespace fails explicitly
- **WHEN** a source has a previously unknown namespace with malformed or non-object JSON
- **THEN** the unary response SHALL fail before HTTP success or the committed stream SHALL reject the event through its existing terminal adaptation path, without numeric-only or metadata-free success

### Requirement: Atomic source lifecycle and observation

Sources SHALL preserve relative output order and SHALL NOT close active text or tool blocks. Accepted sources SHALL mark output as started, disallow subsequent response metadata and reset idle activity through the existing loop. Provider error parts SHALL remain non-terminal; authoritative finish SHALL suppress later sources. Sources SHALL commit fallback candidates without replay. Existing cancellation, frame bounds, timeouts, part limits and cleanup ownership SHALL remain effective.

Reusable logging, Prometheus and Agent Observability SHALL treat source as payload-bearing first output and pass it through unchanged. Gateway metadata-only telemetry SHALL omit source IDs, URLs, titles, filenames and metadata. Agent Observability SHALL NOT manufacture an unsupported source recording representation.

#### Scenario: Source interleaving
- **WHEN** a source appears between text-start and text-end
- **THEN** the text block SHALL remain active and subsequent text deltas SHALL be accepted
- **AND** a later response-metadata part SHALL fail safely

#### Scenario: Source-only observation
- **WHEN** a source is the only payload before finish
- **THEN** reusable observers SHALL record first-output timing without recording its content in metadata-only output

### Requirement: Independent consumption and evidence

The Apache Go client SHALL decode both registered variants without Gateway implementation imports, preserve native ID/display/order and current object-valued metadata, require document title presence while accepting an empty title, and normalize optional strings. Its bounded HTTP/SSE consumption SHALL apply. Exact-pinned Vercel/Go differential tests SHALL exercise unary/stream sources through the handler and authenticated command. Raw schemas SHALL independently verify supported fields, required empties, inactive-field absence and containing-document bounds because TS parses permissively. Provider-independent schema-parsed UI tests SHALL prove native source assembly, repeated/equal-cross-variant IDs and display values with WithUIMessageStreamSources(true). Any response identity in assembled message metadata SHALL be copied explicitly by a test-only WithUIMessageStreamMessageMetadata callback from supplied StreamFinishStep.Response values; it SHALL NOT be represented as automatic UI identity/warning projection or proof of Gateway absent-value semantics. Synthetic transports SHALL NOT be presented as provider recordings. Published dependency, candidate-workspace and image evidence SHALL be reported separately.

#### Scenario: Both clients
- **WHEN** both clients consume bounded URL/document output
- **THEN** native IDs/display/order SHALL agree after declared optional-empty normalization
- **AND** missing document title SHALL fail on the strict Go path while explicit empty title SHALL succeed

#### Scenario: Frontend assembly
- **WHEN** schema-parsed UI chunks contain repeated native IDs, equal IDs across source variants and native file-path display
- **THEN** assembled message parts SHALL preserve order, sourceId and display without deduplication or renaming
