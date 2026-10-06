## ADDED Requirements

### Requirement: Wire package location and scope

The repository SHALL define a `provider/wire/` Go package that hosts JSON+SSE transport helpers for the internal Go-to-Go provider wire. The package MUST contain only routes/headers, request/response envelopes, SSE stream-part encoding/decoding, and error-envelope helpers. It MUST NOT contain protobuf, Connect, or other binary-format machinery. The empty `provider/wire/proto/` and `provider/wire/wirepb/wirepbconnect/` placeholder directories SHALL be removed.

#### Scenario: Package import path
- **WHEN** a Go file in this repository or in `providers/grafana/` imports the wire helpers
- **THEN** it SHALL import `github.com/grafana/ai-sdk/provider/wire`

#### Scenario: No protobuf machinery
- **WHEN** the `provider/wire/` directory is inspected
- **THEN** it SHALL NOT contain `.proto` files, generated `.pb.go` files, `wirepb/`, `wirepbconnect/`, `buf.gen.yaml`, or any Connect-related artifacts

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

For streaming `DoStream` calls, a 2xx response SHALL have `Content-Type: text/event-stream`. Each event SHALL be a single SSE message of the form `data: <JSON of provider.StreamPart>\n\n`. The wire MUST NOT use a `[DONE]` sentinel; normal HTTP body close indicates end of stream. Events MUST NOT use the SSE `event:` field for type discrimination — discrimination lives entirely in the `Type` field of the JSON `StreamPart`.

#### Scenario: Stream events are JSON-encoded StreamParts
- **WHEN** the server emits a stream event for a `provider.StreamPart{Type: PartTextDelta, ID: "b1", Delta: "hello"}`
- **THEN** the wire SHALL transmit `data: {"type":"text-delta","id":"b1","delta":"hello"}\n\n`

#### Scenario: Normal stream end is HTTP body close
- **WHEN** the server has emitted all stream parts
- **THEN** it SHALL close the response body with no terminator event; the client SHALL treat EOF as clean stream completion

#### Scenario: Mid-stream error is a PartError event
- **WHEN** the server encounters an error after emitting some content
- **THEN** it SHALL emit a final SSE event encoding `provider.StreamPart{Type: PartError, APICallError: <reconstructable APICallError>}` and then close the body

### Requirement: Error envelope for non-2xx generate responses

For unary `DoGenerate` calls that fail, the response SHALL be an HTTP non-2xx status with `Content-Type: application/json` and a body that JSON-decodes into `provider.APICallError`. The `IsRetryable` field MUST be populated explicitly. The wire package SHALL provide a decoder that returns `*provider.APICallError` with `IsRetryable`, `StatusCode`, `URL`, `Message`, `ResponseBody`, `ResponseHeaders`, and `Data` populated from the response.

#### Scenario: Retryable failure response
- **WHEN** the server returns HTTP 429 with body `{"message":"rate limit exceeded","statusCode":429,"isRetryable":true}`
- **THEN** the wire decoder SHALL return a `*provider.APICallError` with `IsRetryable == true`, `StatusCode == 429`, and `Message == "rate limit exceeded"`

#### Scenario: Non-retryable failure response
- **WHEN** the server returns HTTP 400 with body containing `"isRetryable":false`
- **THEN** the wire decoder SHALL return a `*provider.APICallError` with `IsRetryable == false`

### Requirement: SSE encoder/decoder helpers

The wire package SHALL export `WriteSSEStreamPart(w io.Writer, part provider.StreamPart) error` for the server side and `ReadSSEStreamPart(r io.Reader) (provider.StreamPart, error)` (or an iterator/scanner equivalent) for the client side. Each SHALL handle exactly one event boundary per call. Both MUST round-trip every `provider.StreamPartType` losslessly.

#### Scenario: Round-trip every StreamPartType
- **WHEN** every defined `provider.StreamPartType` value is encoded via `WriteSSEStreamPart` and then decoded via `ReadSSEStreamPart`
- **THEN** the decoded `StreamPart` SHALL equal the original (using `reflect.DeepEqual`) for every type

#### Scenario: Round-trip APICallError on PartError
- **WHEN** a `StreamPart{Type: PartError, APICallError: &APICallError{StatusCode: 500, IsRetryable: true, Message: "boom"}}` is encoded and decoded
- **THEN** the decoded part SHALL have `Type == PartError`, a non-nil `APICallError`, `StatusCode == 500`, `IsRetryable == true`, and `Message == "boom"`

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
- **WHEN** the `provider/wire/` package is inspected
- **THEN** no symbol SHALL relate to auth, tokens, CAP, or `X-Grafana-Id`

### Requirement: Wire package has no orchestration knowledge

The wire package SHALL NOT depend on the root `aisdk` orchestration package, on `@ai-sdk/react`-style UI message chunks, or on tool-execution machinery. It SHALL depend only on the standard library, the `provider/` package, and small JSON/SSE helpers.

#### Scenario: Dependency boundary
- **WHEN** the wire package's `import` statements are inspected
- **THEN** no import SHALL be from `github.com/grafana/ai-sdk/aisdk` or any other orchestration-layer package

### Requirement: Wire round-trip test suite

The wire package SHALL include a test suite that exercises round-trip serialization for every defined `provider.StreamPartType` value, every `ContentPartType` value, every `ToolType` value, every notable `CallOptions` field, and every `APICallError` field. Tests MUST use white-box JSON+SSE encoding and decoding through the public encode/decode helpers.

#### Scenario: Per-StreamPartType round-trip
- **WHEN** the test suite runs
- **THEN** at least one assertion SHALL exist per defined `provider.StreamPartType` value confirming JSON round-trip with no field loss

#### Scenario: Per-ContentPartType round-trip
- **WHEN** the test suite runs
- **THEN** at least one assertion SHALL exist per defined `ContentPartType` value confirming JSON round-trip with no field loss

#### Scenario: Per-CallOptions-field round-trip
- **WHEN** the test suite runs
- **THEN** every notable `provider.CallOptions` field (`Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `StopSequences`, `ResponseFormat`, `Seed`, `Reasoning`, `IncludeRawChunks`, `Headers`, `ProviderOptions`) SHALL have at least one assertion confirming wire round-trip
