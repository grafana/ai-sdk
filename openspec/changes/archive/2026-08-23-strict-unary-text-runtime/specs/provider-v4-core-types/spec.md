## MODIFIED Requirements

### Requirement: FinishReason as struct with Unified and Raw
The `FinishReason` type SHALL be a struct with two fields: `Unified` (type `UnifiedFinishReason`) and `Raw` (type `string`). The `UnifiedFinishReason` type SHALL be a `string` type alias with constants: `stop`, `length`, `content-filter`, `tool-calls`, `error`, `other`.

#### Scenario: FinishReason struct shape
- **WHEN** a provider returns a finish reason
- **THEN** it SHALL construct `FinishReason{Unified: <mapped-value>, Raw: <provider-native-string>}`

#### Scenario: FinishReason constants
- **WHEN** code references finish reason constants
- **THEN** it SHALL use `UnifiedFinishReason` typed constants: `FinishReasonStop`, `FinishReasonLength`, `FinishReasonContentFilter`, `FinishReasonToolCalls`, `FinishReasonError`, `FinishReasonOther`

#### Scenario: FinishReason JSON serialization
- **WHEN** a `FinishReason` is serialized to JSON
- **THEN** it SHALL produce `{"unified":"stop","raw":"end_turn"}` with the raw field omitted when empty

#### Scenario: ToolChoice field uses ToolChoiceType
- **WHEN** `ToolChoice` is defined in `provider/types.go`
- **THEN** its `Type` field SHALL be typed as `ToolChoiceType`, not bare `string`

#### Scenario: Warning field uses WarningType
- **WHEN** `Warning` is defined in `provider/types.go`
- **THEN** its `Type` field SHALL be typed as `WarningType`, not bare `string`

#### Scenario: ToolResultContentValue field uses ToolResultContentType
- **WHEN** `ToolResultContentValue` is defined in `provider/types.go`
- **THEN** its `Type` field SHALL be typed as `ToolResultContentType`, not bare `string`

#### Scenario: ReasoningEffort typed constants
- **WHEN** reasoning effort constants are defined in `provider/types.go`
- **THEN** they SHALL be typed as `ReasoningEffort`, not untyped `string`

#### Scenario: CallOptions.Reasoning uses ReasoningEffort value
- **WHEN** `CallOptions` is defined in `provider/language_model.go`
- **THEN** its `Reasoning` field SHALL be `ReasoningEffort`, not a pointer or bare string
- **AND** the zero value SHALL mean provider default

#### Scenario: ResponseFormat field uses ResponseFormatType
- **WHEN** `ResponseFormat` is defined in `provider/language_model.go`
- **THEN** its `Type` field SHALL be typed as `ResponseFormatType`, not bare `string`

#### Scenario: GenerateContentPart field uses GenerateContentType
- **WHEN** `GenerateContentPart` is defined in `provider/language_model.go`
- **THEN** its `Type` field SHALL be typed as `GenerateContentType`, not bare `string`

#### Scenario: GenerateContentPart.SourceType uses SourceType
- **WHEN** `GenerateContentPart` is defined in `provider/language_model.go`
- **THEN** its `SourceType` field SHALL be typed as `SourceType`, not bare `string`
