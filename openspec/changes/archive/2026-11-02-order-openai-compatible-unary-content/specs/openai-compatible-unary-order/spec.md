## ADDED Requirements

### Requirement: Unary reasoning precedes text and tools

For supported OpenAI-compatible unary responses, the adapter SHALL append
nonempty selected reasoning before nonempty text, followed by tool calls in
provider order. Ordering SHALL preserve content values and tool metadata.

#### Scenario: Reasoning and text

- **WHEN** a unary response contains reasoning and text
- **THEN** the generated content contains reasoning followed by text
- **AND** consumers that preserve unary content order retain that order in assistant response messages

#### Scenario: Reasoning, text and tools

- **WHEN** a unary response contains reasoning, text and tool calls
- **THEN** generated content and assistant messages derived from it place reasoning before text and the tool calls
- **AND** tool IDs, names, arguments and provider metadata are preserved

#### Scenario: Optional content is empty

- **WHEN** the selected reasoning or text is empty
- **THEN** that content is omitted without changing the relative order or values of the remaining parts
