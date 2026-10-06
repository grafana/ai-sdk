## MODIFIED Requirements

### Requirement: Orchestration routes provider-executed tool results inline in assistant message

The `appendToolResults` function SHALL build a `[]provider.ContentPart` from
the step in stream order (reasoning blocks first, then a `text` part if
non-empty, then for each tool call the call followed immediately by its
matching provider-executed `tool-result`, then any remaining
non-provider-executed `tool-result` parts) and SHALL delegate the conversion
of that slice to the public `ToResponseMessages(parts)` helper. The
returned messages SHALL be appended to the input `msgs` slice and returned.
The function SHALL preserve all behavior previously specified —
provider-executed tool results inline in the assistant message,
non-provider-executed tool results in a separate tool message,
`ProviderMetadata` carried through to `ProviderOptions` on every
`tool-call`, `tool-result`, and `reasoning` part, and `ModelOutput` honored
for provider-executed inline results — with the additional guarantee that
`reasoning` parts and their `ProviderOptions` (notably Anthropic's extended-
thinking signature) survive every tool-result round.

When a step contains only provider-executed tool results and no
non-provider-executed tool results, no tool message SHALL be appended.

#### Scenario: Provider-executed tool result goes inline in assistant message

- **WHEN** `appendToolResults` processes a step with a tool call that has
  `ProviderExecuted: true` and a corresponding tool result with
  `ProviderExecuted: true`
- **THEN** the tool result SHALL appear as a `tool-result` `ContentPart`
  inline in the assistant message's `Content` after the `tool-call`
  `ContentPart`
- **AND** no tool message SHALL be appended for that result

#### Scenario: Non-provider-executed tool result goes in tool message

- **WHEN** `appendToolResults` processes a step with a tool call that has
  `ProviderExecuted: false` and a corresponding tool result with
  `ProviderExecuted: false`
- **THEN** the tool result SHALL appear in a separate tool message
- **AND** the tool result SHALL NOT appear inline in the assistant message's
  `Content`

#### Scenario: Mixed provider-executed and non-provider-executed results

- **WHEN** `appendToolResults` processes a step with both provider-executed
  and non-provider-executed tool calls and results
- **THEN** provider-executed results SHALL be inline in the assistant
  message's `Content`
- **AND** non-provider-executed results SHALL be in a separate tool message
- **AND** the assistant message SHALL contain tool calls for both types and
  inline results only for provider-executed tools

#### Scenario: ProviderMetadata carried through to tool-call ContentPart

- **WHEN** `appendToolResults` processes a step with a `ToolCall` that has
  non-nil `ProviderMetadata`
- **THEN** the resulting `tool-call` `ContentPart` SHALL have
  `ProviderOptions` populated from the `ToolCall.ProviderMetadata`

#### Scenario: ProviderMetadata carried through to tool-result ContentPart

- **WHEN** `appendToolResults` processes a step with a `ToolResult` that
  has non-nil `ProviderMetadata` and `ProviderExecuted: true`
- **THEN** the resulting `tool-result` `ContentPart` inline in the assistant
  message SHALL have `ProviderOptions` populated from the
  `ToolResult.ProviderMetadata`

#### Scenario: ModelOutput preserved for provider-executed inline results

- **WHEN** `appendToolResults` processes a provider-executed tool result
  that has a non-nil `ModelOutput`
- **THEN** the `tool-result` `ContentPart`'s `Output` SHALL use the
  `ModelOutput` value instead of constructing output from the raw `Output`
  JSON

#### Scenario: Only provider-executed results produces no tool message

- **WHEN** `appendToolResults` processes a step where ALL tool results have
  `ProviderExecuted: true`
- **THEN** no tool message SHALL be appended to the result

#### Scenario: Reasoning content survives across tool-result rounds

- **WHEN** `appendToolResults` processes a step that has one reasoning
  block in `step.Reasoning` carrying `ProviderMetadata` with an Anthropic
  `signature`, plus a `tool-call` and a `tool-result`
- **THEN** the appended assistant message SHALL contain a `reasoning`
  `ContentPart` whose `ProviderOptions` carries the signature, ordered
  before the `tool-call` part
- **AND** the resulting message list SHALL be suitable as the next-call
  prompt without any reasoning content being dropped
