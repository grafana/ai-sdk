# providerwire-v4-http-contract Specification

## Purpose

Define the complete baseline-pinned strict ProviderWire V4 HTTP contract and its executable TypeScript compatibility evidence.

## Requirements

### Requirement: Registered ProviderWire V4 contract workspace

The repository SHALL provide a private `ai-gateway/test/providerwire-v4` TypeScript workspace that executes against the exact `@ai-sdk/gateway`, `@ai-sdk/provider`, and `@ai-sdk/provider-utils` versions declared in `test/conformance/upstream.yaml`. The workspace SHALL use the public registered Gateway client with injected transport behavior and SHALL NOT import from a mutable upstream checkout or substitute another package version.

#### Scenario: Workspace dependencies match the baseline
- **WHEN** baseline validation inspects `ai-gateway/test/providerwire-v4/package.json`
- **THEN** every declared `ai` or `@ai-sdk/*` dependency SHALL exactly match `test/conformance/upstream.yaml`

#### Scenario: Real registered client is exercised
- **WHEN** request or response contract tests run
- **THEN** they SHALL invoke the public client exported by the registered `@ai-sdk/gateway` package
- **AND** they SHALL NOT invoke a locally copied implementation of its request projection or response parser

### Requirement: AGPL Gateway repository boundary

The ProviderWire V4 production schema and its Gateway-owned contract workspace SHALL live under the top-level `ai-gateway/` directory. That directory SHALL be the separate Go module `github.com/grafana/ai-sdk/ai-gateway` and, unless a nearer license states otherwise, SHALL be licensed under AGPL-3.0-only. The root `LICENSE` and reusable SDK files outside `ai-gateway/` SHALL remain Apache-2.0.

The dependency boundary SHALL be one-way: Gateway code MAY import explicitly pinned SDK modules, but no Go module or production package outside `ai-gateway/` SHALL import, require, or replace the Gateway module. The Gateway module SHALL remain absent from the root `go.work` and root module graph. Root SDK build and test verification SHALL run successfully with `GOWORK=off`.

Root and Gateway documentation SHALL state the applicable license and contribution scope. Gateway notice material SHALL record the registered Vercel AI SDK provenance and preserve applicable Apache attribution when SDK components are incorporated. The repository SHALL NOT claim that licenses already granted for published revisions are revoked or altered. Grafana legal confirmation of the effective transition, copyright provenance, Apache-derived attribution, and network corresponding-source offer mechanism SHALL be a pre-merge and pre-deployment requirement.

#### Scenario: Gateway license and module are scoped by location
- **WHEN** a file is owned by the Gateway product or its ProviderWire contract
- **THEN** it SHALL live under `ai-gateway/` and be governed by the nearest AGPL-3.0-only license
- **AND** `ai-gateway/go.mod` SHALL declare `github.com/grafana/ai-sdk/ai-gateway`
- **AND** the root Apache-2.0 license SHALL remain unchanged

#### Scenario: Apache modules remain independent
- **WHEN** module-boundary verification runs
- **THEN** no Go source or module outside `ai-gateway/` SHALL import, require, or replace the Gateway module
- **AND** the root `go.work` and root module graph SHALL exclude it
- **AND** the root SDK SHALL build and test with `GOWORK=off`

#### Scenario: License and provenance are documented
- **WHEN** contributors inspect the repository or Gateway contribution guidance
- **THEN** the Apache-2.0 and AGPL-3.0-only scopes SHALL be explicit
- **AND** applicable Gateway notice material SHALL identify registered-client contract provenance and incorporated Apache components

#### Scenario: Legal readiness remains an external gate
- **WHEN** this boundary is proposed for merge or a Gateway build is proposed for deployment
- **THEN** Grafana legal confirmation SHALL be required
- **AND** repository documentation SHALL NOT invent a confirmation or claim to revoke prior license grants

### Requirement: Complete production request schema

The repository SHALL provide `ai-gateway/providerwire/v4/schema/request.json` as a hand-authored draft 2020-12 JSON Schema for the complete JSON serialization projection of `Omit<LanguageModelV4CallOptions, "abortSignal">` at the registered baseline. The schema SHALL describe all registered request capabilities whether or not the first Go runtime supports them, SHALL require the root object and `prompt`, and SHALL close finite protocol-owned objects and tagged unions.

The schema SHALL model role-specific message content, file-data arms, function and provider tools, tool choice, tool-result output arms, approval responses, response format, provider options, body-carried headers, reasoning, raw-chunk selection, and scalar generation settings. `maxOutputTokens`, `topK`, and `seed` SHALL be integers; continuous sampling and penalty settings SHALL be numbers. Schema-valued payloads SHALL remain opaque JSON Schema objects. Each provider-options namespace SHALL be a JSON object whose nested JSON remains opaque. A provider-reference map SHALL contain provider-name string values and SHALL forbid the reserved `type` property. Provider-tool `id` and custom-part `kind` SHALL match the registered `${string}.${string}` shape by containing at least one period.

#### Scenario: Every registered request branch is schema-valid
- **WHEN** a schema case supplies each registered role, content discriminator, file-data arm, tool kind, tool choice, tool-result output arm, approval arm, response format, scalar setting, header map, or provider-options shape with valid required members
- **THEN** the complete request SHALL validate against `request.json`

#### Scenario: Finite objects and unions are closed
- **WHEN** a request contains an unknown root member, unknown finite-object member, unknown discriminator, inactive union member, mixed union arms, or role-incompatible content
- **THEN** validation against `request.json` SHALL fail

#### Scenario: Presence and zero values are preserved
- **WHEN** a request contains registered explicit `false`, integer zero, finite floating-point zero, empty string, empty array, empty object, or nested opaque `null`
- **THEN** validation SHALL preserve and accept the value wherever the registered type permits it
- **AND** omission SHALL remain distinguishable from explicit presence in the parsed semantic request

#### Scenario: Numeric categories are strict
- **WHEN** `maxOutputTokens`, `topK`, or `seed` is fractional
- **THEN** validation SHALL fail
- **AND** finite integer values for those fields and finite numeric values for continuous controls SHALL validate

#### Scenario: Provider option namespaces are objects
- **WHEN** each top-level `providerOptions` value is an object containing arbitrary nested JSON
- **THEN** validation SHALL succeed
- **AND** nested arrays, objects, null, false, zero, and empty values SHALL remain semantically unchanged

#### Scenario: Provider option namespace is malformed
- **WHEN** a top-level `providerOptions` namespace value is an array, scalar, or null
- **THEN** validation SHALL fail

#### Scenario: Provider reference without the tagged-union key is valid
- **WHEN** a reference file-data arm contains a provider-reference object whose values are strings and which omits `type`
- **THEN** validation SHALL succeed

#### Scenario: Provider reference with the tagged-union key is invalid
- **WHEN** a provider-reference object contains a `type` property
- **THEN** validation SHALL fail even when the property value is a string

#### Scenario: Dotted provider and custom identifiers are valid
- **WHEN** a provider-tool `id` or custom-part `kind` contains at least one period
- **THEN** validation SHALL accept that registered template-literal shape

#### Scenario: Undotted provider and custom identifiers are invalid
- **WHEN** a provider-tool `id` or custom-part `kind` contains no period
- **THEN** validation SHALL fail

#### Scenario: Abort signal is not serializable contract input
- **WHEN** a raw request body contains an `abortSignal` member
- **THEN** validation SHALL fail because the registered Gateway client removes that member before serialization

### Requirement: Exhaustive finite TypeScript surface coverage

The contract workspace SHALL contain compile-time exhaustive maps or switches for every finite request and response surface used by the ProviderWire V4 language-model contract. Coverage SHALL include request keys excluding `abortSignal`, prompt roles, role-specific request content discriminators, file-data arms, function/provider tool kinds, tool-choice kinds, tool-result output arms, approval-response arms, unary generated-content types, the nested URL/document `sourceType` discriminator, stream-part discriminators, warning variants, and finish-reason values. The coverage source SHALL NOT generate the production schema or a runtime support classifier.

#### Scenario: Registered request key changes
- **WHEN** a baseline package adds or removes a key from `Omit<LanguageModelV4CallOptions, "abortSignal">`
- **THEN** TypeScript typechecking SHALL fail until the exhaustive request-key witness is reviewed and updated

#### Scenario: Registered discriminator changes
- **WHEN** a baseline package adds or removes a covered request or response discriminator, including a nested generated-source `sourceType`
- **THEN** TypeScript typechecking SHALL fail until the corresponding exhaustive witness and contract impact are reviewed

#### Scenario: Runtime support remains independent
- **WHEN** compile-time coverage includes a registered capability not implemented by the current Go runtime
- **THEN** the coverage SHALL describe contract completeness only
- **AND** it SHALL NOT mark that capability as executable

### Requirement: Real Gateway HTTP request capture

The workspace SHALL capture semantic HTTP requests by invoking the registered `createGateway` client with an injected `fetch`. Each capture SHALL record request order, method, normalized relative path, normalized contract-relevant outer headers, unary or streaming mode, and the parsed semantic JSON body. JSON object member order and volatile transport headers SHALL NOT be contract authority; array order and multi-request order SHALL remain significant.

#### Scenario: Unary language-model envelope is captured without collisions
- **WHEN** the registered client performs a unary language-model call and no configured or call header collides case-insensitively with a protocol header
- **THEN** the capture SHALL contain `POST /language-model`, `Content-Type: application/json`, specification version `4`, the exact model ID supplied to the client, streaming value `false`, and the semantic JSON body

#### Scenario: Streaming language-model envelope is captured without collisions
- **WHEN** the registered client performs a streaming language-model call and no configured or call header collides case-insensitively with a protocol header
- **THEN** the capture SHALL contain `POST /language-model`, specification version `4`, the exact model ID supplied to the client, streaming value `true`, and the semantic JSON body

#### Scenario: Protocol headers occur once
- **WHEN** any captured language-model request is normalized
- **THEN** each `ai-language-model-specification-version`, `ai-language-model-id`, and `ai-language-model-streaming` header SHALL have exactly one effective value

#### Scenario: Abort signal is removed
- **WHEN** call options include an `abortSignal`
- **THEN** the signal SHALL be passed to the injected fetch for cancellation
- **AND** the captured JSON body SHALL omit `abortSignal`

#### Scenario: URL and binary file values are projected
- **WHEN** call options contain URL-valued file data and `Uint8Array` data in registered file, reasoning-file, or tool-result file positions
- **THEN** the captured body SHALL contain URL strings and base64 strings exactly as emitted by the registered client

#### Scenario: Call headers have two distinct roles
- **WHEN** call options include a `headers` object
- **THEN** its serializable members SHALL remain in the captured JSON body
- **AND** those values SHALL participate in the client's outer HTTP header composition independently of body serialization

#### Scenario: Case-variant model header collision is captured
- **WHEN** configured headers seed canonical lowercase `ai-language-model-id`, call headers provide a case-variant model header with value `call`, and the model was created with ID `actual`
- **THEN** the capture SHALL record the registered client's single final case-insensitively normalized `ai-language-model-id` value as `call`
- **AND** the body-carried call headers SHALL retain the serializable case-variant entry
- **AND** a server SHALL treat `call` as the requested model identity because the model object's original `actual` value is not observable on the wire

#### Scenario: Other outer-header precedence is observable
- **WHEN** configured, call-level, protocol, and observability sources provide contract-relevant headers without a case-variant protocol collision
- **THEN** the captured final normalized headers SHALL match the precedence emitted by the registered client

### Requirement: Compact semantic request goldens

The repository SHALL commit compact semantic request goldens emitted by the real registered client. The golden families SHALL cover unary scalar and presence semantics; comprehensive roles, content, files, tools, results, approvals, response format, and provider-option unions; streaming envelope and mode; body-header duplication, ordinary outer-header precedence, and a case-variant protocol-header collision; and an ordered multi-call sequence only when it proves behavior not represented by individual calls. Every committed golden request body SHALL validate against the production request schema.

#### Scenario: Scalar and presence golden is checked
- **WHEN** the unary scalar golden is regenerated in memory
- **THEN** it SHALL prove omission and explicit false, zero, empty string, empty collection, empty object, and opaque nested JSON semantics

#### Scenario: Comprehensive union golden is checked
- **WHEN** the comprehensive request golden is regenerated in memory
- **THEN** it SHALL contain valid representative values for every registered request union family
- **AND** the emitted body SHALL validate against `request.json`

#### Scenario: Streaming and header goldens are checked
- **WHEN** streaming and header cases are regenerated in memory
- **THEN** they SHALL prove the streaming envelope, body-carried headers, ordinary normalized outer-header precedence, and the final emitted value for the case-variant model-header collision

#### Scenario: Request sequence order is checked
- **WHEN** a committed case contains multiple client calls
- **THEN** regenerated captures SHALL match the committed request count and order exactly

### Requirement: Schema and golden drift verification

Normal contract verification SHALL compile the production schema, run focused positive and negative schema cases, regenerate every semantic request capture in memory, validate each captured body against the schema, and compare the captures with committed goldens. Normal verification SHALL NOT write tracked files.

#### Scenario: Schema no longer accepts a client golden
- **WHEN** the registered client emits a body that the production schema rejects
- **THEN** contract verification SHALL fail with the affected case and schema validation error

#### Scenario: Client projection drifts
- **WHEN** an in-memory semantic capture differs from its committed golden
- **THEN** contract verification SHALL fail with a reviewable semantic diff
- **AND** the committed golden SHALL remain unchanged

#### Scenario: Normal verification is non-mutating
- **WHEN** the aggregate ProviderWire V4 contract check runs
- **THEN** it SHALL leave the schema, source cases, goldens, package manifests, and lockfile unchanged

### Requirement: Focused unary client-consumption evidence

The workspace SHALL exercise unary success through the registered client using an injected response. The probe SHALL assert consumption of representative generated content, finish reason, usage, and response headers, and SHALL explicitly assert the client's overwrite behavior for `request`, `response`, and `warnings`.

#### Scenario: Unary result is consumed
- **WHEN** the injected fetch returns a valid representative JSON generate result
- **THEN** the registered client SHALL resolve with the representative content, finish reason, and usage

#### Scenario: Client-owned unary fields replace server fields
- **WHEN** the unary response body includes server-supplied `request`, `response`, and `warnings`
- **THEN** the resolved result SHALL contain the request body, raw response data, response headers, and warning values assigned by the registered client rather than those server-supplied fields

### Requirement: Focused streaming client-consumption evidence

The workspace SHALL exercise streaming success through the registered client using SSE responses. The probes SHALL cover clean EOF after the final JSON event, tolerated `[DONE]`, raw-part filtering based on `includeRawChunks`, and response-metadata timestamp conversion.

#### Scenario: Finish followed by clean EOF is consumed
- **WHEN** the SSE response emits valid stream parts including `finish` and then closes without `[DONE]`
- **THEN** the registered client stream SHALL deliver the parts in order and close successfully

#### Scenario: DONE sentinel is tolerated
- **WHEN** the SSE response contains `data: [DONE]`
- **THEN** the registered client SHALL ignore the sentinel without emitting a stream part or failing the stream

#### Scenario: Raw parts are suppressed by default
- **WHEN** SSE contains raw parts and `includeRawChunks` is absent or false
- **THEN** the registered client SHALL omit those raw parts

#### Scenario: Requested raw parts are preserved
- **WHEN** SSE contains raw parts and `includeRawChunks` is true
- **THEN** the registered client SHALL preserve them in order

#### Scenario: Response metadata timestamp is converted
- **WHEN** a response-metadata part contains a timestamp string
- **THEN** the registered client SHALL expose that timestamp as a `Date` with the same instant

### Requirement: Focused non-success client-consumption evidence

The workspace SHALL exercise representative non-2xx JSON responses through the registered client and assert the public error classification, status, and message observable at the registered package boundary. Any malformed-response coverage SHALL treat registered fallback behavior only as client evidence and SHALL NOT define the server error envelope.

#### Scenario: Structured non-2xx response is consumed
- **WHEN** the injected fetch returns a representative structured non-2xx Gateway response
- **THEN** unary and streaming setup calls SHALL reject with the registered public Gateway error behavior for that status and body

#### Scenario: Error probe remains client evidence
- **WHEN** a non-2xx response is accepted or normalized by the registered client
- **THEN** that result SHALL document client consumption only
- **AND** it SHALL NOT establish which fields the strict server may emit

### Requirement: Explicit golden update workflow

The repository SHALL provide a focused explicit command that regenerates committed semantic request goldens through the registered client. The update command SHALL write only known request golden files and SHALL then validate the updated captures against the production schema. It SHALL NOT generate or modify the production schema, compile-time witnesses, response probes, baseline manifest, or package dependency pins.

#### Scenario: Contributor explicitly updates goldens
- **WHEN** a contributor invokes the golden update command after a reviewed baseline or contract change
- **THEN** only the semantic request golden files SHALL be rewritten from real client captures
- **AND** the command SHALL fail if any updated request body violates `request.json`

#### Scenario: Ordinary check cannot bless drift
- **WHEN** a contributor or CI invokes the normal ProviderWire V4 check
- **THEN** no update mode SHALL be enabled implicitly
- **AND** observed drift SHALL fail rather than rewrite expected files

### Requirement: Contract evidence boundary

The exact registered public `@ai-sdk/gateway` client SHALL be authoritative for its observable request emission and response consumption. The ProviderWire V4 workspace SHALL describe one strict HTTP dialect compatible with that client and SHALL NOT claim compatibility with every request accepted by Vercel's private Gateway service. Private protocol DTOs and test-time schemas SHALL own server-side shapes the client does not observe, while raw HTTP, privacy, and bounds tests SHALL own unobserved server safety properties.

#### Scenario: Observable client behavior is authoritative
- **WHEN** request emission or response consumption is observable through the registered client
- **THEN** contract evidence SHALL treat that behavior as authoritative for the strict ProviderWire V4 dialect
- **AND** it SHALL NOT infer acceptance by Vercel's private Gateway service

#### Scenario: Unobserved server shape and safety remain independent
- **WHEN** the registered client masks a field, permissively accepts arbitrary response JSON, or cannot observe a server safety property
- **THEN** private protocol DTOs and test-time schemas SHALL define the unobserved server shape
- **AND** raw HTTP, privacy, and bounds tests SHALL define the unobserved server safety requirement
- **AND** those authorities SHALL NOT contradict observable registered-client behavior

#### Scenario: Production unary replay is established
- **WHEN** the strict unary runtime is complete
- **THEN** each committed request emitted by the registered client SHALL replay to its expected unary result
- **AND** streaming records SHALL fail unary envelope validation without model resolution
- **AND** unary records SHALL reach complete schema validation and either supported execution or a safe unsupported-family response
- **AND** dedicated supported scalar and focused one-capability requests SHALL cover behavior that multi-capability goldens cannot isolate
- **AND** a pinned registered client SHALL complete a supported minimal unary text call against the real Go handler

#### Scenario: Streaming remains deferred
- **WHEN** this unary runtime change is complete
- **THEN** strict streaming commitment, event state, SSE framing, and clean-EOF behavior SHALL remain unimplemented by the Go handler
- **AND** the phase 2 streaming client probes SHALL remain consumption evidence only
