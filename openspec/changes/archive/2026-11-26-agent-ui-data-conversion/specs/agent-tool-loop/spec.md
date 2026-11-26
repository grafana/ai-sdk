## ADDED Requirements

### Requirement: Agent UI stream helpers support opt-in data conversion

Both Agent UI stream helpers SHALL accept a model data converter as an Agent-specific option separate from UI emission. After validating and normalizing the history, they SHALL delegate data conversion to the existing UI-to-model conversion path and preserve the original normalized history for UI response assembly.

#### Scenario: User and assistant data reaches the model
- **WHEN** either helper receives a converter and valid user and assistant messages with data parts
- **THEN** converted text and file parts SHALL reach the provider in their original order
- **AND** callback mutation of data bytes SHALL NOT affect caller-owned or retained UI history

#### Scenario: No converted result preserves existing behavior
- **WHEN** no converter is supplied, a nil converter is supplied, or the converter returns no content for a part
- **THEN** that data SHALL be omitted from the prompt and kept as data in the UI history

#### Scenario: Conversion failure precedes generation
- **WHEN** a converter fails or returns an unsupported content kind
- **THEN** the helper SHALL return a contextual conversion error before invoking the Agent stream or writing any SSE response
