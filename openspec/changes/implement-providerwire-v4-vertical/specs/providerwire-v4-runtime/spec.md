## ADDED Requirements

### Requirement: Isolated strict ProviderWire V4 package
The repository SHALL provide `gateway/providerwire/v4` as a strict ProviderWire V4 HTTP adapter. The package SHALL own private request and response wire representations, strict validation, explicit domain mapping, safe error rendering, SSE framing, handler lifecycle, and only protocol-relative route, header, MIME, and construction APIs required by hosts or clients. It SHALL consume transport-neutral `provider` values and `gateway/catalog.ModelResolver`; it SHALL NOT import authlib, service configuration, a router, concrete providers, or legacy ProviderWire codecs.

#### Scenario: Host mounts the relative route
- **WHEN** a host mounts the V4 handler under a host-owned prefix
- **THEN** the package SHALL expose `/language-model` as the relative route and SHALL NOT prepend `/api/v1/aisdk`

#### Scenario: Strict package dependencies are inspected
- **WHEN** imports and public types under `gateway/providerwire/v4` are inspected
- **THEN** they SHALL contain no authlib, service YAML, router, concrete provider, legacy wire DTO, or service-prefix dependency

#### Scenario: Legacy dialect input is sent to V4
- **WHEN** a request is valid only under the tolerant legacy ProviderWire dialect
- **THEN** the V4 handler SHALL reject it rather than select a tolerant mode

### Requirement: Configurable protocol-local limits and timeouts
Handler construction SHALL require a non-nil `catalog.ModelResolver` and SHALL expose positive configuration for maximum request-body bytes, unary-response bytes, error-response bytes, complete SSE-event bytes, total model-call timeout, and streaming idle timeout. Omitted configuration SHALL use named documented defaults. Request and unary limits SHALL only require positive values. The error-response limit SHALL fit the complete canonical JSON fallback, while the event limit SHALL separately fit the larger complete terminal-error SSE fallback frame. Complete event size SHALL include the `data: ` prefix, encoded JSON, and terminating `\n\n`. Invalid or nil options and insufficient fallback limits SHALL make construction fail.

#### Scenario: Defaults are selected
- **WHEN** a host constructs a handler without limit or timeout overrides
- **THEN** named positive defaults SHALL govern every request, response, event, and model-call lifecycle

#### Scenario: Host overrides limits
- **WHEN** a host supplies positive request, unary, error, event, total-timeout, or idle-timeout values
- **THEN** the handler SHALL apply those values protocol-locally

#### Scenario: Invalid construction input
- **WHEN** the resolver is nil, an option is nil, or a configured value is non-positive
- **THEN** construction SHALL fail and return no handler

#### Scenario: Fallback limit is insufficient
- **WHEN** the error-response limit cannot hold the canonical JSON fallback or the event limit cannot hold the complete framed terminal SSE fallback
- **THEN** construction SHALL fail while an otherwise positive request or unary limit SHALL remain valid

### Requirement: Mandatory strict HTTP envelope
The V4 handler SHALL accept only `POST` requests with exactly one non-empty `ai-language-model-id`, exactly one `ai-language-model-specification-version` equal to `4`, exactly one `ai-language-model-streaming` equal to `true` or `false`, and exactly one `Content-Type` value equal to bare `application/json` without media-type parameters. This strict content-type form matches the exact pinned client and SHALL remain the Phase 5 Go-client emission contract rather than being weakened for unobserved variants. The handler SHALL preserve the model ID as received rather than trimming or rewriting it. Additional host, authentication, Gateway, observability, user-agent, and custom headers SHALL NOT be rejected merely because they are not protocol inputs. Envelope failures SHALL return a safe non-2xx JSON error before reading the model body, policy, resolution, or invocation.

#### Scenario: Unary envelope is accepted
- **WHEN** a `POST` request carries the required single headers with streaming `false` and JSON content type
- **THEN** the handler SHALL continue to bounded body processing in unary mode

#### Scenario: Streaming envelope is accepted
- **WHEN** a `POST` request carries the required single headers with streaming `true` and JSON content type
- **THEN** the handler SHALL continue to bounded body processing in streaming mode

#### Scenario: Required header is duplicated
- **WHEN** any required language-model header has zero or multiple values
- **THEN** the handler SHALL reject the envelope before body mapping or policy

#### Scenario: Content type is absent or altered
- **WHEN** `Content-Type` is absent, duplicated, malformed, names another media type, or adds parameters
- **THEN** the handler SHALL reject the envelope before body mapping or policy

#### Scenario: Additional host header is present
- **WHEN** the envelope also carries a Gateway, authentication, observability, user-agent, or custom header
- **THEN** the handler SHALL leave that header to the host and SHALL NOT reject or forward it as a provider call option merely because it is present

### Requirement: Bounded duplicate-aware JSON and normative schema validation
After envelope validation, the handler SHALL read at most the configured body limit and SHALL close the body. Before tokenization or semantic normalization it SHALL reject a body for which `utf8.Valid` is false. It SHALL then accept exactly one JSON value and reject duplicate member names within any one object, comments, trailing commas, invalid number syntax, and trailing non-whitespace data while allowing equal names in separate object scopes. The resulting semantic value SHALL validate against the complete normative draft 2020-12 request schema. The production handler and exact-pinned evidence workspace SHALL consume the same committed request-schema bytes.

#### Scenario: Valid request reaches subset mapping
- **WHEN** a bounded body is valid strict JSON and satisfies the normative request schema
- **THEN** syntax validation and schema validation SHALL complete in that order before subset mapping

#### Scenario: Invalid UTF-8 is rejected
- **WHEN** a raw JSON string contains invalid UTF-8 bytes
- **THEN** the handler SHALL reject the body before tokenization rather than replace bytes with `U+FFFD`

#### Scenario: Nested duplicate is rejected
- **WHEN** any nested object repeats a member name within that same object
- **THEN** the handler SHALL return a safe invalid-request error before semantic decoding can collapse the duplicate

#### Scenario: Equal names in separate scopes are accepted
- **WHEN** separate nested objects each use the same member name once
- **THEN** duplicate validation SHALL accept those names and continue to schema validation

#### Scenario: Trailing data is rejected
- **WHEN** a valid JSON value is followed by another value or non-whitespace data
- **THEN** the handler SHALL reject the body before schema validation or mapping

#### Scenario: Body exceeds its limit
- **WHEN** reading reveals more than the configured maximum request bytes
- **THEN** the handler SHALL return a bounded safe invalid-request error without policy, resolution, or invocation

#### Scenario: Schema-invalid body is rejected
- **WHEN** strict JSON contains an unknown finite member, unknown discriminator, inactive-arm member, invalid null, or role-incompatible content
- **THEN** the handler SHALL reject it before subset mapping or provider invocation

#### Scenario: Schema authority is single-sourced
- **WHEN** runtime and evidence checks load the normative request schema
- **THEN** both SHALL read the same production-owned committed file and no separately maintained copy SHALL exist

### Requirement: Explicit Phase 3 text request mapping
After complete schema validation, the handler SHALL explicitly map only ordered system messages, user messages containing text parts, assistant messages containing text parts, and the call settings `maxOutputTokens`, `temperature`, `topP`, `topK`, `presencePenalty`, `frequencyPenalty`, `stopSequences`, `seed`, and `reasoning` into `provider.CallOptions`. Mapping SHALL preserve required empty text, prompt order, absence versus explicit zero, exact representable language-model numbers, and nil versus explicitly empty stop sequences. It SHALL NOT use the generic JSON representation of `provider.CallOptions` as protocol authority.

#### Scenario: Text prompt is mapped
- **WHEN** a schema-valid prompt contains system string content and ordered user or assistant text parts, including empty text
- **THEN** the model SHALL receive equivalent role, part order, text values, and required empties

#### Scenario: Scalar presence is mapped
- **WHEN** a supported optional scalar is absent, explicitly zero, or another valid value
- **THEN** the resulting provider option SHALL preserve that distinct state

#### Scenario: Empty stop sequences are mapped
- **WHEN** `stopSequences` is present as an empty array
- **THEN** the model SHALL receive a non-nil empty slice rather than absence

#### Scenario: Exact number cannot be represented
- **WHEN** a schema-valid request number cannot be represented by the corresponding provider domain field
- **THEN** mapping SHALL fail safely before policy or resolution rather than round or reinterpret it silently

### Requirement: Unsupported valid V4 features fail closed
The Phase 3 handler SHALL reject any schema-valid request that contains a tool or tool choice, tool-role message, non-text user or assistant content, response format, raw-chunk selection, body-carried headers, provider options, or message/part provider options. Presence SHALL be rejected even when the unsupported value is false, empty, or otherwise behavior-neutral. The handler SHALL NOT silently discard, concatenate, normalize, or forward these values.

#### Scenario: Unsupported union arm is present
- **WHEN** a schema-valid request contains a file, reasoning content, custom content, tool call, tool result, approval, tool, tool choice, or structured response format
- **THEN** the handler SHALL return a safe invalid-request error before policy or resolution

#### Scenario: Empty unsupported collection is present
- **WHEN** a request explicitly contains empty `tools`, `headers`, or `providerOptions`
- **THEN** the handler SHALL reject the unsupported presence rather than collapse it to absence

#### Scenario: Explicit false raw selection is present
- **WHEN** a request explicitly contains `includeRawChunks: false`
- **THEN** the handler SHALL reject it as outside the Phase 3 subset rather than silently omit its presence

### Requirement: Policy and canonical catalog resolution ordering
The handler SHALL expose an optional protocol-neutral host-policy seam that receives the request context, requested public model ID, call mode, and normalized `provider.CallOptions` without `*http.Request` or ProviderWire DTOs. Policy implementations SHALL be trusted internal code and contractually SHALL NOT mutate or retain aliased slices, maps, pointers, or raw JSON; the handler SHALL provide no defensive-copy or physically read-only guarantee. The handler SHALL invoke policy at most once after validation and mapping, then invoke `catalog.ModelResolver.ResolveModel` at most once with the same request context and requested ID. It SHALL invoke only the non-nil `ResolvedModel.Model` and SHALL retain `ResolvedModel.ID` as canonical public identity.

#### Scenario: Policy rejects a mapped request
- **WHEN** host policy returns a safe failure
- **THEN** the handler SHALL render that failure without resolving or invoking a model

#### Scenario: Alias resolves canonically
- **WHEN** the requested model ID is an alias and the resolver returns a different canonical `ResolvedModel.ID`
- **THEN** the handler SHALL invoke the returned model once and use the canonical ID in normalized output

#### Scenario: Invalid input bypasses policy and resolution
- **WHEN** envelope, body, syntax, schema, subset, or mapping validation fails
- **THEN** neither policy nor catalog resolution SHALL run

#### Scenario: Resolver returns nil model
- **WHEN** resolution succeeds with a nil model or empty canonical ID
- **THEN** the handler SHALL return a safe internal failure without model invocation

### Requirement: Boundary reduction to safe failures
The V4 handler SHALL reduce validation failures, policy failures, catalog unknown-model errors, other resolver failures, provider call failures, invalid provider results, timeouts, and cancellations to `gateway/failure.Failure` before rendering. It SHALL use fixed safe messages for rich resolver and provider errors and SHALL never copy their raw message, URL, bodies, headers, data, provider identity, or backend model ID. Request cancellation or deadline SHALL take precedence whenever observed during body reading, policy, resolution, or provider execution. Otherwise, a catalog error matching `catalog.ErrUnknownModel` SHALL become not found and other resolver errors SHALL become internal failure. Provider-call context cancellation SHALL become cancellation, provider-call context deadlines and provider HTTP 408 or 504 SHALL become timeout, provider HTTP 429 SHALL become rate limit, and provider HTTP 503 SHALL become overload. After those cases, a retryable `provider.APICallError` SHALL become upstream failure and a non-retryable `provider.APICallError` SHALL become failed dependency. Other provider failures SHALL become upstream failure. Backend authentication, permission, and model-not-found statuses SHALL therefore not be exposed as caller classifications, while permanent backend failures SHALL remain non-retryable.

#### Scenario: Unknown public model is reduced
- **WHEN** catalog resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** the handler SHALL render a non-retryable not-found failure without listing catalog entries

#### Scenario: Rich provider error is reduced
- **WHEN** a provider call returns an error carrying backend URL, headers, body, data, provider identity, or backend model ID
- **THEN** the public response SHALL contain only the mapped category, approved safe message, and V4 error fields

#### Scenario: Permanent provider error is reduced
- **WHEN** a provider returns a non-retryable `provider.APICallError` outside the timeout, rate-limit, and overload special cases
- **THEN** the handler SHALL render failed dependency with HTTP 424 rather than a retryable upstream failure

#### Scenario: Arbitrary resolver error is reduced
- **WHEN** catalog resolution returns an error that is not an unknown public model and request context remains active
- **THEN** the handler SHALL render a fixed safe internal failure and SHALL not copy the resolver message

#### Scenario: Resolver observes request cancellation
- **WHEN** request cancellation or deadline is observable while resolution returns
- **THEN** cancellation or timeout SHALL take precedence over internal resolver-failure classification

### Requirement: Complete ProviderWire V4 safe error mapping
Before success commitment, every safe failure SHALL be encoded as bounded `application/json` with the closed shape `{"error":{"message":string,"type":string,"param":null,"code":string}}`. The V4 mapping SHALL be exactly:

- invalid request: HTTP 400, `invalid_request_error`, `invalid_request`;
- authentication: HTTP 401, `authentication_error`, `authentication_error`;
- permission: HTTP 403, `forbidden`, `forbidden`;
- not found: HTTP 404, `model_not_found`, `model_not_found`;
- rate limit: HTTP 429, `rate_limit_exceeded`, `rate_limit_exceeded`;
- overload: HTTP 503, `internal_server_error`, `overloaded`;
- failed dependency: HTTP 424, `failed_dependency`, `failed_dependency`;
- upstream failure: HTTP 502, `internal_server_error`, `upstream_error`;
- timeout: HTTP 504, `internal_server_error`, `timeout`;
- cancellation: HTTP 499, `internal_server_error`, `canceled`;
- internal failure: HTTP 500, `internal_server_error`, `internal_error`.

The encoder SHALL validate the safe-failure value and the complete error before commitment. An invalid or zero failure, an approved message that exceeds the error limit, or an encoding failure SHALL use the canonical internal fallback. The status SHALL make the pinned Gateway client derive the same retryability carried by the safe failure.

#### Scenario: Every category is rendered
- **WHEN** each safe category is passed to the V4 error encoder
- **THEN** status, type, code, retryability through the pinned client, content type, and schema-valid bytes SHALL match the table

#### Scenario: Authentication message is asserted at the server boundary
- **WHEN** authentication failure rendering is tested
- **THEN** raw response bytes SHALL assert the server-owned message while pinned-client assertions SHALL cover class, status, and retryability without requiring that original message

#### Scenario: Zero safe failure is rendered
- **WHEN** an invalid or zero safe-failure value reaches the encoder
- **THEN** it SHALL emit the canonical internal failure rather than an empty or unknown envelope

#### Scenario: Error output is bounded
- **WHEN** an otherwise safe message would exceed the configured complete error limit
- **THEN** the handler SHALL emit the bounded canonical internal fallback rather than truncate JSON or expose the original message

#### Scenario: Private fields are absent
- **WHEN** a rendered error body is decoded
- **THEN** it SHALL contain no URL, request or response body, headers, credentials, raw cause, provider identity, backend model ID, provider data, or extra envelope member

### Requirement: Bounded unary text success
For unary mode, the handler SHALL invoke `DoGenerate` exactly once with a context derived from the request and the mapped options. A successful result SHALL contain only text content, a supported finish reason, and representable usage for this vertical. The explicit encoder SHALL preserve text order and empty text, emit the required `warnings` member as an empty array, reject non-empty provider warnings, omit raw usage, provider metadata, request bodies, response bodies and headers, provider identity, backend model IDs, and set normalized `response.modelId` to the canonical public catalog ID. It SHALL schema-validate and bound the complete JSON before committing HTTP 200 `application/json`.

#### Scenario: Unary text succeeds
- **WHEN** the resolved model returns text content, a valid finish reason, and usage within the configured limit
- **THEN** the pinned client SHALL receive equivalent ordered text, finish reason, and usage
- **AND** raw HTTP bytes plus the local response schema and golden SHALL prove the server emitted canonical public model identity and `warnings: []`

#### Scenario: Empty unary text is preserved
- **WHEN** a valid result contains a text part whose text is empty
- **THEN** the encoded content SHALL retain that part and required empty string

#### Scenario: Unary warnings are explicit and bounded
- **WHEN** a result has no provider warnings
- **THEN** the encoded result SHALL contain `warnings: []` as required by the registered LanguageModelV4 result type

#### Scenario: Backend metadata is normalized
- **WHEN** a result contains provider metadata, raw usage, request or response bodies, response headers, provider identity, or a backend model ID
- **THEN** those values SHALL be absent and any emitted model identity SHALL equal `ResolvedModel.ID`

#### Scenario: Unary result is invalid or oversized
- **WHEN** the model returns nil, non-text content, unsupported warnings, an invalid finish or usage value, unencodable output, or output over the unary limit
- **THEN** the handler SHALL return a bounded safe non-2xx failure without committing HTTP 200

### Requirement: Normalized minimal bounded text SSE lifecycle
For streaming mode, the handler SHALL invoke `DoStream` exactly once and SHALL keep failures returned before a non-nil `StreamResult` with a non-nil channel as non-2xx JSON. That non-nil result and channel SHALL be the success-commitment boundary. Immediately after commitment, the handler SHALL emit exactly one server-owned `stream-start` with `warnings: []`. A provider `stream-start` SHALL be validated and consumed without a second emitted start, and an absent provider start SHALL be valid. A provider start with non-empty warnings SHALL produce a terminal safe error. Empty warnings carried on a provider finish SHALL be ignored as a Go representation detail; non-empty finish warnings SHALL produce a terminal safe error.

After start normalization, the handler SHALL accept at most one optional `response-metadata`, zero or more sequential text start/delta/end blocks with matching non-empty IDs, and exactly one `finish`. It SHALL preserve empty deltas, replace response metadata model identity with `ResolvedModel.ID`, and remove provider identity, response headers, and backend model IDs. Unsupported parts, non-empty warnings, or invalid lifecycle transitions SHALL fail safely rather than pass through legacy serialization. An invalid first channel part SHALL be a post-commit SSE failure, not a non-2xx response.

#### Scenario: Provider emits stream start
- **WHEN** the provider emits an empty-warning stream start followed by valid text lifecycle parts and finish
- **THEN** the handler SHALL consume that start and the client SHALL receive exactly one server-owned stream start

#### Scenario: Provider omits stream start
- **WHEN** the provider begins with response metadata or text lifecycle parts and later emits finish
- **THEN** the handler SHALL accept the stream and the client SHALL still receive exactly one server-owned stream start

#### Scenario: Provider start warnings are non-empty
- **WHEN** a provider stream start carries one or more warnings
- **THEN** the handler SHALL emit a terminal safe error rather than drop the warnings or emit a second start

#### Scenario: Finish warnings reflect Go representation
- **WHEN** a provider finish carries an empty warnings slice
- **THEN** the handler SHALL ignore that empty representation detail and emit the normalized finish
- **AND** a non-empty finish warnings slice SHALL instead produce a terminal safe error

#### Scenario: Empty delta is preserved
- **WHEN** the provider emits a text delta whose delta is empty
- **THEN** the event SHALL retain the required `delta: ""` member

#### Scenario: Stream response identity is canonical
- **WHEN** response metadata carries a backend provider or model identity
- **THEN** the emitted part SHALL omit provider identity and headers and SHALL use the canonical public catalog ID as `modelId`

#### Scenario: Unsupported stream part is observed
- **WHEN** the provider emits a reasoning, tool, file, source, raw, custom, approval, or other unsupported part
- **THEN** the handler SHALL reduce it to a bounded safe stream error and terminate rather than forwarding it

#### Scenario: Stream setup fails
- **WHEN** `DoStream` returns an error, nil result, or nil channel before success commitment
- **THEN** the handler SHALL return a non-2xx safe JSON error rather than HTTP 200 SSE

#### Scenario: First channel part is invalid
- **WHEN** a non-nil stream channel first yields an invalid or unsupported part
- **THEN** HTTP 200 SSE SHALL already be committed and the handler SHALL emit a terminal safe error rather than replace it with a non-2xx response

### Requirement: Complete-event SSE framing, headers, and clean EOF
On successful stream commitment, the handler SHALL write HTTP 200 with `Content-Type: text/event-stream` and `Cache-Control: no-cache, no-transform`; it SHALL NOT require or set `Connection: keep-alive` as a protocol invariant. Each server-owned or mapped Phase 3 stream part SHALL be mapped to a private DTO, schema-validated, encoded as one complete `data: <json>\n\n` event no larger than the configured event limit, and flushed after a successful write when `http.Flusher` is available. The size SHALL include the complete frame prefix and delimiters. The handler SHALL never emit `[DONE]`. A valid finish followed by provider channel close SHALL end at clean EOF without a synthetic event. After commitment, a provider, lifecycle, timeout, or cancellation failure SHALL produce at most one bounded event of the closed shape `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":number,"retryable":boolean}}` when the writer remains usable and then terminate. Its message, type, code, status, and retryability SHALL use the same safe-failure mapping as a pre-commit error. An observable encoding or write failure SHALL cancel the model context and SHALL NOT attempt another write on the failed writer.

#### Scenario: SSE success headers are committed
- **WHEN** `DoStream` returns a non-nil result and channel
- **THEN** the handler SHALL commit HTTP 200 with event-stream content type and no-cache/no-transform policy before the server-owned start
- **AND** it SHALL not require a connection-specific keep-alive header

#### Scenario: Event is framed and flushed
- **WHEN** a valid part is emitted and the writer supports flushing
- **THEN** exactly one complete JSON `data:` event SHALL be written and flushed before the next provider part is awaited

#### Scenario: Event exceeds its limit
- **WHEN** a complete mapped event exceeds the configured event limit
- **THEN** the original event SHALL not be partially written and the handler SHALL terminate with one bounded safe error event when possible

#### Scenario: Terminal safe error is consumed
- **WHEN** a safe failure occurs after SSE commitment and the writer remains usable
- **THEN** the pinned client SHALL receive exactly one `error` part with the closed safe payload, mapped status, and retryability and no backend detail

#### Scenario: Finish ends with clean EOF
- **WHEN** a finish event is written and the provider closes the channel
- **THEN** the response SHALL close without `[DONE]` or another synthetic event

#### Scenario: Writer fails
- **WHEN** event encoding or writing returns an error after commitment
- **THEN** the model context SHALL be canceled and the handler SHALL stop without attempting a second event on that writer

### Requirement: Request cancellation and basic timeout enforcement
The model context SHALL derive from the request context. Request cancellation or deadline SHALL take classification precedence whenever observed during body reading, policy, resolution, or provider execution. The configured total timeout SHALL begin after successful policy and catalog resolution and cover model invocation and stream consumption. The idle timeout SHALL begin only after a stream is established, cover the first provider part after the server-owned start, and reset after every successfully written provider-derived event. Pre-commit cancellation or timeout SHALL render the corresponding safe non-2xx error when writable; post-commit cancellation or timeout SHALL cancel the model and make one best-effort bounded safe error event.

#### Scenario: Unary total timeout occurs
- **WHEN** unary model invocation exceeds the configured total timeout
- **THEN** the model context SHALL be canceled and a retryable safe timeout SHALL be returned before commitment when possible

#### Scenario: Stream becomes idle
- **WHEN** an established stream produces no next part within the idle timeout
- **THEN** the model context SHALL be canceled and a retryable bounded timeout error event SHALL be attempted

#### Scenario: Activity resets idle timeout
- **WHEN** each valid event is written within the configured idle window
- **THEN** the idle timer SHALL reset and SHALL not expire solely because total stream age exceeds one idle interval

#### Scenario: Consumer cancels
- **WHEN** the request context is canceled during body reading, policy, resolution, or provider execution
- **THEN** cancellation SHALL take precedence over stage-specific errors, propagate promptly where applicable, and be reduced to a non-retryable cancellation failure

### Requirement: Exact-pinned runtime interoperability with independent response authority
Deterministic tests SHALL exercise the strict handler through direct unary and streaming calls from exactly `@ai-sdk/gateway@4.0.52` with provider types from the registered baseline. The test host SHALL mount the relative route under a test-owned prefix and use a deterministic catalog containing a canonical ID and alias. Tests SHALL assert request mapping, unary text, normalized server-owned stream start with and without provider start parts, streaming canonical identity, finish, safe pre-stream error consumption, one safe post-commit error part, and clean EOF without `[DONE]`. Raw HTTP bytes plus local response schemas and goldens SHALL prove unary `warnings: []`, unary canonical `response.modelId`, response headers, and omission of `[DONE]` because the pinned client overwrites unary warnings and response metadata and ignores the sentinel. Local schemas, explicit encoder tests, state-machine tests, raw response bytes, and golden bytes SHALL remain the correctness authority; pinned-client success SHALL be classified only as consumption evidence. Authentication-error tests SHALL assert the server message from raw bytes because the pinned client replaces that message contextually, while client assertions SHALL cover classification, status, and retryability.

#### Scenario: Pinned client performs unary call
- **WHEN** the exact registered Gateway client calls the strict handler through an alias
- **THEN** the deterministic model SHALL receive the intended options and the client SHALL receive unary text, finish reason, and usage
- **AND** raw HTTP plus local schema and golden assertions SHALL establish the server-emitted canonical identity and empty warnings

#### Scenario: Pinned client performs streaming call
- **WHEN** the exact registered Gateway client calls streaming mode against providers that emit or omit their own stream start
- **THEN** it SHALL consume exactly one server-owned start and ordered normalized events through finish and clean EOF without requiring `[DONE]`

#### Scenario: Pinned client consumes authentication failure
- **WHEN** the exact registered Gateway client receives a safe authentication response
- **THEN** assertions SHALL verify its authentication class, HTTP status, and non-retryability
- **AND** raw response assertions rather than the contextual client message SHALL verify the server-owned safe message

#### Scenario: Permissive or normalizing client hides server behavior
- **WHEN** the pinned client accepts a malformed success, overwrites unary warnings or response metadata, or ignores `[DONE]`
- **THEN** server verification SHALL still fail through raw-byte, schema, encoder, state-machine, or golden assertions rather than claiming client observability

### Requirement: Legacy compatibility and bounded parity claims
The strict runtime SHALL leave `gateway/providerwire`, `providers/grafana` legacy mode, parent-pinned request bytes, legacy response bytes, SSE framing, and existing interop behavior unchanged. Parity reporting SHALL classify only the Phase 3 text vertical as automated strict-runtime coverage and SHALL retain explicit gaps for full request adaptation, complete response families, raw behavior, privacy hardening, and lifecycle hardening.

#### Scenario: Legacy compatibility suite runs
- **WHEN** strict V4 is added
- **THEN** the parent request corpus, legacy package tests, Grafana provider tests, and existing interop tests SHALL remain unchanged and pass

#### Scenario: Reviewer reads parity coverage
- **WHEN** the parity map describes strict ProviderWire V4 after this change
- **THEN** it SHALL identify the exact bounded vertical and SHALL NOT claim full ProviderWire V4 runtime compatibility
