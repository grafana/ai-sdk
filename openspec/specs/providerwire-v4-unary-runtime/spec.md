## Purpose

Define the production, strict, bounded ProviderWire V4 unary text runtime and its compatibility evidence.

## Requirements

### Requirement: Constructed strict unary handler

The `gateway/providerwire/v4` package SHALL provide a production HTTP handler for relative `POST /language-model` unary requests. Construction SHALL require a `catalog.ModelResolver`, SHALL accept an optional host-policy boundary, and SHALL require named positive limits for request bytes, unary response bytes, error response bytes, and total model duration. Construction SHALL reject nil dependencies and byte limits that cannot safely use `limit+1`. It SHALL reject an error-response limit too small for the canonical internal-error document. The unary-response limit SHALL have no fallback-fit requirement because oversized success encoding transitions to the separate bounded error path.

#### Scenario: Valid handler construction
- **WHEN** a caller supplies a non-nil resolver and valid named limits
- **THEN** construction SHALL return a handler whose runtime behavior is fixed by those values

#### Scenario: Unsafe limit configuration
- **WHEN** a limit is zero or negative, a byte limit overflows safe `limit+1` arithmetic, or the error-response limit cannot contain the canonical internal-error document
- **THEN** construction SHALL fail before the handler serves a request

#### Scenario: Small positive unary limit
- **WHEN** the unary-response limit is positive and supports safe `limit+1` arithmetic but cannot contain a successful response
- **THEN** construction SHALL succeed
- **AND** any oversized success SHALL transition to the separately bounded error path at runtime

### Requirement: Strict unary HTTP envelope

The unary handler SHALL accept only `POST /language-model` with JSON content and exactly one effective value for each `ai-language-model-specification-version`, `ai-language-model-id`, and `ai-language-model-streaming` header. The required values SHALL be specification version `4`, a non-empty model ID preserved without trimming or rewriting, and streaming value `false`. Additional host headers SHALL be accepted but SHALL NOT become `provider.CallOptions.Headers` automatically.

#### Scenario: Valid unary envelope
- **WHEN** a request uses the exact route and method, a JSON media type, specification version `4`, a non-empty model ID, and streaming `false`
- **THEN** envelope validation SHALL succeed and retain the exact model ID bytes represented by the header value

#### Scenario: Invalid protocol envelope
- **WHEN** the method or path is wrong, content is not JSON, a required protocol header is missing, empty, repeated, or has an invalid value, or streaming is `true`
- **THEN** the handler SHALL return a bounded invalid-request error before reading semantic request values, applying policy, resolving a model, or invoking a model

#### Scenario: Unrelated host headers are present
- **WHEN** a valid request contains additional HTTP headers
- **THEN** envelope validation SHALL accept them
- **AND** mapped provider call headers SHALL remain empty unless a future explicit body-header capability and host policy allow them

### Requirement: Bounded raw request processing

After envelope validation, the handler SHALL read and close the body through a configured `limit+1` boundary, reject over-limit input without retaining bytes beyond that boundary, and reject invalid UTF-8. JSON syntax and structural validation SHALL remain the responsibility of complete request-schema validation rather than a separate protocol-local JSON parser.

#### Scenario: Request body crosses its byte boundary
- **WHEN** a body is below, exactly at, or one byte above the configured request limit
- **THEN** the first two requests SHALL continue to schema validation and the over-limit request SHALL fail before schema validation, policy, resolution, or invocation

#### Scenario: Request body contains invalid UTF-8
- **WHEN** the bounded body is not valid UTF-8
- **THEN** raw processing SHALL reject it before schema validation instead of allowing replacement-character normalization

### Requirement: Complete schema validation precedes mapping

The handler SHALL compile the embedded `gateway/providerwire/v4/schema/request.json` draft 2020-12 schema during construction and SHALL validate the complete bounded request using the existing shared schema-validation behavior. It SHALL NOT pass `json.Number` into the schema library or change shared schema validation. Malformed JSON, schema-invalid input, and schema-instance-decoding failures SHALL fail safely before explicit mapping, policy, catalog resolution, or model invocation. The runtime SHALL NOT replace the complete schema with a text-only schema or infer support from schema acceptance.

#### Scenario: Complete registered request is schema-valid
- **WHEN** a request uses any registered V4 branch with valid shape
- **THEN** schema validation SHALL succeed even when the runtime does not yet execute that branch
- **AND** explicit mapping SHALL make the later support decision

#### Scenario: Schema-invalid request
- **WHEN** a request has an unknown member or discriminator, forbidden typed null, inactive union arm, role-incompatible content, fractional integer control, or malformed provider-options namespace
- **THEN** schema validation SHALL fail before policy, resolution, or invocation

#### Scenario: Numerically unrepresentable schema instance
- **WHEN** a numeric value cannot be represented by the shared schema-instance decoder
- **THEN** schema validation MAY reject it safely before explicit mapping
- **AND** the runtime SHALL NOT invoke arbitrary-precision numeric processing to preserve it

### Requirement: Explicit unary text and scalar mapping

The explicit mapper SHALL preserve prompt order and map each system message to one system text value and each user or assistant text part to one provider text part. Required empty system and part text SHALL remain present. Private ProviderWire DTOs SHALL retain scalar numeric fields as raw JSON lexemes. The mapper SHALL preserve presence for `maxOutputTokens`, `topK`, `seed`, `temperature`, `topP`, `presencePenalty`, and `frequencyPenalty`; require `maxOutputTokens`, `topK`, and `seed` to use plain base-10 integer tokens without decimal points or exponent notation; use checked `strconv.ParseInt` followed by checked Go `int` conversion for those integer controls; use `strconv.ParseFloat` plus an explicit finite-value check for continuous controls; and preserve stop-sequence order. Absent and empty stop sequences SHALL both map to no stop sequence. Absent or explicit text response format SHALL map to ordinary text behavior. Absent reasoning and explicit `provider-default` SHALL both map to zero-valued `ReasoningProviderDefault`; other registered reasoning values SHALL map to their typed constants.

#### Scenario: Ordered text and zero values map exactly
- **WHEN** a request contains multiple system, user, and assistant text values plus explicit integer and floating-point zeros
- **THEN** the model SHALL receive the same message and part order, one system value per system message, required empty text, and non-nil scalar pointers containing zero

#### Scenario: Integer controls use canonical lexical syntax
- **WHEN** `maxOutputTokens`, `topK`, or `seed` uses `1`, `0`, or `-1`
- **THEN** mapping SHALL preserve the integer value
- **WHEN** one of those controls instead uses `1.0`, `1e0`, or `-0.0`
- **THEN** mapping SHALL return a safe invalid-request error before policy, resolution, or invocation

#### Scenario: Integer cannot map to Go int
- **WHEN** a schema-valid integer token is outside the supported Go `int` range
- **THEN** mapping SHALL return a safe invalid-request error before policy, resolution, or invocation

#### Scenario: Continuous number is not finite after conversion
- **WHEN** a numeric token reaches explicit mapping and `strconv.ParseFloat` reports range failure or produces a non-finite value
- **THEN** mapping SHALL return a safe invalid-request error before policy, resolution, or invocation

#### Scenario: Stop sequences are absent or empty
- **WHEN** `stopSequences` is omitted or explicitly empty
- **THEN** both requests SHALL map to no provider stop sequence

#### Scenario: Reasoning uses provider-domain zero semantics
- **WHEN** reasoning is omitted or explicitly `provider-default`
- **THEN** both requests SHALL map to `ReasoningProviderDefault`
- **AND** `none`, `minimal`, `low`, `medium`, `high`, and `xhigh` SHALL remain distinct typed values

### Requirement: Valid unsupported capabilities fail deterministically

After complete schema validation, the explicit mapper SHALL identify every registered but unimplemented request branch and return a typed unsupported-capability failure naming a stable capability. Empty optional values SHALL not activate unsupported behavior: empty `tools`, provider-options maps whose namespace objects are all empty, an exactly empty `{}` headers map, `includeRawChunks: false`, and text response format SHALL normalize to no-op values. A headers map containing any member SHALL activate body-header support even when that member's value is `""`. Non-empty tools or tool choice, tool or approval prompt content, files, reasoning content, custom content, JSON response format, non-empty provider-option values at any registered scope, non-empty body headers, or `includeRawChunks: true` SHALL activate their corresponding unsupported capability. The mapper SHALL use a fixed traversal and capability-priority order and report exactly one deterministic first capability when multiple unsupported families are active. Unsupported failures SHALL be non-retryable invalid requests and SHALL occur before host policy, resolution, or invocation.

#### Scenario: Semantically empty optional values
- **WHEN** a text request contains empty tools, only empty provider-option namespaces, `headers: {}`, disabled raw chunks, and text response format
- **THEN** mapping SHALL normalize those values and continue as a supported text request

#### Scenario: Empty-valued header is present
- **WHEN** a text request contains `headers: {"x-example":""}`
- **THEN** mapping SHALL report the `body-headers` unsupported capability before policy, resolution, or invocation

#### Scenario: Registered capability is not executable
- **WHEN** a schema-valid request activates any unimplemented capability
- **THEN** the handler SHALL return a deterministic invalid-request error whose approved message identifies that capability
- **AND** policy, resolution, and invocation counts SHALL remain zero

#### Scenario: Unsupported branch follows full schema validation
- **WHEN** a request both attempts an unsupported capability and violates that capability's registered schema
- **THEN** the handler SHALL report an invalid request shape rather than treating malformed input as a valid unsupported capability

### Requirement: Policy, resolution, and invocation ordering

For a successfully mapped request, the handler SHALL apply host policy exactly once, resolve the exact requested model ID exactly once, validate that resolution returned a non-empty canonical public ID and a non-nil V4 language model, and invoke `DoGenerate` exactly once with the policy-approved `provider.CallOptions`. It SHALL derive model cancellation from the HTTP request and configured total model duration. `DoGenerate` SHALL run in a child goroutine with panic recovery inside that goroutine; a recovered panic and a `nil, nil` return SHALL become safe internal failures. A buffered result handoff SHALL allow a late return after handler cancellation or timeout without blocking the model goroutine. Envelope, body, schema, and mapping failures SHALL produce zero policy, resolution, and invocation calls; policy failure SHALL produce zero resolution and invocation calls; resolution failure SHALL produce zero invocation calls.

#### Scenario: Supported request executes once
- **WHEN** a valid supported request passes host policy and resolves successfully
- **THEN** policy, resolution, and `DoGenerate` SHALL each run exactly once in that order

#### Scenario: Alias resolves to canonical identity
- **WHEN** the requested model ID is an alias
- **THEN** resolution SHALL receive the exact alias
- **AND** later response mapping SHALL use the resolver's canonical public ID

#### Scenario: Invalid catalog result
- **WHEN** resolution returns an empty canonical ID, nil model, or model whose specification version is not `v4`
- **THEN** the handler SHALL return a safe internal error without invoking the model

#### Scenario: Caller cancellation or model timeout
- **WHEN** the caller cancels or the configured total model duration expires before `DoGenerate` completes
- **THEN** the derived model context SHALL be canceled and the handler SHALL return the corresponding safe category without waiting indefinitely for a non-compliant model

#### Scenario: Model panics
- **WHEN** `DoGenerate` panics in the model-call goroutine
- **THEN** that goroutine SHALL recover the panic and return a safe internal failure
- **AND** the panic SHALL NOT escape and terminate the process

#### Scenario: Model returns nil result without error
- **WHEN** `DoGenerate` returns `nil, nil`
- **THEN** the handler SHALL return a safe internal failure before success mapping

#### Scenario: Model returns after terminal handler condition
- **WHEN** `DoGenerate` returns after caller cancellation or total timeout has already ended the handler
- **THEN** its result handoff SHALL not block

#### Scenario: Model ignores cancellation forever
- **WHEN** `DoGenerate` never returns after its context is canceled
- **THEN** handler latency SHALL remain bounded
- **AND** the runtime SHALL NOT claim that the non-compliant provider's retained goroutine resource is bounded

### Requirement: Closed safe unary errors

Every failure crossing the HTTP boundary SHALL use exactly this nested closed JSON shape, with no retryability member or other root/error member:

```json
{"error":{"message":"<approved>","type":"<table>","param":null,"code":"<table>"}}
```

Fixed retryability SHALL be represented by the HTTP status and SHALL equal the pinned client's status-derived `GatewayError.isRetryable` value. The authoritative category mapping SHALL be:

| Category | HTTP | `error.type` | `error.code` | `error.param` | Retryable | Pinned `@ai-sdk/gateway` class |
| --- | ---: | --- | --- | --- | --- | --- |
| invalid request | 400 | `invalid_request_error` | `invalid_request` | `null` | no | `GatewayInvalidRequestError` |
| authentication | 401 | `authentication_error` | `authentication_error` | `null` | no | `GatewayAuthenticationError` |
| permission | 403 | `forbidden` | `forbidden` | `null` | no | `GatewayForbiddenError` |
| public model not found | 404 | `model_not_found` | `model_not_found` | `null` | no | `GatewayModelNotFoundError` |
| rate limit | 429 | `rate_limit_exceeded` | `rate_limit_exceeded` | `null` | yes | `GatewayRateLimitError` |
| overload | 503 | `internal_server_error` | `overloaded` | `null` | yes | `GatewayInternalServerError` |
| failed dependency | 424 | `failed_dependency` | `failed_dependency` | `null` | no | `GatewayFailedDependencyError` |
| upstream failure | 502 | `internal_server_error` | `upstream_error` | `null` | yes | `GatewayInternalServerError` |
| timeout | 504 | `internal_server_error` | `timeout` | `null` | yes | `GatewayInternalServerError` |
| cancellation | 499 | `internal_server_error` | `canceled` | `null` | no | `GatewayInternalServerError` |
| internal failure | 500 | `internal_server_error` | `internal_error` | `null` | yes | `GatewayInternalServerError` |

Overload, upstream failure, timeout, and cancellation SHALL intentionally use `internal_server_error` because the pinned client recognizes no dedicated response type for those categories. Their status and code SHALL preserve the strict raw category. Messages SHALL come from a closed per-category allowlist; invalid request MAY use the closed `unsupported capability: <typed-capability>` message pattern.

Provider and model failures SHALL reduce exactly as follows in table order, so context cancellation and deadline take precedence over wrapped provider or transport classification:

| Input failure | Safe category |
| --- | --- |
| any error matching `context.Canceled` | cancellation |
| any error matching `context.DeadlineExceeded` | timeout |
| `APICallError.StatusCode` 408 or 504 | timeout |
| `APICallError.StatusCode` 429 | rate limit |
| `APICallError.StatusCode` 503 or 529 | overload |
| `APICallError.StatusCode` 401, 403, 404, or any other 4xx | failed dependency |
| `APICallError.StatusCode` zero | upstream failure |
| any other 5xx `APICallError` | upstream failure |
| any remaining `APICallError` status | upstream failure |
| remaining timeout-capable `net.Error` or `*url.Error` with `Timeout() == true` | timeout |
| remaining `net.Error` or `*url.Error` transport failure | upstream failure |
| any other non-`APICallError` | internal failure |
| recovered model panic or `nil, nil` result | internal failure |

Catalog unknown-model errors SHALL map to public model-not-found. Causes, arbitrary error strings, URLs, request or response bodies, headers, credentials, provider names, backend model IDs, and provider metadata SHALL never enter the DTO. Unknown or invalid safe values SHALL fall back to the canonical internal-error document.

#### Scenario: Public model is unknown
- **WHEN** resolution returns `catalog.ErrUnknownModel`
- **THEN** the handler SHALL return the model-not-found status, code, and retryability without enumerating models or exposing backend identity

#### Scenario: Backend authentication or model error
- **WHEN** `DoGenerate` returns a provider `APICallError` with status 401, 403, or 404
- **THEN** the handler SHALL return failed dependency rather than caller authentication, permission, or public model-not-found

#### Scenario: Provider failure is normalized
- **WHEN** `DoGenerate` returns a provider rate-limit, overload, timeout, other non-retryable 4xx, or other upstream failure
- **THEN** the handler SHALL select the corresponding closed safe category from status and context
- **AND** no field from the provider error SHALL be copied into the public document

#### Scenario: Provider transport connection fails
- **WHEN** `DoGenerate` returns a non-timeout DNS, connection, `net.Error`, or `*url.Error` transport failure without an `APICallError`
- **THEN** the handler SHALL return upstream failure rather than internal failure

#### Scenario: Provider transport times out
- **WHEN** `DoGenerate` returns a timeout-capable `net.Error` or `*url.Error` whose `Timeout()` is true
- **THEN** the handler SHALL return timeout

#### Scenario: Arbitrary internal cause
- **WHEN** an arbitrary non-transport internal component returns an unapproved error string containing a URL, credential, header, backend body, or provider identity
- **THEN** the response SHALL contain only the category's approved safe message and fields

### Requirement: Strict unary text response mapping

A successful provider result SHALL be mapped through private DTOs. The runtime SHALL accept only ordered text content for this phase and SHALL preserve required empty text. It SHALL map every registered warning variant with exactly its registered allowlisted fields: `unsupported` and `compatibility` always emit required `feature` and optionally carry `details`; `deprecated` always emits required `setting` and `message`; and `other` always emits required `message`. Required warning keys SHALL be emitted even when their Go string value is empty, because registered required strings permit `""` and `provider.Warning` cannot distinguish missing from explicit empty. Only an unknown warning discriminator SHALL fail provider-domain warning mapping. It SHALL map only registered finish reasons and optional raw finish reason. Usage token counts SHALL be absent or non-negative integers no greater than JavaScript's maximum safe integer, with the registered input/output groups always present; provider raw usage SHALL be omitted.

#### Scenario: Complete text result maps successfully
- **WHEN** a provider returns ordered text, valid warnings of all registered variants, valid usage, and a valid finish reason
- **THEN** the strict response SHALL preserve the public values and required empty text in registered V4 shape

#### Scenario: Provider emits an unsupported output family
- **WHEN** a provider returns non-text generated content during a text-only call
- **THEN** response mapping SHALL fail safely before HTTP 200 is committed

#### Scenario: Usage is invalid
- **WHEN** any known usage count is negative or exceeds `9007199254740991`
- **THEN** response mapping SHALL fail safely before HTTP 200 is committed

#### Scenario: Required warning strings are empty
- **WHEN** a known warning variant contains empty Go values for its registered required strings
- **THEN** response mapping SHALL emit the required keys with empty string values
- **AND** the response SHALL remain valid

#### Scenario: Warning or finish discriminator is invalid
- **WHEN** a provider returns an unknown warning type or unknown unified finish reason
- **THEN** response mapping SHALL fail safely before HTTP 200 is committed

### Requirement: Canonical response identity and privacy

The strict unary response SHALL always contain response metadata whose `modelId` is the canonical public ID returned by resolution. It MAY preserve allowlisted provider response ID and timestamp, but SHALL discard the provider-reported provider name and backend model ID. It SHALL omit provider request bodies, provider response bodies and headers, provider metadata, raw usage, and content-part provider metadata. No provider-domain JSON marshaler SHALL control public response bytes.

#### Scenario: Provider metadata contains private material
- **WHEN** a provider result contains backend model identity, provider name, URL, headers, bodies, credentials, raw usage, or provider metadata
- **THEN** none of those values SHALL appear in the normalized response
- **AND** `response.modelId` SHALL equal the canonical public ID

#### Scenario: Alias was requested
- **WHEN** an alias resolves to a canonical public model
- **THEN** the successful raw HTTP response SHALL contain the canonical model ID and SHALL NOT use the alias or backend model ID as response identity

### Requirement: Validate and bound complete unary documents before commitment

The package SHALL embed closed draft 2020-12 schemas for unary success and error documents. It SHALL map a complete private DTO, encode it incrementally into a `limit+1` bounded buffer without first accumulating an unbounded encoded document, validate the bounded bytes against the corresponding schema, and only then write status, headers, and body. The complete success and error documents SHALL honor their separate configured byte limits. An over-limit success SHALL become a bounded safe error; an invalid or over-limit error SHALL become canonical internal-error bytes guaranteed by construction to fit.

#### Scenario: Unary response boundary
- **WHEN** an encoded success is below, exactly at, or one byte above the configured unary response limit
- **THEN** the first two documents SHALL be eligible for HTTP 200 and the over-limit document SHALL produce a bounded non-200 safe error before commitment

#### Scenario: Error response boundary
- **WHEN** an ordinary safe error exceeds its configured limit or fails its schema
- **THEN** the handler SHALL emit the canonical internal-error document within the configured error limit

#### Scenario: Success schema validation fails
- **WHEN** internal response mapping or encoding produces bytes that violate the unary schema
- **THEN** the handler SHALL emit a safe internal error and SHALL NOT commit HTTP 200

### Requirement: Runtime contract and cross-language evidence

Go tests SHALL replay every committed phase 2 request golden according to this stage matrix:

| Golden record | Expected stage and result |
| --- | --- |
| `streaming.json` | unary envelope rejection because streaming is `true` |
| `sequence.json` record 1 | supported unary text execution |
| `sequence.json` record 2 | unary envelope rejection because streaming is `true` |
| `scalar-presence.json` | schema success, then first unsupported capability `body-headers` because the map contains an empty-valued header member |
| `headers.json` records 1 and 2 | schema success, then first unsupported capability `body-headers` |
| `comprehensive-unions.json` | schema success, then first unsupported capability `provider-options` from the first system message |

A separate dedicated supported scalar request SHALL assert exact scalar presence and execution. Focused requests SHALL activate each unsupported capability independently; a multi-capability golden SHALL be required to report only the deterministic first capability. Raw HTTP tests SHALL cover malformed protocol input, privacy, exact safe-error bytes/classes, canonical identity, and every configured boundary below, at, and above its limit. A pinned `@ai-sdk/gateway@4.0.52` client test SHALL complete a supported unary text request through the production Go handler and assert client-observable content, usage, finish behavior, and non-success classification while raw Go assertions remain response authority.

#### Scenario: Committed golden replay
- **WHEN** the phase 2 semantic request goldens are replayed through Go
- **THEN** each record SHALL stop at the exact stage and result in the stage matrix
- **AND** no multi-capability golden SHALL be treated as evidence for every activated capability

#### Scenario: Dedicated supported scalar request
- **WHEN** a unary request contains only supported text, scalar, stop-sequence, text-format, and reasoning values
- **THEN** it SHALL execute once with exact mapped presence and values

#### Scenario: Focused unsupported requests
- **WHEN** focused schema-valid requests activate one unsupported capability each
- **THEN** each SHALL report its own typed capability after schema validation and before policy, resolution, or invocation

#### Scenario: Registered client completes unary text
- **WHEN** the pinned Gateway client sends a supported unary text request to the production handler
- **THEN** the request SHALL invoke the recording model once and the client SHALL consume the successful result

#### Scenario: Client overwrite behavior does not hide server assertions
- **WHEN** the registered client replaces server `request`, `response`, or `warnings` fields
- **THEN** raw Go tests SHALL independently assert server warnings, canonical response identity, response schemas, privacy, and byte bounds
