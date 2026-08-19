## ADDED Requirements

### Requirement: Public strict unary handler boundary

`gateway/providerwire/v4` SHALL provide a reusable `net/http` handler, a request-aware model resolver interface with a function adapter, positive-valued configuration options, and exported route, language-model header, specification-version, JSON media-type, and total-timeout sentinel values needed by hosts. Construction SHALL reject a nil or typed-nil resolver, nil options, non-positive option values, and limits too small for the fixed fallback error.

The handler SHALL depend only on protocol/runtime dependencies and `provider`; it MUST NOT import router, authentication, catalog, credential, IAM, billing, deployment, frontend, or observability packages. Wire request/result/error representations, schema registry, adapters, and policy implementation SHALL remain private.

#### Scenario: Host constructs the unary handler
- **WHEN** a host supplies a valid resolver and either defaults or positive limit options
- **THEN** construction SHALL return a handler that can serve unary `/language-model` calls

#### Scenario: Invalid construction fails
- **WHEN** construction receives a nil resolver, typed-nil resolver, nil option, non-positive timeout or byte limit, or an error limit that cannot hold the fallback error
- **THEN** construction SHALL return an error and no handler

#### Scenario: Host concerns stay outside
- **WHEN** the package imports and public API are inspected
- **THEN** they SHALL contain no authentication, catalog, credential, billing, routing-framework, deployment, or public wire-DTO dependency

### Requirement: Ordered strict unary request acceptance

The handler SHALL process requests in this order: HTTP envelope validation, bounded raw-body read, strict JSON syntax, request-schema validation, private wire decoding, host-control extraction, request policy, bounded provider adaptation including resource validation, model resolution, and invocation. A failure at any stage SHALL prevent every later stage.

The handler SHALL accept only `POST /language-model` with mandatory `Content-Type: application/json`, exact unpadded non-empty model ID, specification version `4`, and streaming value `false`. Content-type parameters SHALL be allowed. Optional `Accept` SHALL follow the H1 positive-quality exact, type-wildcard, or full-wildcard rules for `application/json`. A syntactically valid streaming value `true` SHALL fail non-retryably as unsupported before decoding, resolution, or invocation.

The body SHALL be limited to 8 MiB by default and SHALL be rejected at limit plus one. Syntax validation SHALL require exactly one value plus trailing whitespace and SHALL reject duplicate decoded names, escaped-equivalent duplicates, invalid UTF-8, lone surrogates, malformed escapes, truncation, and trailing values before schema validation. The original validated bytes SHALL then be validated against the checked-in request schema before private decoding.

#### Scenario: Valid unary envelope advances in order
- **WHEN** a request uses the exact unary path and routing headers, valid JSON content type and Accept value, and a schema-valid body
- **THEN** each validation stage SHALL run in the specified order before policy and resolution

#### Scenario: Invalid envelope bypasses body and resolver
- **WHEN** method, path, routing header, content type, Accept, or streaming selection is invalid or unsupported
- **THEN** the handler SHALL return the corresponding non-retryable safe 4xx error without invoking the resolver

#### Scenario: Oversized body bypasses syntax and resolver
- **WHEN** a request body exceeds the configured raw-body limit by one byte
- **THEN** the handler SHALL return HTTP 413 without syntax/schema decoding or resolver invocation

#### Scenario: Syntax precedes schema
- **WHEN** a body contains both duplicate names or a trailing value and a schema violation
- **THEN** the handler SHALL classify the failure as invalid JSON syntax and SHALL not run provider adaptation or resolution

#### Scenario: Schema precedes private adaptation
- **WHEN** a syntactically valid body has an unknown standard field, inactive union sibling, typed null, missing required field, or wrong field type
- **THEN** the handler SHALL return a non-retryable schema error with at most a safe instance path and SHALL not invoke the resolver

### Requirement: Embedded production contract validation

The handler SHALL embed and compile the checked-in Draft 2020-12 schema graph once through the protocol-local registry and SHALL reuse the immutable compiled schemas concurrently. Handler construction SHALL fail if the embedded graph cannot compile. Production syntax validation SHALL use the same focused `jsontext` behavior and syntax corpus as contract validation.

Request bytes SHALL be schema-validated before conversion. Every success and error response SHALL be schema-validated and, for errors, HTTP-status-correlated before commitment. The checked-in schemas SHALL remain normative and MUST NOT be generated from private wire values or Go provider structs.

#### Scenario: Built binary has no schema file dependency
- **WHEN** the package is built and run outside the repository working directory
- **THEN** handler construction and request/response validation SHALL use embedded checked-in schemas without filesystem or network access

#### Scenario: Concurrent handlers reuse compiled schemas
- **WHEN** multiple handlers validate requests concurrently
- **THEN** they SHALL use one safely shared immutable compiled schema graph

#### Scenario: Response contract is checked before commit
- **WHEN** a private success or error projection is encoded
- **THEN** the handler SHALL validate it against the matching embedded schema before writing its HTTP status

### Requirement: Request host policy and private provider adaptation

After schema validation and before provider adaptation, the handler SHALL isolate the top-level `providerOptions.gateway` object, body `headers`, `includeRawChunks`, and nested reserved host namespaces. Bounded provider adaptation SHALL then validate and account for decoded inline-file resource usage before resolution.

The initial policy SHALL remove an empty top-level Gateway object and reject one containing any member; reject a `gateway` member nested in another provider namespace; remove absent or explicitly empty body headers; remove only the exact pinned orchestration body-header map `{"user-agent":"ai/7.0.65"}`; reject every other non-empty body-header map; reject `includeRawChunks: true`; and enforce the configured aggregate decoded inline-file limit. These policy failures SHALL be non-retryable and SHALL occur before resolution. Other provider option namespaces SHALL survive as semantic opaque objects and become `provider.RawProviderOption` values only after reserved controls are removed.

Private adapters SHALL preserve system text, role-specific ordered content, tagged file data, tools, tool choice, response format, opaque JSON, provider options, zero scalar values, and explicit empty arrays or maps. Base64 inline data SHALL be decoded with aggregate resource accounting. Integer-designated `maxOutputTokens`, `topK`, and `seed` SHALL be integral and within Go `int` range. Other standard numeric values SHALL remain finite, SHALL NOT underflow a non-zero value to zero, and SHALL survive canonical float64 decimal round-tripping; ordinary decimals such as `0.1` SHALL remain accepted. Numeric lexeme and exponent work SHALL be bounded before arbitrary-precision conversion. Unsupported numeric values SHALL fail before resolution without rounding or truncation. Optional absent versus explicit false or empty-string values that the Go provider domain cannot distinguish SHALL canonicalize to the same provider-domain value as a parity-preserving Go adaptation.

#### Scenario: Unsupported Gateway control is rejected
- **WHEN** `providerOptions.gateway` contains any member, including a member whose value is empty, false, zero, or null
- **THEN** the handler SHALL return a non-retryable policy error without invoking the resolver

#### Scenario: Empty Gateway namespace is isolated
- **WHEN** `providerOptions.gateway` is an explicit empty object
- **THEN** policy SHALL remove it before adaptation while preserving every other provider namespace

#### Scenario: Nested reserved namespace is rejected
- **WHEN** another provider option object contains a nested member named `gateway`
- **THEN** the handler SHALL reject the request before provider adaptation and resolution

#### Scenario: Pinned orchestration marker is isolated
- **WHEN** body `headers` is exactly `{"user-agent":"ai/7.0.65"}`
- **THEN** policy SHALL remove it and the provider SHALL receive no `provider.CallOptions.Headers`

#### Scenario: Other body headers cannot reach the provider
- **WHEN** body `headers` contains any other member, value, or additional member
- **THEN** the handler SHALL reject the request before resolution
- **AND** absent or explicitly empty body headers SHALL produce no `provider.CallOptions.Headers`

#### Scenario: Raw request is rejected
- **WHEN** `includeRawChunks` is true
- **THEN** the handler SHALL reject the request consistently before resolution

#### Scenario: Explicit empty intent survives adaptation
- **WHEN** the request explicitly contains empty tools, stop sequences, or a non-reserved provider namespace object
- **THEN** the resolved model SHALL receive corresponding non-nil empty provider values rather than absence

#### Scenario: Opaque JSON survives semantically
- **WHEN** an allowed opaque tool value or provider option contains nested JSON or allowed null
- **THEN** the resolved model SHALL receive the same semantic JSON without requiring object-member order

#### Scenario: Decoded inline data is bounded
- **WHEN** aggregate decoded inline file data exceeds the configured limit
- **THEN** the handler SHALL return a non-retryable request error before resolution or provider invocation

#### Scenario: Integer-designated numbers are lossless
- **WHEN** `maxOutputTokens`, `topK`, or `seed` is fractional or outside Go `int` range
- **THEN** the handler SHALL return a non-retryable request error before resolution without rounding or truncation

### Requirement: Resolver and unary invocation lifecycle

After all request and policy gates pass, the handler SHALL derive one total-timeout context from the request context. The default SHALL be 120 seconds. The timeout SHALL cover resolver execution and one `DoGenerate` call. The resolver SHALL receive the request carrying that context and the validated public model ID, SHALL run at most once, and SHALL not run for rejected requests. A returned nil model SHALL fail safely. The resolved model's `DoGenerate` SHALL run exactly once; `DoStream` SHALL never run in this phase.

Request cancellation and total-timeout cancellation SHALL propagate to the resolver and model. Total timeout SHALL use an exported sentinel cause and normalize to retryable HTTP 504. Observable consumer cancellation before commitment SHALL normalize to non-retryable HTTP 499. Host HTTP server settings SHALL remain responsible for transport header/read/write deadlines.

#### Scenario: Valid request invokes once
- **WHEN** a request passes envelope, syntax, schema, policy, adaptation, and resource checks
- **THEN** the resolver SHALL run once and the returned model's `DoGenerate` SHALL run once with equivalent accepted call options
- **AND** `DoStream` SHALL not run

#### Scenario: Resolver receives request context
- **WHEN** a host wrapper attaches caller context and a valid request selects model ID `public-model`
- **THEN** the resolver SHALL receive that context through the request, the validated ID `public-model`, and the handler total-timeout deadline

#### Scenario: Resolver failure does not invoke model
- **WHEN** the resolver returns an error or nil model
- **THEN** the handler SHALL return a safe error and SHALL not invoke either model method

#### Scenario: Unary timeout cancels work
- **WHEN** resolver or `DoGenerate` exceeds the configured total timeout
- **THEN** its context SHALL be canceled with the total-timeout cause and the handler SHALL return a retryable HTTP 504 safe error when still writable

#### Scenario: Consumer cancellation propagates
- **WHEN** the request context is canceled during resolution or `DoGenerate`
- **THEN** the derived context SHALL become done promptly and the handler SHALL return a non-retryable 499 safe error when still writable

### Requirement: Privacy-safe bounded unary success

A successful non-nil model result SHALL be adapted into the exact H1 generate-result projection while preserving ordered content, standardized finish reason, standardized usage, warnings, safe response ID and non-zero timestamp, and representable public content fields.

The response policy SHALL remove all top-level and per-content provider metadata, `usage.raw`, request metadata/body, response headers/body, provider identity, and backend model ID. Required `content` and `warnings` arrays SHALL encode as arrays even when empty. The handler SHALL encode into a pre-commit buffer bounded to 8 MiB by default, validate the complete semantic result against `generate-result.json`, and only then commit HTTP 200 `application/json`.

Nil, unrepresentable, schema-invalid, unencodable, or oversized model results SHALL produce a bounded safe non-2xx JSON error without committing a partial or empty success response.

#### Scenario: Valid result commits once
- **WHEN** `DoGenerate` returns a representable result within the configured limit
- **THEN** the handler SHALL return HTTP 200 `application/json` whose complete body validates against `generate-result.json`

#### Scenario: Required empty arrays remain arrays
- **WHEN** a model result has no content or warnings
- **THEN** the wire result SHALL contain `content: []` and `warnings: []` rather than null or omission

#### Scenario: Provider metadata stays private
- **WHEN** top-level or content parts contain provider metadata and usage contains raw provider usage
- **THEN** none of those values SHALL appear in the public result

#### Scenario: Backend request and response details stay private
- **WHEN** a model result contains provider request body, response headers/body, provider identity, or backend model ID
- **THEN** those values SHALL be omitted while a safe response ID and timestamp MAY remain

#### Scenario: Invalid result fails before success commitment
- **WHEN** a model returns nil, an unknown or unrepresentable content arm, invalid raw JSON, or a result over the configured response limit
- **THEN** the handler SHALL return a safe non-2xx error and SHALL not commit HTTP 200 or any result prefix

### Requirement: Closed bounded safe unary errors

Every handler-generated non-2xx response SHALL be a closed H1 safe error projection with a nested status equal to the HTTP status, explicit retryability, one allowed Gateway error type, a stable privacy-safe message, and only an arm-appropriate public model ID or rule ID parameter. The default maximum error body SHALL be 16 KiB.

The handler MAY inspect a wrapped `provider.APICallError` status and explicit retryability but MUST NOT serialize its URL, request body values, response headers/body, data, cause text, provider identity, backend model, credentials, or arbitrary message. Envelope, syntax, schema, policy, and resource failures SHALL be non-retryable request errors. Provider 429 SHALL map to rate limit; other provider 4xx failures SHALL map to safe invalid request; provider 5xx or arbitrary provider failures SHALL map to failed dependency. Resolver classification MAY additionally produce authentication, forbidden, or model-not-found errors. Invalid statuses SHALL normalize to internal HTTP 500.

Errors SHALL be encoded, status-correlated, schema-validated, and size-checked before commitment. If the selected safe error cannot fit or encode, the handler SHALL use a fixed validated HTTP 500 fallback that fits the construction-time minimum. A write failure after commitment SHALL not cause a second response attempt.

#### Scenario: Unsafe API-call details are removed
- **WHEN** resolver or provider returns a wrapped API-call error containing backend URL, headers, bodies, data, credentials, provider identity, or diagnostic text
- **THEN** the response SHALL contain only the normalized safe category, stable message, status, retryability, and an allowed safe param

#### Scenario: Explicit retryability remains on local wire
- **WHEN** a usable provider API-call error has explicit retryability different from status-based inference
- **THEN** the safe error SHALL retain the explicit boolean while pinned stock-client status inference remains a separately asserted behavior

#### Scenario: Provider rate limit is classified safely
- **WHEN** `DoGenerate` returns a status-429 API-call error
- **THEN** the handler SHALL return a contract-valid `rate_limit_exceeded` response without backend diagnostics

#### Scenario: Provider dependency failure is classified safely
- **WHEN** `DoGenerate` returns a 5xx or arbitrary non-API error
- **THEN** the handler SHALL return a bounded `failed_dependency` response with no original error text

#### Scenario: Invalid status falls back
- **WHEN** a resolver or provider API-call error has a status that cannot be used as the final HTTP error status
- **THEN** the handler SHALL return the fixed validated HTTP 500 error without panic or implicit success

#### Scenario: Error write fails
- **WHEN** writing a fully prepared error body fails after response commitment
- **THEN** the handler SHALL stop without attempting another body or status

### Requirement: Pinned unary runtime interoperability

At the exact registered package versions, direct stock-Gateway `doGenerate` and orchestration-level `generateText` SHALL call the real Go V4 handler successfully for supported requests. Tests SHALL cover request adaptation, explicit empty values, opaque values, safe disclosure, resolver and invocation counts, and safe error consumption. Status 429 and representative 5xx responses SHALL demonstrate the pinned client's status-derived retryability separately from the local explicit wire boolean.

The same tests SHALL prove unsupported body headers, Gateway controls, raw intent, and streaming mode fail before the resolver. Existing legacy `gateway/providerwire`, Grafana, and interop behavior SHALL remain unchanged and separately tested.

#### Scenario: Stock direct unary call succeeds
- **WHEN** the registered `@ai-sdk/gateway` model performs `doGenerate` with a supported unary request against the Go V4 handler
- **THEN** the resolver and `DoGenerate` SHALL each run once and the client SHALL consume the policy-safe result

#### Scenario: Stock orchestration unary call succeeds
- **WHEN** registered `ai` runs `generateText` through the stock Gateway model and V4 handler
- **THEN** the orchestration result SHALL preserve the supported semantic text, finish reason, usage, tools, and structured-output behavior exercised by the scenario

#### Scenario: Stock client consumes safe failures
- **WHEN** the real V4 handler returns safe 429 and 5xx errors
- **THEN** the pinned client SHALL expose their Gateway categories and infer retryability from HTTP status as documented for the registered baseline

#### Scenario: Policy rejection bypasses resolver over HTTP
- **WHEN** the pinned client emits a schema-valid request with an unsupported body header map other than the exact pinned orchestration marker, Gateway control, or raw-chunk intent
- **THEN** the real handler SHALL return a non-retryable safe error without resolver or model invocation

#### Scenario: Legacy transport remains unchanged
- **WHEN** existing legacy provider-wire, Grafana, and frontend interop tests run without explicit V4 selection
- **THEN** their API, request/response bytes, SSE behavior, and defaults SHALL remain unchanged

## MODIFIED Requirements

### Requirement: Language-model HTTP envelope

The OpenAPI 3.1 contract SHALL describe only `POST /language-model`. Requests SHALL require `Content-Type: application/json`, `ai-language-model-specification-version: 4`, a non-empty unpadded `ai-language-model-id`, and `ai-language-model-streaming: true` or `false`. Header names SHALL be case-insensitive; routing-header values SHALL be exact and case-sensitive.

Unary success SHALL be HTTP 200 `application/json`. Streaming success SHALL be HTTP 200 `text/event-stream`. Non-2xx responses SHALL use the JSON error schema. Content-type parameters SHALL be allowed. Optional `Accept` values SHALL be syntactically valid and permit the selected representation with positive quality through an exact, type-wildcard, or full-wildcard range.

Authorization, Gateway protocol, team, observability, custom, and user-agent headers SHALL remain host concerns rather than required language-model routing headers.

#### Scenario: Unary and streaming selection is exact
- **WHEN** a valid request selects streaming `false` or `true`
- **THEN** the response media type SHALL be `application/json` or `text/event-stream`, respectively

#### Scenario: Stock client full wildcard is compatible
- **WHEN** the pinned stock client sends `Accept: */*` with positive quality
- **THEN** envelope validation SHALL permit the selected unary or streaming representation

#### Scenario: Invalid envelopes fail predictably
- **WHEN** method, path, routing headers, content type, or Accept is invalid
- **THEN** envelope validation SHALL reject it with the corresponding stable category

#### Scenario: Host headers remain optional
- **WHEN** the pinned client emits broader Gateway or observability headers
- **THEN** captures MAY record safe normalized evidence without making those headers contract requirements

### Requirement: Repeatable validation and coordinated evolution

`mise run validate-providerwire-v4-contract` SHALL validate OpenAPI, offline references, strict syntax, HTTP envelopes, schema compilation, positive payloads, mutation-derived negative failures, response seeds, and selected conformance transport inputs. `mise run test-interop-contract` SHALL validate baseline pins, type-check capture and unary policy tooling, verify request captures, consume seeded and derived response projections, compare selected transported conformance inputs with existing UI expectations, and exercise pinned unary runtime interoperability.

`mise run check-providerwire-v4` SHALL aggregate those non-mutating checks with focused V4 unary Go tests and unary raw/policy conformance checks. Committed artifact replacement SHALL require an explicit update command that validates generated content before atomically replacing each generated artifact.

The parity map SHALL classify V4 contract evidence, strict unary runtime, and legacy transport separately. A baseline change SHALL update the manifest, dependency pins, required source-equivalence evidence, schemas, captures, semantic seeds, recipes, unary raw/policy evidence, parity classification, and lockfiles together.

#### Scenario: Contract validation is one command
- **WHEN** a contributor runs `mise run validate-providerwire-v4-contract`
- **THEN** every machine-readable contract, curated seed, recipe, and selected transport input SHALL be checked without network access

#### Scenario: Interop verification uses baseline pins
- **WHEN** a contributor runs `mise run test-interop-contract`
- **THEN** baseline validation, non-mutating client evidence, and pinned unary handler interoperability SHALL run

#### Scenario: Aggregate verification does not update artifacts
- **WHEN** a contributor runs `mise run check-providerwire-v4`
- **THEN** contract, runtime, interop, and unary conformance evidence SHALL be checked without changing committed files

#### Scenario: Artifact refresh is explicit and atomic
- **WHEN** a contributor runs the documented ProviderWire V4 artifact update command
- **THEN** every generated request or unary expectation SHALL be validated before its committed destination is replaced atomically

#### Scenario: Legacy parity remains separate
- **WHEN** the parity coverage map is reviewed
- **THEN** V4 SHALL claim only checked contract and strict unary handler behavior supported by its evidence
- **AND** it SHALL not claim streaming service, Go client, Grafana adoption, frontend runtime, private-server, or uncovered live-provider behavior

#### Scenario: Baseline drift is atomic
- **WHEN** a registered package or relied-on serialized behavior changes
- **THEN** all affected machine-readable artifacts, runtime assumptions, and evidence SHALL change in the same parity-governed update
