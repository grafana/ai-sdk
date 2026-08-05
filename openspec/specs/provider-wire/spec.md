# provider-wire Specification

## Purpose

Define the upstream-compatible JSON+SSE transport for remote `provider.LanguageModel` calls, preserving lossless Go round-trips and legacy decoding without protobuf or orchestration-layer coupling.
## Requirements
### Requirement: Wire package location and scope

The repository SHALL define a `gateway/providerwire/` Go package that owns the complete JSON+HTTP/SSE transport for the remote `provider.LanguageModel` protocol. The package SHALL contain the route/header constants, request/response envelopes, SSE stream-part encoding/decoding, error-envelope helpers, and reusable server handler. It MUST NOT contain protobuf, Connect, or other binary-format machinery. The former `provider/wire/` package SHALL be deleted, and the repository MUST NOT provide aliases, compatibility re-exports, or a forwarding shim at its old import path. Moving the exported helpers is an intentional source-breaking import-path change; canonical encoded bytes and protocol shapes SHALL remain unchanged, except that the SSE reader SHALL apply the explicitly specified final-line EOF correction.

#### Scenario: Package import path
- **WHEN** a Go file in this repository or in `providers/grafana/` imports the wire helpers
- **THEN** it SHALL import `github.com/grafana/ai-sdk/gateway/providerwire`

#### Scenario: No protobuf machinery
- **WHEN** the `gateway/providerwire/` directory is inspected
- **THEN** it SHALL NOT contain `.proto` files, generated `.pb.go` files, `wirepb/`, `wirepbconnect/`, `buf.gen.yaml`, or any Connect-related artifacts

#### Scenario: Old package is deleted without compatibility
- **WHEN** the repository is inspected after the move
- **THEN** `provider/wire/` SHALL not exist and no package SHALL alias or re-export `gateway/providerwire` symbols from the old path

#### Scenario: Canonical wire output remains stable across the source break
- **WHEN** existing request, response, error-envelope, or SSE values are encoded through `gateway/providerwire`
- **THEN** their encoded bytes and protocol shapes SHALL match the former `provider/wire` implementation

### Requirement: Single endpoint with streaming header switch

The wire package SHALL expose a single HTTP endpoint path constant `PathLanguageModel = "/language-model"`. Both `DoStream` and `DoGenerate` SHALL be transported via `POST` to that path. The body SHALL carry the JSON encoding of `provider.CallOptions`. The HTTP request SHALL distinguish unary vs streaming via the `ai-language-model-streaming` header (boolean string `true`/`false`).

#### Scenario: Generate request shape
- **WHEN** the client transports a `DoGenerate` call
- **THEN** the request SHALL be `POST /language-model` with header `ai-language-model-streaming: false`, header `ai-language-model-id: <modelID>`, header `ai-language-model-specification-version: 4`, and body equal to the JSON encoding of the `provider.CallOptions`

#### Scenario: Stream request shape
- **WHEN** the client transports a `DoStream` call
- **THEN** the request SHALL be `POST /language-model` with header `ai-language-model-streaming: true`, the same model-id and spec-version headers, and body equal to the JSON encoding of the `provider.CallOptions`

### Requirement: Header constants

The wire package SHALL export the following exported header-name constants and use them at every read/write site:

- `HeaderModelID = "ai-language-model-id"`
- `HeaderStreaming = "ai-language-model-streaming"`
- `HeaderSpecVersion = "ai-language-model-specification-version"`

Header names MUST match these values exactly to align with upstream Vercel AI SDK gateway conventions.

#### Scenario: Constants exist and have expected values
- **WHEN** the wire package is inspected
- **THEN** the three exported constants SHALL be present with the values listed above

### Requirement: Generate response is JSON GenerateResult

For unary `DoGenerate` calls, a 2xx response body SHALL be the JSON encoding of `provider.GenerateResult` with `Content-Type: application/json`. The decoder MUST round-trip every populated field of `GenerateResult` losslessly.

#### Scenario: Successful generate response
- **WHEN** the server returns HTTP 200 with body containing the JSON encoding of a `provider.GenerateResult` carrying content, finish reason, usage, and provider metadata
- **THEN** the wire package's decoder SHALL produce an equivalent `*provider.GenerateResult` with no field loss

### Requirement: Stream response is text/event-stream of JSON StreamParts

For streaming `DoStream` calls, a 2xx response SHALL have `Content-Type: text/event-stream`. Each event SHALL be a single SSE message of the form `data: <JSON of provider.StreamPart>\n\n`. The wire MUST NOT use a `[DONE]` sentinel; normal HTTP body close indicates end of stream. Events MUST NOT use the SSE `event:` field for type discrimination -- discrimination lives entirely in the `Type` field of the JSON `StreamPart`.

#### Scenario: Stream events are JSON-encoded StreamParts
- **WHEN** the server emits a stream event for a `provider.StreamPart{Type: PartTextDelta, ID: "b1", Delta: "hello"}`
- **THEN** the wire SHALL transmit `data: {"type":"text-delta","id":"b1","delta":"hello"}\n\n`

#### Scenario: Normal stream end is HTTP body close
- **WHEN** the server has emitted all stream parts
- **THEN** it SHALL close the response body with no terminator event; the client SHALL treat EOF as clean stream completion

#### Scenario: Handler timeout or cancellation is a final PartError event
- **WHEN** the request is canceled or times out after SSE commitment and the writer remains available
- **THEN** the server SHALL emit a final SSE event encoding `provider.StreamPart{Type: PartError, APICallError: <reconstructable APICallError>}` and then close the body

### Requirement: Error envelope for non-2xx generate responses

For unary `DoGenerate` calls that fail, the response SHALL be an HTTP non-2xx status with `Content-Type: application/json` and an upstream-compatible `{"error":{...}}` body carrying `provider.APICallError` fields. The decoder SHALL also accept the legacy bare error object. The `IsRetryable` field MUST be populated explicitly. The wire package SHALL provide a decoder that returns `*provider.APICallError` with `IsRetryable`, `StatusCode`, `URL`, `Message`, `ResponseBody`, `ResponseHeaders`, and `Data` populated from the response.

#### Scenario: Retryable failure response
- **WHEN** the server returns HTTP 429 with body `{"error":{"message":"rate limit exceeded","statusCode":429,"isRetryable":true}}`
- **THEN** the wire decoder SHALL return a `*provider.APICallError` with `IsRetryable == true`, `StatusCode == 429`, and `Message == "rate limit exceeded"`

#### Scenario: Non-retryable failure response
- **WHEN** the server returns HTTP 400 with a wrapped or legacy bare body containing `"isRetryable":false`
- **THEN** the wire decoder SHALL return a `*provider.APICallError` with `IsRetryable == false`

### Requirement: SSE encoder/decoder helpers

The `gateway/providerwire` package SHALL export `WriteSSEStreamPart(w io.Writer, part provider.StreamPart) error` for the server side and `NewSSEReader(r io.Reader)` with `SSEReader.Next()` for the client side. Each SHALL handle exactly one event boundary per call and SHALL round-trip every `provider.StreamPartType` losslessly. When the underlying reader returns both non-empty final-line bytes and `io.EOF`, `Next` SHALL process those bytes before deciding whether the stream ended cleanly, so a valid unterminated final event is decoded and an invalid one returns a decode error rather than a false clean EOF.

#### Scenario: Round-trip every StreamPartType

- **WHEN** every defined `provider.StreamPartType` value is encoded via `WriteSSEStreamPart` and then decoded via `SSEReader.Next`
- **THEN** the decoded `StreamPart` SHALL equal the original for every type

#### Scenario: Round-trip APICallError on PartError

- **WHEN** a `StreamPart{Type: PartError, APICallError: &APICallError{StatusCode: 500, IsRetryable: true, Message: "boom"}}` is encoded and decoded
- **THEN** the decoded part SHALL have `Type == PartError`, a non-nil `APICallError`, `StatusCode == 500`, `IsRetryable == true`, and `Message == "boom"`

#### Scenario: Unterminated final data line is decoded

- **WHEN** the final SSE event has a valid `data:` line but no trailing newline or blank event boundary
- **THEN** `SSEReader.Next` SHALL decode and return that final `provider.StreamPart`

#### Scenario: Unterminated multiline final event is decoded

- **WHEN** an SSE event has multiple `data:` lines and its final line has no trailing newline
- **THEN** `SSEReader.Next` SHALL join all data lines and decode the final `provider.StreamPart`

#### Scenario: Invalid unterminated final event is observable

- **WHEN** the final `data:` line has no trailing newline and contains invalid JSON
- **THEN** `SSEReader.Next` SHALL return a decoding error rather than `io.EOF`

### Requirement: Request/response envelope helpers

The wire package SHALL export the following helpers and MUST use them at every encode/decode site:

- `EncodeCallOptions(opts provider.CallOptions) ([]byte, error)`
- `DecodeCallOptions(data []byte) (provider.CallOptions, error)`
- `EncodeGenerateResult(result *provider.GenerateResult) ([]byte, error)`
- `DecodeGenerateResult(data []byte) (*provider.GenerateResult, error)`
- `EncodeAPICallError(err *provider.APICallError) ([]byte, error)`
- `DecodeAPICallError(data []byte) (*provider.APICallError, error)`

These wrappers exist so future schema-evolution helpers (versioning, validation) have a single home.

#### Scenario: CallOptions round-trip
- **WHEN** a `provider.CallOptions` carrying `Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `Reasoning`, `ResponseFormat`, `StopSequences`, `Headers`, and `ProviderOptions` is encoded and decoded
- **THEN** the decoded value SHALL be equal to the original (using `reflect.DeepEqual`) with no field loss

#### Scenario: GenerateResult round-trip
- **WHEN** a `provider.GenerateResult` carrying `Content`, `FinishReason`, `Usage`, `ProviderMetadata`, `Warnings`, `Request`, and `Response` is encoded and decoded
- **THEN** the decoded value SHALL be equal to the original with no field loss

#### Scenario: APICallError round-trip
- **WHEN** a `provider.APICallError` carrying `Message`, `StatusCode`, `URL`, `RequestBodyValues`, `ResponseHeaders`, `ResponseBody`, `IsRetryable`, and `Data` is encoded and decoded
- **THEN** the decoded value SHALL be equal to the original (with `cause` not preserved across the wire, by design)

### Requirement: Auth metadata is provider-side, not wire-side

The wire package SHALL NOT define authentication helpers, token exchange, or auth-header constants. Headers such as `Authorization` and `X-Grafana-Id` are responsibility of the provider implementation that uses the wire package (e.g. `providers/grafana/`).

#### Scenario: Wire package has no auth helpers
- **WHEN** the `gateway/providerwire/` package is inspected
- **THEN** no symbol SHALL relate to auth, tokens, CAP, or `X-Grafana-Id`

### Requirement: Wire package has no orchestration knowledge

The wire package SHALL NOT depend on the root `aisdk` orchestration package, on `@ai-sdk/react`-style UI message chunks, or on tool-execution machinery. It SHALL depend only on the standard library and the transport-agnostic `provider/` package.

#### Scenario: Dependency boundary
- **WHEN** the wire package's `import` statements are inspected
- **THEN** no import SHALL be from `github.com/grafana/ai-sdk/aisdk` or any other orchestration-layer package

### Requirement: Wire round-trip test suite

The `gateway/providerwire` package SHALL include the moved wire test suite that exercises round-trip serialization for every defined `provider.StreamPartType` value, every `ContentPartType` value, every `ToolType` value, every notable `CallOptions` field, and every `APICallError` field. Tests MUST use white-box JSON+SSE encoding and decoding through the public encode/decode helpers, and their existing byte expectations SHALL remain unchanged by the package move.

#### Scenario: Per-StreamPartType round-trip
- **WHEN** the test suite runs
- **THEN** at least one assertion SHALL exist per defined `provider.StreamPartType` value confirming JSON round-trip with no field loss

#### Scenario: Per-ContentPartType round-trip
- **WHEN** the test suite runs
- **THEN** at least one assertion SHALL exist per defined `ContentPartType` value confirming JSON round-trip with no field loss

#### Scenario: Per-CallOptions-field round-trip
- **WHEN** the test suite runs
- **THEN** every notable `provider.CallOptions` field (`Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `StopSequences`, `ResponseFormat`, `Seed`, `Reasoning`, `IncludeRawChunks`, `Headers`, `ProviderOptions`) SHALL have at least one assertion confirming wire round-trip

### Requirement: Provider wire excludes obsolete tool approval result stream part

The provider wire package SHALL round-trip `PartToolApprovalRequest` stream parts and SHALL NOT include tests, fixtures, or compatibility aliases for the obsolete `tool-approval-result` stream part. Approval decisions SHALL cross the provider wire as prompt content parts of type `tool-approval-response` when included in `CallOptions.Prompt`.

#### Scenario: Tool approval request stream part round-trips
- **WHEN** a `provider.StreamPart{Type: PartToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "call_1"}` is encoded and decoded through the provider SSE wire helpers
- **THEN** the decoded stream part SHALL preserve `Type`, `ApprovalID`, and `ToolCallID`

#### Scenario: Tool approval result is absent from stream part coverage
- **WHEN** the provider wire round-trip test enumerates every defined `provider.StreamPartType`
- **THEN** it SHALL NOT include `tool-approval-result`

#### Scenario: Approval response prompt content round-trips
- **WHEN** `provider.CallOptions` contains a tool-role message with `ContentPartTypeToolApprovalResponse`
- **THEN** provider wire request encoding and decoding SHALL preserve the approval ID, approved value, reason, and provider-executed flag

### Requirement: PartResponseMeta carries provider over the wire

The provider wire SHALL round-trip the `Provider` field on `StreamPart` for `PartResponseMeta` events, alongside the existing `ResponseID` and `ModelID` fields, with no field loss.

#### Scenario: Response-meta provider round-trip
- **WHEN** a `provider.StreamPart{Type: PartResponseMeta, ResponseID: "r1", ModelID: "claude-x", Provider: "anthropic.vertex"}` is encoded and decoded via the SSE helpers
- **THEN** the decoded part SHALL equal the original (using `reflect.DeepEqual`), preserving `Provider == "anthropic.vertex"`

#### Scenario: Empty provider omitted on the wire
- **WHEN** a `StreamPart` with an empty `Provider` is encoded
- **THEN** the `provider` key SHALL be omitted from the JSON (`omitempty`), keeping the wire backward compatible

### Requirement: Decoders accept upstream LanguageModelV4 request shapes

The provider wire decoders SHALL accept the upstream Vercel AI SDK
`LanguageModelV4` JSON encodings for request-path payloads, in addition to the
canonical Go-to-Go encodings, so an upstream `@ai-sdk/gateway` client can POST
to the wire endpoint. The encoders now emit the upstream shapes as well (see the
"Encoders emit upstream LanguageModelV4 request/response shapes" requirement),
superseding the Go-to-Go-only emitted form from
`2026-04-30-lossless-provider-wire` (D6, D4); decoders remain tolerant of both
the upstream and the legacy Go-to-Go encodings.

The decoders SHALL accept:

- a `system` message whose `content` is a JSON **string**, decoding it into a
  single `text` `ContentPart`;
- a tool-result `output` in the upstream single-`value` shape
  (`{"type":"text","value":...}`, `{"type":"json","value":...}`,
  `{"type":"error-text","value":...}`, `{"type":"error-json","value":...}`,
  `{"type":"content","value":[...]}`), mapping `value` into the corresponding
  Go field;
- a file/reasoning-file `data` in the upstream tagged union
  (`{"type":"data",...}`, `{"type":"url",...}`, `{"type":"reference",...}`, or
  `{"type":"text",...}`), mapping it into `DataContent`.

This tolerance SHALL fail closed: a request-path payload whose shape this build
cannot represent losslessly (an unknown tagged file-data variant, a malformed
tool-result `value`, a tool-result `content` value carrying an unrepresentable
item, or string content on a non-system role) SHALL return a decode error rather
than silently dropping or emptying the field.

#### Scenario: System message content as a string decodes
- **WHEN** `providerwire.DecodeCallOptions` receives a prompt message
  `{"role":"system","content":"be helpful"}`
- **THEN** it SHALL produce a `Message` with `Role == RoleSystem` and
  `Content == []ContentPart{{Type: ContentPartTypeText, Text: "be helpful"}}`

#### Scenario: Tool-result output single-value decodes
- **WHEN** a tool message content part is
  `{"type":"tool-result","toolCallId":"tc_1","toolName":"search","output":{"type":"text","value":"ok"}}`
- **THEN** the decoded `ToolResultOutput` SHALL have `Type == ToolOutputText`
  and `Text == "ok"`

#### Scenario: File data tagged union decodes
- **WHEN** a user message content part is
  `{"type":"file","mediaType":"image/png","data":{"type":"url","url":"https://example.com/x.png"}}`
- **THEN** the decoded `ContentPart.Data` SHALL be a `DataContent` with
  `URL == "https://example.com/x.png"`

#### Scenario: Legacy Go-to-Go encodings still decode
- **WHEN** a payload carrying the legacy Go shapes (system `content` as an
  array, split tool-result `output`, `DataContent{URL: ...}`) is decoded with
  `providerwire.DecodeCallOptions`
- **THEN** the decoded value SHALL equal the equivalent upstream-encoded payload
  (using `reflect.DeepEqual`)

#### Scenario: Unknown file-data variant fails closed
- **WHEN** a file content part carries an unknown tagged `type` (not one of
  data, url, reference, text)
- **THEN** decoding SHALL return an error identifying the unsupported file-data
  variant, and SHALL NOT produce an empty `DataContent`

#### Scenario: Malformed tool-result value fails closed
- **WHEN** a tool-result `output` is `{"type":"text","value":123}` or
  `{"type":"content","value":[{"type":"file","data":{"type":"data","data":"aGk="},"mediaType":"image/png"}]}`
- **THEN** decoding SHALL return an error rather than an empty/partial
  `ToolResultOutput`

#### Scenario: Non-system string content fails closed
- **WHEN** a non-system message is `{"role":"user","content":"hi"}`
- **THEN** decoding SHALL return an error (string content is valid only for the
  system role)

### Requirement: Encoders emit upstream LanguageModelV4 request/response shapes

The provider wire encoders SHALL emit upstream Vercel AI SDK `LanguageModelV4`
JSON for every shape that previously diverged, so a stock upstream
`@ai-sdk/gateway` client interoperates in both directions. Decoders SHALL
continue to accept both the upstream and the legacy Go encodings.

The encoders SHALL emit:

- a `system` message with `content` as a JSON **string**;
- tool-result `output` in the single-`value` union
  (`{"type":"text","value":...}`, `{"type":"json","value":...}`, etc.);
- prompt file `data` as the full upstream tagged union (`data`, `url`,
  `reference`, or `text`), and generated file / reasoning-file stream `data` as
  the constrained upstream union (`data` or `url`);
- the streaming tool-result as the upstream flat `result` (JSON value) plus
  `isError`, represented separately from the prompt-only `ToolResultOutput`
  union; `providerMetadata` SHALL remain opaque and unchanged;
- the error stream part as `{"type":"error","error":{...}}`.

#### Scenario: System message encodes as a string
- **WHEN** a `Message{Role: RoleSystem, Content: [{Type:text, Text:"hi"}]}` is
  encoded
- **THEN** the JSON SHALL be `{"role":"system","content":"hi"}`

#### Scenario: Tool-result output encodes as single value
- **WHEN** a `ToolResultOutput{Type: ToolOutputText, Text: "ok"}` is encoded
- **THEN** the JSON SHALL be `{"type":"text","value":"ok"}`

#### Scenario: Streaming tool-result round-trips independently from prompt output
- **WHEN** a `StreamPart{Type: PartToolResult, Result: <JSON value>, IsError: <boolean>}`
  is encoded and decoded again
- **THEN** the wire JSON SHALL carry the exact flat `result` plus `isError`, and
  every `ProviderMetadata` namespace SHALL round-trip unchanged

#### Scenario: File data encodes as tagged union
- **WHEN** a `DataContent{URL: "https://x/y.png"}` is encoded
- **THEN** the JSON SHALL be `{"type":"url","url":"https://x/y.png"}`

#### Scenario: Generated URL file encodes as tagged union
- **WHEN** a `PartFile` or `PartReasoningFile` carries
  `StreamFileData{URL: "https://x/y.png"}`
- **THEN** its `data` field SHALL be
  `{"type":"url","url":"https://x/y.png"}` and SHALL round-trip without loss

#### Scenario: Error stream part encodes with an `error` field
- **WHEN** a `StreamPart{Type: PartError, APICallError: {Message:"boom",StatusCode:500}}` is encoded
- **THEN** the JSON SHALL be `{"type":"error","error":{...}}` carrying the
  APICallError payload under `error`

#### Scenario: Legacy Go encodings still decode
- **WHEN** the decoder receives the legacy Go encodings (system `content` array,
  split tool-result output, `DataContent{bytes|base64|url}`, error part with
  `apiCallError`)
- **THEN** it SHALL decode them equivalently to the upstream encodings

### Requirement: HTTP error envelope matches upstream gateway

For non-2xx unary and pre-stream failures, the response body SHALL be
`{"error": {<APICallError fields>}}` so the upstream gateway's error parser
(`createGatewayErrorFromResponse`) surfaces the real message and status. The
decoder SHALL accept both the wrapped `{"error":{...}}` form and the legacy bare
`APICallError` form.

#### Scenario: Wrapped error envelope is produced
- **WHEN** the server returns HTTP 429 for a rate-limited call
- **THEN** the body SHALL be `{"error":{"message":"...","statusCode":429,"isRetryable":true}}`
  and an upstream `@ai-sdk/gateway` client SHALL surface the message (not
  "Invalid error response format")

### Requirement: Bidirectional upstream-client conformance

The repository SHALL include a conformance harness that runs a real upstream
`@ai-sdk/gateway` + `ai` client against the Go provider-wire server and asserts
two-way compatibility for: streaming text, a tool-call round-trip, a provider-
executed tool result, file input and inline/URL-valued output, and mid-stream
plus pre-stream errors.

#### Scenario: Upstream client streams text with a system prompt
- **WHEN** the harness runs `streamText` with a `system` + user prompt against
  the Go server
- **THEN** the client SHALL receive the streamed text, a mapped `finishReason`,
  and `usage`, with no decode error

#### Scenario: Upstream client receives URL-valued generated files
- **WHEN** the Go server emits file and reasoning-file parts whose `data` is a URL variant
- **THEN** the upstream client SHALL receive each exact URL string in the corresponding generated file value

#### Scenario: Upstream client continues after a provider error
- **WHEN** the server emits a `PartError` followed by additional stream parts
- **THEN** the upstream client SHALL receive the non-empty error carrying the
  server's message and every subsequent part in order
