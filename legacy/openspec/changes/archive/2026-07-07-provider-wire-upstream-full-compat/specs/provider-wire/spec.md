## ADDED Requirements

### Requirement: Encoders emit upstream LanguageModelV4 request/response shapes

The provider wire encoders SHALL emit upstream Vercel AI SDK `LanguageModelV4`
JSON for every shape that previously diverged, so a stock upstream
`@ai-sdk/gateway` client interoperates in both directions. Decoders SHALL
continue to accept both the upstream and the legacy Go encodings.

The encoders SHALL emit:

- a `system` message with `content` as a JSON **string**;
- tool-result `output` in the single-`value` union
  (`{"type":"text","value":...}`, `{"type":"json","value":...}`, etc.);
- file / reasoning-file `data` as the upstream tagged union
  (`{"type":"data","data":<base64>}`, `{"type":"url","url":...}`,
  `{"type":"reference","reference":...}`, `{"type":"text","text":...}`);
- the error stream part as `{"type":"error","error":{...}}`.

#### Scenario: System message encodes as a string
- **WHEN** a `Message{Role: RoleSystem, Content: [{Type:text, Text:"hi"}]}` is
  encoded
- **THEN** the JSON SHALL be `{"role":"system","content":"hi"}`

#### Scenario: Tool-result output encodes as single value
- **WHEN** a `ToolResultOutput{Type: ToolOutputText, Text: "ok"}` is encoded
- **THEN** the JSON SHALL be `{"type":"text","value":"ok"}`

#### Scenario: File data encodes as tagged union
- **WHEN** a `DataContent{URL: "https://x/y.png"}` is encoded
- **THEN** the JSON SHALL be `{"type":"url","url":"https://x/y.png"}`

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
executed tool result, file input and output, and mid-stream plus pre-stream
errors.

#### Scenario: Upstream client streams text with a system prompt
- **WHEN** the harness runs `streamText` with a `system` + user prompt against
  the Go server
- **THEN** the client SHALL receive the streamed text, a mapped `finishReason`,
  and `usage`, with no decode error

#### Scenario: Upstream client surfaces a mid-stream error
- **WHEN** the server emits a `PartError` mid-stream
- **THEN** the upstream client SHALL surface a non-empty error carrying the
  server's message (not `error: undefined`)
