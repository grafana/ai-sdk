# gateway-sources Specification

## Purpose
Define bounded, privacy-preserving Gateway source projection, independent client decoding, and the lifecycle and observation contract for URL and document citations.
## Requirements
### Requirement: Registered source output

The Gateway SHALL emit atomic flat source parts in unary content and streams. URL sources SHALL contain type, sourceType, id and url; nonempty title MAY be included. Document sources SHALL contain type, sourceType, id, mediaType and title even when title is empty; nonempty filename MAY be included. Optional empty URL titles and document filenames SHALL be omitted. Nonempty unary Title SHALL take precedence over legacy Text; empty Title SHALL fall back to Text because the existing string API cannot distinguish absent and explicitly empty Title. Unknown discriminators, nil stream sources, invalid UTF-8 and invalid provider IDs SHALL fail with the existing safe error. Inactive fields SHALL NOT be serialized. URLs SHALL NOT be fetched or canonicalized.

#### Scenario: Title compatibility
- **WHEN** a unary document contains Title "current" and Text "legacy"
- **THEN** its wire title SHALL be "current"
- **AND** Title absent/empty with Text "legacy" SHALL retain "legacy"

#### Scenario: Required empty title
- **WHEN** a document has no title or legacy text
- **THEN** its wire title SHALL be the empty string
- **AND** optional empty filename SHALL be absent

### Requirement: Bounded response-local source identity

Source scalar identity/validation SHALL remain governed by #320/PR #325 rather than this metadata transport change. The current base maps (sourceType, native ID) to response-local source-N IDs with a 1024-byte native-ID bound; those facts describe the base and SHALL NOT renew a native-identity suppression policy. On rebase/merge, this requirement and its tests SHALL preserve #325's native source identity behavior instead of restoring response-local substitution. Unary preflight SHALL bound total input strings and all original metadata namespace key/value bytes by UnaryResponseBytes, and content/metadata cardinality SHALL bound map growth. Source metadata SHALL share the aggregate result/content unary budget, not reset it for each source. Streaming SHALL use StreamParts and complete-frame bounds. Identity entries SHALL be inserted only after successful bounded source encoding.

#### Scenario: Repeated source
- **WHEN** a provider emits the same URL source ID twice and a document with the same native ID
- **THEN** identity SHALL follow the separately owned scalar policy without changing source metadata semantics, and integrated #320/PR #325 native source IDs SHALL NOT be replaced by source-N IDs

### Requirement: Closed public source metadata

Source providerMetadata SHALL preserve all supplied native object-valued namespaces and nested JSON under gateway-provider-metadata, with no synthetic citation namespace, numeric-only projection, recognized-namespace byte cap or key inventory. Absence SHALL remain absent and an explicit empty object SHALL remain present. All original namespace bytes including whitespace, namespace key bytes and cardinality SHALL participate in aggregate unary or complete-frame preflight before validation and encoding. Malformed JSON, invalid UTF-8, null/non-object namespaces or excess budget SHALL fail the response/event explicitly rather than selectively omitting fields. Useful native IDs, cited text and unknown fields inside ordinary provider metadata SHALL NOT be discarded merely because operator capture excludes them. Concrete credential and tenant-source protections SHALL remain independent from ordinary metadata transport.

Source title, URL and filename behavior SHALL remain owned by #320/PR #325. The current base replaces file_path titles with Document and omits filenames; this is implementation context, not renewed privacy policy. Metadata-only work SHALL leave scalar code unchanged on that base and SHALL preserve #325's native display/filename behavior when it integrates, reconciling copied requirements/tests rather than restoring substitutions. Ordinary source strings SHALL NOT acquire a generic redaction policy.

#### Scenario: Native file path
- **WHEN** OpenAI or Azure file_path supplies a native file ID as both title and filename
- **THEN** bounded native metadata SHALL survive independently of the separately owned display contract, and integrated #320/PR #325 native title/filename/identity behavior SHALL be preserved without restoring Document/filename substitution
- **AND** a valid native index SHALL remain in its original namespace rather than being renamed to citation

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

The Apache Go client SHALL decode both registered variants independently of Gateway implementation imports, preserve public identity and metadata, require document title presence while accepting its empty string, and normalize optional strings. Its existing bounded HTTP/SSE consumption SHALL apply. The maintained exact-pinned Vercel/Go differential suite SHALL exercise unary and streaming sources through the handler and authenticated command. Raw schemas and privacy tests SHALL independently verify output because the registered Vercel client parses permissively. Provider-independent UI tests SHALL prove source assembly. Synthetic transport responses SHALL NOT be presented as recorded provider evidence. Published dependency tests and disposable workspace overlay tests SHALL be reported separately.

#### Scenario: Both clients
- **WHEN** the pinned Vercel and independent Go clients consume the same normalized source output
- **THEN** URL/document fields, metadata and order SHALL agree after the declared optional-empty normalization
