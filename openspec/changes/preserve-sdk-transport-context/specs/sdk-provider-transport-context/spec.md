## ADDED Requirements

### Requirement: Anthropic per-call headers
The official-SDK direct Anthropic and Vertex adapters SHALL apply `CallOptions.Headers` in both `DoGenerate` and `DoStream`. For ordinary headers supplied in both configured request options and one call, the call value SHALL take precedence case-insensitively. For `anthropic-beta`, configured and per-call comma-delimited tokens SHALL be trimmed, lowercased, unioned and deduplicated in configured-then-call order, preserving SDK-added beta tokens, as in the registered upstream. These operations SHALL NOT change SDK authentication, construction, transport, or provider-owned request options. Calls on a shared model SHALL NOT exchange per-call header values.

#### Scenario: Configured and per-call headers collide
- **WHEN** a direct or Vertex model is configured with an ordinary `X-Shared` header and a call supplies `x-shared` plus an Agent marker
- **THEN** the actual outbound unary and streaming requests carry the call's shared value and Agent marker, plus nonconflicting configured headers and SDK-controlled authentication

#### Scenario: Configured and per-call beta tokens merge
- **WHEN** a direct or Vertex model has configured `anthropic-beta: CONFIG-beta1,config-beta2` and a generate or stream call supplies `anthropic-beta: REQUEST-beta1,config-beta2`
- **THEN** the outbound beta header retains the configured tokens and adds the nonduplicate request token in lowercased configured-then-call order, alongside any SDK-added beta tokens, rather than replacing the configured value

#### Scenario: Concurrent calls do not share headers
- **WHEN** two calls to the same model use distinct call headers
- **THEN** each captured request contains only its own call-specific values

### Requirement: SDK-backed success metadata
The Anthropic and OpenAI Responses adapters SHALL fill the existing successful `GenerateResult.Request.Body` and `StreamResult.Request.Body` with the corresponding outbound JSON request and `GenerateResult.Response.Headers` and `StreamResult.Response.Headers` with actual HTTP response headers. A successful unary `GenerateResult.Response.Body` SHALL contain the available raw JSON response; streaming `Response` SHALL contain only headers. Successful results SHALL preserve existing response identity, provider name, timestamp, warnings, content, and usage. Capture SHALL be call-local, SHALL NOT consume an SDK-owned stream or pre-read a request body that the SDK might transmit, and SHALL NOT report made-up request bytes or headers for unavailable HTTP responses. Where a custom transport bypasses or rewrites the captured outgoing body so the final transmitted JSON cannot be established, the optional `Request.Body` SHALL remain absent instead of carrying guessed bytes.

#### Scenario: Unary response metadata is returned
- **WHEN** a deterministic HTTP endpoint receives a successful direct/Vertex Anthropic or OpenAI Responses unary call and returns a distinctive JSON body and response headers
- **THEN** `DoGenerate` returns the actual request JSON and response headers/body in the existing result fields while preserving decoded content and identity

#### Scenario: Streaming response metadata is returned
- **WHEN** the same endpoint receives a successful streaming call, including SDK-added streaming fields
- **THEN** `DoStream` returns an outbound request body and HTTP response headers before its stream is consumed, with no streaming response body field

#### Scenario: Vertex rewrite and SDK retries
- **WHEN** a Vertex call rewrites the model and injects its version field, or a direct/Vertex/OpenAI call retries before success
- **THEN** the successfully returned request body's bytes match the final request received by the deterministic HTTP endpoint, not a marshaled pre-rewrite parameter or an earlier attempt; capturing shall not drain or alter the request actually transmitted

#### Scenario: Configured clients and overlapping calls
- **WHEN** concurrent calls, including an OpenAI `NewResponsesWithClient` model, use different request bodies and HTTP response headers
- **THEN** each result reports only its own request/response and the configured SDK client's endpoint, credentials, and transport remain authoritative

### Requirement: Opt-in raw SDK provider stream events
Anthropic and OpenAI Responses `DoStream` SHALL emit `provider.PartRaw` before normalized or error parts for each observable decoded stream event when `IncludeRawChunks` is true. The raw value SHALL contain an owned `json.RawMessage` for parsed JSON, or nil for an observable syntactically invalid JSON frame (matching upstream `rawValue: undefined`); it SHALL NOT be fabricated SSE text or an entire response body. With the option omitted or false, they SHALL emit no raw parts. Raw events SHALL NOT cause extra provider finish parts or change normalized output. Neither adapter SHALL invent raw parts for `[DONE]`, framing-only keepalives, or frames hidden by the SDK's decoder. The Anthropic SDK currently hides malformed/error/ping frames; this remains an explicit, unapproved parity coverage gap pending a reviewed disposition, not evidence of full upstream parity. Existing initial error preflight SHALL still return an error rather than a stream when it rejects a call.

#### Scenario: Raw mode matrix and ordering
- **WHEN** a successful stream carries multiple valid events and the call omits `IncludeRawChunks`, sets it false, or sets it true
- **THEN** the first two deliver no `PartRaw`, while the last delivers one raw JSON part immediately before each corresponding event's normalized parts, including events already buffered by preflight

#### Scenario: Malformed JSON and recoverable decoding
- **WHEN** an OpenAI Responses stream already accepted by preflight exposes a syntactically malformed JSON frame and then a valid event
- **THEN** opt-in raw output includes a nil-valued `PartRaw` immediately before the malformed-frame `PartError`, followed by the valid event's raw JSON and normalized parts; omitted/false raw output has no raw parts

#### Scenario: Anthropic SDK hides a raw frame
- **WHEN** the Anthropic SDK consumes a malformed JSON, SSE error, or ping frame before exposing `Stream.Current()`
- **THEN** the adapter shall not fabricate a raw event; a focused test shall document the discrepancy with pinned upstream and the implementation shall not declare full raw-frame parity until that discrepancy has an explicit disposition

#### Scenario: Error frame and preflight
- **WHEN** a transport or SDK-intercepted SSE error occurs before an accepted stream
- **THEN** `DoStream` preserves its initial error return and does not return a successful result or raw parts
- **WHEN** a parsed OpenAI error envelope occurs after the stream was returned
- **THEN** opt-in output emits the owned raw error JSON immediately before the provider error part, after earlier raw and normalized parts; SDK-hidden Anthropic error frames have no fabricated raw JSON

#### Scenario: Cancellation and decoder ownership
- **WHEN** a caller cancels a stream while its consumer is stalled and more than 64 normalized/raw parts are pending
- **THEN** context-aware sends throughout Anthropic's converter unblock promptly, its single owner closes the stream body, no cancellation-only error/finish parts are emitted, and no raw parts are emitted for unread events; OpenAI's stream remains similarly cancelable

### Requirement: Backend transport data remains private to provider callers
Adding provider request, response, and raw values SHALL NOT change Gateway success normalization, error serialization, or the frontend SSE wire protocol. Gateway public output SHALL NOT include backend credentials, headers, raw request or response bodies, backend model identity, or raw parts as a side effect of this change.

#### Scenario: Gateway handles enriched provider metadata
- **WHEN** a Gateway provider result contains secret request JSON, HTTP headers, response JSON and raw stream parts
- **THEN** the existing bounded allowlisted public success/error output omits that backend data and retains its prior handling of unsupported raw output
