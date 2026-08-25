## Purpose

Define the production ProviderWire V4 unary text runtime and the observable contract proven against the registered Gateway client.

## Requirements

### Requirement: Constructed unary handler

The `gateway/providerwire/v4` package SHALL provide an HTTP handler for relative `POST /language-model` unary requests. Construction SHALL require a non-nil `catalog.ModelResolver` and positive limits for request bytes, unary response bytes, and total model duration. Byte limits SHALL support safe `limit+1` arithmetic.

#### Scenario: Valid construction
- **WHEN** a caller supplies a resolver and valid limits
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the resolver is nil, a limit is non-positive, or a byte limit cannot safely use `limit+1`
- **THEN** construction SHALL fail before serving traffic

### Requirement: Unary HTTP envelope

The handler SHALL accept only `POST /language-model` with JSON content and exactly one effective value for each ProviderWire protocol header. The specification version SHALL be `4`, the model ID SHALL be non-empty and preserved without rewriting, and streaming SHALL be `false`. Unrelated HTTP headers SHALL not become provider call headers automatically.

#### Scenario: Valid envelope
- **WHEN** method, route, media type, specification, model ID, and unary mode are valid
- **THEN** processing SHALL continue with the exact model ID

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

### Requirement: Unsupported capability families

Schema-valid files, reasoning content, custom content, tools, tool approvals, structured output, non-empty provider options, body headers, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

### Requirement: Resolution and bounded model invocation

For a supported request, the handler SHALL resolve the exact requested model ID once, require a non-empty canonical catalog ID and a non-nil V4 language model, and invoke `DoGenerate` once. The canonical ID is an internal routing and telemetry invariant and SHALL not be emitted in the unary response. Invocation SHALL derive from the request context and configured duration. A child goroutine with panic recovery and buffered completion SHALL bound handler latency when a model ignores cancellation; a permanently blocked model may retain that goroutine.

#### Scenario: Supported execution
- **WHEN** resolution returns a valid V4 model
- **THEN** resolution and `DoGenerate` SHALL each run once

#### Scenario: Invalid resolution
- **WHEN** resolution fails, returns an empty canonical ID, nil model, non-V4 model, or panics while resolving or inspecting the model
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

A successful unary response SHALL contain only ordered text `content`, `finishReason`, and `usage`. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers, bodies, raw usage, provider metadata, and content metadata SHALL be omitted. The registered Gateway client owns unary `warnings`, `request`, and `response`; raw response-body details outside this minimal contract are not guaranteed.

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and emit no other top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns non-text content, an unknown finish reason, invalid usage, `nil, nil`, or panics
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

The runtime SHALL replay every committed ProviderWire request golden without modifying it. A cross-language integration test SHALL call the production handler through the exact registered `@ai-sdk/gateway` version and verify minimal unary success, representative errors, and cancellation. Raw Go tests SHALL remain authoritative for exact documents, privacy, sequencing, and byte bounds. Streaming request emission and response consumption evidence SHALL not imply a Go streaming runtime.

#### Scenario: Registered client success
- **WHEN** the pinned Gateway client sends a supported unary request
- **THEN** it SHALL consume content, finish reason, and usage from the production handler and supply its own warnings/request/response fields

#### Scenario: Streaming request
- **WHEN** the registered client sends streaming mode to the unary handler
- **THEN** the handler SHALL reject it without creating a streaming protocol commitment
