# to-response-messages Specification

## Purpose

Define the public `ToResponseMessages` helper that converts collected response content parts into the assistant + tool messages to feed into the next provider call, and the `ResponseMetadata.Messages` field that surfaces the helper's output per step. This mirrors upstream Vercel AI SDK's `toResponseMessages` and `result.response.messages`, preserving reasoning blocks (and provider signatures) across multi-step tool execution so consumers driving their own multi-step loops do not have to reimplement the conversion.

## Requirements
### Requirement: Public ToResponseMessages helper

aisdk SHALL export `func ToResponseMessages(parts []provider.ContentPart) []provider.Message`, converting collected response parts to next-call assistant/tool messages, mirroring packages/ai/src/generate-text/to-response-messages.ts. Conversion SHALL be pure, perform no I/O and return no error. Tool-output normalization SHALL be caller-owned; *provider.ToolResultOutput on tool-result parts SHALL pass through unchanged.

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

### Requirement: Tool output normalization precedes response-message conversion

The Go port SHALL eagerly run Tool.ToModelOutput during execution and store ToolResult.ModelOutput before the helper. This SHALL remain the intentional adaptation of upstream toResponseMessages({content, tools}), which converts lazily in the helper. Public callers constructing raw-output parts MAY call Tool.ToModelOutput directly first.

#### Scenario: Public caller normalizes raw output explicitly
- **WHEN** a caller constructs content from raw tool output requiring conversion
- **THEN** it MAY call Tool.ToModelOutput before ToResponseMessages, which SHALL pass the resulting ToolResultOutput through unchanged.

### Requirement: Reasoning parts carry ProviderOptions to the next call

Each ContentPartTypeReasoning SHALL become assistant reasoning with copied ProviderOptions, preserving provider signatures such as Anthropic extended-thinking signature. ContentPartTypeReasoningFile SHALL become assistant reasoning-file retaining Data, MediaType and ProviderOptions. Relative order of reasoning and other input parts SHALL remain unchanged.

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

### Requirement: Provider-executed tool results stay in the assistant message at their input position

The function SHALL emit each `tool-result` `ContentPart` with
`ProviderExecuted: true` as part of the assistant message at its original
position in the input slice, mirroring upstream's main-loop traversal
where provider-executed tool results fall through to the assistant
content array (rather than being skipped and inlined after the matching
call). When a step contains only provider-executed tool results, no
tool message SHALL be appended.

#### Scenario: Provider-executed result is preserved in input order

- **WHEN** the input is `[tool-call(srv-1, providerExecuted=true),
  tool-result(srv-1, providerExecuted=true)]`
- **THEN** the result SHALL contain exactly one assistant message
- **AND** that message's content SHALL be `[tool-call(srv-1),
  tool-result(srv-1)]` in that order
- **AND** no tool message SHALL be appended

#### Scenario: Provider-executed result keeps interleaved parts in their input position

- **WHEN** the input is `[tool-call(srv-1, providerExecuted=true),
  text("note"), tool-result(srv-1, providerExecuted=true)]`
- **THEN** the assistant message content SHALL be `[tool-call(srv-1),
  text("note"), tool-result(srv-1)]` in that exact order

#### Scenario: Mixed inline + tool-message routing

- **WHEN** the input contains both a provider-executed call+result pair and
  a non-provider-executed call+result pair
- **THEN** the assistant message SHALL contain the two calls plus the
  provider-executed result at their input positions
- **AND** the tool message SHALL contain only the non-provider-executed
  result

#### Scenario: ModelOutput preserved for provider-executed results

- **WHEN** a provider-executed `tool-result` has `Output` populated by
  `toolResultOutput` (e.g. `Output.Type == ToolOutputText` with a custom
  `Text`)
- **THEN** the assistant-message `tool-result` part SHALL carry that exact
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

ContentPartTypeToolApprovalResponse SHALL enter the tool message in input order. Approved false SHALL also append synthetic tool-result with Output.Type ToolOutputExecutionDenied and matching response Reason. Provider-executed responses SHALL enter the tool message but SHALL NOT add a synthetic result for Approved true.

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

aisdk.ResponseMetadata SHALL define Messages []provider.Message tagged json:"-". After every completed StreamText step, step.Response.Messages SHALL hold ToResponseMessages output for the same step content contributed to next-call messages. result.Response() SHALL hold last-step messages; result.Steps()[i].Response SHALL hold per-step messages. Messages SHALL NOT enter any wire format.

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

Orchestration SHALL pass step.Reasoning blocks, a single step.Text part only if nonempty, each step.ToolCalls call immediately followed by matching provider-executed result if any, then remaining non-provider-executed step.ToolResults. This SHALL preserve Anthropic expectations: reasoning precedes supported text/tool-use, and provider results immediately follow originating calls within the same assistant message.

#### Scenario: Reasoning precedes tool calls

- **WHEN** a step has `step.Reasoning` containing one block with a
  signature, `step.Text == ""`, and `step.ToolCalls` containing one entry
- **THEN** the slice passed to `ToResponseMessages` SHALL be `[reasoning,
  tool-call]` in that order

#### Scenario: Empty text is dropped from the input slice

- **WHEN** a step has `step.Text == ""`
- **THEN** the slice passed to `ToResponseMessages` SHALL NOT contain a
  `text` part for that step

#### Scenario: Provider-executed tool-result follows its call

- **WHEN** a step has `step.ToolCalls = [srv-1 (PE), tc-1]` and
  `step.ToolResults = [srv-1 (PE), tc-1]`
- **THEN** the slice passed to `ToResponseMessages` SHALL be
  `[tool-call(srv-1, PE), tool-result(srv-1, PE), tool-call(tc-1),
  tool-result(tc-1)]` in that order

