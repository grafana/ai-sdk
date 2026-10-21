# Spec Delta

## MODIFIED Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil host-owned request selector independent of catalog interfaces and positive total model duration, stream idle duration and bounded post-cancellation drain duration. The handler SHALL NOT accept request, response, frame or stream-part byte/count limits; request-body and upstream bounds SHALL be owned by the host under gateway-resource-policy.

#### Scenario: Valid construction
- **WHEN** a caller supplies a request selector and positive durations
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the selector is nil or a duration is non-positive
- **THEN** construction SHALL fail before serving traffic

### Requirement: Bounded complete request validation

After envelope validation, the handler SHALL read the request body to its end and close it, relying on the host route's shared request-body bound, and SHALL reject invalid UTF-8 input. A host request-body overflow SHALL fail as the fixed invalid-request document. Any other read failure SHALL fail as the fixed internal-error document. The handler SHALL validate the complete document against the embedded draft 2020-12 ProviderWire request schema before supported-subset mapping. Malformed JSON and schema-invalid input SHALL fail before resolution or model invocation.

#### Scenario: Request byte boundary
- **WHEN** a request is below, exactly at, or one byte above the host route request-body bound
- **THEN** the first two SHALL continue and the last SHALL fail as an invalid request before schema validation, resolution or model invocation

#### Scenario: Invalid document
- **WHEN** the body is invalid UTF-8, malformed JSON, contains trailing JSON, or violates the request schema
- **THEN** it SHALL fail before resolution or model invocation

#### Scenario: Standard JSON normalization
- **WHEN** a request repeats an object member
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
- **AND** Optional native candidate-local summaries and observed configured identity MAY accompany the base error under gateway-attempt-failure-evidence, encoded together with the base error without fit-based omission; they SHALL NOT change classification or retryability. Only an encoding failure of that optional data SHALL fall back to the base document.
- **AND** Host-owned BYOK validation and unsupported-operation failures SHALL use fixed approved messages without copying rejected values or private diagnostics.
- **AND** Stopped or exhausted credential fallback SHALL retain existing fallback error propagation and safe runtime classification rather than use a blanket exhaustion status.
- **AND** Request-only selections SHALL remain eligible for available event-local SSE native summaries. Their current missing unary/setup summary and overview capabilities SHALL be tracked under #317, not treated as a confidentiality policy.

#### Scenario: Actionable credential shape failure
- **WHEN** a credential entry is missing apiKey or the selected provider's array exceeds the account attempt bound
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

Arbitrary cause trees and raw bodies/headers SHALL NOT be serialized by failure projection. Optional native candidate-local summaries and observed configured identity MAY accompany the base error under gateway-attempt-failure-evidence, without a complete-response fit check; they SHALL NOT change classification or retryability. Summary protection SHALL follow gateway-attempt-failure-evidence.

#### Scenario: Runtime error detail exclusions
- **WHEN** a resolver or provider failure carries URL, credentials and metadata
- **THEN** arbitrary cause trees and raw bodies/headers SHALL NOT be serialized; governed native candidate-local summaries SHALL redact exact configured API-key matches while preserving other scalar echoes and fixed base fields

#### Scenario: Large native failure summary
- **WHEN** a configured candidate's native error message exceeds the former 64 KiB error-envelope budget while its native response stayed within the outbound bound
- **THEN** the classified error SHALL carry the complete protected summary rather than omitting it

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
- **WHEN** provider usage contains a valid object including provider-native nested values, an empty object, or no Raw bytes
- **THEN** the unary response SHALL respectively include the object under `usage.raw` with its native keys, include `{}`, or omit the `raw` member without changing normalized counts

#### Scenario: Large raw usage is preserved
- **WHEN** provider usage contains a valid JSON object larger than 1,048,576 bytes
- **THEN** the unary response SHALL include it under `usage.raw` without a raw-usage-specific cap

#### Scenario: Supplied raw usage is invalid
- **WHEN** provider `Usage.Raw` is nonempty but malformed JSON, invalid UTF-8, JSON null, an array or scalar
- **THEN** the handler SHALL emit the fixed internal-error document before HTTP success is committed, without serializing the raw data or returning normalized-only success

### Requirement: Independent unary raw authority frontend evidence and fixture provenance

Raw Go/HTTP tests and local schemas SHALL independently verify active union fields, identity presence, output order, invalid output and lifecycle rather than treating permissive parsing as validation. Authenticated command and provider-neutral frontend assembly scenarios SHALL accompany the behavior change. Synthetic responses SHALL NOT be presented as recorded provider evidence, and authentic provider fixture inputs SHALL NOT be rewritten.

#### Scenario: Independent unary raw authority frontend evidence and fixture provenance
- **WHEN** the pinned client accepts an output union permissively
- **THEN** raw/schema/lifecycle tests SHALL independently verify it, authenticated command/frontend assembly evidence SHALL accompany behavior changes and synthetic responses SHALL NOT become recorded inputs

### Requirement: Optional unary execution overview

The handler SHALL use synchronized request-local observation to optionally enrich a valid unary result or classified invocation failure with a compact execution overview.

#### Scenario: Optional unary execution overview policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Original primary validation SHALL apply before enrichment. A valid result SHALL be encoded once with its enrichment. Only an encoding failure of the optional overview SHALL preserve the original document and classification; size SHALL NOT cause omission. Original adaptation failures SHALL NOT replay generation. Configured candidate identity SHALL remain separate from native-summary eligibility; only explicit configured API keys SHALL supply summary protection; request credentials SHALL NOT be inventoried. Configured direct calls SHALL retain their existing invocation/cancellation ownership.

#### Scenario: Valid result with no enrichment room
- **WHEN** a valid native result would have exceeded the former unary response budget once configured execution attribution was added
- **THEN** the successful result SHALL include both the complete native output and the overview, with any native gateway namespace relocated under gateway-execution-overview

#### Scenario: Owned cancellation and late native result
- **WHEN** cancellation wins invocation ownership before a late native result
- **THEN** the existing context error SHALL be returned and published attribution SHALL NOT capture the unowned late native failure

## ADDED Requirements

### Requirement: Unary output validity without size accounting
Before HTTP 200, the handler SHALL validate represented strings as UTF-8, validate nonzero timestamps as registered date-times, validate each providerMetadata namespace as an object with valid UTF-8 names and values, and validate present raw usage as one valid UTF-8 JSON object. Validation SHALL NOT estimate, accumulate or cap byte sizes or cardinality.

#### Scenario: Invalid represented value
- **WHEN** a source, warning, identity, text, tool or metadata field contains invalid UTF-8, or a timestamp cannot encode a registered date-time
- **THEN** the response SHALL fail with the fixed internal-error document before HTTP 200 without substituting or reflecting invalid values

#### Scenario: Valid surrogate escapes
- **WHEN** raw usage or a metadata namespace contains valid JSON with lone or paired escaped surrogates
- **THEN** the handler SHALL preserve those escapes

#### Scenario: Many content parts and warnings
- **WHEN** a valid result contains more content parts, warnings or metadata namespaces than the former unary budget allowed
- **THEN** the handler SHALL return them all in order

### Requirement: Standard single unary encoding
The complete private DTO SHALL be encoded once through standard Go JSON after validation and optional enrichment. HTTP 200 SHALL be committed only after encoding succeeds. Provider-domain JSON marshalers SHALL NOT control public output, and no value SHALL be truncated. An encoding failure of primary output SHALL return the fixed internal-error document without committing HTTP 200.

#### Scenario: Primary encoding fails
- **WHEN** standard encoding of validated primary output fails
- **THEN** no HTTP 200 SHALL be written and the fixed internal-error document SHALL be returned without replaying generation

#### Scenario: Escaping expands output
- **WHEN** valid strings expand during standard JSON escaping
- **THEN** the complete escaped document SHALL be written without truncation or a size-based failure

## REMOVED Requirements

### Requirement: Safe handler limit arithmetic and fixed-frame capacity
**Reason**: The handler no longer accepts request, unary, frame or part-count limits, so `limit+1` arithmetic and fixed-frame capacity checks have nothing to validate.
**Migration**: The host route applies `max_request_bytes` and the shared provider response bound under gateway-resource-policy; durations remain handler configuration.

### Requirement: Bounded preflight and standard success encoding
**Reason**: Per-field, string and cardinality preflight against a unary budget is adapter-local resource machinery replaced by the shared route request-body and outbound bounds.
**Migration**: Validity checks move to "Unary output validity without size accounting"; encoding moves to "Standard single unary encoding"; resource bounds come from gateway-resource-policy.

### Requirement: Aggregate unary warning and raw-usage preflight
**Reason**: Conservative warning sizing and the independent 1,048,576-byte raw-usage cap are removed.
**Migration**: Raw usage and warnings are validated for shape and UTF-8 only; size is bounded by the outbound response bound.

### Requirement: Private standard unary encoding and final byte bounds
**Reason**: The final unary byte limit is a response-writer budget that cannot prevent completed allocation.
**Migration**: Use "Standard single unary encoding"; output size follows gateway-resource-policy.

### Requirement: Shared unary metadata budget and namespace validity
**Reason**: Metadata no longer shares a unary byte or cardinality budget.
**Migration**: Namespace validity is covered by "Unary output validity without size accounting" and gateway-provider-metadata.
