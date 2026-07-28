## Purpose

Define wire-compatible part-type identifiers for data and tool-invocation parts while preserving existing serialization and all other part types.

## Requirements

### Requirement: DataPart.PartType returns wire-compatible type
`DataPart.PartType()` SHALL return `"data-" + p.DataName`, matching the JSON wire format `type` discriminator used during serialization.

#### Scenario: DataPart with DataName "usage"
- **WHEN** a `DataPart` has `DataName` set to `"usage"`
- **THEN** `PartType()` SHALL return `"data-usage"`

#### Scenario: DataPart round-trip preserves PartType
- **WHEN** a `DataPart` with `DataName` `"usage"` is marshaled to JSON and unmarshaled back
- **THEN** the unmarshaled part's `PartType()` SHALL return `"data-usage"`

#### Scenario: DataPart with empty DataName
- **WHEN** a `DataPart` has an empty `DataName` (zero value)
- **THEN** `PartType()` SHALL return `"data-"`

### Requirement: ToolInvocationPart.PartType returns wire-compatible type
`ToolInvocationPart.PartType()` SHALL return `"tool-" + p.ToolName`, matching the JSON wire format `type` discriminator used during serialization.

#### Scenario: ToolInvocationPart with ToolName "searchWeb"
- **WHEN** a `ToolInvocationPart` has `ToolName` set to `"searchWeb"`
- **THEN** `PartType()` SHALL return `"tool-searchWeb"`

#### Scenario: ToolInvocationPart round-trip preserves PartType
- **WHEN** a `ToolInvocationPart` with `ToolName` `"searchWeb"` is marshaled to JSON and unmarshaled back
- **THEN** the unmarshaled part's `PartType()` SHALL return `"tool-searchWeb"`

#### Scenario: ToolInvocationPart with empty ToolName
- **WHEN** a `ToolInvocationPart` has an empty `ToolName` (zero value)
- **THEN** `PartType()` SHALL return `"tool-"`

### Requirement: Other Part types unchanged
All other `Part` implementations (`TextPart`, `ReasoningPart`, `DynamicToolUIPart`, `FilePart`, `SourceURLPart`, `SourceDocumentPart`, `StepStartPart`) SHALL continue to return their existing fixed `PartType()` values.

#### Scenario: TextPart PartType unchanged
- **WHEN** a `TextPart` is created
- **THEN** `PartType()` SHALL return `"text"`

#### Scenario: ReasoningPart PartType unchanged
- **WHEN** a `ReasoningPart` is created
- **THEN** `PartType()` SHALL return `"reasoning"`

### Requirement: Wire format unchanged
The JSON serialization format for `DataPart` and `ToolInvocationPart` SHALL NOT change. The `type` field in the serialized JSON SHALL continue to be `"data-{DataName}"` and `"tool-{toolName}"` respectively.

#### Scenario: DataPart serialization unchanged
- **WHEN** a `DataPart` with `DataName` `"usage"` is marshaled to JSON
- **THEN** the JSON `type` field SHALL be `"data-usage"` (same as before)

#### Scenario: ToolInvocationPart serialization unchanged
- **WHEN** a `ToolInvocationPart` with `ToolName` `"searchWeb"` is marshaled to JSON
- **THEN** the JSON `type` field SHALL be `"tool-searchWeb"` (same as before)
