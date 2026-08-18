## ADDED Requirements

### Requirement: Registered protocol authority and baseline

The repository SHALL define the ProviderWire V4 HTTP contract against Vercel AI SDK source commit `c527d7b3b26287598d2c80e7bce8f16b21653363` and the exact registered package set `@ai-sdk/provider@4.0.4`, `@ai-sdk/gateway@4.0.33`, `@ai-sdk/provider-utils@5.0.16`, and `ai@7.0.44`. Executable stock-client evidence SHALL run the exact package pins. Source inspection SHALL use the registered commit and, where its workspace package version differs, SHALL record verified equivalence to the corresponding registered release source before relying on that path.

The authority order SHALL be pinned Gateway HTTP behavior, deterministic stock-client captures and response consumption, checked-in OpenAPI and JSON Schemas plus these requirements, Go implementations, and language-specific in-memory types. Every discrepancy SHALL be classified as pinned-client behavior, local serialized projection, intentional host-policy restriction, parity-preserving Go adaptation, implementation defect, or coverage gap.

#### Scenario: Exact client packages produce captures
- **WHEN** the contract capture suite runs
- **THEN** it SHALL execute Gateway 4.0.33 and ai 7.0.44 with provider 4.0.4 and provider-utils 5.0.16 rather than packages inferred from another checkout

#### Scenario: Registered commit package mismatch is explicit
- **WHEN** a relied-on source path belongs to a package whose manifest version at the registered commit differs from the registered npm version
- **THEN** the contract evidence SHALL identify the mismatch and prove path equivalence to the registered release or stop without making a parity claim

#### Scenario: Private server acceptance is not inferred
- **WHEN** a stock-client request capture or response-consumption test passes
- **THEN** the repository SHALL claim pinned client emission or consumption only and SHALL NOT claim that Vercel's private server accepts the complete local contract

### Requirement: Contract-only V4 capability and legacy coexistence

The repository SHALL add a protocol-local `gateway/providerwire/v4` capability containing machine-readable contract artifacts, package documentation, validation tests, and maintainer evidence. During this phase it MUST NOT expose or implement a production request decoder, HTTP handler, model resolver, provider adapter, host policy, SSE server, Go client, or public wire DTO hierarchy. The existing `gateway/providerwire` package and Grafana transport SHALL remain the active legacy implementation with unchanged defaults and wire behavior.

#### Scenario: Contract phase has no V4 invocation path
- **WHEN** the V4 package and public symbols are inspected
- **THEN** no V4 code path SHALL resolve or invoke a `provider.LanguageModel`

#### Scenario: Legacy behavior remains available
- **WHEN** existing provider-wire, Grafana client/server, and stock-client interop tests run
- **THEN** they SHALL retain their existing behavior without switching to the V4 contract package

#### Scenario: Contract artifacts do not become public DTOs
- **WHEN** consumers inspect the V4 Go package API
- **THEN** they SHALL find contract documentation and no exported request, result, stream-part, error, union codec, or generic runtime types

### Requirement: Pinned language-model HTTP envelope

The OpenAPI 3.1 contract SHALL describe only `POST /language-model`. Requests SHALL require `Content-Type: application/json`, `ai-language-model-specification-version: 4`, a non-empty `ai-language-model-id`, and `ai-language-model-streaming: true` or `false`. The model ID SHALL contain no leading or trailing whitespace and SHALL otherwise be preserved. Header names SHALL follow HTTP case-insensitivity; the specification-version and streaming values SHALL be exact and case-sensitive.

Unary selection SHALL describe an HTTP 200 `application/json` success body. Streaming selection SHALL describe an HTTP 200 `text/event-stream` success body whose event data uses the stream-part schema. Non-2xx responses SHALL describe the JSON error schema. `Content-Type` parameters SHALL be permitted. `Accept` SHALL be optional; when present it SHALL be syntactically valid and include an exact or type-wildcard range with positive quality for the selected response media type. Empty ranges, malformed values, incompatible ranges, and `q=0` SHALL NOT permit the representation.

Configured authorization, `ai-gateway-protocol-version`, team, observability, custom, and user-agent headers MAY be recorded safely as stock-client evidence but SHALL remain host concerns rather than required reusable handler headers.

#### Scenario: Unary envelope is represented
- **WHEN** a unary stock-client capture is validated
- **THEN** it SHALL use `POST /language-model`, the three required routing headers with streaming `false`, JSON content type, and a request body valid under the request schema

#### Scenario: Streaming envelope is represented
- **WHEN** a streaming stock-client capture is validated
- **THEN** it SHALL use the same method, path, and routing headers with streaming `true` and select an SSE success response

#### Scenario: Invalid routing values are rejected by the contract corpus
- **WHEN** an envelope fixture has a missing or whitespace-padded model ID, a version other than exact `4`, or a streaming value other than exact lowercase `true` or `false`
- **THEN** contract validation SHALL reject it for the intended envelope rule

#### Scenario: Content type is strict
- **WHEN** an envelope omits content type or uses a media type other than `application/json`
- **THEN** contract validation SHALL reject it, while an `application/json` content type with valid parameters SHALL remain compatible

#### Scenario: Accept quality is respected
- **WHEN** Accept contains only incompatible ranges, malformed or empty entries, or a compatible range with `q=0`
- **THEN** the selected representation SHALL be incompatible rather than inheriting legacy permissive behavior

#### Scenario: Broader Gateway headers remain host-owned
- **WHEN** the stock client emits protocol, authorization, team, observability, custom, or user-agent headers
- **THEN** captures SHALL classify them without making them mandatory language-model routing headers or storing secret values

### Requirement: Offline-valid OpenAPI and schema graph

The repository SHALL check in an OpenAPI 3.1 document and JSON Schema 2020-12 resources for the request, generate result, stream part, and error payloads. Every schema SHALL declare a stable identifier and dialect. All references SHALL resolve from checked-in local resources without network access. Contract validation SHALL lint and bundle the complete OpenAPI document and compile every schema through the repository's JSON Schema validator configured for Draft 2020-12.

OpenAPI SHALL own the HTTP envelope and payload references. H1 requirements and focused tests SHALL own strict syntax, detailed content negotiation, streaming-header response selection, JSON SSE event framing, and EOF termination. A later streaming-service capability SHALL own server commitment, flush timing, cancellation, timeouts, post-commit failures, and other runtime lifecycle semantics.

#### Scenario: Complete OpenAPI validates offline
- **WHEN** the contract validation task runs without network access
- **THEN** the complete OpenAPI document SHALL lint, bundle, and resolve every local payload reference

#### Scenario: Every schema compiles
- **WHEN** the protocol-local schema registry loads the checked-in resources
- **THEN** request, generate-result, stream-part, and error schemas SHALL compile as JSON Schema 2020-12

#### Scenario: OpenAPI limitations remain assigned
- **WHEN** a lifecycle or conditional behavior cannot be expressed accurately in OpenAPI
- **THEN** it SHALL be specified and tested outside OpenAPI rather than omitted or weakened

### Requirement: Exact serialized request contract

The request schema SHALL describe the post-serialization LanguageModelV4 call options emitted by the pinned Gateway client. It SHALL require `prompt` and represent every registered option: output-token and sampling settings, stop sequences, response format, seed, tools, tool choice, raw-chunk intent, body headers, reasoning, and provider options. `abortSignal` SHALL NOT be a body property. Undefined JavaScript properties SHALL be absent from semantic JSON.

Message roles SHALL have exact selected content membership: system content is a string; user content contains text or file parts; assistant content contains only the pinned text, file, custom, reasoning, reasoning-file, tool-call, and tool-result parts; tool content contains tool-result or tool-approval-response parts. Function tools, provider tools, tool choices, response formats, tool-result outputs, file-data variants, and nested tool-result content SHALL use exact discriminator-selected arms.

Inline `Uint8Array` data in supported prompt and nested tool-result file positions SHALL appear as base64 strings after Gateway preprocessing. File URLs SHALL serialize as strings. Explicit empty arrays and objects SHALL remain valid and distinguishable from absent fields where they communicate intent.

#### Scenario: Abort signal is omitted
- **WHEN** a call supplies an abort signal and the pinned Gateway client serializes the request
- **THEN** the semantic request body SHALL not contain an `abortSignal` property

#### Scenario: Inline bytes become base64
- **WHEN** supported prompt or nested tool-result file data is a `Uint8Array`
- **THEN** the captured semantic JSON SHALL carry the equivalent base64 string in the selected `data` arm

#### Scenario: Message role membership is exact
- **WHEN** a message contains a part not permitted for its selected role or uses string content for a non-system role
- **THEN** the request schema SHALL reject it

#### Scenario: Inactive union siblings fail
- **WHEN** a tool, tool choice, response format, file-data, content-part, or tool-result arm includes a field belonging only to another arm
- **THEN** the request schema SHALL reject the complete object

#### Scenario: Explicit empty intent survives
- **WHEN** the pinned client emits explicit empty tools, stop sequences, headers, provider options, or other contractually present collections
- **THEN** semantic capture comparison and schema validation SHALL preserve the empty collection rather than treating it as absent

### Requirement: Closed standard objects and declared extension boundaries

Every standardized request, result, stream-part, error, message, tool, warning, usage, metadata envelope, file, source, and selected union-arm object SHALL reject unknown standard fields. Typed fields SHALL reject null unless their declared opaque JSON type permits null.

The contract SHALL maintain a field and boundary ledger covering every object and union arm, required versus optional wire fields after undefined omission, nullable versus non-null opaque values, keyed map value schemas, explicit-empty behavior, Go-only fields excluded from the strict projection, and future Go representability.

Open boundaries SHALL be limited to declared values: JSON Schema objects; opaque tool-call input and allowed tool-result values; provider-tool argument objects; keyed provider option and metadata maps whose values are JSON objects; provider raw values; and declared request/response bodies. Provider references SHALL be string maps that MAY be empty and SHALL reject a member named `type`. JSONValue and JSONObject SHALL remain distinct. Streamed tool-result `result` SHALL reject null even though prompt JSON tool-result output MAY be null.

#### Scenario: Unknown standard field fails
- **WHEN** a standardized object contains an unregistered property
- **THEN** schema validation SHALL reject it before any future adaptation or invocation

#### Scenario: Opaque JSON retains semantic values
- **WHEN** a declared opaque value contains nested objects, arrays, strings, numbers, booleans, or an allowed null
- **THEN** validation and semantic comparison SHALL preserve that JSON value without interpreting its member order

#### Scenario: Keyed extensions remain object-valued
- **WHEN** a provider option or metadata namespace maps to a scalar, array, or null instead of a JSON object
- **THEN** schema validation SHALL reject it

#### Scenario: Typed null fails
- **WHEN** a string, number, boolean, array, exact object, or non-null opaque field is explicitly null
- **THEN** schema validation SHALL reject it

#### Scenario: Representability gap stops implementation
- **WHEN** the ledger identifies a required wire distinction that the future Go domain adapter cannot preserve without loss
- **THEN** the phase SHALL record the gap and stop rather than open a standard object or silently normalize the value

### Requirement: Curated unary result schema

The generate-result schema SHALL require ordered content, finish reason, usage, and warnings and SHALL represent the complete pinned generate-content union. It SHALL model optional provider metadata, request body, and response metadata/body/headers structurally while treating their presence as representability rather than authorization to disclose them. Finish-reason raw values and individual usage counters that are undefined in JavaScript SHALL be omittable in serialized JSON. Response timestamps SHALL be RFC 3339 date-time strings.

#### Scenario: Complete unary projection validates
- **WHEN** a curated generate result covers every content arm, finish reason, usage groups, warnings, provider metadata, request data, and response metadata
- **THEN** the result SHALL validate and preserve content order and semantic opaque values

#### Scenario: Required top-level result fields are enforced
- **WHEN** content, finish reason, usage, or warnings is absent or null
- **THEN** the generate-result schema SHALL reject the result

#### Scenario: Undefined nested counters are omitted
- **WHEN** a pinned usage counter or raw finish reason is undefined before serialization
- **THEN** its property MAY be absent while the enclosing exact object remains valid

#### Scenario: Go-only response identity is excluded
- **WHEN** a response metadata object includes a local provider-identity field not defined by the pinned serialized contract
- **THEN** the generate-result schema SHALL reject it

### Requirement: Complete stream-part JSON and SSE projection

The stream-part schema SHALL define exact arms for stream start, response metadata, text start/delta/end, reasoning start/delta/end, tool-input start/delta/end, tool approval request, tool call, tool result, custom content, file, reasoning file, URL/document source, finish, raw, and error. It SHALL preserve event order only as a stream lifecycle concern; each schema instance represents one complete event JSON value.

A streaming success SHALL use HTTP 200 and `Content-Type: text/event-stream`. The HTTP contract SHALL frame each complete stream-part JSON value as one SSE event with `data: <JSON>\n\n` and no `event:` discriminator. Normal protocol completion SHALL be EOF after the final event, with no required or emitted `[DONE]` sentinel. Pinned-client evidence MAY verify that `[DONE]` is tolerated, but `[DONE]` SHALL NOT be a schema-valid stream part or a required protocol element. H1 SHALL NOT define server commitment, flushing, cancellation, timeout, write-failure, or post-commit error behavior; those belong to the later streaming-service capability.

#### Scenario: Every stream arm has positive and negative coverage
- **WHEN** contract tests enumerate the pinned stream taxonomy
- **THEN** every arm SHALL have a valid representative and an inactive-sibling or required-field negative case

#### Scenario: Generated file unions are constrained
- **WHEN** a generated file or reasoning-file stream part selects its data arm
- **THEN** only pinned generated `data` and `url` variants SHALL be valid

#### Scenario: Tool result nullability is exact
- **WHEN** a streamed tool-result has `result: null`
- **THEN** the stream-part schema SHALL reject it while allowing other JSON value families

#### Scenario: Stream event framing is exact
- **WHEN** a complete stream-part JSON value is represented as an SSE event
- **THEN** it SHALL be framed as one `data:` field followed by a blank line and SHALL NOT use an `event:` discriminator

#### Scenario: Clean EOF is the protocol terminator
- **WHEN** a pinned client consumes the final complete SSE event followed by response-body EOF
- **THEN** it SHALL receive that event and complete without requiring a sentinel

#### Scenario: Runtime stream lifecycle is deferred
- **WHEN** the H1 contract artifacts are inspected
- **THEN** they SHALL define status, media type, event framing, and EOF but SHALL NOT claim server commitment, flush, cancellation, timeout, write-failure, or post-commit behavior

#### Scenario: DONE is tolerance evidence only
- **WHEN** a response-consumption fixture includes `data: [DONE]`
- **THEN** the pinned client MAY ignore it, but the contract corpus SHALL not classify it as a stream-part payload

### Requirement: Safe error schema with explicit retryability

The non-2xx error schema SHALL be a closed JSON object containing an `error` object and optional string `generationId`. Every nested error arm SHALL require a safe message, integer `statusCode`, and boolean `isRetryable`. Its discriminator SHALL be exactly one of `authentication_error`, `invalid_request_error`, `rate_limit_exceeded`, `model_not_found`, `internal_server_error`, `failed_dependency`, or `forbidden`; client-originated `response_error` and `timeout_error` SHALL NOT be wire-valid types. Only `model_not_found` MAY contain an exact optional `param` object with one string `modelId`, and that value SHALL identify the requested public model rather than a backend model. Only `forbidden` MAY contain an exact optional `param` object with one string `ruleId`. Every other arm SHALL reject `param`, and every arm SHALL reject `code`. The nested status code SHALL equal the HTTP response status. Explicit retryability SHALL be authoritative for the future Go client even when it contradicts status-based inference.

The envelope SHALL NOT contain backend URLs, request values, response headers or bodies, provider identity, credentials, backend model IDs, arbitrary backend data, raw validator messages, unrestricted `param` or `code`, prompts, tool inputs, provider options, or schema fragments. The added status and retryability fields SHALL be classified as a local serialized projection accepted by the permissive pinned Gateway client.

#### Scenario: Pinned Gateway recognizes the envelope
- **WHEN** the stock client receives a representative non-2xx error body valid under the schema
- **THEN** it SHALL surface the nested message and classification rather than an invalid-format fallback

#### Scenario: HTTP status and body status agree
- **WHEN** an error fixture's nested status differs from the HTTP status represented by its envelope metadata
- **THEN** contract validation SHALL reject the fixture

#### Scenario: Wire error types are enumerated
- **WHEN** an error uses `response_error`, `timeout_error`, or another unrecognized type
- **THEN** the error schema SHALL reject it rather than rely on the pinned client's default classification

#### Scenario: Classification params are arm-specific
- **WHEN** an error carries `param`
- **THEN** it SHALL be an exact public-model `modelId` object on `model_not_found` or exact `ruleId` object on `forbidden`, and all other shapes or arms SHALL fail

#### Scenario: Explicit retryability overrides are representable
- **WHEN** fixtures represent a retryable 400 and a non-retryable 500
- **THEN** the schema and future-Go-client projection SHALL preserve the explicit boolean while stock-client behavior is documented separately

#### Scenario: Unsafe backend fields fail closed
- **WHEN** an error object includes a legacy URL, headers, body, request values, provider identity, credentials, backend model ID, `code`, unrestricted `param`, or arbitrary backend data field
- **THEN** the closed error schema SHALL reject it

### Requirement: Strict JSON syntax policy

Before schema validation, contract syntax evidence SHALL require exactly one top-level JSON value followed only by whitespace. It SHALL reject duplicate decoded object names at every depth, including escaped-equivalent names, invalid raw UTF-8, lone high or low surrogate escapes, malformed escape sequences, truncation, and trailing values. It SHALL accept valid surrogate pairs and valid trailing whitespace. Syntax validation SHALL preserve the original bytes for subsequent semantic parsing.

The selected H1 mechanism SHALL remain contract-test or tooling code and SHALL NOT become a production request decoder in this phase.

#### Scenario: Nested duplicate name fails
- **WHEN** any object contains a repeated decoded member name at the same depth
- **THEN** strict syntax validation SHALL reject the document before schema validation

#### Scenario: Escaped-equivalent name fails
- **WHEN** one object contains both `a` and `\u0061` as member names
- **THEN** strict syntax validation SHALL treat them as duplicates and reject the document

#### Scenario: Unicode scalar validity is enforced
- **WHEN** a string contains invalid UTF-8 or a lone surrogate escape
- **THEN** strict syntax validation SHALL reject it, while a valid surrogate pair SHALL pass

#### Scenario: Trailing value fails
- **WHEN** a valid JSON value is followed by another non-whitespace value
- **THEN** strict syntax validation SHALL reject the complete body

#### Scenario: Contract syntax tooling is not a handler
- **WHEN** H1 production packages are inspected
- **THEN** no HTTP request path SHALL call the strict syntax mechanism

### Requirement: Reproducible and privacy-safe stock-client captures

The repository SHALL provide a deterministic recording server independent of the Go provider-wire handler. It SHALL capture direct `doGenerate` and `doStream` calls and orchestration-level `generateText` and `streamText` calls from the exact pinned packages. Method/path, allowlisted normalized headers, and parsed semantic JSON SHALL be stored separately with provenance metadata.

Capture coverage SHALL include unary and streaming envelopes, all message roles, representative prompt and nested tool-result files, function/provider tools, tool choice, client- and provider-executed tool flows, structured response formats, explicit empty collections, opaque null where allowed, body headers, provider options including Gateway controls, raw-chunk intent, and HTTP header precedence/collisions.

Authorization values, credentials, complete volatile user-agent values, and environment-derived observability identifiers SHALL NOT be committed. Normal verification SHALL recapture into a temporary directory and compare semantic artifacts without rewriting them. Updating committed captures SHALL require a separate explicit command.

#### Scenario: Captures do not depend on the Go handler
- **WHEN** the contract capture command runs
- **THEN** requests SHALL terminate at the deterministic recorder without starting or importing the legacy or future V4 handler

#### Scenario: Capture components are separated
- **WHEN** a committed scenario is inspected
- **THEN** its method/path, relevant headers, semantic JSON, and provenance SHALL be independently reviewable

#### Scenario: Header precedence is executable evidence
- **WHEN** configured, call-option, model-routing, and observability header sources collide
- **THEN** a capture SHALL record the final safe header projection while preserving body `headers` as a separate request field

#### Scenario: Verification does not mutate fixtures
- **WHEN** the ordinary capture verification task runs against unchanged pinned behavior
- **THEN** it SHALL leave committed fixtures unchanged and succeed by semantic comparison

#### Scenario: Capture privacy is enforced
- **WHEN** committed capture artifacts are scanned
- **THEN** they SHALL contain no credential, authorization value, machine-local path, volatile request identifier, or unredacted secret-bearing header

### Requirement: Provenance-separated response and negative corpus

Stock-client request captures SHALL be distinguished from locally authored response-consumption projections and local negative fixtures. Response projections SHALL cover unary JSON, EOF-terminated SSE, tolerated `[DONE]`, raw-part filtering based on `includeRawChunks`, response-metadata timestamp conversion, representative non-2xx classifications, and retryability inference. Negative fixtures SHALL cover envelope, syntax, schema, unknown-field, inactive-arm, missing/null/type, file/reference, and extension-boundary failures.

Provider response projections and transport-failure cases SHALL NOT be labeled as live provider recordings or Vercel server captures. Each negative fixture SHALL declare its expected validation stage, stable category, and safe instance path where applicable.

#### Scenario: Response projection provenance is truthful
- **WHEN** a locally authored response is used to test pinned client consumption
- **THEN** its index entry SHALL identify it as a local serialized projection rather than captured server behavior

#### Scenario: Negative failure is intentional
- **WHEN** contract validation evaluates a negative fixture
- **THEN** it SHALL fail at the declared syntax, envelope, or schema stage with the expected stable category and safe path

#### Scenario: Raw filtering is observed
- **WHEN** equivalent streams are consumed with raw chunks disabled and enabled
- **THEN** the pinned client SHALL filter or retain the raw part according to `includeRawChunks`

#### Scenario: Timestamp conversion is observed
- **WHEN** a response-metadata stream event contains an RFC 3339 timestamp string
- **THEN** the pinned client SHALL expose the corresponding date value after consumption

### Requirement: Repeatable contract validation and parity classification

The repository SHALL provide `mise run validate-providerwire-v4-contract` to validate OpenAPI, compile schemas, validate every positive payload, and verify every negative fixture. It SHALL provide `mise run test-interop-contract` to verify pinned request captures and response consumption. Capture replacement SHALL use a separate explicit task.

The parity coverage map SHALL classify ProviderWire V4 HTTP contract evidence separately from legacy Grafana provider-wire transport. The H1 classification SHALL claim automated request emission, curated schema validation, and pinned response consumption only. It SHALL explicitly state that no V4 runtime or private-server acceptance is covered.

#### Scenario: Contract validation is one command
- **WHEN** a contributor runs `mise run validate-providerwire-v4-contract`
- **THEN** OpenAPI/reference, schema compilation, positive corpus, and negative corpus checks SHALL all execute and fail on drift

#### Scenario: Interop contract verification uses baseline pins
- **WHEN** a contributor runs `mise run test-interop-contract`
- **THEN** baseline validation SHALL confirm the TypeScript consumer pins and the suite SHALL verify captures and response consumption

#### Scenario: Legacy parity claims remain separate
- **WHEN** the parity coverage map is reviewed
- **THEN** existing legacy transport coverage SHALL remain intact and the V4 row SHALL not claim handler, client, provider, or frontend runtime behavior

### Requirement: Bounded code-generation evaluation

The phase SHALL evaluate one pinned JSON-Schema-native Go generator and one pinned OpenAPI-oriented Go generator against a standalone difficult-union corpus. The evaluation SHALL record tool versions and commands, deterministic clean generation, compilation, semantic round trips, absent/empty/false presence preservation, exact selected-arm behavior, opaque JSON preservation, and whether manual edits are required.

Generated output SHALL remain temporary or ignored and SHALL NOT become production DTOs in this phase. Production generation SHALL remain deferred unless a candidate passes every gate without manual edits; a passing evaluation alone SHALL NOT authorize generated production code in H1.

#### Scenario: Difficult unions gate a generator
- **WHEN** a candidate is evaluated
- **THEN** the corpus SHALL include role, tool, and file unions, nullable and non-null opaque JSON, keyed object extensions, explicit empty collections, and optional false values

#### Scenario: Manual repair rejects adoption
- **WHEN** generated code requires manual edits or loses any gated semantic distinction
- **THEN** the evaluation SHALL record deferral and SHALL NOT commit the generated types

#### Scenario: Evaluation is reproducible and cleaned
- **WHEN** the documented evaluation command runs twice from clean inputs
- **THEN** it SHALL produce deterministic results and remove or ignore generated output after evidence is recorded

### Requirement: Coordinated baseline evolution

Any future change to the registered ProviderWire V4 package set or relied-on serialized behavior SHALL update the baseline manifest, TypeScript dependency pins, source-equivalence evidence, OpenAPI, JSON Schemas, boundary ledger, captures, response projections, negative corpus, parity classification, generated lockfiles, and affected implementations in one parity-governed change. The stable package set SHALL satisfy the repository's minimum release age.

#### Scenario: Baseline upgrade is atomic
- **WHEN** a tracked upstream package version changes
- **THEN** contract evidence and machine-readable schemas SHALL be regenerated and reviewed in the same change rather than silently retaining the old projection

#### Scenario: Closed-schema drift is classified
- **WHEN** a new upstream field causes a capture or schema failure under specification version 4
- **THEN** the difference SHALL be classified and deliberately incorporated or recorded as a gap before compatibility is claimed

#### Scenario: Contract phase hands off cleanly
- **WHEN** H1 is complete
- **THEN** its OpenSpec change SHALL be validated, verified, synchronized, and archived with zero active changes before a strict unary service phase begins
