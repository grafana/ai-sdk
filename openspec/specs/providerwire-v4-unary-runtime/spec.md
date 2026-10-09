## Purpose

Define the production ProviderWire V4 unary text and client-executed function-tool runtime and the observable contract proven against the registered Gateway client.
## Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil host-owned request selector independent of catalog interfaces and positive limits for request bytes, unary response bytes, provider stream-part count, complete SSE frame bytes, total model duration, stream idle duration, and bounded post-cancellation drain duration.

#### Scenario: Valid construction
- **WHEN** a caller supplies a request selector and valid limits
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the selector is nil, a limit is non-positive, or a byte limit cannot safely use `limit+1`
- **THEN** construction SHALL fail before serving traffic

### Requirement: Safe handler limit arithmetic and fixed-frame capacity

Request and unary byte limits and the stream-part limit SHALL support safe `limit+1` arithmetic, and the frame limit SHALL contain the fixed start and stream-error frames.

#### Scenario: Safe handler limit arithmetic and fixed-frame capacity
- **WHEN** construction receives an integer limit that cannot support limit+1 or a frame limit smaller than a fixed error frame
- **THEN** construction SHALL fail before serving requests

### Requirement: Host selection preserves mapped options
The selector SHALL consume validated call-level host controls under authenticated request policy and return a non-nil executable model and logical identity. The handler SHALL retain its mapped native options after removing Gateway controls; model selection SHALL NOT require returning or replacing those options. Configured catalog resolution and request-only BYOK selection SHALL be separate host implementations; the wire package SHALL NOT require BYOK to implement a catalog.

#### Scenario: Host selection returns an executable model
- **WHEN** a configured or request-only selector returns model and identity
- **THEN** the handler SHALL retain mapped native options after consuming Gateway controls without requiring the selector to return or replace them

### Requirement: Language-model HTTP envelope

The handler SHALL accept only `POST /language-model` with JSON content and exactly one effective value for each ProviderWire protocol header. The specification version SHALL be `4`, the model ID SHALL be non-empty and preserved without rewriting, and streaming SHALL be exact `false` for unary execution or exact `true` for streaming execution. Unrelated HTTP headers SHALL not become provider call headers automatically.

#### Scenario: Valid envelope
- **WHEN** method, route, media type, specification, model ID, and execution mode are valid
- **THEN** processing SHALL continue with the exact model ID and selected unary or streaming mode

#### Scenario: Invalid envelope
- **WHEN** any required envelope value is absent, repeated, or invalid
- **THEN** the handler SHALL return an invalid-request response before resolution or model invocation

### Requirement: Bounded complete request validation

After envelope validation, the handler SHALL read and close the request body through the configured `limit+1` boundary and reject oversized or invalid UTF-8 input. It SHALL validate the complete bounded document against the embedded draft 2020-12 ProviderWire request schema before supported-subset mapping. Malformed JSON and schema-invalid input SHALL fail before resolution or model invocation.

#### Scenario: Request byte boundary
- **WHEN** a request is below, exactly at, or one byte above the configured body limit
- **THEN** the first two SHALL continue and the last SHALL fail without retaining bytes beyond `limit+1`

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
- **WHEN** a schema-valid request has no unsupported features except a `custom` content part of kind `example.custom` carrying an object-valued opaque provider-options namespace
- **THEN** complete schema validation SHALL succeed before supported-subset mapping reports the `custom-content` family without interpreting that part's provider options

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

Schema-valid custom content, provider tools and tool approvals, structured output, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Ordinary file inputs and message/file-part options SHALL execute within gateway-file-inputs, alongside existing call-level options and body-carried headers.

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

### Requirement: Supported function continuation and deferred result option scopes

Function definitions/choices and assistant-call/tool-result history SHALL execute only within the gateway-unary-function-tools and gateway-streaming-function-tools subsets, including the ordinary file-result extension. Function-tool and ordinary file-entry provider options SHALL be supported within those subsets; deferred output-level and non-file nested result options SHALL remain unsupported.

#### Scenario: Supported function continuation and deferred result option scopes
- **WHEN** a request carries function definitions and file-bearing tool history with file-entry options
- **THEN** it SHALL execute only in the unary/streaming function subsets without enabling deferred output-level or non-file nested options

### Requirement: Reasoning history options and unsupported-family precedence boundary

Assistant reasoning text and reasoning-file history SHALL execute within gateway-reasoning-content, preserving scoped options under gateway-native-provider-options rather than selected-backend filtering. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: Reasoning history options and unsupported-family precedence boundary
- **WHEN** supported reasoning continuation has scoped native options and another request activates several unsupported families
- **THEN** reasoning options SHALL survive without selected-backend filtering and no client-visible precedence SHALL be defined among simultaneously unsupported families

### Requirement: Reserved provider options and protected call headers
Host namespaces SHALL remain reserved and be consumed only by their owning selector. Protected call headers and native-consumption controls SHALL prevent account/model/transport bypass before I/O without blanket filtering or value-echoing refusals.

#### Scenario: Host-control and header processing policy
- **WHEN** the handler maps host controls, native options and call headers
- **THEN** The runtime SHALL reserve the `grafana`, `gateway`, and `grafana-ai-sdk` provider-option namespaces for the host.
- **AND** A request carrying any of them SHALL be rejected with a stable invalid-request document before resolution or model invocation unless an owning host feature explicitly consumes it.
- **AND** Such controls SHALL never reach native adapters as provider options.
- **AND** Call-level gateway controls SHALL be consumed by the host selector before native middleware; the catalog selector SHALL reject them, while request-only selection SHALL follow gateway-request-byok.
- **AND** Nested host namespaces and unsupported controls SHALL remain rejected before provider I/O.
- **AND** Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted.
- **AND** The runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key` and `anthropic-api-key`.
- **AND** The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends.
- **AND** The refused set SHALL cover every header name the inbound authenticated edge refuses in outer headers, which a test SHALL assert by driving the edge with a valid stack assertion and each candidate name, with an accepted ordinary header as a negative control, and requiring the body mapper to refuse every name the edge refused.
- **AND** Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.
- **AND** Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass.
- **AND** A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass.
- **AND** Native-specific checks SHALL fail before external provider I/O; schema, reserved-host and protected-body-header refusals SHALL retain their pre-resolution processing order.
- **AND** These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a request carries the `grafana`, `gateway`, or `grafana-ai-sdk` provider-option namespace without an owning feature consuming it
- **THEN** the response SHALL be a stable invalid-request document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be resolved or invoked

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

#### Scenario: Protocol header in the body
- **WHEN** a request carries a protocol header name such as `AI-Language-Model-Id` as a body-carried call header
- **THEN** the request SHALL be accepted, because the HTTP contract retains that name in the body
- **AND** the name SHALL NOT reach the selected backend

#### Scenario: Reserved namespace in a different case
- **WHEN** a request carries a provider-option namespace such as `Grafana`
- **THEN** it SHALL be mapped like any other ordinary namespace, because namespace names are compared exactly

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

The credential-header refusal set SHALL cover every header name the inbound authenticated edge refuses in outer headers, which a test SHALL assert by driving the edge with a valid stack assertion and each candidate name, with an accepted ordinary header as a negative control, and requiring the body mapper to refuse every name the edge refused. Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.

#### Scenario: Protected-header edge evidence and outer authentication isolation
- **WHEN** the authenticated edge is driven with valid stack assertion and each candidate credential header plus an ordinary negative control
- **THEN** the body mapper SHALL refuse every name the edge refused, without forwarding unrelated outer headers or coupling outer auth to body headers

### Requirement: Concrete native option bypass checks without universal blacklist

Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass. A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass.

#### Scenario: Concrete native option bypass checks without universal blacklist
- **WHEN** a harmless scoped field shares a protected field spelling
- **THEN** it SHALL remain accepted unless actual consumption/precedence at that namespace/scope causes a concrete bypass

### Requirement: Protection processing order and fixed refusal privacy

Native-specific checks SHALL fail before external provider I/O; schema, reserved-host and protected-body-header refusals SHALL retain their pre-resolution processing order. Provider-option and protected-header refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Protection processing order and fixed refusal privacy
- **WHEN** a reserved host namespace or native bypass is detected
- **THEN** host/schema/header refusals SHALL remain pre-resolution, native-specific checks SHALL precede external I/O and fixed documents SHALL echo no offending names or values

### Requirement: Resolution and bounded model invocation
The handler SHALL select and invoke one logical model under authenticated request policy. Configured access SHALL resolve its catalog once; BYOK SHALL use request accounts without a catalog. Both SHALL retain logical identity and one bounded selection/execution deadline.

#### Scenario: Selection and shared execution policy
- **WHEN** a supported request enters host selection and invocation
- **THEN** For a supported request, the handler SHALL select execution once under authenticated request policy and invoke the resulting logical model once.
- **AND** Configured selection SHALL resolve the exact catalog ID/alias; BYOK selection SHALL use only request provider/model/credentials.
- **AND** Selection SHALL return a nonempty valid-UTF-8 logical identity and a non-nil V4 model.
- **AND** Logical identity SHALL NOT substitute for native response identity, including when a native modelId equals it.
- **AND** One request execution deadline SHALL begin immediately before host selection, after bounded protocol input validation, and SHALL cover selection/construction, logical invocation and all underlying credential attempts; streaming consumption SHALL use the same deadline.
- **AND** Selection, invocation and attempt boundaries SHALL NOT reset or extend it.
- **AND** An earlier request-context deadline SHALL remain effective.
- **AND** Selection and invocation SHALL retain context cancellation, panic containment, buffered completion and bounded handler latency when work ignores cancellation.
- **AND** Late selection completion SHALL NOT invoke a model and SHALL retain bounded request-owned cleanup.
- **AND** A permanently blocked selection or native function may retain its worker; no exactly-once or guaranteed-zeroization claim SHALL be made.

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
- **WHEN** cancellation or the execution deadline becomes observable during selection or any attempt
- **THEN** handler latency SHALL remain bounded and no later credential attempt SHALL begin

#### Scenario: Selector consumes the execution budget
- **WHEN** selection ignores cancellation or completes only after the shared deadline expires
- **THEN** unary and streaming handler latency SHALL remain bounded, with a pre-commit JSON failure, zero model invocations and bounded cleanup of any late selection result

#### Scenario: Selection leaves only a partial budget
- **WHEN** selection consumes part of the configured execution duration before returning a credential-attempt model
- **THEN** logical invocation, every credential attempt and streaming consumption SHALL receive only the remaining time, without a fresh deadline

### Requirement: Bounded unary invocation worker and provider-owned blockage

A child goroutine with panic recovery and buffered completion SHALL bound handler latency when a model ignores cancellation; a permanently blocked model may retain that goroutine.

#### Scenario: Bounded unary invocation worker and provider-owned blockage
- **WHEN** a model ignores context cancellation and never completes
- **THEN** panic-recovered buffered completion SHALL keep handler latency bounded, while the permanently blocked model may retain its child goroutine

### Requirement: Fixed privacy-safe errors

Every runtime error response SHALL be selected from precomputed documents with fixed status, message, type, code, and `param: null`. Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability. Unknown or invalid internal categories SHALL fall back to the fixed internal-error document.

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL reduce it to the corresponding fixed safe document without serializing the cause

#### Scenario: Unknown model
- **WHEN** catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** the registered Gateway client consumes a fixed error document
- **THEN** its status, type, and retryability SHALL map to the expected client class

### Requirement: Runtime error detail exclusions

Provider, transport, resolver, panic, body, URL, header, credential, backend identity, and metadata details SHALL never be serialized.

#### Scenario: Runtime error detail exclusions
- **WHEN** a resolver or provider failure carries URL, credentials and metadata
- **THEN** none of those details SHALL be serialized into the fixed safe response

### Requirement: Minimal unary success response

A successful response SHALL contain ordered supported text, function-tool-call, source, reasoning and reasoning-file content, finishReason, usage and a warnings array. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Supported result/content providerMetadata SHALL retain opaque namespace objects and represented presence under gateway-provider-metadata.

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
#### Scenario: Ordinary metadata is not transport
- **WHEN** a model result contains supported result/content providerMetadata alongside unrelated request or response transport fields
- **THEN** the metadata SHALL survive in its registered scope without adding those transport fields or consulting operator capture flags

#### Scenario: Provider raw usage is present or absent
- **WHEN** provider usage contains a valid in-limit object including provider-native nested values, an empty object, or no Raw bytes
- **THEN** the unary response SHALL respectively include the object under `usage.raw` with its native keys, include `{}`, or omit the `raw` member without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** provider `Usage.Raw` is nonempty but malformed JSON, JSON null, an array or scalar, or exceeds its input byte limit
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

Native unary identity SHALL remain in the raw server document and client-owned response body, not be advertised as surviving in typed Response identity fields. Required empty reasoning text and selected empty inline file data SHALL be retained. usage.raw SHALL be omitted when absent and contain a valid bounded supplied provider JSON object unchanged in meaning.

#### Scenario: Unary raw identity required empties and raw usage
- **WHEN** a success contains empty reasoning, selected empty inline data and a valid raw usage object
- **THEN** required empties and raw usage SHALL survive, while native identity SHALL remain raw-body-only rather than typed client identity

### Requirement: Bounded preflight and standard success encoding

Before output allocation, UTF-8 scanning or encoding, the handler SHALL reject content/warning/metadata cardinality or aggregate represented strings, original result/content metadata namespace key/value bytes and raw-usage bytes that cannot fit the configured unary budget using overflow-safe accounting. Accounting SHALL include native source ID/display, active warning fields, registered response identity and serialized timestamps alongside existing content/raw-finish/usage values.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content/warning count or aggregate represented bytes exceed the unary budget, or raw usage exceeds 1,048,576 bytes
- **THEN** the result SHALL fail before allocating mapped slices, scanning UTF-8 or encoding JSON

#### Scenario: Separate values exceed the aggregate budget
- **WHEN** content, warnings and identity each individually fit but together exceed the unary budget
- **THEN** the response SHALL fail before success commitment rather than granting each group a full independent budget

#### Scenario: Represented values are malformed
- **WHEN** an in-limit source/warning/identity field contains invalid UTF-8 or a nonzero timestamp cannot encode a registered date-time
- **THEN** the response SHALL fail before HTTP 200 without substituting or reflecting invalid values

#### Scenario: Escaping crosses the final boundary
- **WHEN** raw bytes pass preflight but standard JSON escaping makes the encoded response exceed the limit
- **THEN** the handler SHALL return the fixed internal error before committing HTTP 200

#### Scenario: Raw UTF-8 and JSON escapes
- **WHEN** in-limit provider raw usage contains invalid UTF-8 in an object key or nested value
- **THEN** the handler SHALL reject it before JSON encoding and return the fixed internal error without committing HTTP 200
- **WHEN** in-limit raw usage contains valid JSON with lone or paired escaped surrogates
- **THEN** the handler SHALL preserve the raw JSON escapes, with success still subject to object and complete-response limits

#### Scenario: Response byte boundary
- **WHEN** the encoded response is below, exactly at, or above the configured limit
- **THEN** only complete in-limit documents SHALL receive HTTP 200

#### Scenario: Raw usage exactly meets its input cap
- **WHEN** raw usage bytes, including JSON whitespace, are at or one byte above the smaller of 1,048,576 bytes and the configured unary response limit
- **THEN** only the at-limit value SHALL reach JSON validation, and success SHALL still require the final complete response to fit the unary limit

#### Scenario: Combined metadata exceeds budget
- **WHEN** individually small metadata objects across multiple content parts and the result collectively exceed the unary budget
- **THEN** the complete result SHALL fail before JSON parsing or encoding rather than returning partial or metadata-free success

### Requirement: Aggregate unary warning and raw-usage preflight

Warning cardinality SHALL use conservative minimum registered encoded sizes, including required empty values. Independent field/list checks SHALL NOT bypass the aggregate complete-response budget. The handler SHALL count raw-usage bytes before parsing/marshaling and reject raw usage longer than 1,048,576 bytes or the unary limit, including whitespace. After size preflight it SHALL validate original UTF-8 and any present raw as one JSON object.

#### Scenario: Aggregate unary warning and raw-usage preflight
- **WHEN** separately fitting warnings/content together exceed the budget or raw usage exceeds its whitespace-inclusive input cap
- **THEN** aggregate overflow SHALL fail before success, with conservative warning cardinality and pre-parse raw byte accounting

### Requirement: Private standard unary encoding and final byte bounds

Standard encoding SHALL preserve valid raw JSON escapes, including lone and paired UTF-16 surrogates. The complete private DTO SHALL then encode through standard Go JSON and SHALL receive HTTP 200 only after the final bytes fit. Provider-domain JSON marshalers SHALL NOT control public output. Encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping; no value SHALL be truncated to fit.

#### Scenario: Private standard unary encoding and final byte bounds
- **WHEN** valid raw JSON has surrogate escapes and ordinary strings expand during escaping
- **THEN** standard private DTO encoding SHALL preserve escapes and commit HTTP 200 only when complete bytes fit, without provider marshalers or truncation and with bounded encoding allocation

### Requirement: Shared unary metadata budget and namespace validity

Metadata SHALL share the result/content budget under gateway-provider-metadata, accounting original namespace bytes including whitespace and cardinality before scanning or allocation. Namespace UTF-8/object shape SHALL be checked in preflight, and standard JSON encoding SHALL reject malformed namespace syntax before HTTP 200, without projection within those bounds.

#### Scenario: Shared unary metadata budget and namespace validity
- **WHEN** many individually small namespace objects collectively exceed the response budget
- **THEN** the whole result SHALL fail using original bytes/cardinality before scanning/allocation, without projection or metadata-free success

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

Raw Go/HTTP tests and local schemas SHALL independently verify active union fields, identity presence, output order, invalid output, lifecycle and complete-byte bounds rather than treating permissive parsing as validation. Authenticated command and provider-neutral frontend assembly scenarios SHALL accompany the behavior change. Synthetic responses SHALL NOT be presented as recorded provider evidence, and authentic provider fixture inputs SHALL NOT be rewritten.

#### Scenario: Independent unary raw authority frontend evidence and fixture provenance
- **WHEN** the pinned client accepts an output union permissively
- **THEN** raw/schema/lifecycle/bounds tests SHALL independently verify it, authenticated command/frontend assembly evidence SHALL accompany behavior changes and synthetic responses SHALL NOT become recorded inputs

### Requirement: Host-safe ProviderWire error writer

The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including service authentication, permission, and discovery failures. Construction SHALL be non-fallible and SHALL accept no configuration.

#### Scenario: Host emits authentication failure
- **WHEN** authenticated service middleware selects the closed authentication category
- **THEN** the writer SHALL emit the exact fixed 401 authentication document owned by the protocol package
- **AND** no host or verifier error text SHALL be accepted or copied

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

### Requirement: Closed host error categories and fixed document reuse

Callers SHALL select only an exported closed authentication, permission, or internal category; they SHALL NOT supply public messages, causes, arbitrary status codes, error types, error codes, retryability, or byte limits. The writer SHALL reuse package-owned fixed ProviderWire documents directly. It SHALL perform no runtime schema compilation or validation and no dynamic JSON encoding.

#### Scenario: Closed host error categories and fixed document reuse
- **WHEN** host middleware selects authentication rather than providing its own error message
- **THEN** the writer SHALL directly reuse fixed documents without accepting caller-controlled fields, compiling schemas or dynamically encoding JSON

### Requirement: Host error status bytes and private API boundary

Authentication SHALL emit the package's exact fixed 401 document, permission SHALL emit the exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document. The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

#### Scenario: Host error status bytes and private API boundary
- **WHEN** host middleware selects permission, internal or an invalid category
- **THEN** permission SHALL use the exact fixed 403 and internal/invalid the exact fixed 500, with no exposed DTOs or weakened handler failure bytes
