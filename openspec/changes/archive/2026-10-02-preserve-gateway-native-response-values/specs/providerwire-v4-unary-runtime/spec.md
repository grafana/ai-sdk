## MODIFIED Requirements

### Requirement: Resolution and bounded model invocation

For a supported request, the handler SHALL resolve the exact requested model ID once, require a non-empty valid-UTF-8 canonical catalog ID and a non-nil V4 language model, and invoke DoGenerate once. Canonical catalog identity SHALL remain an internal routing and telemetry invariant and SHALL NOT substitute for native response identity. A supplied native modelId SHALL survive even when it equals the canonical ID. Invocation SHALL derive from the request context and configured duration. A child goroutine with panic recovery and buffered completion SHALL bound handler latency when a model ignores cancellation; a permanently blocked model may retain that goroutine.

#### Scenario: Supported execution
- **WHEN** resolution returns a valid V4 model
- **THEN** resolution and DoGenerate SHALL each run once

#### Scenario: Invalid resolution
- **WHEN** resolution fails, returns an empty or invalid-UTF-8 canonical ID, nil model, non-V4 model, or panics while resolving or inspecting the model
- **THEN** the handler SHALL return a safe error without invoking an invalid model

#### Scenario: Cancellation or timeout
- **WHEN** caller cancellation or the configured duration becomes observable before model completion
- **THEN** handler latency SHALL remain bounded and the corresponding safe response SHALL be selected

### Requirement: Minimal unary success response

A successful response SHALL contain ordered supported text, function-tool-call, source, reasoning and reasoning-file content, finishReason, usage and a warnings array. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Reasoning content metadata SHALL retain the existing bounded continuation projection in gateway-reasoning-content; source metadata SHALL retain the current projection described by gateway-sources pending its separately owned metadata implementation. This change SHALL NOT claim complete native metadata parity.

Warnings SHALL preserve the order and active fields of the registered unsupported, compatibility, deprecated and other variants. Required feature, setting and message fields SHALL be present even when empty. Unsupported/compatibility details SHALL preserve nonempty values; absent and empty Go details SHALL normalize to omission as an explicitly documented Go representation adaptation. Inactive fields SHALL be omitted. Nil warnings SHALL emit an empty array. Unknown warning types and invalid represented output SHALL fail safely before HTTP 200.

When provider Response is non-nil, the server SHALL emit a response object containing only representable native id, modelId and timestamp. Nonempty identity strings SHALL remain unchanged; empty optional Go strings and zero timestamps SHALL be omitted because optional presence is unrepresentable in the current Go API. Nonzero timestamps SHALL encode a validated UTC RFC3339Nano instant. A present response with no representable fields SHALL emit an empty object; nil Response SHALL omit response. Requested/canonical route identity SHALL NOT substitute for missing native values. Request diagnostics, provider identity and native headers/body SHALL remain outside this change.

The registered Gateway client SHALL combine server warnings before its local warnings and replace typed request/response with Gateway-hop information. Native unary identity SHALL remain in the raw server document and client-owned response body, not be advertised as surviving in typed Response identity fields. Required empty reasoning text and selected empty inline file data SHALL be retained. usage.raw SHALL be omitted when absent and contain a valid bounded supplied provider JSON object unchanged in meaning.

#### Scenario: Reasoning-only paid success
- **WHEN** a provider returns a valid reasoning-only result
- **THEN** the Gateway SHALL return success rather than a retryable adaptation error
- **AND** default high-level retries SHALL not invoke the provider again for that valid result

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason and valid usage without warnings or Response
- **THEN** the handler SHALL preserve those values, emit warnings as an empty array and omit response

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

#### Scenario: Provider raw usage is present or absent
- **WHEN** provider usage contains a valid in-limit object including nested values, an empty object or no Raw bytes
- **THEN** the unary response SHALL respectively include the native object, include an empty object or omit raw without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** supplied Raw is malformed, null, an array/scalar or exceeds its input byte limit
- **THEN** the handler SHALL emit the fixed internal error before HTTP success without reflecting raw data or returning normalized-only success

### Requirement: Bounded preflight and standard success encoding

Before output allocation, UTF-8 scanning or encoding, the handler SHALL reject content/warning cardinality or aggregate represented string and raw-usage bytes that cannot fit the configured unary budget using overflow-safe accounting. Accounting SHALL include native source ID/display, active warning fields, registered response identity and serialized timestamps alongside existing content/raw-finish/usage values. Warning cardinality SHALL use conservative minimum registered encoded sizes, including required empty values. Independent field/list checks SHALL NOT bypass the aggregate complete-response budget.

The handler SHALL count raw-usage bytes before parsing/marshaling and reject raw usage longer than 1,048,576 bytes or the unary limit, including whitespace. After size preflight it SHALL validate original UTF-8 and any present raw as one JSON object. Standard encoding SHALL preserve valid raw JSON escapes, including lone and paired UTF-16 surrogates. The complete private DTO SHALL then encode through standard Go JSON and SHALL receive HTTP 200 only after the final bytes fit. Provider-domain JSON marshalers SHALL NOT control public output. Encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping; no value SHALL be truncated to fit.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content/warning count or aggregate represented bytes exceed the unary budget, or raw usage exceeds 1,048,576 bytes
- **THEN** the result SHALL fail before allocating mapped slices, scanning UTF-8 or encoding JSON

#### Scenario: Separate values exceed the aggregate budget
- **WHEN** content, warnings and identity each individually fit but together exceed the unary budget
- **THEN** the response SHALL fail before success commitment rather than granting each group a full independent budget

#### Scenario: Escaping crosses the final boundary
- **WHEN** raw string sizes fit but JSON escaping makes the complete response exceed the limit
- **THEN** the handler SHALL return the fixed internal error without partial HTTP success

#### Scenario: Represented values are malformed
- **WHEN** an in-limit source/warning/identity field contains invalid UTF-8 or a nonzero timestamp cannot encode a registered date-time
- **THEN** the response SHALL fail before HTTP 200 without substituting or reflecting invalid values

#### Scenario: Raw UTF-8 and JSON escapes
- **WHEN** in-limit raw usage contains invalid UTF-8 in a key or nested value
- **THEN** the handler SHALL reject it before encoding without HTTP success
- **WHEN** in-limit raw usage contains valid lone or paired escaped surrogates
- **THEN** those escapes SHALL be preserved subject to complete-response limits

#### Scenario: Response byte boundary
- **WHEN** the complete encoded response is below, exactly at or one byte above the configured limit
- **THEN** only complete in-limit documents SHALL receive HTTP 200

#### Scenario: Raw usage exactly meets its input cap
- **WHEN** raw bytes including whitespace are at or one byte above the smaller of 1,048,576 bytes and the unary limit
- **THEN** only the at-limit value SHALL reach JSON validation and final success SHALL still require the complete response to fit

### Requirement: Compatibility evidence

The runtime SHALL replay every committed ProviderWire request golden without modifying it. Cross-language tests SHALL use the exact registered Gateway client through production handlers and compare independent Go-client consumption for represented warnings, sources and stream identity. Raw Go/HTTP tests and local schemas SHALL independently verify active union fields, identity presence, output order, invalid output, lifecycle and complete-byte bounds rather than treating permissive parsing as validation. Authenticated command and provider-neutral frontend assembly scenarios SHALL accompany the behavior change. Synthetic responses SHALL NOT be presented as recorded provider evidence, and authentic provider fixture inputs SHALL NOT be rewritten.

#### Scenario: Registered client success
- **WHEN** the pinned Gateway client sends a supported unary request with provider warnings and native identity
- **THEN** it SHALL consume the supported content/usage/finish and preserve server warning order before local warnings
- **AND** tests SHALL independently prove raw native identity retention and typed unary response replacement

#### Scenario: Streaming request
- **WHEN** the registered client sends a supported streaming text request
- **THEN** the handler SHALL invoke DoStream once and both clients SHALL consume strict streaming output through clean EOF

#### Scenario: Raw authority rejects permissively accepted output
- **WHEN** the pinned client accepts a malformed or incomplete union in a consumption probe
- **THEN** independent schema/raw tests SHALL still reject that shape as server output
