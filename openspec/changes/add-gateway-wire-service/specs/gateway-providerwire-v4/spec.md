## ADDED Requirements

### Requirement: Independent strict LanguageModelV4 package

The repository SHALL provide `github.com/grafana/ai-sdk/gateway/providerwire/v4`, declared as `providerwirev4`, for the pinned LanguageModelV4 JSON and SSE wire. It SHALL use private explicit DTOs and MUST NOT import legacy `gateway/providerwire`. The existing legacy package and behavior SHALL remain unchanged.

#### Scenario: Strict package excludes legacy imports

- **WHEN** production imports in the strict package are inspected
- **THEN** none SHALL reference `github.com/grafana/ai-sdk/gateway/providerwire`

#### Scenario: Legacy package remains compatible

- **WHEN** existing legacy public API and behavior fixtures compile and run
- **THEN** their symbols, tolerant decoding, resolver contract, headers, and error behavior SHALL remain unchanged

### Requirement: Private explicit V4 DTO boundary

Request, result, stream, content, tool, usage, metadata, file-data, and error serialization SHALL use unexported wire DTOs with field-by-field provider conversion. DTOs MUST NOT embed or directly marshal provider types with transport JSON methods. Intrinsically opaque valid JSON such as schemas, provider options, provider metadata, raw values, custom values, and tool JSON MAY pass as `json.RawMessage`.

#### Scenario: Provider JSON changes do not alter strict bytes

- **WHEN** a provider-domain JSON tag or custom marshaler changes
- **THEN** strict bytes SHALL remain controlled by V4 DTO conversion and goldens

#### Scenario: Nested conversion remains independent

- **WHEN** strict conversion handles nested messages, file data, tool results, generated content, or stream parts
- **THEN** it SHALL NOT invoke the legacy provider JSON path

### Requirement: Pinned strict semantic conversion

The codec SHALL bidirectionally preserve every supported semantic value in registered `@ai-sdk/provider@4.0.4`. It SHALL reject unknown discriminators, missing required fields, invalid active-field types, malformed tool or opaque JSON, known legacy or private fields, invalid provider references, and unrepresentable provider-domain values. A canonical discriminator SHALL select its union arm; unrelated additive fields, including inactive sibling-arm fields, SHALL be ignored.

#### Scenario: Unknown discriminator fails closed

- **WHEN** a request, result, file-data, tool-output, or stream union carries an unknown discriminator
- **THEN** conversion SHALL fail without a partial provider value

#### Scenario: Discriminator selects union arm

- **WHEN** a known union value contains fields belonging to an inactive sibling arm
- **THEN** strict conversion SHALL use the discriminated arm and ignore those unrelated fields

#### Scenario: Ambiguous domain value fails closed

- **WHEN** a provider-domain value lacks a trustworthy discriminator and populates multiple representations
- **THEN** strict encoding SHALL return an error rather than choose a representation

#### Scenario: Empty inline text round trips

- **WHEN** canonical file data is `{"type":"text","text":""}`
- **THEN** strict decode followed by encode SHALL preserve that tagged empty-text meaning

#### Scenario: Additive field remains harmless

- **WHEN** a known V4 object contains an unrelated additive field
- **THEN** decoding SHALL preserve supported semantics and ignore that field

### Requirement: Gateway namespace is unsupported

The handler's unexported request decoder SHALL return `provider.CallOptions` without exposing a gateway-control model. It SHALL remove top-level `providerOptions.gateway` when absent or an empty object and SHALL reject any non-empty key before catalog resolution. Reserved `gateway` namespaces nested in messages, content, tools, or tool outputs SHALL remain rejected.

#### Scenario: Empty gateway object is removed

- **WHEN** a canonical request contains `providerOptions.gateway: {}`
- **THEN** decoding SHALL succeed and provider-bound options SHALL not contain `gateway`

#### Scenario: Nonempty gateway object is rejected

- **WHEN** a canonical request contains any key under `providerOptions.gateway`
- **THEN** the handler SHALL return a safe invalid-request response before catalog resolution

#### Scenario: Nested gateway object is rejected

- **WHEN** a reserved gateway namespace appears in nested provider options
- **THEN** strict conversion SHALL fail as before

### Requirement: Catalog backed handler composition

`NewHandler` SHALL accept a non-nil `catalog.ModelResolver`. After adapter-local validation, it SHALL resolve the exact requested model header value with a context derived from the HTTP request. It SHALL invoke the returned non-nil `provider.LanguageModel` directly and exactly once for the selected operation. Nil resolved models SHALL be internal adapter defects.

#### Scenario: Exact public ID reaches catalog

- **WHEN** a valid request carries a non-empty model ID including surrounding characters
- **THEN** the resolver SHALL receive the exact header value rather than a normalized substitute

#### Scenario: Request context reaches catalog and model

- **WHEN** the request context is canceled or contains host values
- **THEN** the derived resolution and model contexts SHALL preserve cancellation and values

#### Scenario: Nil resolved model is internal

- **WHEN** catalog resolution succeeds with a nil model
- **THEN** the handler SHALL return a safe non-retryable HTTP 500 without invoking a model

### Requirement: Strict pinned HTTP request validation

The handler SHALL remain path-agnostic and expose `/language-model` plus the exact model, streaming, and specification-version headers. It SHALL accept only `POST`, require a non-empty model ID, require specification version `4`, require streaming exactly `true` or `false`, require `application/json`, and apply quality-aware `Accept` negotiation. Adapter-local method, negotiation, request-size, and media failures SHALL retain 405, 406, 413, and 415 statuses.

#### Scenario: Stock unary request dispatches

- **WHEN** registered `@ai-sdk/gateway@4.0.33` sends a valid generate request
- **THEN** the handler SHALL decode and dispatch one direct generate call

#### Scenario: Stock stream request dispatches

- **WHEN** the registered client sends a valid streaming request
- **THEN** the handler SHALL decode and dispatch one direct stream call

#### Scenario: Invalid adapter input bypasses catalog

- **WHEN** method, required headers, media type, negotiation, body limit, DTO decoding, gateway controls, or raw chunks are invalid
- **THEN** catalog resolution and model invocation SHALL not run

#### Scenario: Zero quality is rejected

- **WHEN** every compatible `Accept` range has quality zero
- **THEN** the handler SHALL return HTTP 406 before catalog resolution

### Requirement: Handler owned total and idle lifecycle

The handler SHALL default to a positive 120-second total timeout and 60-second streaming idle timeout and SHALL expose positive options for both. Total timeout SHALL cover catalog resolution, synchronous model invocation/setup, and established stream consumption through context cooperation. Established streaming SHALL select directly over the provider channel, total/request context, and idle timer without an invocation goroutine, proxy channel, public session, or terminal wait method.

Idle time SHALL measure only waits for the next provider part after the preceding event was written and flushed. Synchronous write time SHALL not count as provider inactivity. Host server deadlines govern blocked writes.

#### Scenario: Cooperative total timeout maps safely

- **WHEN** resolution or model work observes total timeout
- **THEN** the handler SHALL cancel its derived context and map the failure as retryable timeout

#### Scenario: Idle timeout maps after commitment

- **WHEN** an established provider channel produces no part within the idle interval
- **THEN** the handler SHALL cancel model context and emit one best-effort safe timeout event

#### Scenario: Provider close completes cleanly

- **WHEN** the provider channel closes without lifecycle failure
- **THEN** the response SHALL end without a synthetic part or `[DONE]`

#### Scenario: Blocking provider remains cooperative boundary

- **WHEN** synchronous provider code ignores context
- **THEN** the deadline SHALL make its supplied context done but the handler SHALL NOT create another goroutine to force return

### Requirement: Private safe V4 failure projection

Unexported V4 helpers SHALL project only failures produced by current composition. Mappings SHALL be unknown public model to 404 `model_not_found`; provider rate limit to 429 `rate_limit_exceeded`; timeout to 504 `internal_server_error`; cancellation to 499 `internal_server_error`; permanent provider dependency to 424 `failed_dependency`; transient provider dependency to 502 `failed_dependency`; and adapter defect to 500 `internal_server_error`. Generic model invocation errors SHALL be permanent failed dependencies, not internal defects.

Projection MUST NOT serialize original messages, provider URLs, request values, response headers/body, provider data, backend identity, or private causes. Original causes SHALL remain privately reachable where applicable. The public strict decoder MAY continue accepting registered Grafana categories without requiring a handler producer.

#### Scenario: Catalog miss exposes only requested ID

- **WHEN** resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** the 404 response MAY contain only the requested public model ID as its safe parameter

#### Scenario: Provider rate limit stays retryable

- **WHEN** model invocation returns a provider HTTP 429 error
- **THEN** the response SHALL be redacted HTTP 429 with explicit retryability

#### Scenario: Generic invocation is dependency failure

- **WHEN** model invocation returns an otherwise unclassified error
- **THEN** the response SHALL be redacted HTTP 424 rather than HTTP 500

#### Scenario: Provider diagnostics remain private

- **WHEN** a provider API error carries backend URL, bodies, headers, data, and messages
- **THEN** none SHALL appear in unary or stream public errors

### Requirement: Strict privacy allowlist

The handler SHALL omit provider request bodies, backend response headers/bodies, stream setup headers, provider identity, and backend `modelId`. It MAY expose response ID and timestamp and SHALL preserve V4 warnings, provider metadata, content, usage, and finish values. Raw-chunk requests SHALL be rejected before resolution and provider-emitted raw parts SHALL not be published.

#### Scenario: Unary backend details are omitted

- **WHEN** a provider result contains request and backend response metadata
- **THEN** strict public JSON SHALL include only allowlisted response metadata

#### Scenario: Response model is not substituted

- **WHEN** catalog identity differs from backend identity
- **THEN** `response.modelId` SHALL be omitted rather than exposing or substituting either value

#### Scenario: Raw exposure is denied

- **WHEN** a caller requests raw chunks
- **THEN** the request SHALL fail before catalog resolution because no host policy approves exposure

### Requirement: Direct SSE forwarding and flushing

A valid stream SHALL emit every supported provider part in order as one `data: <canonical-json>\n\n` event, with no `event:` field or `[DONE]`. `PartError` SHALL be redacted repeatable data and SHALL not stop later parts. The handler SHALL set event-stream content type, no-cache/no-transform, and no-buffering headers; it MUST NOT set `Connection: keep-alive`. It SHALL use `http.ResponseController` for initial and per-event flushes.

Write or flush failure SHALL cancel model context and terminate without another write. Stream setup failures SHALL remain non-2xx JSON. Post-commit lifecycle and encoding failures SHALL use one best-effort safe error event when possible.

#### Scenario: Error part continues to finish

- **WHEN** a provider emits `PartError`, later content, and `PartFinish`
- **THEN** the client SHALL receive a safe error and every later part in original order

#### Scenario: Wrapped writer flushes

- **WHEN** a response wrapper exposes an underlying flusher through supported response-controller behavior
- **THEN** initial headers and each complete event SHALL be flushed

#### Scenario: Failed writer stops immediately

- **WHEN** event writing or flushing fails
- **THEN** model context SHALL be canceled and the handler SHALL not attempt a second event

### Requirement: Strict service transport limits

Defaults SHALL remain 8 MiB request body, 16 MiB encoded unary success, and 8 MiB complete framed SSE event, with positive construction options. Requests at the limit SHALL succeed and limit-plus-one SHALL fail. Unary results and events SHALL be encoded and checked before their bytes are committed or written. Complete event accounting SHALL include `data: `, JSON, and `\n\n` and SHALL match Grafana strict mode. These are transport commitment limits, not allocation bounds.

#### Scenario: Oversized request bypasses catalog

- **WHEN** a request exceeds its configured limit by one byte
- **THEN** the handler SHALL return HTTP 413 without catalog resolution

#### Scenario: Oversized unary avoids partial success

- **WHEN** encoded unary success exceeds its configured limit
- **THEN** the handler SHALL return a safe HTTP 500 instead of partial HTTP 200

#### Scenario: Oversized event is not written

- **WHEN** one encoded event exceeds its configured limit
- **THEN** that event SHALL not be written and the handler SHALL best-effort emit a safe adapter error that fits

### Requirement: Pinned dual deployment evidence

Tests SHALL use the registered npm versions rather than upstream main. Evidence SHALL retain exhaustive literal strict discriminator goldens at the codec boundary plus representative malformed rejection, public privacy, limits, flushing, catalog-before-model behavior, current error mappings, Grafana strict mode, and stock TypeScript interop. Legacy and strict handlers SHALL use distinct base URLs while shared canonical success and `PartError` continuation scenarios pass through both. The unreleased V4 public API SHALL expose only handler construction, handler options and constants, strict Grafana client codec operations, and its bounded SSE reader.

#### Scenario: Canonical clients pass both endpoints

- **WHEN** representative TypeScript and Grafana scenarios run against legacy and strict base URLs
- **THEN** canonical success and stream-continuation behavior SHALL pass without codec negotiation

#### Scenario: Parity validation uses registered baseline

- **WHEN** validation runs
- **THEN** baseline, parity, focused provider-wire, Grafana, and interop checks SHALL pass without invented provider fixture input
