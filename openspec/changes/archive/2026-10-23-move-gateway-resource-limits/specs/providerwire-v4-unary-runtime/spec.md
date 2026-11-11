## ADDED Requirements

### Requirement: Unary output validity without size accounting
Before HTTP 200 the handler SHALL validate represented strings as UTF-8, warning and content discriminators, response timestamps, token ranges, tool lifecycle, metadata namespace names and object values, and raw usage as one JSON object. It SHALL NOT count, budget or preflight the size or cardinality of content, warnings, metadata, identity or raw usage; size is owned by gateway-resource-policy.

#### Scenario: Large valid result succeeds
- **WHEN** a valid result has more content parts, warnings or metadata namespaces, or longer strings, than the former 8 MiB unary budget could hold
- **THEN** the handler SHALL return HTTP 200 with the complete document

#### Scenario: Raw usage above the former cap
- **WHEN** provider usage carries a valid raw JSON object above 1,048,576 bytes, including a whitespace-heavy object
- **THEN** `usage.raw` SHALL contain that object with its native keys

#### Scenario: Represented values are malformed
- **WHEN** a source, warning or identity field contains invalid UTF-8, or a nonzero timestamp cannot encode a registered date-time
- **THEN** the response SHALL fail before HTTP 200 without substituting or reflecting invalid values

#### Scenario: Raw UTF-8 and JSON escapes
- **WHEN** provider raw usage contains invalid UTF-8 in an object key or nested value
- **THEN** the handler SHALL return the fixed internal error without committing HTTP 200
- **WHEN** raw usage contains valid JSON with lone or paired escaped surrogates
- **THEN** the handler SHALL preserve the raw JSON escapes

#### Scenario: Invalid metadata namespace
- **WHEN** a result or content metadata namespace has an invalid UTF-8 name, or a value that is not a valid UTF-8 JSON object
- **THEN** the whole result SHALL fail before HTTP 200, without projection or metadata-free success

### Requirement: Single unary success encoding
The handler SHALL encode the complete private success DTO once through standard Go JSON, including any optional execution metadata, and SHALL write HTTP 200 only after that encoding succeeds. Valid raw JSON escapes SHALL be preserved, provider-domain JSON marshalers SHALL NOT control public output, and no value SHALL be truncated.

#### Scenario: Encoding succeeds
- **WHEN** a validated result and its optional overview encode successfully
- **THEN** HTTP 200 SHALL carry exactly that one encoded document

#### Scenario: Encoding fails
- **WHEN** standard encoding of the success document fails, for example on malformed namespace JSON
- **THEN** the handler SHALL return the fixed internal-error document, with its optional execution overview, before HTTP 200 and without replaying generation

### Requirement: Single HTTP error encoding
Each HTTP error document SHALL be encoded once together with its optional providerMetadata. When that encoding fails, the handler SHALL write the fixed base document for the same category. The document SHALL NOT be size-limited, and the optional fields SHALL NOT change status, type, code, message or retryability.

#### Scenario: Large error overview
- **WHEN** a classified failure carries an execution overview whose encoded document exceeds 64 KiB
- **THEN** the complete document with the overview SHALL be written

#### Scenario: Optional metadata cannot be encoded
- **WHEN** encoding the error with its optional metadata fails
- **THEN** the fixed base document with the same status and fields SHALL be written

## MODIFIED Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil host-owned request selector independent of catalog interfaces and positive limits for total model duration, stream idle duration, and bounded post-cancellation drain duration. Construction SHALL NOT accept request, response, frame or stream-part size limits.

#### Scenario: Valid construction
- **WHEN** a caller supplies a request selector and positive duration limits
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the selector is nil or a duration limit is non-positive
- **THEN** construction SHALL fail before serving traffic

### Requirement: Bounded complete request validation

After envelope validation, the handler SHALL read the request body to EOF and close it. The host SHALL own the body byte bound under gateway-resource-policy. A read that fails because the host bound was exceeded SHALL return the fixed invalid-request document, and any other read or close failure SHALL return the fixed internal error. The handler SHALL reject invalid UTF-8 input and SHALL validate the complete document against the embedded draft 2020-12 ProviderWire request schema before supported-subset mapping. Malformed JSON and schema-invalid input SHALL fail before resolution or model invocation.

#### Scenario: Request byte boundary
- **WHEN** a request served behind the host bound is below, exactly at, or one byte above that bound
- **THEN** the first two SHALL continue and the last SHALL return the fixed invalid-request document before resolution or model invocation

#### Scenario: Body read fails
- **WHEN** reading or closing the body fails for a reason other than the host bound
- **THEN** the handler SHALL return the fixed internal error before resolution or model invocation

#### Scenario: Invalid document
- **WHEN** the body is invalid UTF-8, malformed JSON, contains trailing JSON, or violates the request schema
- **THEN** it SHALL fail before resolution or model invocation

#### Scenario: Standard JSON normalization
- **WHEN** a bounded request repeats an object member
- **THEN** the last decoded value SHALL be authoritative
- **WHEN** a JSON string contains an escaped lone UTF-16 surrogate
- **THEN** it SHALL normalize to U+FFFD

#### Scenario: Registered unsupported branch
- **WHEN** a request uses a schema-valid registered branch that the unary text runtime does not execute
- **THEN** schema validation SHALL succeed and supported-subset mapping SHALL make the support decision

### Requirement: Fixed privacy-safe errors
Every runtime error response SHALL retain fixed classified status, message, type, code, and `param: null` fields from protocol-owned definitions.

#### Scenario: Fixed privacy-safe errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Typed standard JSON encoding SHALL preserve the canonical base response bytes. Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability. Unknown or invalid internal categories SHALL fall back to the fixed internal-error document.

#### Scenario: Authenticated fixed error policy
- **WHEN** configured or BYOK invocation produces a classified failure
- **THEN** Every runtime error response SHALL retain fixed classified status, message, type, code, and `param: null` fields from protocol-owned definitions.
- **AND** Typed standard JSON encoding SHALL preserve the canonical base response bytes.
- **AND** Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability.
- **AND** Unknown or invalid internal categories SHALL fall back to the fixed internal-error document.
- **AND** Arbitrary aggregate/configuration/cause trees and raw bodies/headers SHALL NOT be attached as diagnostic shortcuts. Gateway-produced native summary fields SHALL protect exact configured provider/exporter keys under gateway-attempt-failure-evidence, preserving unrelated BYOK echoes and source data.
- **AND** Optional native candidate-local summaries and observed configured identity MAY accompany the base error under gateway-attempt-failure-evidence, encoded once with the base error; they SHALL NOT change classification or retryability.
- **AND** Host-owned BYOK validation and unsupported-operation failures SHALL use fixed approved messages without copying rejected values or private diagnostics.
- **AND** Stopped or exhausted credential fallback SHALL retain existing fallback error propagation and safe runtime classification rather than use a blanket exhaustion status.
- **AND** Request-only selections SHALL remain eligible for available event-local SSE native summaries. Their current missing unary/setup summary and overview capabilities SHALL be tracked under #317, not treated as a confidentiality policy.

#### Scenario: Actionable credential shape failure
- **WHEN** a credential entry is missing apiKey or the selected provider's array exceeds the account count
- **THEN** the invalid-request response SHALL identify the known field/capability without copying the entry or value

#### Scenario: Discovery unsupported for BYOK
- **WHEN** authenticated BYOK access requests catalog discovery
- **THEN** the host SHALL emit HTTP 400, invalid_request_error, invalid_request, param null and a fixed unsupported-discovery message

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL retain the corresponding fixed safe fields without serializing arbitrary causes; optional native configured execution attribution SHALL follow gateway-attempt-failure-evidence

#### Scenario: Unknown model
- **WHEN** configured catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** Go and pinned Vercel consume BYOK validation, exhaustion or discovery failures
- **THEN** status/type/retryability SHALL match the existing client categories without misclassifying provider rejection as failed Gateway authentication

#### Scenario: BYOK provider echoes request accounts
- **WHEN** authenticated request-only selection receives a native failure containing request-account keys or identifiers before or after stream commitment
- **THEN** fixed classified error fields SHALL remain unchanged, and existing SSE native summaries SHALL preserve unrelated BYOK echoes and protect exact configured provider/exporter-key matches independently of catalog attribution
- **AND** absent BYOK unary/setup summaries and attempt overviews SHALL reflect today's missing capability, not a permanent output-suppression rule

### Requirement: Runtime error detail exclusions

Arbitrary cause trees and raw bodies/headers SHALL NOT be serialized by failure projection. Optional native candidate-local summaries and observed configured identity MAY accompany the base error under gateway-attempt-failure-evidence, encoded once with the base error; they SHALL NOT change classification or retryability. Summary protection SHALL follow gateway-attempt-failure-evidence.

#### Scenario: Runtime error detail exclusions
- **WHEN** a resolver or provider failure carries URL, credentials and metadata
- **THEN** arbitrary cause trees and raw bodies/headers SHALL NOT be serialized; governed native candidate-local summaries SHALL redact exact configured API-key matches while preserving other scalar echoes and fixed base fields

### Requirement: Minimal unary success response

A successful response SHALL contain ordered supported text, function-tool-call, source, reasoning, reasoning-file and custom content, finishReason, usage and a warnings array. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Supported result/content providerMetadata SHALL retain opaque namespace objects and represented presence under gateway-provider-metadata.

#### Scenario: Reasoning-only paid success
- **WHEN** a provider returns a valid reasoning-only result
- **THEN** the Gateway SHALL return success rather than a retryable adaptation error
- **AND** default high-level retries SHALL not invoke the provider again for that valid result

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason and valid usage without warnings or Response
- **THEN** the handler SHALL preserve those values, emit warnings as an empty array and omit response

#### Scenario: Valid custom result
- **WHEN** the model returns text followed by custom content with a dotted kind and metadata
- **THEN** the handler SHALL preserve both parts in order with the kind and metadata unchanged

#### Scenario: Unsupported provider result
- **WHEN** the model returns unsupported content, an unknown finish reason or warning type, invalid usage, nil result or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Provider-private fields
- **WHEN** the model result includes native transport headers/body, provider identity and unrelated provider metadata alongside supported warnings/source/response identity
- **THEN** only the contracted warning/source/response identity fields, existing reasoning/source metadata projections and provider usage.raw SHALL cross the unary boundary
- **AND** native transport diagnostics and unrelated provider metadata SHALL remain omitted

#### Scenario: Native warning variants
- **WHEN** ordered warnings include all registered variants, required empty strings and optional empty details
- **THEN** active native values and order SHALL survive, required empty fields SHALL be present and optional empty details/inactive fields SHALL be omitted

#### Scenario: Native unary identity and client replacement
- **WHEN** provider Response contains an id, modelId and timestamp different from route identity, plus native headers/body/provider fields
- **THEN** raw response SHALL retain only the registered native identity fields at response
- **AND** pinned TS and independent Go clients SHALL retain that raw body while replacing typed response with Gateway-hop information
- **AND** the typed response SHALL NOT adopt native id, modelId or timestamp

#### Scenario: Unary identity presence
- **WHEN** provider Response is nil, present with zero-valued identity, or present with partial identity
- **THEN** response SHALL respectively be absent, an empty object, or contain only the supplied representable fields without fabricated defaults
#### Scenario: Ordinary metadata is not transport
- **WHEN** a model result contains supported result/content providerMetadata alongside unrelated request or response transport fields
- **THEN** the metadata SHALL survive in its registered scope without adding those transport fields or consulting operator capture flags

#### Scenario: Provider raw usage is present or absent
- **WHEN** provider usage contains a valid object of any size including provider-native nested values, an empty object, or no Raw bytes
- **THEN** the unary response SHALL respectively include the object under `usage.raw` with its native keys, include `{}`, or omit the `raw` member without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** provider `Usage.Raw` is nonempty but malformed JSON, JSON null, an array or scalar, or invalid UTF-8
- **THEN** the handler SHALL emit the fixed internal-error document before HTTP success is committed, without serializing the raw data or returning normalized-only success

### Requirement: Unary raw identity required empties and raw usage

Native unary identity SHALL remain in the raw server document and client-owned response body, not be advertised as surviving in typed Response identity fields. Required empty reasoning text and selected empty inline file data SHALL be retained. usage.raw SHALL be omitted when absent and contain a valid supplied provider JSON object unchanged in meaning.

#### Scenario: Unary raw identity required empties and raw usage
- **WHEN** a success contains empty reasoning, selected empty inline data and a valid raw usage object
- **THEN** required empties and raw usage SHALL survive, while native identity SHALL remain raw-body-only rather than typed client identity

### Requirement: Independent unary raw authority frontend evidence and fixture provenance

Raw Go/HTTP tests and local schemas SHALL independently verify active union fields, identity presence, output order, invalid output, lifecycle and large-output acceptance rather than treating permissive parsing as validation. Authenticated command and provider-neutral frontend assembly scenarios SHALL accompany the behavior change. Synthetic responses SHALL NOT be presented as recorded provider evidence, and authentic provider fixture inputs SHALL NOT be rewritten.

#### Scenario: Independent unary raw authority frontend evidence and fixture provenance
- **WHEN** the pinned client accepts an output union permissively
- **THEN** raw/schema/lifecycle/large-output tests SHALL independently verify it, authenticated command/frontend assembly evidence SHALL accompany behavior changes and synthetic responses SHALL NOT become recorded inputs

### Requirement: Optional unary execution overview

The handler SHALL use synchronized request-local observation to optionally enrich a valid unary result or classified invocation failure with a compact execution overview.

#### Scenario: Optional unary execution overview policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Original primary validation SHALL apply before enrichment, and the enriched document SHALL be encoded once. A failure document whose enriched encoding fails SHALL fall back to its fixed base document and classification; original adaptation failures SHALL NOT replay generation. Configured candidate identity SHALL remain separate from native-summary eligibility; only explicit configured API keys SHALL supply summary protection; request credentials SHALL NOT be inventoried. Configured direct calls SHALL retain their existing invocation/cancellation ownership.

#### Scenario: Valid result with no enrichment room
- **WHEN** a valid native result would fit the former 8 MiB unary budget only without its overview
- **THEN** the successful result SHALL be written with the overview and its native metadata intact

#### Scenario: Owned cancellation and late native result
- **WHEN** cancellation wins invocation ownership before a late native result
- **THEN** the existing context error SHALL be returned and published attribution SHALL NOT capture the unowned late native failure

## REMOVED Requirements

### Requirement: Safe handler limit arithmetic and fixed-frame capacity
**Reason**: The handler no longer receives byte or part-count limits, so it has no `limit+1` arithmetic and no frame limit that the fixed frames must fit.
**Migration**: Configure only the duration limits. The request-body bound is applied by the host under gateway-resource-policy.

### Requirement: Bounded preflight and standard success encoding
**Reason**: Unary output is no longer budgeted. Size and cardinality preflight and the final byte check are removed.
**Migration**: Validity is defined in "Unary output validity without size accounting", and encoding in "Single unary success encoding". Size is bounded by gateway-resource-policy.

### Requirement: Aggregate unary warning and raw-usage preflight
**Reason**: Warning cardinality and the 1,048,576-byte raw-usage cap were size budgets.
**Migration**: Warnings and raw usage are validated for shape and UTF-8 under "Unary output validity without size accounting".

### Requirement: Private standard unary encoding and final byte bounds
**Reason**: The final byte bound is removed.
**Migration**: Standard private encoding, escape preservation and no truncation remain under "Single unary success encoding".

### Requirement: Shared unary metadata budget and namespace validity
**Reason**: The shared metadata budget is removed.
**Migration**: Namespace name and object validity remain under "Unary output validity without size accounting" and gateway-provider-metadata.
