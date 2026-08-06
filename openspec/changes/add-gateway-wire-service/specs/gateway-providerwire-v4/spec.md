## ADDED Requirements

### Requirement: Separate strict LanguageModelV4 service package

The repository SHALL provide `github.com/grafana/ai-sdk/gateway/providerwire/v4`, declared as package `providerwirev4`, as a new strict LanguageModelV4 HTTP service. It SHALL depend on `gateway/runtime`, `gateway/failure`, and `provider`, and MUST NOT import the legacy `gateway/providerwire` implementation. The existing `gateway/providerwire` package, its exported APIs, legacy-tolerant decoding, and HTTP-aware resolver SHALL remain available and behaviorally unchanged.

#### Scenario: Legacy public API remains source compatible

- **WHEN** an external-package compile test imports legacy route/header/MIME/version constants; `ModelResolver` and `ModelResolverFunc`; `NewHandler` with every existing handler option and default; request/result/error codecs; `SSEReader`, `NewSSEReader`, and both SSE writers; and `WriteErrorResponse`/`DecodeErrorResponse`
- **THEN** every existing symbol and call shape SHALL compile unchanged while the strict package compiles independently

#### Scenario: New service is independent

- **WHEN** imports in `gateway/providerwire/v4` are inspected
- **THEN** they SHALL NOT include `github.com/grafana/ai-sdk/gateway/providerwire`

#### Scenario: Version denotes LanguageModelV4

- **WHEN** the new service receives a request
- **THEN** it SHALL require LanguageModel specification version `4` and SHALL NOT introduce a transport-v2 header or negotiation mechanism

### Requirement: Private explicit V4 DTO boundary

The strict service SHALL implement request, generate-result, stream-part, content, tool, usage, metadata, file-data, and error serialization through unexported wire DTOs and explicit field-by-field conversion. No wire DTO SHALL embed or directly marshal a provider type that defines transport-specific JSON methods. Only intrinsically opaque valid JSON values such as schemas, provider options, provider metadata, raw values, custom values, and tool input/result JSON MAY pass through as `json.RawMessage`.

#### Scenario: Provider struct changes do not silently change the strict wire

- **WHEN** an unrelated JSON tag or custom marshaler changes on a provider domain type
- **THEN** strict service bytes SHALL remain controlled by its V4 DTO and conversion tests

#### Scenario: Nested values avoid legacy marshalers

- **WHEN** the strict codec converts messages, data content, tool results, generated content, or stream parts
- **THEN** it SHALL NOT invoke the custom JSON methods used by the legacy provider-wire path at any nested level

#### Scenario: Opaque provider data is preserved

- **WHEN** valid provider options or metadata contain provider-specific JSON
- **THEN** the strict conversion SHALL preserve that JSON without interpreting its provider-specific fields

### Requirement: Strict semantic validation

The strict conversion SHALL preserve the semantics of every valid supported value in the registered `@ai-sdk/provider@4.0.4` LanguageModelV4 contract. It SHALL reject unknown union discriminators, missing required fields, contradictory union representations, malformed or invalid opaque JSON, malformed tool-call input JSON, and domain states that cannot be represented by the pinned contract. It SHALL ignore harmless additive object fields that do not change a known variant's meaning.

#### Scenario: Contradictory file data is rejected

- **WHEN** a provider file value populates more than one mutually exclusive data representation
- **THEN** strict encoding SHALL return an error rather than selecting the first populated field

#### Scenario: Unknown discriminator is rejected

- **WHEN** a request or response carries an unregistered content, file-data, tool-output, generate-content, or stream-part discriminator
- **THEN** strict conversion SHALL return an error and SHALL NOT produce a partial provider value

#### Scenario: Malformed tool input is rejected

- **WHEN** a tool call contains input that is not one complete valid JSON value
- **THEN** strict conversion SHALL fail before request dispatch or response commitment

#### Scenario: Additive field is ignored

- **WHEN** a known canonical V4 object includes an unknown non-discriminator field that does not conflict with a known union member and is outside `providerOptions.gateway`
- **THEN** decoding SHALL preserve all supported semantics and ignore that field

#### Scenario: Unknown gateway option is preserved

- **WHEN** `providerOptions.gateway` contains an unknown key with a valid JSON value
- **THEN** strict request conversion SHALL preserve it byte-equivalently in `GatewayOptions.Extensions` because the registered gateway option type permits service-owned additions

### Requirement: Canonical-only bidirectional decoding

The strict codec SHALL encode and decode canonical registered LanguageModelV4 requests, generate results, and stream parts through private DTOs and provider-domain conversion entry points usable by the handler and Grafana client. It SHALL reject legacy Go-only alternatives, including system content arrays, split tool-result output fields, legacy file discriminators, and legacy result/event shapes. Legacy-tolerant decoding SHALL remain available only through `gateway/providerwire`.

#### Scenario: Canonical system message is accepted

- **WHEN** a request contains `{"role":"system","content":"be concise"}`
- **THEN** the strict decoder SHALL produce the equivalent provider system message

#### Scenario: Legacy system array is rejected

- **WHEN** a request contains a system message whose content uses the legacy part array
- **THEN** the strict decoder SHALL return an invalid-request failure while the legacy decoder remains able to accept it

#### Scenario: Canonical tool-result file is accepted

- **WHEN** a canonical tool-result content value contains `type: "file"` and a tagged `data` union
- **THEN** the strict decoder SHALL preserve the file data, media type, and optional filename

#### Scenario: Canonical result and stream decode strictly

- **WHEN** the Grafana strict client decodes a canonical generate result or stream part
- **THEN** it SHALL recover the provider-domain semantics without invoking legacy provider JSON methods

#### Scenario: Legacy response shape is rejected

- **WHEN** strict result or stream decoding receives a legacy-only field arrangement
- **THEN** it SHALL return a protocol error rather than normalizing the legacy shape

### Requirement: Pinned provider-wire HTTP request contract

The strict handler SHALL be path-agnostic and SHALL expose constants for mounting at `/language-model` and reading the exact headers `ai-language-model-id`, `ai-language-model-streaming`, and `ai-language-model-specification-version`. It SHALL accept only `POST`, require a non-empty model ID, require specification version `4`, require streaming to be exactly `true` or `false`, and require `Content-Type` to parse as `application/json` with optional parameters.

After decoding, the adapter SHALL construct `runtime.GatewayCall` with `ProtocolLanguageModelV4`, the exact requested public model ID, provider call options, separately parsed gateway options, and host-supplied trusted metadata. It SHALL extract `providerOptions.gateway` into gateway options and remove it from provider-bound options.

#### Scenario: Stock gateway generate request

- **WHEN** `@ai-sdk/gateway@4.0.33` sends a generate request
- **THEN** the handler SHALL accept its path, headers, content type, and V4 body and dispatch a runtime generate call

#### Scenario: Stock gateway stream request

- **WHEN** the pinned client sends the same endpoint with streaming header `true`
- **THEN** the handler SHALL accept the request and dispatch a runtime stream call

#### Scenario: Missing content type is rejected

- **WHEN** a request omits `Content-Type`
- **THEN** the strict handler SHALL return a safe non-retryable HTTP 415 JSON error before runtime resolution

#### Scenario: Invalid request bypasses runtime

- **WHEN** method, required headers, media type, body limit, or strict DTO decoding fails
- **THEN** the handler SHALL not invoke call policy, resolve a model, or invoke the runtime

#### Scenario: Gateway controls are separated

- **WHEN** a valid request contains the registered `providerOptions.gateway` namespace
- **THEN** the adapter SHALL validate it into `GatewayCall.GatewayOptions` and provider call options SHALL no longer contain that namespace

#### Scenario: Trusted metadata comes from the host

- **WHEN** a configured metadata extractor runs after host authentication
- **THEN** its authenticated attributes SHALL populate immutable `GatewayCall.CallMetadata`, while request-body claims and provider headers remain untrusted

#### Scenario: Request ID is always present

- **WHEN** the trusted metadata extractor does not provide a request ID
- **THEN** the handler SHALL use its configurable request-ID generator and invoke policy only with a non-empty ID

### Requirement: Standards-consistent Accept negotiation

An omitted `Accept` header SHALL permit the selected response representation. When present, `Accept` SHALL authorize `application/json` for unary mode or `text/event-stream` for streaming mode through an exact media type, matching type wildcard, or `*/*` with quality greater than zero. Parameters SHALL be parsed, `q=0` SHALL reject that range, malformed ranges SHALL not match, and empty comma-separated entries SHALL not match.

#### Scenario: Positive wildcard is accepted

- **WHEN** a stream request supplies `Accept: text/*;q=0.5`
- **THEN** content negotiation SHALL succeed

#### Scenario: Zero quality is rejected

- **WHEN** every otherwise compatible range has `q=0`
- **THEN** the handler SHALL return safe HTTP 406 JSON before runtime invocation

#### Scenario: Empty entry is not permissive

- **WHEN** `Accept` contains only empty or incompatible entries
- **THEN** the handler SHALL return HTTP 406 rather than inheriting the legacy permissive behavior

### Requirement: Strict service transport limits

The strict handler SHALL default to an 8 MiB request-body read limit, 16 MiB encoded unary-success transport limit, and 8 MiB encoded complete-SSE-event transport limit. It SHALL provide positive construction options for each limit and a positive streaming idle timeout whose default is 60 seconds. A request at the limit SHALL be accepted and limit-plus-one SHALL fail.

Request reads SHALL be bounded before unbounded allocation. Unary results and events SHALL be fully encoded, validated, and size-checked before their bytes are committed or written. The result/event limits SHALL be documented as encoded transport and commitment guarantees, not as guarantees that encoding never allocates the rejected value. Complete SSE event size SHALL count exactly the bytes in `data: `, canonical JSON, and terminating `\n\n`; the Grafana strict client SHALL use the same accounting.

#### Scenario: Oversized request is rejected before resolution

- **WHEN** a request body exceeds the configured limit by one byte
- **THEN** the handler SHALL return safe non-retryable HTTP 413 and SHALL not call the runtime

#### Scenario: Oversized unary result is rejected before success commit

- **WHEN** canonical encoding of a generate result exceeds the configured success limit
- **THEN** the handler SHALL return a safe non-retryable HTTP 500 JSON failure rather than a partial HTTP 200 result
- **AND** the limit SHALL NOT be described as preventing allocation of the complete encoded value

#### Scenario: Oversized stream event is bounded

- **WHEN** canonical encoding of one stream part exceeds the configured event limit
- **THEN** the handler SHALL cancel the runtime invocation, SHALL not write that oversized event, and SHALL treat the deterministic size failure as non-retryable

#### Scenario: Server and strict client count the same SSE bytes

- **WHEN** one event contains canonical JSON of a known size
- **THEN** both sides SHALL count the `data: ` prefix, JSON bytes, and final `\n\n`, producing the same at-limit and limit-plus-one result

#### Scenario: Invalid limit option is rejected

- **WHEN** handler construction receives a zero or negative byte limit or idle timeout
- **THEN** construction SHALL return an error

### Requirement: Strict service exposes only allowlisted response metadata

The strict handler SHALL NOT serialize provider request bodies, backend response headers/bodies, stream setup headers, or resolved provider/model identity into public unary or stream results. Unary and response-metadata parts MAY expose response ID and timestamp. They SHALL omit LanguageModelV4 `response.modelId` when backend identity is private and MUST NOT substitute canonical catalog identity because that field means the model actually used. Provider warnings and provider metadata defined by the V4 contract remain available. Raw stream parts MAY be emitted only when the caller explicitly requests raw chunks and call policy permits that exposure.

The bidirectional codec MAY represent the full canonical V4 metadata shape when used as a library; the strict service adapter SHALL apply this privacy allowlist before encoding public output.

#### Scenario: Backend transport details are omitted

- **WHEN** a provider result contains request body, response headers, response body, backend provider, and backend model identity
- **THEN** none of those values SHALL appear in the strict public response

#### Scenario: Private model identity is omitted without semantic substitution

- **WHEN** runtime canonical model ID differs from a private resolved model-reported ID
- **THEN** strict V4 response metadata SHALL omit `modelId` rather than expose the resolved ID or substitute the semantically different canonical alias

#### Scenario: Raw chunks require explicit policy approval

- **WHEN** a request enables raw chunks
- **THEN** call policy SHALL be able to reject that provider-bound data exposure before resolution, and the handler SHALL emit raw parts only when the request and policy both permit it

### Requirement: Runtime-backed unary dispatch

For streaming header `false`, the strict handler SHALL call the shared runtime generate operation once with the normalized `GatewayCall`. It SHALL encode a successful non-nil result through the public metadata allowlist as canonical V4 JSON before committing HTTP 200 with `Content-Type: application/json`. Runtime or encoding failures before commitment SHALL use the safe classified JSON envelope.

#### Scenario: Successful unary result

- **WHEN** runtime generate succeeds
- **THEN** the response SHALL preserve ordered content, finish reason, usage, warnings, provider metadata, and allowlisted response ID/timestamp
- **AND** it SHALL omit provider request and backend transport metadata

#### Scenario: Runtime failure is safe

- **WHEN** runtime generate returns a provider error containing private backend data
- **THEN** the handler SHALL return the mapped safe error envelope without any private field or message

### Requirement: Runtime-backed SSE dispatch and flushing

For streaming header `true`, the strict handler SHALL create one runtime stream invocation before committing success. It SHALL emit each supported part as one `data: <canonical-json>\n\n` event in received order, use no SSE `event:` field, and close cleanly without `[DONE]` when the runtime ends successfully. It SHALL set `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-transform`, and `X-Accel-Buffering: no`; it MUST NOT set `Connection: keep-alive`.

The handler SHALL use `http.ResponseController` to flush immediately after successful stream commitment and after every complete event. A response-writer wrapper MAY expose flush support through `Unwrap`. Unsupported or failed flush and event write failures SHALL cancel the runtime invocation and terminate without attempting another event on that writer.

#### Scenario: Wrapper exposes flushing

- **WHEN** middleware wraps a flusher-capable response writer and implements `Unwrap`
- **THEN** initial headers and every event SHALL be flushed through `http.ResponseController`

#### Scenario: Clean stream completion

- **WHEN** the runtime parts channel closes and `Wait()` returns nil
- **THEN** the HTTP response SHALL end without a synthetic part or `[DONE]`

#### Scenario: Write failure cancels model work

- **WHEN** writing or flushing an event fails
- **THEN** the handler SHALL cancel the runtime invocation with the failure cause and SHALL not attempt a second write

### Requirement: Error stream parts are safe repeatable data

Every provider-emitted `PartError` SHALL be converted to a safe canonical V4 `error` part using gateway failure classification before it is written. Provider error detail MUST NOT cross the service boundary. An error part SHALL NOT terminate forwarding; later error, content, metadata, usage, and finish parts SHALL continue in order until a separate runtime or adapter termination condition occurs.

#### Scenario: Error is followed by finish

- **WHEN** the provider emits `PartError` followed by `PartFinish` containing usage and metadata
- **THEN** the client SHALL receive a safe error part followed by the canonical finish part with usage and metadata preserved

#### Scenario: Multiple provider errors continue

- **WHEN** the provider emits multiple error parts and then more content
- **THEN** every safe error and later content part SHALL be forwarded in order

#### Scenario: Error details are removed

- **WHEN** `PartError.APICallError` contains provider URL, request values, response headers/body, and data
- **THEN** its V4 event SHALL contain only the safe message, type, status, retryability, safe parameter fields, and a safe structured category copy in `data` for Grafana normalization
- **AND** the safe `data` value SHALL NOT copy the provider's original data

### Requirement: Commit-aware lifecycle errors

A runtime or idle-timeout failure before SSE commitment SHALL be a safe non-2xx JSON response. After commitment, timeout or cancellation SHALL be represented by one best-effort safe canonical V4 error event when the writer remains usable, then the response SHALL end. A write or flush failure SHALL not trigger a best-effort error event on the failed writer. The idle timer SHALL run only while the handler is waiting for the next runtime part after the preceding event has been written and flushed successfully; synchronous write time SHALL not count as provider idle time, and host HTTP write deadlines SHALL govern a blocked writer.

#### Scenario: Pre-stream failure remains JSON

- **WHEN** runtime stream creation fails before a valid invocation is available
- **THEN** the handler SHALL return the mapped non-2xx JSON error rather than HTTP 200 SSE

#### Scenario: Idle timeout after commitment

- **WHEN** the handler is waiting for the next runtime part and none arrives within the configured idle interval
- **THEN** the handler SHALL cancel the runtime invocation, emit one safe timeout error event marked retryable when possible, and terminate

#### Scenario: Synchronous write time is not provider idle time

- **WHEN** writing or flushing the current event has not returned
- **THEN** the idle timer SHALL be paused and SHALL NOT classify the writer as an idle provider

#### Scenario: Total timeout after commitment

- **WHEN** runtime `Wait` reports total timeout after SSE commitment
- **THEN** the handler SHALL make one best-effort safe timeout error event and terminate

### Requirement: Exact-baseline compatibility evidence

The repository SHALL test the strict service against the exact registered npm baseline and SHALL not substitute upstream `main`. Evidence SHALL include bidirectional canonical JSON goldens for every supported request, generate-content, file-data, tool-output, and stream-part discriminator; successful stock-TypeScript unary output for fields the pinned client preserves; strict service assertions for allowlisted public metadata and omitted backend transport detail; streaming text, tools, files, sources, errors, metadata, and usage; Grafana strict-codec calls; client-observed error/retry matrices; malformed and legacy rejection; policy-before-resolution; middleware context; resource limits; and flushing through wrapped writers.

#### Scenario: Successful stock-client unary coverage

- **WHEN** the interop suite calls direct `doGenerate` through `@ai-sdk/gateway@4.0.33`
- **THEN** it SHALL verify content, tool calls/results, files, sources, finish reason, provider metadata, and usage from one successful canonical result
- **AND** it SHALL verify the pinned client supplies its own request body, response headers/body, and local warnings rather than claiming those values came from the server

#### Scenario: Strict public metadata coverage

- **WHEN** a provider result with warnings and backend request/response metadata is inspected through raw HTTP and Grafana strict-mode tests
- **THEN** the tests SHALL verify warnings and allowlisted response ID/timestamp
- **AND** they SHALL verify omission of provider request body, backend response headers/body, and resolved backend identity

#### Scenario: Tool-result file input coverage

- **WHEN** the stock client sends a tool-result content value containing a canonical file data union
- **THEN** the strict server SHALL decode the exact file semantics and the interop assertion SHALL observe them

#### Scenario: Existing and strict handlers share canonical success clients

- **WHEN** canonical TypeScript and Grafana success and `PartError`-continuation scenarios run through a dual-handler harness
- **THEN** they SHALL pass against both handlers while legacy-only payload tests remain accepted only by `gateway/providerwire`
- **AND** the strict Grafana-to-strict-handler path SHALL use only V4 request and response codecs

#### Scenario: Error assertions remain handler specific

- **WHEN** error scenarios run against both handlers
- **THEN** strict-handler assertions SHALL require typed redacted errors while legacy-handler assertions SHALL preserve its existing `APICallError` fields, legacy shapes, and private-detail behavior

#### Scenario: Legacy HTTP behavior is frozen

- **WHEN** focused tests call the legacy handler with omitted `Content-Type`, permissive `Accept` values including `q=0`, or streaming mode
- **THEN** its existing acceptance behavior, error shape, and `Connection: keep-alive` response header SHALL remain unchanged

#### Scenario: Gateway policy executes before resolution

- **WHEN** a canonical request includes prohibited provider headers or unsupported gateway routing controls
- **THEN** interop tests SHALL observe safe rejection before catalog resolution

#### Scenario: Parity commands pass

- **WHEN** the change is complete
- **THEN** `mise run validate-parity-baseline`, focused provider-wire and Grafana tests, `mise run test-interop`, and `mise run parity-check` SHALL pass without invented provider fixture inputs
