# sdk-provider-transport-context Specification

## Purpose
Define native Anthropic and OpenAI transport observability through existing
provider and core result surfaces, preserving SDK ownership, bounded startup,
ordered opt-in raw evidence, and Gateway privacy.

## Requirements

### Requirement: SDK-backed providers honor per-call headers

The native Anthropic and OpenAI Responses adapters SHALL honor `CallOptions.Headers` in both `DoGenerate` and `DoStream`. Ordinary call headers SHALL override constructor headers for the same effective HTTP header name, while unrelated configured headers remain present. Header composition SHALL NOT mutate caller maps or reusable model configuration. This requirement SHALL preserve SDK-owned authentication behavior and SHALL NOT authorize changes to Gateway protected-header policy.

#### Scenario: Ordinary header precedence in both modes
- **WHEN** either provider receives constructor headers `X-Configured: configured` and `X-Shared: configured` and call headers `X-Call: call` and `X-Shared: call`
- **THEN** unary and streaming HTTP requests contain the configured-only and call-only headers and `X-Shared: call`
- **AND** a later call without overrides retains constructor headers

#### Scenario: Agent marker reaches Anthropic
- **WHEN** a ToolLoopAgent generate or stream call supplies the `ai-sdk-agent/tool-loop` User-Agent marker to Anthropic
- **THEN** the actual HTTP request contains that marker without discarding unrelated caller headers

### Requirement: Anthropic beta headers retain every source

Anthropic SHALL combine configured, call-level, and feature-required beta header values in both modes. It SHALL recognize the HTTP header name case-insensitively, lowercase and trim beta tokens, remove empty tokens and duplicates, and retain every required beta. Other headers SHALL retain ordinary override behavior.

#### Scenario: Beta union instead of replacement
- **WHEN** configured and call beta headers contain mixed-case overlapping tokens and request conversion requires an additional beta
- **THEN** the effective HTTP beta header contains each normalized token once, including the feature-required token
- **AND** neither configured nor call beta tokens are lost

### Requirement: Returned metadata represents the actual HTTP exchange

For successful native Anthropic and OpenAI unary and streaming calls with outbound JSON, the adapters SHALL populate Request.Body with the final serialized outbound JSON after SDK options and transport body transformations. Unary results SHALL additionally contain the full response JSON in Response.Body and actual HTTP response headers in Response.Headers; stream results SHALL contain actual HTTP headers in StreamResult.Response.Headers. Response identity fields SHALL retain their existing mapping. Multi-valued headers SHALL be flattened using `", "` between values, matching the native HTTP adapters, and metadata SHALL be invocation-local. An explicitly configured non-JSON request body SHALL remain unchanged and SHALL NOT be stored as invalid JSON in Request.Body.

#### Scenario: Body-changing SDK options are observable
- **WHEN** a JSON request is modified by a configured SDK JSON option and sent through either adapter in either mode
- **THEN** returned Request.Body is semantically equal to the body captured by the HTTP endpoint, including the streaming flag when present
- **AND** returned response headers equal the actual HTTP response headers

#### Scenario: Unary unknown response fields survive
- **WHEN** a successful unary response includes fields not used by typed response conversion
- **THEN** Response.Body retains the full response JSON, including those fields
- **AND** normalized content and response identity remain available

#### Scenario: Vertex transforms the outbound body
- **WHEN** Vertex rewrites an Anthropic JSON request before sending it
- **THEN** Request.Body reflects the rewritten payload, including its injected version and removed model field
- **AND** authentication and endpoint routing remain controlled by the existing SDK path

#### Scenario: Non-JSON body override stays outside JSON metadata
- **WHEN** an explicit SDK request-body override sends non-JSON bytes and the call succeeds
- **THEN** the adapter leaves those outbound bytes unchanged and omits Request.Body rather than reporting invalid JSON

### Requirement: Native transport metadata reaches core results

For completed steps served by either SDK-backed adapter, StreamText and the streaming-backed GenerateText and Agent entry points SHALL expose the step's request body and actual response headers through their existing step/result surfaces. The adapter SHALL enrich its existing response-metadata part with HTTP headers rather than emit a duplicate metadata part. Raw administrative parts SHALL NOT become model output, step content, or tool execution authority.

#### Scenario: Completed high-level result exposes transport context
- **WHEN** StreamText, GenerateText, or Agent completes a native provider call with an identity metadata event
- **THEN** the completed step's Request.Body equals captured outbound JSON and its Response.Headers equal the HTTP headers
- **AND** the final response accessor exposes the final completed step's headers

#### Scenario: Multiple steps retain separate exchanges
- **WHEN** a tool loop completes two provider steps with different HTTP response headers and request bodies
- **THEN** each step retains its own transport metadata without later-call mutation

### Requirement: Raw stream evidence is opt-in and ordered

When IncludeRawChunks is true, both native adapters SHALL emit PartStreamStart first and a PartRaw before each consumed data event's normalized parts after successful preflight. Raw evidence SHALL preserve the original valid JSON, including unknown fields and post-preflight provider error envelopes. Anthropic ping and ignored JSON events SHALL remain raw-observable even when they have no normalized output. Comments, empty non-data frames, and `[DONE]` SHALL NOT produce raw parts. Omitted or false IncludeRawChunks SHALL produce no PartRaw, and selecting raw output SHALL NOT change normalized recovery, retry, finish, or initial-error behavior.

#### Scenario: Normal events retain ordering
- **WHEN** a successful stream includes buffered preflight events followed by text and finish events with IncludeRawChunks true
- **THEN** stream-start precedes all raw parts and each event's raw part precedes its corresponding normalized parts
- **AND** normalized parts have the same order and values as with raw output disabled

#### Scenario: Raw output is disabled by default
- **WHEN** IncludeRawChunks is omitted or explicitly false
- **THEN** neither provider emits PartRaw

#### Scenario: Ping and framing-only input
- **WHEN** an Anthropic stream contains a JSON ping, an SSE comment, and an empty non-data frame with raw enabled
- **THEN** the ping produces one raw part without normalized content
- **AND** the comment and empty non-data frame produce no raw parts

#### Scenario: Unknown JSON event stays visible
- **WHEN** a decoder delivers an ignored JSON event with fields outside the typed SDK union and raw output is enabled
- **THEN** its original JSON appears in PartRaw even if no normalized content part is emitted

### Requirement: Malformed and error frames preserve the raw-value boundary

For a frame delivered after successful preflight, valid JSON SHALL remain valid RawValue even if typed decoding fails. Syntactically invalid JSON in events eligible for SDK typed decoding SHALL produce a PartRaw with nil RawValue when raw is enabled, followed by the normalized error; the adapter SHALL NOT put invalid bytes in json.RawMessage or substitute a JSON string. Valid JSON null SHALL remain distinguishable from missing RawValue. Error preflight SHALL retain its existing return-versus-stream behavior regardless of raw selection; no stream result SHALL be returned solely to expose raw evidence of an initial failure.

#### Scenario: Valid JSON cannot decode into a typed event
- **WHEN** a post-preflight frame is valid JSON but fails typed decoding with raw enabled
- **THEN** its complete valid JSON raw part precedes the decoding error

#### Scenario: Invalid JSON after preflight
- **WHEN** a post-preflight frame eligible for SDK typed decoding contains syntactically invalid JSON with raw enabled
- **THEN** a raw part with nil RawValue precedes the error
- **AND** provider stream-part serialization does not fail because of invalid RawValue bytes

#### Scenario: JSON null is not undefined
- **WHEN** a post-preflight data frame contains valid JSON null with raw enabled
- **THEN** its raw part retains JSON null rather than nil RawValue

#### Scenario: Provider error after stream handoff
- **WHEN** a provider error frame is consumed after successful preflight with raw enabled
- **THEN** the original JSON error envelope's raw part precedes the normalized error
- **AND** existing provider error classification remains intact

#### Scenario: Initial error still fails setup
- **WHEN** existing preflight recognizes an initial provider failure with IncludeRawChunks true
- **THEN** DoStream returns the existing classified error without a stream result

#### Scenario: Anthropic ignored malformed payload retains SDK behavior
- **WHEN** an Anthropic ping or ignored SSE event contains invalid JSON
- **THEN** normalization ignores that event as the existing SDK does
- **AND** raw selection exposes nil RawValue without introducing a normalized error

### Requirement: Startup retention is bounded independently of raw selection

Both native adapters SHALL limit startup processing to 64 data frames and 1 MiB of event data before stream handoff. Exceeding either budget SHALL return a non-retryable setup error without a stream result and cancel decoder ownership. The same policy SHALL apply with raw output enabled or disabled; neither truncation nor an earlier handoff SHALL substitute for overflow failure.

#### Scenario: Startup prefix exceeds the budget
- **WHEN** pings, ignored events, or other startup frames exceed a startup budget before handoff
- **THEN** DoStream returns a non-retryable setup error without a stream result
- **AND** acquired decoder ownership is released once without consumer reads

### Requirement: Transport capture preserves SDK ownership

The adapters SHALL retain existing SDK client construction, configured transports, authentication, retries, and endpoint rewriting. Capture SHALL preserve request bytes and replayability and SHALL associate successful results with the successful/final attempt. Each acquired framing decoder SHALL be constructed and closed once. Streaming sends and cleanup SHALL observe cancellation without requiring a consumer to drain an indefinitely blocked channel. Capture failures SHALL propagate rather than silently report misleading metadata.

#### Scenario: Retry returns the successful exchange
- **WHEN** an SDK call retries a transient failure before receiving a successful response
- **THEN** the SDK retains its retry behavior and request replay capability
- **AND** returned headers and request metadata describe the successful exchange without leaking earlier attempt metadata

#### Scenario: Preconfigured OpenAI client remains authoritative
- **WHEN** NewResponsesWithClient receives a provider-owned client, including a Mantle signing client
- **THEN** metadata and raw capture preserve its authentication, endpoint, custom transport, and unchanged signed payload

#### Scenario: Cancellation with a stalled consumer
- **WHEN** a native provider stream consumer stops reading and cancels the context
- **THEN** producer work and cleanup terminate without waiting for new consumer reads
- **AND** the acquired decoder and response-body ownership are released exactly once

#### Scenario: Concurrent calls do not share captures
- **WHEN** two calls on the same model execute concurrently with distinct requests and responses
- **THEN** each result and raw stream retains only its own exchange metadata

### Requirement: Gateway privacy remains independent

Native transport observability SHALL NOT enable Gateway raw-output requests or add private backend request bodies, response bodies, headers, or unknown metadata to Gateway normalization. Existing public response projection and protected-header enforcement SHALL remain unchanged.

#### Scenario: Enriched native result passes through Gateway
- **WHEN** Gateway serves a provider result containing native transport metadata
- **THEN** its public unary or streaming output omits the private native request/response bodies and headers

#### Scenario: Gateway raw request remains unsupported
- **WHEN** an authenticated Gateway request enables IncludeRawChunks
- **THEN** the current unsupported-capability response occurs before backend invocation
