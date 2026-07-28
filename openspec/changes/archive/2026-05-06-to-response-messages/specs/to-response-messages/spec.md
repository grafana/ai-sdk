## ADDED Requirements

### Requirement: Public ToResponseMessages helper

The `aisdk` package SHALL export a `ToResponseMessages` function with the
signature:

```go
func ToResponseMessages(parts []provider.ContentPart) []provider.Message
```

The function SHALL convert a slice of collected response content parts into
the assistant + tool messages that should be fed into the next provider
call. It SHALL mirror the behavior of upstream
`packages/ai/src/generate-text/to-response-messages.ts`. The function SHALL
NOT return an error: the conversion is pure (no I/O), and tool-output
normalization is the caller's responsibility — `*provider.ToolResultOutput`
values on `tool-result` parts are passed through unchanged. The Go port
runs `Tool.ToModelOutput` eagerly during tool execution and stores the
result on `ToolResult.ModelOutput`, so by the time content reaches this
helper the per-tool conversion is already done. (This intentionally
diverges from upstream's `toResponseMessages({content, tools})` signature,
which performs the same conversion lazily inside the helper.) Public
callers constructing parts from raw tool output can call
`Tool.ToModelOutput` directly before passing parts to this helper.

#### Scenario: Empty input produces empty result

- **WHEN** `ToResponseMessages` is called with an empty `parts` slice
- **THEN** it SHALL return an empty (or `nil`) `[]provider.Message`

#### Scenario: Text-only input produces a single assistant message

- **WHEN** `ToResponseMessages` is called with one `provider.ContentPart`
  of type `text` and non-empty `Text`
- **THEN** it SHALL return one `provider.Message` with `Role == RoleAssistant`
  containing one `text` `ContentPart`

#### Scenario: Empty text parts are dropped

- **WHEN** `ToResponseMessages` is called with one `text` part whose `Text`
  is the empty string
- **THEN** that part SHALL be omitted from the assistant message

### Requirement: Reasoning parts carry ProviderOptions to the next call

The function SHALL convert each `ContentPartTypeReasoning` entry into an
assistant `reasoning` `ContentPart`, copying its `ProviderOptions` (so
provider-specific signatures such as Anthropic's extended-thinking
`signature` survive). The function SHALL convert each
`ContentPartTypeReasoningFile` entry into an assistant `reasoning-file`
`ContentPart` preserving `Data`, `MediaType`, and `ProviderOptions`. The
function SHALL preserve the relative order of reasoning parts and other
parts as given in the input.

#### Scenario: Reasoning with provider signature is preserved

- **WHEN** `ToResponseMessages` is called with a `reasoning` part whose
  `ProviderOptions` contains an entry under key `"anthropic"` carrying a
  `signature` field
- **THEN** the resulting assistant message SHALL contain a `reasoning`
  `ContentPart` whose `ProviderOptions` is equal to the input's

#### Scenario: Multiple reasoning blocks preserve order

- **WHEN** `ToResponseMessages` receives `[reasoning(redacted),
  reasoning(thinking), text(final)]`
- **THEN** the assistant message content SHALL contain those parts in the
  same order

#### Scenario: Reasoning-file parts pass through

- **WHEN** `ToResponseMessages` receives a `reasoning-file` part with `Data`,
  `MediaType`, and `ProviderOptions`
- **THEN** the assistant message SHALL contain a `reasoning-file`
  `ContentPart` with all three fields preserved

### Requirement: Tool calls become assistant tool-call parts

The function SHALL convert each `ContentPartTypeToolCall` entry into an
assistant `tool-call` `ContentPart`, copying `ToolCallID`, `ToolName`,
`Input`, `ProviderExecuted`, and `ProviderOptions`. The function SHALL
sanitize a non-object `Input` (one that does not begin with `{` after
whitespace) by replacing it with `{}`, matching upstream's
`invalid && typeof input !== 'object'` collapse.

#### Scenario: Tool call with valid object input is preserved verbatim

- **WHEN** the input is `{"q":"x"}`
- **THEN** the resulting `tool-call` part's `Input` SHALL equal `{"q":"x"}`

#### Scenario: Tool call with non-object input is sanitized to {}

- **WHEN** the input is `"raw string, not an object"`
- **THEN** the resulting `tool-call` part's `Input` SHALL equal `{}`

#### Scenario: Tool call ProviderOptions and ProviderExecuted carry through

- **WHEN** the input `tool-call` part has non-nil `ProviderOptions` and
  `ProviderExecuted: true`
- **THEN** both fields SHALL appear unchanged on the resulting `tool-call`
  part

### Requirement: Provider-executed tool results are inlined in the assistant message

The function SHALL place each `tool-result` `ContentPart` whose
`ToolCallID` matches a `tool-call` part with `ProviderExecuted: true` inline
in the assistant message, immediately after the matching call (matching
upstream's "provider-executed tool results stay in the assistant message"
behavior). When a step contains only provider-executed tool results, no
tool message SHALL be appended.

#### Scenario: Provider-executed result is inlined

- **WHEN** the input is `[tool-call(srv-1, providerExecuted=true),
  tool-result(srv-1, providerExecuted=true)]`
- **THEN** the result SHALL contain exactly one assistant message
- **AND** that message's content SHALL be `[tool-call(srv-1),
  tool-result(srv-1)]` in that order
- **AND** no tool message SHALL be appended

#### Scenario: Mixed inline + tool-message routing

- **WHEN** the input contains both a provider-executed call+result pair and
  a non-provider-executed call+result pair
- **THEN** the assistant message SHALL contain the two calls plus the
  inline provider-executed result
- **AND** the tool message SHALL contain only the non-provider-executed
  result

#### Scenario: ModelOutput preserved for provider-executed inline results

- **WHEN** a provider-executed `tool-result` has `Output` populated by
  `toolResultOutput` (e.g. `Output.Type == ToolOutputText` with a custom
  `Text`)
- **THEN** the inlined `tool-result` part SHALL carry that exact
  `ToolResultOutput` value

### Requirement: Non-provider-executed tool results go in a separate tool message

The function SHALL emit a single tool message containing every
non-provider-executed `tool-result` `ContentPart` from the input, in the
same order they appear. The tool message SHALL NOT contain provider-executed
tool results. The tool message SHALL be omitted entirely if no
non-provider-executed tool results (and no `tool-approval-response` parts)
are present.

#### Scenario: Single non-provider-executed result emits a tool message

- **WHEN** the input is `[tool-call(tc-1), tool-result(tc-1)]` with both
  having `ProviderExecuted == false`
- **THEN** the result SHALL contain two messages: an assistant message
  with `[tool-call(tc-1)]` and a tool message with `[tool-result(tc-1)]`

#### Scenario: ProviderOptions on tool result carry through

- **WHEN** a non-provider-executed `tool-result` has non-nil
  `ProviderOptions`
- **THEN** the resulting tool-message `tool-result` part SHALL carry the
  same `ProviderOptions`

### Requirement: Tool approval response routing

The function SHALL place each `ContentPartTypeToolApprovalResponse` entry in
the tool message in the same order it appears. When a `tool-approval-response`
has `Approved == false`, the function SHALL also append a synthetic
`tool-result` part to the tool message whose `Output.Type` is
`ToolOutputExecutionDenied` and whose `Reason` matches the approval
response's `Reason`. Provider-executed approval responses SHALL be routed to
the tool message but no synthetic tool-result is added when `Approved == true`.

#### Scenario: Denied approval adds an execution-denied tool result

- **WHEN** the input contains a `tool-approval-response` with
  `Approved == false` and `Reason == "user denied"`
- **THEN** the tool message SHALL contain that approval response followed
  by a `tool-result` part with `Output.Type == ToolOutputExecutionDenied`
  and `Output.Reason == "user denied"`

#### Scenario: Approved provider-executed approval response routes to tool message

- **WHEN** the input contains a `tool-approval-response` with
  `Approved == true` and `ProviderExecuted == true`
- **THEN** the tool message SHALL contain the approval response
- **AND** no synthetic `tool-result` part SHALL be appended for it

### Requirement: File and custom parts pass through

The function SHALL convert each `ContentPartTypeFile` entry to an assistant
`file` `ContentPart` preserving `Data`, `MediaType`, `Filename`, and
`ProviderOptions`. The function SHALL convert each `ContentPartTypeCustom`
entry to an assistant `custom` `ContentPart` preserving `Kind` and
`ProviderOptions`.

#### Scenario: File part is appended to the assistant message

- **WHEN** the input contains a `file` part with `Data`, `MediaType`, and
  `Filename`
- **THEN** the assistant message SHALL contain a `file` `ContentPart`
  with those three fields preserved

#### Scenario: Custom part is appended to the assistant message

- **WHEN** the input contains a `custom` part with `Kind ==
  "openai.compaction"` and `ProviderOptions` set
- **THEN** the assistant message SHALL contain a `custom` `ContentPart`
  with both fields preserved

### Requirement: ToResponseMessages output is exposed on per-step Response

The `aisdk.ResponseMetadata` struct SHALL define a `Messages
[]provider.Message` field tagged `json:"-"`. After every step in
`StreamText` completes, the orchestration layer SHALL populate
`step.Response.Messages` with the slice returned by `ToResponseMessages`
applied to that step's content (using the same content the step contributes
to the next-call message list). The `Messages` field SHALL hold the
last-step messages on `result.Response()` and the per-step messages on
`result.Steps()[i].Response`. The field SHALL NOT be serialized as part of
any wire format.

#### Scenario: Last step's response.messages is populated

- **WHEN** a multi-step `StreamText` run completes
- **THEN** `result.Response().Messages` SHALL be non-nil and equal to
  `result.Steps()[len-1].Response.Messages`

#### Scenario: Per-step response.messages is populated

- **WHEN** a multi-step `StreamText` run completes with N steps
- **THEN** for every `i` in `[0, N)`, `result.Steps()[i].Response.Messages`
  SHALL equal `ToResponseMessages` applied to that step's content

#### Scenario: Messages is not part of the wire format

- **WHEN** `result.Steps()[i].Response` is marshaled to JSON
- **THEN** the resulting JSON SHALL NOT contain a `messages` key

### Requirement: Stream-order content forwarded to ToResponseMessages

The orchestration layer SHALL pass parts to `ToResponseMessages` in the
order: reasoning blocks (from `step.Reasoning`), then a single text part
(from `step.Text`, omitted if empty), then tool calls (from
`step.ToolCalls`), then tool results (from `step.ToolResults`). This order
SHALL preserve provider expectations such as Anthropic's requirement that
reasoning blocks precede the text or tool-use they support.

#### Scenario: Reasoning precedes tool calls

- **WHEN** a step has `step.Reasoning` containing one block with a
  signature, `step.Text == ""`, and `step.ToolCalls` containing one entry
- **THEN** the slice passed to `ToResponseMessages` SHALL be `[reasoning,
  tool-call]` in that order

#### Scenario: Empty text is dropped from the input slice

- **WHEN** a step has `step.Text == ""`
- **THEN** the slice passed to `ToResponseMessages` SHALL NOT contain a
  `text` part for that step
