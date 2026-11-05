## MODIFIED Requirements

### Requirement: Step content preserves provider order

StreamText SHALL build StepResult.Content and response content from recorded provider sequence. Text/reasoning/sources/regular files/tool calls/tool results/approval requests SHALL retain relative provider order and metadata. StepResult.Text SHALL join each text part's whole text in the order the parts first appeared, as upstream's step `text` does, not in delta arrival order. Locally created post-stream approvals/results SHALL append exactly once. Manually constructed state without recorded content SHALL use deterministic grouped fallback.

#### Scenario: Provider content is interleaved

- **WHEN** a provider emits files, tool calls, text, reasoning, sources, and provider tool results in an interleaved sequence
- **THEN** `StepResult.Content` and response-message content SHALL preserve that sequence
- **AND** each represented part SHALL retain its provider metadata

#### Scenario: Text part deltas interleave

- **WHEN** text parts `t1` and `t2` start in that order and their deltas arrive as `t1` `{"name":`, `t2` `note`, `t1` `"a"}`
- **THEN** `StepResult.Text` SHALL be `{"name":"a"}note`

#### Scenario: Empty recorded text remains step content

- **WHEN** a provider emits a text start and end without a text delta
- **THEN** `StepResult.Content` SHALL retain the empty text part in its recorded position
- **AND** response-message content SHALL omit the empty text part

#### Scenario: Local tool content follows recorded content

- **WHEN** tool approval requests, tool approval responses, or client-executed tool results are created after provider streaming
- **THEN** they SHALL appear exactly once after the recorded provider content

#### Scenario: Step state has no recorded content

- **WHEN** content is built from manually constructed step fields without a recorded provider sequence
- **THEN** reasoning, text, tools, sources, and files SHALL be emitted in that grouped order
