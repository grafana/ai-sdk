## ADDED Requirements

### Requirement: Decoders accept upstream LanguageModelV4 request shapes

The provider wire decoders SHALL accept the upstream Vercel AI SDK
`LanguageModelV4` JSON encodings for request-path payloads, in addition to the
canonical Go-to-Go encodings, so an upstream `@ai-sdk/gateway` client can POST
to the wire endpoint. This tolerance is **decode-only**: the encoders SHALL
continue to emit the canonical Go form defined by
`2026-04-30-lossless-provider-wire` (D6 system content as `[]ContentPart`, split
tool-result output, `{bytes|base64|url}` file data), so Go-to-Go wire bytes are
unchanged.

The decoders SHALL accept:

- a `system` message whose `content` is a JSON **string**, decoding it into a
  single `text` `ContentPart`;
- a tool-result `output` in the upstream single-`value` shape
  (`{"type":"text","value":...}`, `{"type":"json","value":...}`,
  `{"type":"error-text","value":...}`, `{"type":"error-json","value":...}`,
  `{"type":"content","value":[...]}`), mapping `value` into the corresponding
  Go field;
- a file/reasoning-file `data` in the upstream tagged union
  (`{"type":"data","data":<base64>}` or `{"type":"url","url":<url>}`), mapping
  it into `DataContent`.

#### Scenario: System message content as a string decodes
- **WHEN** `wire.DecodeCallOptions` receives a prompt message
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

#### Scenario: Canonical Go-to-Go encodings still decode unchanged
- **WHEN** a `provider.CallOptions` carrying a `NewSystemMessage`, a split
  tool-result `output`, and a `DataContent{URL: ...}` is encoded with
  `wire.EncodeCallOptions` and decoded with `wire.DecodeCallOptions`
- **THEN** the decoded value SHALL equal the original (using `reflect.DeepEqual`)
  and the emitted JSON SHALL be byte-for-byte identical to the pre-change form
  (system `content` as an array, tool-result split fields, `{"url":...}` data)
