## MODIFIED Requirements

### Requirement: Constructed language-model handler

The `gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil `catalog.ModelResolver` and positive limits for request bytes, unary response bytes, provider stream-part count, complete SSE frame bytes, total model duration, stream idle duration, and bounded post-cancellation drain duration. Byte limits and the stream-part limit SHALL support safe `limit+1` arithmetic, and the frame limit SHALL contain the fixed start and stream-error frames.

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

### Requirement: Compatibility evidence

The runtime SHALL replay every committed ProviderWire request golden without modifying it. Cross-language integration tests SHALL call the production handler through the exact registered `@ai-sdk/gateway` version and verify minimal unary success, streaming text through clean EOF, representative errors, and cancellation. Raw Go tests SHALL remain authoritative for exact documents, privacy, sequencing, lifecycle, and byte bounds.

#### Scenario: Registered client success
- **WHEN** the pinned Gateway client sends a supported unary request
- **THEN** it SHALL consume content, finish reason, and usage from the production handler and supply its own warnings/request/response fields

#### Scenario: Streaming request
- **WHEN** the registered client sends a supported streaming text request
- **THEN** the handler SHALL invoke `DoStream` once and the client SHALL consume the strict stream through clean EOF
