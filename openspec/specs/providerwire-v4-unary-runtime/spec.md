## Purpose

Define the production ProviderWire V4 unary text and client-executed function-tool runtime and the observable contract proven against the registered Gateway client.

## Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil `catalog.ModelResolver` and positive limits for request bytes, unary response bytes, provider stream-part count, complete SSE frame bytes, total model duration, stream idle duration, and bounded post-cancellation drain duration. Request and unary byte limits and the stream-part limit SHALL support safe `limit+1` arithmetic, and the frame limit SHALL contain the fixed start and stream-error frames.

#### Scenario: Valid construction
- **WHEN** a caller supplies a resolver and valid limits
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the resolver is nil, a limit is non-positive, or a byte limit cannot safely use `limit+1`
- **THEN** construction SHALL fail before serving traffic

### Requirement: Language-model HTTP envelope

The handler SHALL accept only `POST /language-model` with JSON content and exactly one effective value for each ProviderWire protocol header. The specification version SHALL be `4`, the model ID SHALL be non-empty and preserved without rewriting, and streaming SHALL be exact `false` for unary execution or exact `true` for streaming execution. Unrelated HTTP headers SHALL not become provider call headers automatically.

#### Scenario: Valid envelope
- **WHEN** method, route, media type, specification, model ID, and execution mode are valid
- **THEN** processing SHALL continue with the exact model ID and selected unary or streaming mode

#### Scenario: Invalid envelope
- **WHEN** any required envelope value is absent, repeated, or invalid
- **THEN** the handler SHALL return an invalid-request response before resolution or model invocation

### Requirement: Bounded complete request validation

After envelope validation, the handler SHALL read and close the request body through the configured `limit+1` boundary and reject oversized or invalid UTF-8 input. It SHALL validate the complete bounded document against the embedded draft 2020-12 ProviderWire request schema before supported-subset mapping. Malformed JSON and schema-invalid input SHALL fail before resolution or model invocation. Standard Go/schema JSON semantics SHALL apply: duplicate object members use the last decoded value, and escaped lone UTF-16 surrogates normalize to U+FFFD.

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

### Requirement: Unary text and scalar mapping

The handler SHALL preserve ordered system messages and user or assistant text parts, including required empty strings. It SHALL map optional integer and continuous generation controls with ordinary Go JSON numeric range checks, preserve explicit zero values, preserve stop-sequence order, and map typed reasoning values. Omitted reasoning and wire `provider-default` SHALL both map to zero-valued `ReasoningProviderDefault`.

The handler SHALL map call-level, message-level and text-part provider options to opaque provider options, preserving each namespace's nested JSON byte for byte, including null, false, zero, empty string, empty object and empty array members. A namespace value that is not a JSON object SHALL fail as an invalid request before resolution or model invocation. A namespace present with an empty object SHALL be preserved and not dropped. Namespace names SHALL be compared exactly, because provider-option namespaces are case-significant. Provider options carried by a message role or part type the runtime does not support SHALL NOT be inspected, so such a request SHALL report the family of that role or part type.

The handler SHALL map body-carried call headers to provider call headers, preserving each key's original case. Two body headers whose names differ only in case SHALL fail as an invalid request, because either value could otherwise reach the backend depending on map order. An empty header map SHALL map to no headers. Body-carried headers SHALL remain distinct from the outer HTTP headers of the gateway request, which the runtime reads only for the protocol headers it owns. A body-carried header whose name belongs to the runtime's own protocol namespaces SHALL be accepted and SHALL NOT be forwarded to a backend, because the HTTP contract round-trips such a name in the body while the name addresses this runtime.

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

### Requirement: Unsupported capability families

Schema-valid files, reasoning content, custom content, provider tools and tool approvals, structured output, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Unary function definitions/choices and assistant-call/tool-result history SHALL execute only within the gateway-unary-function-tools subset. Function-tool provider options SHALL be supported within that subset; deferred nested result options SHALL remain unsupported. Streaming function requests SHALL remain unsupported until WP12. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

### Requirement: Reserved provider options and protected call headers

The runtime SHALL reserve the `grafana` provider-option namespace for the host. A request carrying it SHALL be rejected with a stable invalid-request document before resolution or model invocation, rather than silently stripped, so nothing it carries reaches a backend and the caller learns the option was refused.

The runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key` and `anthropic-api-key`. The openai and openai-compatible providers apply call headers after setting their own authorization header, so an accepted credential-bearing header would choose the credential presented to those backends. The refused set SHALL cover every header name the inbound authenticated edge refuses in outer headers, which a test SHALL assert by driving the edge with a valid stack assertion and each candidate name, with an accepted ordinary header as a negative control, and requiring the body mapper to refuse every name the edge refused.

The runtime SHALL refuse a provider-option namespace carrying a field that names a decision the runtime has already made, covering at least `model`, `fallbacks`, `messages`, `prompt`, `tools`, `toolChoice`, `functions`, `function_call`, `mcpServers`, `container`, `responseFormat`, `stream`, `streamOptions`, and the message and part fields `role`, `content`, `tool_calls`, `tool_call_id`, `type`, `image_url`, `input_audio` and `file`, which providers spread over the entry they build and which would otherwise restore tools or files. A `content` field SHALL be refused because it carries parts of any type underneath the field checks, which is how it reinstates an image or file the runtime refused to map. Field names SHALL be compared after folding case and removing `_` and `-`, because a provider may read a field under another spelling: `providers/anthropic` decodes options with `encoding/json`, which matches names case-insensitively, so `MCPServers` and `mcp_servers` reach the same field. The set SHALL cover every capability the runtime refuses at the wire level, so a refused capability cannot be restated as a provider option. Providers merge unknown option fields into the request body, so an accepted model, fallbacks or prompt field would redirect the call away from the resolved catalog model that telemetry reports, an accepted tool field, including the legacy `functions` pair, would run tools the runtime never mapped on the host's credentials, an accepted response-format field would restate the structured output the runtime refuses at the wire level, and an accepted stream field would answer in a transport the runtime is not reading.

These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a request carries the `grafana` provider-option namespace
- **THEN** the response SHALL be a stable invalid-request document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protected provider option
- **WHEN** a request carries a provider-option field naming the model, the prompt, or server-side tools
- **THEN** the response SHALL be a stable invalid-request document
- **AND** no model SHALL be resolved or invoked

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
- **THEN** it SHALL be mapped like any other namespace, because namespace names are compared exactly

### Requirement: Selected-backend provider options

After resolution, the handler SHALL forward only the provider options the resolved backend reads, at call, message and content-part level, as described by the resolved model's `catalog.ProviderOptionPolicy`. A namespace outside the policy's namespaces SHALL NOT reach the model, and SHALL NOT fail the request, because an option for another backend is one every provider ignores. Where the policy lists fields for a namespace, other top-level fields SHALL be removed, compared after folding case and removing `_` and `-`; a namespace with nothing removed SHALL keep its bytes exactly. The zero-value policy SHALL forward no provider options.

The command SHALL set a policy for every provider type it constructs, and a model whose fallback candidates resolve to different policies SHALL forward no caller provider options, because no single policy is safe for every attempt. For `anthropic`, the policy SHALL forward the `anthropic` namespace restricted to the fields `providers/anthropic` reads, excluding the fields the runtime refuses, and a test SHALL fail when the provider's typed option structs gain a field that is neither forwarded nor refused. For `openai`, the policy SHALL forward the `openai` namespace and its `azure` parity fallback, restricted to the fields `providers/openai` reads at call and part level. For `openai-compatible`, the policy SHALL forward the namespaces the provider reads for its configured provider name, without restricting fields, and a test SHALL check those namespaces against the provider itself.

#### Scenario: Options for another backend
- **WHEN** a request to an openai-compatible model carries `anthropic` and `openai` provider options alongside its own namespace
- **THEN** the model SHALL receive only its own namespace, byte for byte
- **AND** the response SHALL succeed

#### Scenario: Unclassified Anthropic field
- **WHEN** a request to an Anthropic model carries an `anthropic` field the policy does not list
- **THEN** that field SHALL be removed before the model runs, and the listed fields SHALL keep their values

#### Scenario: Unclassified backend
- **WHEN** a resolved model carries the zero-value policy
- **THEN** it SHALL receive no caller provider options

### Requirement: Resolution and bounded model invocation

For a supported request, the handler SHALL resolve the exact requested model ID once, require a non-empty valid-UTF-8 canonical catalog ID and a non-nil V4 language model, and invoke `DoGenerate` once. The canonical ID is an internal routing and telemetry invariant and SHALL not be emitted in the unary response. Invocation SHALL derive from the request context and configured duration. A child goroutine with panic recovery and buffered completion SHALL bound handler latency when a model ignores cancellation; a permanently blocked model may retain that goroutine.

#### Scenario: Supported execution
- **WHEN** resolution returns a valid V4 model
- **THEN** resolution and `DoGenerate` SHALL each run once

#### Scenario: Invalid resolution
- **WHEN** resolution fails, returns an empty or invalid-UTF-8 canonical ID, nil model, non-V4 model, or panics while resolving or inspecting the model
- **THEN** the handler SHALL return a safe error without invoking an invalid model

#### Scenario: Cancellation or timeout
- **WHEN** caller cancellation or the configured duration becomes observable before model completion
- **THEN** handler latency SHALL remain bounded and the corresponding safe response SHALL be selected

### Requirement: Fixed privacy-safe errors

Every runtime error response SHALL be selected from precomputed documents with fixed status, message, type, code, and `param: null`. Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability. Unknown or invalid internal categories SHALL fall back to the fixed internal-error document. Provider, transport, resolver, panic, body, URL, header, credential, backend identity, and metadata details SHALL never be serialized.

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL reduce it to the corresponding fixed safe document without serializing the cause

#### Scenario: Unknown model
- **WHEN** catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** the registered Gateway client consumes a fixed error document
- **THEN** its status, type, and retryability SHALL map to the expected client class

### Requirement: Minimal unary success response

A successful unary response SHALL contain only ordered supported text and function-tool-call `content`, `finishReason`, and `usage`. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers, bodies, raw usage, provider metadata, and content metadata SHALL be omitted. The registered Gateway client owns unary `warnings`, `request`, and `response`; raw response-body details outside this minimal contract are not guaranteed.

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and emit no other top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns content outside the supported text/function-tool-call subset, an unknown finish reason, invalid usage, `nil, nil`, or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Provider-private fields
- **WHEN** the model result contains warnings, response metadata, raw usage, backend identity, or provider metadata
- **THEN** none of those values SHALL appear in the unary response document

### Requirement: Bounded preflight and standard success encoding

Before encoding, the handler SHALL reject content cardinality or aggregate content and raw-finish string bytes that cannot fit the configured unary budget using overflow-safe accounting. It SHALL validate UTF-8 only after the size preflight so scanning remains bounded. The complete minimal private DTO SHALL then be encoded with standard Go JSON, rejected when the final bytes exceed the configured limit, and committed only after successful encoding and the final size check. Provider-domain JSON marshalers SHALL NOT control the response. Standard encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content count or aggregate raw string bytes exceed the unary budget
- **THEN** the result SHALL fail before UTF-8 scanning or JSON encoding

#### Scenario: Escaping crosses the final boundary
- **WHEN** raw bytes pass preflight but standard JSON escaping makes the encoded response exceed the limit
- **THEN** the handler SHALL return the fixed internal error before committing HTTP 200

#### Scenario: Response byte boundary
- **WHEN** the encoded response is below, exactly at, or above the configured limit
- **THEN** only complete in-limit documents SHALL receive HTTP 200

### Requirement: Compatibility evidence

The runtime SHALL replay every committed ProviderWire request golden without modifying it. Cross-language integration tests SHALL call the production handler through the exact registered `@ai-sdk/gateway` version and verify minimal unary success, streaming text through clean EOF, representative errors, and cancellation. Raw Go tests SHALL remain authoritative for exact documents, privacy, sequencing, lifecycle, and byte bounds.

#### Scenario: Registered client success
- **WHEN** the pinned Gateway client sends a supported unary request
- **THEN** it SHALL consume content, finish reason, and usage from the production handler and supply its own warnings/request/response fields

#### Scenario: Streaming request
- **WHEN** the registered client sends a supported streaming text request
- **THEN** the handler SHALL invoke `DoStream` once and the client SHALL consume the strict stream through clean EOF

### Requirement: Host-safe ProviderWire error writer
The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including service authentication, permission, and discovery failures. Construction SHALL be non-fallible and SHALL accept no configuration. Callers SHALL select only an exported closed authentication, permission, or internal category; they SHALL NOT supply public messages, causes, arbitrary status codes, error types, error codes, retryability, or byte limits.

The writer SHALL reuse package-owned fixed ProviderWire documents directly. It SHALL perform no runtime schema compilation or validation and no dynamic JSON encoding. Authentication SHALL emit the package's exact fixed 401 document, permission SHALL emit the exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document. The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

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
