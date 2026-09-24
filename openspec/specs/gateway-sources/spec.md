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

The Gateway SHALL map the structured pair (sourceType, native ID) to a response-local sequential public ID. Native IDs SHALL be nonempty, valid UTF-8 and at most 1024 bytes, checked before map lookup. The same pair SHALL retain one public ID; distinct pairs SHALL remain distinct. No native ID SHALL be included or hashed into public identity. Unary preflight SHALL bound total input strings and recognized metadata by UnaryResponseBytes, and content cardinality SHALL bound map growth. Streaming SHALL use StreamParts and complete-frame bounds. Identity entries SHALL be inserted only after successful bounded source encoding.

#### Scenario: Repeated source
- **WHEN** a provider emits the same URL source ID twice and a document with the same native ID
- **THEN** the repeated URL SHALL keep one public ID and the document SHALL get a distinct ID

### Requirement: Closed public source metadata

The only public source metadata namespace SHALL be citation. Its only fields SHALL be index, startPageNumber, endPageNumber, startCharIndex and endCharIndex, each an integer from 0 through 1000000000 inclusive. index SHALL originate only from the native openai namespace; page/character fields SHALL originate only from anthropic. Malformed, null, fractional, negative or out-of-range approved values SHALL be omitted. Unknown namespaces/fields, native IDs, encrypted indexes, cited text, credentials and transport fields SHALL be omitted without recursive sanitization. Each inspected native namespace SHALL be at most 8192 bytes and within response/frame preflight bounds before JSON decoding; oversize SHALL fail safely. Malformed namespace JSON SHALL be omitted. Empty public metadata SHALL be absent. This is an intentional Gateway privacy projection, not native-provider metadata parity.

For document sources with recognized openai.type equal to file_path, title SHALL be "Document" and filename SHALL be omitted regardless of their original display values. Native providers SHALL retain their upstream representation outside the Gateway. Other title, URL and filename fields SHALL be treated as public application content, not generically redacted.

#### Scenario: Native file path
- **WHEN** OpenAI file_path supplies a native file ID as both title and filename
- **THEN** Gateway output SHALL use title "Document", omit filename, replace source identity and omit native metadata
- **AND** a valid index MAY survive under citation

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
