## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption

For successful unary responses, the client SHALL require JSON media type, read within its unary limit, accept one complete valid document and map its supported registered content, finishReason, usage and warnings into provider.GenerateResult. It SHALL reject malformed required fields, unknown content/finish/warning discriminators, invalid usage, trailing JSON and oversized input. Supported source and reasoning families SHALL remain governed by their capabilities.

The client SHALL replace server request/response: Request.Body SHALL be the locally encoded request; Response.Headers/Body SHALL be the bounded Gateway HTTP response. Native response id/modelId/timestamp SHALL remain available inside that raw body but SHALL NOT populate typed Response identity, matching the registered TS client's replacement. No native diagnostic access carrier SHALL be introduced by this behavior.

Warnings SHALL preserve active registered fields, required empty strings, order and multiplicity, defaulting to a non-nil empty slice when absent/null. Unary/stream decoding SHALL share warning validation. Optional absent/empty details SHALL decode to the same empty Go string as a documented representation adaptation. This change SHALL NOT expand unrelated provider metadata decoding or claim that existing metadata loss is a privacy policy.

#### Scenario: Minimal unary success is consumed
- **WHEN** valid supported content, finish and usage arrive without warnings
- **THEN** the client SHALL return them with local request metadata, Gateway headers/body and non-nil empty warnings

#### Scenario: Server supplies client-owned fields
- **WHEN** a body contains native nested response identity, transport fields and ordered warnings
- **THEN** the client SHALL preserve warnings and the raw body while replacing typed request/response with Gateway-hop information
- **AND** typed Response id/modelId/timestamp SHALL remain unset rather than falsely exposing native typed identity

#### Scenario: Unary response exceeds its bound
- **WHEN** the response exceeds the unary byte limit by one byte or has trailing JSON
- **THEN** the client SHALL fail without a partial result or unbounded retained body

#### Scenario: Unary result is outside supported families
- **WHEN** an output discriminator has no supported client mapper
- **THEN** the client SHALL fail explicitly rather than decoding through provider-domain JSON

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown discriminator, malformed field type or missing required feature/setting/message
- **THEN** the client SHALL fail through its existing bounded protocol-error path

#### Scenario: Required warning strings are empty
- **WHEN** known variants supply present empty required strings and absent or explicitly empty details
- **THEN** decoding SHALL preserve required values/order and normalize details to empty Go strings

### Requirement: Registered stream normalization

The client SHALL ignore exact DONE data payloads, accept transport clean EOF with or without finish, filter raw parts unless requested, convert valid response-metadata timestamps into time.Time and preserve event order. Cancellation SHALL end without manufacturing a provider error. A received finish SHALL be delivered before closure; the client SHALL not invent finish or require DONE.

Native response-metadata id/modelId/timestamp SHALL be optional. Supplied identity strings SHALL be preserved without public route syntax checks, trimming, canonical replacement or fabricated defaults. Absent and explicitly empty optional wire strings SHALL decode to empty Go strings; absent timestamp SHALL decode to zero time. Bounded original-document UTF-8/type validation and valid RFC3339Nano timestamp parsing SHALL remain effective. Present null/wrong-type identity fields or invalid timestamps SHALL fail through the bounded protocol-error path. Native provider identity, native diagnostic bodies/headers and opaque metadata are not added by this identity mapper.

#### Scenario: DONE terminates no value
- **WHEN** exact DONE appears before clean EOF
- **THEN** it SHALL not become a StreamPart and EOF SHALL be accepted

#### Scenario: Response timestamp is present
- **WHEN** response metadata contains a valid registered timestamp
- **THEN** StreamPart.Timestamp SHALL represent the same instant

#### Scenario: Native stream identity is not a public route
- **WHEN** native id/modelId contain valid Unicode or punctuation outside route-ID syntax
- **THEN** those values SHALL be retained without route validation or normalization

#### Scenario: Response metadata is partial or empty
- **WHEN** metadata supplies no identity, only one field or explicit empty optional strings
- **THEN** the client SHALL decode only the supplied values with Go zero-value normalization and no canonical fallback

#### Scenario: Response metadata is malformed
- **WHEN** an optional identity field is null/wrong-type, timestamp is invalid or the bounded original document has malformed UTF-8
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without returning substituted identity

#### Scenario: Raw output is not requested
- **WHEN** a valid raw part arrives without IncludeRawChunks
- **THEN** it SHALL be consumed but not delivered

#### Scenario: Raw output is requested
- **WHEN** a valid raw part arrives with IncludeRawChunks
- **THEN** its bounded value SHALL be delivered unchanged

#### Scenario: Clean EOF occurs without finish
- **WHEN** a valid stream or DONE is followed by EOF without finish
- **THEN** the client SHALL close cleanly to match the registered client's behavior

### Requirement: No implicit client retry or backend selection

The client SHALL issue at most one Gateway model request per DoGenerate/DoStream invocation after token acquisition. It SHALL preserve existing retryability for caller orchestration but SHALL NOT select physical providers, traverse server candidates, retry through retired transport or retry after response/events. Native source and streaming response identity SHALL be retained; retaining those values SHALL NOT enable client backend selection. This change SHALL NOT add fallback topology, attempt evidence or provider failure-detail transport.

#### Scenario: Retryable setup error occurs
- **WHEN** the Gateway returns a retryable non-2xx response
- **THEN** one invocation SHALL return that error after one request and leave retries to its caller

#### Scenario: Stream error occurs after output
- **WHEN** a stream error or transport failure occurs after delivery
- **THEN** the client SHALL not issue another request or change its requested model identity

### Requirement: Exact-pinned differential and black-box evidence

Tests SHALL compare Go and the exact Gateway version registered in test/conformance/upstream.yaml for semantic method/path/headers/body, supported result normalization, errors/retryability, cancellation, discovery, DONE, raw filtering, timestamp conversion and EOF. Native-value cases SHALL cover all warning variants/order/required empties, URL/document IDs/display/order and optional native stream identity. Unary tests SHALL prove warning preservation, raw native response identity and typed transport replacement rather than comparing only permissively parsed success. Raw HTTP/schema assertions SHALL independently establish strict server correctness.

A baseline change SHALL update pins/lockfiles/captures/classification/client behavior coherently. Hostile fake-server tests SHALL prove bounded reads and cleanup independently of Gateway implementation. Authenticated black-box command tests SHALL run over HTTP without Apache production imports of Gateway code. Synthetic responses SHALL NOT establish live provider or private Vercel-service parity, and authentic provider fixture inputs SHALL NOT be rewritten.

#### Scenario: Equivalent text calls are compared
- **WHEN** both clients issue representable unary/stream calls including supported native values
- **THEN** observable results SHALL agree except for explicitly documented Go presence adaptations and unary typed transport replacement

#### Scenario: Registered baseline changes
- **WHEN** a package pin changes
- **THEN** validation SHALL require reviewed differential evidence and divergence classification

#### Scenario: Client bounds are tested
- **WHEN** fake endpoints cross unary, SSE, event-count or discovery/error limits and cancellation blocks delivery
- **THEN** tests SHALL prove bounded reads, body/channel closure and client-owned resource cleanup

#### Scenario: Authenticated command is exercised
- **WHEN** deterministic provider/auth fakes serve supported native-value output through the real command
- **THEN** both clients SHALL preserve contracted values while configured credentials and cross-tenant state remain absent
- **AND** canonical operator identity and metadata-only capture SHALL remain independent of returned native values

### Requirement: Source response consumption

The independent Go client SHALL decode URL/document sources in unary/stream responses without Gateway imports. Required document title and source ID SHALL accept empty strings but reject missing/wrong-type required fields. Optional URL title/document filename absence and empty string SHALL normalize to empty Go strings. Native IDs, display, order and currently supplied object-valued metadata SHALL survive without rewriting, deduplication or variant-specific collision repair. Unknown source discriminators and malformed metadata SHALL use the existing bounded protocol-error path. Unary Title SHALL be populated and Text retained for legacy consumers. Metadata preservation in this decoder SHALL NOT imply that the server's outstanding metadata projection gap is solved.

#### Scenario: URL and document consumption
- **WHEN** bounded readers receive both registered variants
- **THEN** native variant fields, identity and display SHALL survive, including empty required document title and source ID
- **AND** missing required title/ID SHALL fail

#### Scenario: Equal and repeated source IDs
- **WHEN** repeated URL sources and a document share a native ID
- **THEN** all SHALL retain the supplied ID and relative order without deduplication
