# Spec Delta

## MODIFIED Requirements

### Requirement: Exact namespace comparison unsupported-scope refusal and header spelling

Namespace names SHALL be compared exactly, because provider-option namespaces are case-significant. Provider options carried by a message role or part type the runtime does not support SHALL NOT be inspected, so such a request SHALL report the family of that role or part type. The handler SHALL map body-carried call headers to provider call headers, preserving each key's original case.

#### Scenario: Unsupported custom-part options are not interpreted during mapping
- **WHEN** a schema-valid request has an assistant `custom` content part of kind `example.custom` carrying an object-valued opaque provider-options namespace
- **THEN** complete schema validation SHALL succeed and, because custom content is now supported under gateway-custom-content, the part's options SHALL reach the model unchanged without a `custom-content` refusal

#### Scenario: Options on a still-unsupported part type are not interpreted
- **WHEN** a schema-valid request has no unsupported features except a tool-approval response carrying an object-valued opaque provider-options namespace
- **THEN** supported-subset mapping SHALL report the `tool-approvals` family without interpreting that part's provider options

#### Scenario: Supported namespace and header case is preserved
- **WHEN** a supported text request carries distinct `anthropic` and `Anthropic` provider-option namespaces and a body header named `X-Request-ID`
- **THEN** the namespaces SHALL remain distinct and the mapped body header key SHALL remain exactly `X-Request-ID`

### Requirement: Unsupported capability families

Schema-valid provider tools and tool approvals, structured output, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Ordinary file inputs and message/file-part options SHALL execute within gateway-file-inputs, alongside existing call-level options and body-carried headers. Assistant custom history and custom tool-result content SHALL execute within gateway-custom-content.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

#### Scenario: Reasoning continuation is supported
- **WHEN** a valid assistant reasoning part carries supported scoped continuation options
- **THEN** the model SHALL receive them without enabling deferred capability families

#### Scenario: Ordinary files no longer trigger blanket rejection
- **WHEN** a schema-valid unary or streaming request contains only supported text, ordinary files, supported tool history, and permitted message/file options
- **THEN** it SHALL map without a blanket files or provider-options failure and continue through the existing execution boundary

#### Scenario: Custom content no longer triggers rejection
- **WHEN** a schema-valid unary or streaming request contains assistant custom history or custom tool-result content
- **THEN** it SHALL map without a `custom-content` or `tools` refusal and continue through the existing execution boundary

### Requirement: Supported function continuation and deferred result option scopes

Function definitions/choices and assistant-call/tool-result history SHALL execute only within the gateway-unary-function-tools and gateway-streaming-function-tools subsets, including the ordinary file-result and custom-result extensions. Function-tool, ordinary file-entry and custom-entry provider options SHALL be supported within those subsets; deferred output-level and text-entry nested result options SHALL remain unsupported.

#### Scenario: Supported function continuation and deferred result option scopes
- **WHEN** a request carries function definitions and tool history whose content has file entries with options and custom entries with options
- **THEN** it SHALL execute only in the unary/streaming function subsets without enabling deferred output-level or text-entry nested options

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
- **WHEN** provider usage contains a valid in-limit object including provider-native nested values, an empty object, or no Raw bytes
- **THEN** the unary response SHALL respectively include the object under `usage.raw` with its native keys, include `{}`, or omit the `raw` member without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** provider `Usage.Raw` is nonempty but malformed JSON, JSON null, an array or scalar, or exceeds its input byte limit
- **THEN** the handler SHALL emit the fixed internal-error document before HTTP success is committed, without serializing the raw data or returning normalized-only success

## ADDED Requirements

### Requirement: Unsupported-functionality rejection classification
When unary invocation or stream setup fails and every attempted failure is the provider unsupported-functionality rejection, the handler SHALL return the fixed non-retryable invalid-request error, which MAY be accompanied by the optional execution overview. Existing unsupported-request, cancellation, deadline and API-call-error classification SHALL take precedence, and an empty join SHALL NOT qualify. Any other mix SHALL keep the existing classification.

#### Scenario: Direct rejection
- **WHEN** a direct route's adapter returns a wrapped unsupported-functionality rejection before any I/O, in unary or streaming setup
- **THEN** the handler SHALL return the fixed invalid-request document and neither registered client SHALL retry the request

#### Scenario: All candidates reject
- **WHEN** every configured-fallback candidate returns the rejection
- **THEN** the handler SHALL return the fixed invalid-request document

#### Scenario: Mixed failures keep existing classification
- **WHEN** fallback attempts end with the rejection and a retryable 503, in either order, or with the rejection and an unknown internal error
- **THEN** the handler SHALL NOT return the invalid-request document solely because the rejection is present, and SHALL keep the existing classification

#### Scenario: API call error wrapping the sentinel
- **WHEN** an API call error's cause wraps the sentinel
- **THEN** the handler SHALL keep the existing API-call-error classification

#### Scenario: Cancellation wins
- **WHEN** the request is canceled after a candidate rejected
- **THEN** the handler SHALL keep the existing cancellation classification
