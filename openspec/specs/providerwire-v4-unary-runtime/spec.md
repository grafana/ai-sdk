## Purpose

Define the production ProviderWire V4 unary text and client-executed function-tool runtime and the observable contract proven against the registered Gateway client.

## Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil execution runtime, which owns the request selector and the stream idle and drain durations under gateway-execution-runtime. Construction SHALL NOT accept a total model duration, or request, response, frame or stream-part size limits.

#### Scenario: Valid construction
- **WHEN** a caller supplies a constructed execution runtime
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the runtime is nil
- **THEN** construction SHALL fail before serving traffic

### Requirement: Host selection preserves mapped options
The selector SHALL consume validated call-level host controls under authenticated request policy and return a non-nil executable model and logical identity. The execution runtime SHALL retain the mapped native options after removing call-level Gateway controls, without requiring the selector to return them. Configured catalog resolution and request-only BYOK selection SHALL be separate host implementations; neither the runtime nor the wire package SHALL require BYOK to implement a catalog.

#### Scenario: Host selection returns an executable model
- **WHEN** a configured or request-only selector returns model and identity
- **THEN** the runtime SHALL retain mapped native options after consuming Gateway controls without requiring the selector to return or replace them

### Requirement: Language-model HTTP envelope

The handler SHALL accept only `POST /language-model` with JSON content and exactly one effective value for each ProviderWire protocol header. The specification version SHALL be `4`, the model ID SHALL be non-empty and preserved without rewriting, and streaming SHALL be exact `false` for unary execution or exact `true` for streaming execution. Unrelated HTTP headers SHALL not become provider call headers automatically.

#### Scenario: Valid envelope
- **WHEN** method, route, media type, specification, model ID, and execution mode are valid
- **THEN** processing SHALL continue with the exact model ID and selected unary or streaming mode

#### Scenario: Invalid envelope
- **WHEN** any required envelope value is absent, repeated, or invalid
- **THEN** the handler SHALL return an invalid-request response before resolution or model invocation

### Requirement: Bounded complete request validation

After envelope validation, the handler SHALL read the request body to EOF and close it. The host SHALL own the body byte bound under gateway-resource-policy. The handler SHALL reject invalid UTF-8 input and SHALL validate the complete document against the embedded draft 2020-12 ProviderWire request schema before supported-subset mapping. Malformed JSON and schema-invalid input SHALL fail before resolution or model invocation.

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

### Requirement: Standard request duplicate and surrogate normalization

Standard Go/schema JSON semantics SHALL apply: duplicate object members use the last decoded value, and escaped lone UTF-16 surrogates normalize to U+FFFD.

#### Scenario: Standard request duplicate and surrogate normalization
- **WHEN** a bounded request repeats a member and contains an escaped lone surrogate
- **THEN** the last decoded member SHALL win and the surrogate SHALL normalize to U+FFFD

### Requirement: Unary text and scalar mapping

The handler SHALL preserve ordered system messages and user or assistant text parts, including required empty strings. It SHALL map optional integer and continuous generation controls with ordinary Go JSON numeric range checks, preserve explicit zero values, preserve stop-sequence order, and map typed reasoning values. Omitted reasoning and wire `provider-default` SHALL both map to zero-valued `ReasoningProviderDefault`.

#### Scenario: Supported request
- **WHEN** a schema-valid unary request contains text messages and supported scalar controls
- **THEN** the model SHALL receive the same text order, scalar presence, zero values, stop-sequence order, and reasoning value

#### Scenario: Integer lexical form
- **WHEN** an integer control uses a plain integer token such as `1`, `0`, or `-1`
- **THEN** it SHALL map to that Go integer
- **WHEN** it uses `1.0`, `1e0`, `-0.0`, or exceeds the Go integer range
- **THEN** mapping SHALL return an invalid request before resolution or invocation

#### Scenario: Empty optional values
- **WHEN** tools, headers, and provider-options namespaces are empty, raw chunks are false, and response format is text
- **THEN** those values SHALL normalize to the supported ordinary text behavior

#### Scenario: Populated provider options and headers
- **WHEN** a request carries call-level, message-level and text-part provider options and ordinary body headers
- **THEN** the model SHALL receive each namespace with its nested JSON unchanged and each header with its original key case

#### Scenario: Malformed provider option namespace
- **WHEN** a provider-option namespace value is null, an array, or a scalar
- **THEN** mapping SHALL return an invalid request before resolution or invocation

### Requirement: Opaque scoped text request options and empty namespaces

The handler SHALL map call-level, message-level and text-part provider options to opaque provider options, preserving each namespace's nested JSON byte for byte, including null, false, zero, empty string, empty object and empty array members. A namespace value that is not a JSON object SHALL fail as an invalid request before resolution or model invocation. A namespace present with an empty object SHALL be preserved and not dropped.

#### Scenario: Opaque scoped text request options and empty namespaces
- **WHEN** call, message and text-part options include nested null/false/zero/empty values and an empty namespace object
- **THEN** nested JSON SHALL be preserved byte for byte and empty objects SHALL remain present; non-object namespaces SHALL fail before resolution

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

### Requirement: Body header collisions empty maps and outer-header separation

Two body headers whose names differ only in case SHALL fail as an invalid request, because either value could otherwise reach the backend depending on map order. An empty header map SHALL map to no headers. Body-carried headers SHALL remain distinct from the outer HTTP headers of the gateway request, which the runtime reads only for the protocol headers it owns.

#### Scenario: Body header collisions empty maps and outer-header separation
- **WHEN** body headers contain two names differing only in case
- **THEN** mapping SHALL fail before invocation; empty maps SHALL mean no headers and unrelated outer headers SHALL remain distinct

### Requirement: Protocol body header acceptance without backend forwarding

A body-carried header whose name belongs to the runtime's own protocol namespaces SHALL be accepted and SHALL NOT be forwarded to a backend, because the HTTP contract round-trips such a name in the body while the name addresses this runtime.

#### Scenario: Protocol body header acceptance without backend forwarding
- **WHEN** a body header names AI-Language-Model-Id
- **THEN** it SHALL be accepted in the round-tripped body but SHALL NOT be forwarded to the backend

### Requirement: Unsupported capability families

Schema-valid provider tools and tool approvals SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Structured output SHALL execute within gateway-structured-output. Ordinary file inputs and message/file-part options SHALL execute within gateway-file-inputs, alongside existing call-level options and body-carried headers. Requested raw output SHALL follow gateway-raw-output.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

#### Scenario: Reasoning continuation is supported
- **WHEN** a valid assistant reasoning part carries supported scoped continuation options
- **THEN** the model SHALL receive them without enabling deferred capability families

#### Scenario: Custom content is supported
- **WHEN** a valid request carries assistant custom history or custom tool-result content
- **THEN** it SHALL execute within gateway-custom-content without a blanket unsupported-family failure

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

### Requirement: Reasoning history options and unsupported-family precedence boundary

Assistant reasoning text and reasoning-file history SHALL execute within gateway-reasoning-content, preserving scoped options under gateway-native-provider-options rather than selected-backend filtering. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: Reasoning history options and unsupported-family precedence boundary
- **WHEN** supported reasoning continuation has scoped native options and another request activates several unsupported families
- **THEN** reasoning options SHALL survive without selected-backend filtering and no client-visible precedence SHALL be defined among simultaneously unsupported families

### Requirement: Reserved provider options and protected call headers
Call-level `gateway` controls SHALL be consumed only by the selector, and protected call headers SHALL be refused, both by the execution runtime under gateway-execution-runtime. No other namespace or scope SHALL be reserved. Native-consumption controls SHALL prevent account/model/transport bypass before I/O without blanket filtering or value-echoing refusals.

#### Scenario: Host-control and header processing policy
- **WHEN** the handler maps host controls, native options and call headers
- **THEN** the handler SHALL map every provider-option namespace at every supported scope, including `gateway`, `grafana` and `grafana-ai-sdk`, as an ordinary opaque namespace.
- **AND** The execution runtime SHALL remove call-level `gateway` before selection and pass it to the selector; it SHALL never reach native adapters as a provider option.
- **AND** Configured and request-only selection SHALL apply call-level `gateway` controls under gateway-request-routing and gateway-request-byok, and SHALL refuse any other control with the fixed unsupported-gateway-control document.
- **AND** Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted.
- **AND** The execution runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering the set gateway-execution-runtime lists.
- **AND** The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends.
- **AND** Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.
- **AND** Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass.
- **AND** A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass.
- **AND** Native-specific checks SHALL fail before external provider I/O; schema and mapping refusals SHALL precede the protected-header refusal, and both SHALL precede selection.
- **AND** These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a configured request carries a call-level `providerOptions.gateway` member other than `models`, `order` or `only`
- **THEN** the response SHALL be the fixed unsupported-gateway-control document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be invoked

#### Scenario: Former host namespaces are ordinary
- **WHEN** a request carries `grafana` or `grafana-ai-sdk` at any supported scope, or `gateway` at a message, part or tool scope
- **THEN** the request SHALL succeed and the model SHALL receive the namespace unchanged at its scope

#### Scenario: Protected provider option
- **WHEN** an option can cause a concrete native model/prompt, credential/account/destination, role/union, tool-ownership or transport/execution bypass
- **THEN** the request SHALL fail with a stable invalid-request document before external provider I/O unless native precedence already prevents the override

#### Scenario: Harmless field with the same spelling
- **WHEN** an ordinary option field has a spelling used by a protected field but its namespace and scope cannot cause the native bypass
- **THEN** it SHALL remain intact for native interpretation rather than be refused under a universal blacklist

#### Scenario: Protected call header
- **WHEN** a request carries a credential-bearing body header in any letter case
- **THEN** the response SHALL be a stable invalid-request document
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protected call header beside a mapping failure
- **WHEN** a request carries a credential-bearing body header and also fails a mapping rule
- **THEN** the response SHALL be the mapping failure's fixed invalid-request document, and no model SHALL be resolved or invoked

#### Scenario: Protocol header in the body
- **WHEN** a request carries a protocol header name such as `AI-Language-Model-Id` as a body-carried call header
- **THEN** the request SHALL be accepted, because the HTTP contract retains that name in the body
- **AND** the name SHALL NOT reach the selected backend

#### Scenario: Reserved namespace in a different case
- **WHEN** a request carries a call-level provider-option namespace such as `Gateway`
- **THEN** it SHALL be mapped like any other ordinary namespace and SHALL NOT reach the selector, because namespace names are compared exactly

### Requirement: Ordinary namespace acceptance and protected body credential headers

Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted. The runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key` and `anthropic-api-key`.

#### Scenario: Ordinary namespace acceptance and protected body credential headers
- **WHEN** a body header uses a case-variant API-key name alongside an unlisted ordinary namespace
- **THEN** the credential-bearing header SHALL be refused while ordinary unlisted namespaces/fields SHALL NOT become host controls

### Requirement: Native authorization header overwrite risk

The runtime SHALL refuse credential-bearing body headers that could select credentials through native call-header precedence.

#### Scenario: Native authorization header overwrite risk
- **WHEN** the openai and openai-compatible providers apply call headers after setting their own authorization header
- **THEN** an accepted credential-bearing header would choose the credential presented to those backends; the runtime SHALL refuse such headers

### Requirement: Protected-header edge evidence and outer authentication isolation

The plugin host authenticates outer requests under gateway-plugin-host, and the Gateway refuses no outer header names of its own. The credential-header refusal set SHALL cover the platform credential and tenant headers the host delegates, and the native provider credential headers. Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.

#### Scenario: Protected-header edge evidence and outer authentication isolation
- **WHEN** a test sends each listed credential header name as a body call header, plus an ordinary header as a negative control
- **THEN** the runtime SHALL refuse every listed name and accept the ordinary header, without forwarding unrelated outer headers or coupling outer auth to body headers

### Requirement: Concrete native option bypass checks without universal blacklist

Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass. A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass.

#### Scenario: Concrete native option bypass checks without universal blacklist
- **WHEN** a harmless scoped field shares a protected field spelling
- **THEN** it SHALL remain accepted unless actual consumption/precedence at that namespace/scope causes a concrete bypass

### Requirement: Protection processing order and fixed refusal privacy

Native-specific checks SHALL fail before external provider I/O. Schema and mapping refusals SHALL precede the protected-header refusal, and all of them SHALL precede selection. Provider-option and protected-header refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Protection processing order and fixed refusal privacy
- **WHEN** a protected call header or native bypass is detected
- **THEN** schema, mapping and header refusals SHALL remain pre-resolution, native-specific checks SHALL precede external I/O and fixed documents SHALL echo no offending names or values

### Requirement: Resolution and bounded model invocation
The handler SHALL select and invoke one logical model through the execution runtime under authenticated request policy. Configured access SHALL resolve its catalog once; BYOK SHALL use request accounts without a catalog. Both SHALL retain logical identity and run selection and execution synchronously under the request context's deadline.

#### Scenario: Selection and shared execution policy
- **WHEN** a supported request enters host selection and invocation
- **THEN** For a supported request, the runtime SHALL select execution once under authenticated request policy and invoke the resulting logical model once.
- **AND** Configured selection SHALL resolve the exact catalog ID/alias; BYOK selection SHALL use only request provider/model/credentials.
- **AND** Selection SHALL return a nonempty valid-UTF-8 logical identity and a non-nil V4 model.
- **AND** Logical identity SHALL NOT substitute for native response identity, including when a native modelId equals it.
- **AND** The request context's deadline, set by the host, SHALL be the one execution deadline. It SHALL cover selection/construction, logical invocation and all underlying credential attempts, and streaming consumption SHALL use the same deadline.
- **AND** The handler SHALL NOT add a deadline of its own, and selection, invocation and attempt boundaries SHALL NOT reset or extend the request deadline.
- **AND** When the request context's deadline expires, the handler SHALL classify the outcome as a timeout; when the request context is canceled, it SHALL classify it as a cancellation.
- **AND** Selection and invocation SHALL run on the request goroutine with panic containment; a result returned after the request context ended SHALL be discarded.
- **AND** A selection returned after the request context ended SHALL NOT invoke a model.
- **AND** A selector or native function that ignores cancellation SHALL hold its own request until it returns; no exactly-once or guaranteed-zeroization claim SHALL be made.

#### Scenario: Supported execution
- **WHEN** configured selection returns a valid model
- **THEN** catalog resolution and logical DoGenerate SHALL each run once

#### Scenario: BYOK execution
- **WHEN** BYOK selection returns a valid credential-attempt model
- **THEN** the logical invocation SHALL run once, the catalog SHALL not run and bounded physical attempts SHALL follow the BYOK contract

#### Scenario: Invalid resolution
- **WHEN** selection fails, panics or returns invalid identity/model values
- **THEN** the handler SHALL return a safe error without invoking an invalid model

#### Scenario: Cancellation or timeout
- **WHEN** cancellation or the request deadline becomes observable during selection or any attempt
- **THEN** no later credential attempt SHALL begin and the handler SHALL return the context outcome once the work returns

#### Scenario: Request deadline is a timeout
- **WHEN** the request context's deadline expires during a unary attempt
- **THEN** the handler SHALL return the fixed timeout error, not the cancellation error

#### Scenario: Selector consumes the execution budget
- **WHEN** selection completes only after the request deadline expired
- **THEN** unary and streaming requests SHALL receive a pre-commit timeout JSON failure with zero model invocations

#### Scenario: Model returns after the deadline
- **WHEN** `DoGenerate` returns a valid result only after the request deadline expired
- **THEN** the handler SHALL return the fixed timeout error and SHALL NOT write the result

#### Scenario: Selection leaves only a partial budget
- **WHEN** selection consumes part of the time before the request deadline and returns a credential-attempt model
- **THEN** logical invocation, every credential attempt and streaming consumption SHALL receive only the remaining time, without a fresh deadline

#### Scenario: Request without a deadline
- **WHEN** the request context carries no deadline
- **THEN** the handler SHALL run selection and invocation without a deadline of its own, and caller cancellation SHALL still end them

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
- **AND** Optional native candidate-local summaries and observed configured or request-account identity MAY accompany the base error under gateway-attempt-failure-evidence, encoded once with the base error; they SHALL NOT change classification or retryability.
- **AND** Host-owned BYOK validation and unsupported-operation failures SHALL use fixed approved messages without copying rejected values or private diagnostics.
- **AND** Stopped or exhausted credential fallback SHALL retain existing fallback error propagation and safe runtime classification rather than use a blanket exhaustion status.
- **AND** Request-only selections SHALL remain eligible for available event-local SSE native summaries, and their unary/setup failures SHALL carry the request-account overview with candidate-local native summaries when at least one account was invoked.

#### Scenario: Actionable credential shape failure
- **WHEN** a credential entry is missing apiKey or the selected provider's array exceeds the account count
- **THEN** the invalid-request response SHALL identify the known field/capability without copying the entry or value

#### Scenario: Discovery unsupported for BYOK
- **WHEN** authenticated BYOK access requests catalog discovery
- **THEN** the host SHALL emit HTTP 400, invalid_request_error, invalid_request, param null and a fixed unsupported-discovery message

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL retain the corresponding fixed safe fields without serializing arbitrary causes; optional native configured or request-account execution attribution SHALL follow gateway-attempt-failure-evidence

#### Scenario: Unknown model
- **WHEN** configured catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** Go and pinned Vercel consume BYOK validation, exhaustion or discovery failures
- **THEN** status/type/retryability SHALL match the existing client categories without misclassifying provider rejection as failed Gateway authentication

#### Scenario: BYOK provider echoes request accounts
- **WHEN** authenticated request-only selection receives a native failure containing request-account keys or identifiers before or after stream commitment
- **THEN** fixed classified error fields SHALL remain unchanged, and unary/setup overview summaries and SSE native summaries SHALL preserve unrelated BYOK echoes and protect exact configured provider/exporter-key matches independently of catalog attribution

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

### Requirement: Unary warning active union fields and empty values

Warnings SHALL preserve the order and active fields of the registered unsupported, compatibility, deprecated and other variants. Required feature, setting and message fields SHALL be present even when empty. Unsupported/compatibility details SHALL preserve nonempty values; absent and empty Go details SHALL normalize to omission as an explicitly documented Go representation adaptation. Inactive fields SHALL be omitted. Nil warnings SHALL emit an empty array.

#### Scenario: Unary warning active union fields and empty values
- **WHEN** warnings contain all four registered variants with required empty values
- **THEN** active fields and order SHALL survive, optional empty details/inactive fields SHALL be omitted and nil warnings SHALL emit an empty array

### Requirement: Unary native identity representation and invalid-output rejection

Unknown warning types and invalid represented output SHALL fail safely before HTTP 200. When provider Response is non-nil, the server SHALL emit a response object containing only representable native id, modelId and timestamp. Nonempty identity strings SHALL remain unchanged; empty optional Go strings and zero timestamps SHALL be omitted because optional presence is unrepresentable in the current Go API. Nonzero timestamps SHALL encode a validated UTC RFC3339Nano instant.

#### Scenario: Unary native identity representation and invalid-output rejection
- **WHEN** Response is non-nil with native identity and a nonzero timestamp
- **THEN** representable native values SHALL remain unchanged with UTC RFC3339Nano time and omitted unrepresentable optional zeros; invalid represented output SHALL fail before HTTP 200

### Requirement: Unary response presence privacy and client-hop replacement

A present response with no representable fields SHALL emit an empty object; nil Response SHALL omit response. Requested/canonical route identity SHALL NOT substitute for missing native values. Request diagnostics, provider identity and native headers/body SHALL remain outside this change. The registered Gateway client SHALL combine server warnings before its local warnings and replace typed request/response with Gateway-hop information.

#### Scenario: Unary response presence privacy and client-hop replacement
- **WHEN** Response is present with no identity and contains native transport details
- **THEN** response SHALL be an empty object without route defaults or transport diagnostics, and the client SHALL combine warnings and replace typed transport with Gateway-hop information

### Requirement: Unary raw identity required empties and raw usage

Native unary identity SHALL remain in the raw server document and client-owned response body, not be advertised as surviving in typed Response identity fields. Required empty reasoning text and selected empty inline file data SHALL be retained. usage.raw SHALL be omitted when absent and contain a valid supplied provider JSON object unchanged in meaning.

#### Scenario: Unary raw identity required empties and raw usage
- **WHEN** a success contains empty reasoning, selected empty inline data and a valid raw usage object
- **THEN** required empties and raw usage SHALL survive, while native identity SHALL remain raw-body-only rather than typed client identity

### Requirement: Compatibility evidence

The runtime SHALL replay every committed ProviderWire request golden without modifying it. Cross-language tests SHALL use the exact registered Gateway client through production handlers and compare independent Go-client consumption for represented warnings, sources and stream identity.

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

### Requirement: Independent unary raw authority frontend evidence and fixture provenance

Raw Go/HTTP tests and local schemas SHALL independently verify active union fields, identity presence, output order, invalid output, lifecycle and large-output acceptance rather than treating permissive parsing as validation. Authenticated command and provider-neutral frontend assembly scenarios SHALL accompany the behavior change. Synthetic responses SHALL NOT be presented as recorded provider evidence, and authentic provider fixture inputs SHALL NOT be rewritten.

#### Scenario: Independent unary raw authority frontend evidence and fixture provenance
- **WHEN** the pinned client accepts an output union permissively
- **THEN** raw/schema/lifecycle/large-output tests SHALL independently verify it, authenticated command/frontend assembly evidence SHALL accompany behavior changes and synthetic responses SHALL NOT become recorded inputs

### Requirement: Closed host error categories and fixed document reuse
Callers SHALL select only the closed permission, internal or unsupported-BYOK-discovery categories, never public messages, causes, arbitrary statuses/types/codes, retryability or byte limits. Encoding SHALL use the handler's typed fixed definitions without schema compilation/validation or optional execution metadata.

#### Scenario: Closed host error categories and fixed document reuse
- **WHEN** host middleware selects permission rather than providing its own error message
- **THEN** the writer SHALL encode the shared fixed public definitions without accepting caller-controlled fields, compiling schemas or attaching optional execution metadata

#### Scenario: Unsupported discovery uses a closed category
- **WHEN** an authenticated BYOK caller requests discovery
- **THEN** the host SHALL select the fixed unsupported-discovery category without catalog access

### Requirement: Host error status bytes and private API boundary

Permission SHALL emit the package's exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document. The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

#### Scenario: Host error status bytes and private API boundary
- **WHEN** host middleware selects permission, internal or an invalid category
- **THEN** permission SHALL use the exact fixed 403 and internal/invalid the exact fixed 500, with no exposed DTOs or weakened handler failure bytes

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

### Requirement: Unsupported-functionality rejection classification
When unary invocation or stream setup fails with the provider unsupported-functionality rejection, the handler SHALL return the fixed non-retryable invalid-request error, which MAY be accompanied by the optional execution overview. Existing unsupported-request, cancellation, deadline, API-call-error and transport classification SHALL take precedence.

#### Scenario: Direct rejection
- **WHEN** a direct route's adapter returns a wrapped unsupported-functionality rejection before any I/O, in unary or streaming setup
- **THEN** the handler SHALL return the fixed invalid-request document and neither registered client SHALL retry the request

#### Scenario: All candidates reject
- **WHEN** every configured-fallback candidate returns the rejection
- **THEN** the handler SHALL return the fixed invalid-request document

#### Scenario: Operational failures keep existing classification
- **WHEN** fallback attempts end with the rejection and a retryable 503, in either order
- **THEN** the handler SHALL keep the existing 503 classification

#### Scenario: API call error wrapping the sentinel
- **WHEN** an API call error's cause wraps the sentinel
- **THEN** the handler SHALL keep the existing API-call-error classification

#### Scenario: Cancellation wins
- **WHEN** the request is canceled after a candidate rejected
- **THEN** the handler SHALL keep the existing cancellation classification

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

### Requirement: ProviderWire host contract error writer
The ProviderWire package SHALL expose a non-fallible, unconfigured host error writer that implements the host contract's closed categories: permission, internal and unsupported-BYOK-discovery failures. Caller messages, causes or arbitrary fields SHALL NOT cross that API.

#### Scenario: Authenticated host error policy
- **WHEN** host middleware selects a closed error category
- **THEN** The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including permission and discovery failures.
- **AND** Construction SHALL be non-fallible and SHALL accept no configuration.
- **AND** Callers SHALL select only the closed permission, internal or unsupported-BYOK-discovery categories; they SHALL NOT supply public messages, causes, arbitrary status codes, error types, error codes, retryability, or byte limits.
- **AND** The writer SHALL use the same protocol-owned public error definitions and typed HTTP encoder as the language-model handler, without optional execution metadata.
- **AND** It SHALL perform no runtime schema compilation or validation; encoding SHALL use only fixed package-owned values.
- **AND** Permission SHALL emit the exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document.
- **AND** The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

#### Scenario: Host emits permission failure
- **WHEN** host authorization selects the closed permission category
- **THEN** the writer SHALL emit the exact fixed 403 forbidden document owned by the protocol package
- **AND** no caller-controlled field SHALL affect the document

#### Scenario: Host emits internal discovery failure
- **WHEN** bounded discovery encoding fails and the host selects the closed internal category
- **THEN** the writer SHALL emit the exact fixed internal-error document without a partial discovery response

#### Scenario: Host selects an invalid category
- **WHEN** an invalid typed category reaches the writer
- **THEN** it SHALL emit the same fixed canonical internal-error document
- **AND** it SHALL not serialize the invalid value

#### Scenario: Host refuses BYOK discovery
- **WHEN** authenticated BYOK discovery selects the closed unsupported category
- **THEN** the writer SHALL emit the fixed HTTP 400 invalid-request discovery document without catalog access or caller-supplied prose

### Requirement: Fixed routing error documents
Routing and gateway-control failures SHALL use fixed ProviderWire documents with `param: null`, not retryable, chosen from category and reason alone. Their messages SHALL never echo supplied model IDs, slugs, numbers or control names.

#### Scenario: Routing documents
- **WHEN** selection fails with each routing outcome
- **THEN** the documents SHALL be:
  - unsupported gateway control: HTTP 400, `invalid_request_error`, `invalid_request`, "unsupported gateway control; supported controls are models, order and only, plus byok for request credentials"
  - routing controls: HTTP 400, `invalid_request_error`, `invalid_request`, "providerOptions.gateway models, order and only must be arrays of nonempty strings; conditional models entries are unsupported"
  - no candidates: HTTP 400, `invalid_request_error`, `invalid_request`, "providerOptions.gateway.only excludes every candidate"
  - attempt bound: HTTP 400, `invalid_request_error`, `invalid_request`, "routing exceeds the bound of 8 provider attempts"
  - fallback model not found: HTTP 404, `model_not_found`, `model_not_found`, "a providerOptions.gateway.models entry is not an available model"

#### Scenario: Client classification of routing documents
- **WHEN** the pinned TypeScript client and the Go client receive each routing document
- **THEN** each SHALL classify it with its existing invalid-request or model-not-found error category, as not retryable
