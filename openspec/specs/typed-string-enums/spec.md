## Purpose

Typed string enum definitions for all discriminator fields across `provider` and `aisdk` packages, providing compile-time safety without affecting JSON wire compatibility.

## Requirements

### Requirement: ToolChoiceType typed string enum

The `provider` package SHALL define a `ToolChoiceType` typed string with constants: `ToolChoiceAuto` ("auto"), `ToolChoiceNone` ("none"), `ToolChoiceRequired` ("required"), `ToolChoiceTool` ("tool"). The `ToolChoice.Type` field SHALL be typed as `ToolChoiceType`.

#### Scenario: ToolChoice uses typed constant
- **WHEN** a caller constructs a `ToolChoice`
- **THEN** the `Type` field SHALL accept only `ToolChoiceType` values, not bare strings

#### Scenario: ToolChoiceType JSON round-trip
- **WHEN** a `ToolChoice{Type: ToolChoiceAuto}` is marshaled to JSON
- **THEN** it SHALL produce `{"type":"auto"}`, identical to the previous bare string behavior

### Requirement: WarningType typed string enum

The `provider` package SHALL define a `WarningType` typed string. The existing constants `WarnUnsupported`, `WarnCompatibility`, `WarnOther` SHALL become typed as `WarningType`. The `Warning.Type` field SHALL be typed as `WarningType`.

#### Scenario: Warning uses typed constant
- **WHEN** a caller constructs a `Warning{Type: WarnUnsupported, Feature: "logprobs"}`
- **THEN** the code SHALL compile without change because the constant names are preserved

#### Scenario: WarningType constants are typed
- **WHEN** code attempts `var w WarningType = "arbitrary"`
- **THEN** it SHALL fail to compile because bare strings are not assignable to `WarningType`

### Requirement: ResponseFormatType typed string enum

The `provider` package SHALL define a `ResponseFormatType` typed string with constants: `ResponseFormatText` ("text"), `ResponseFormatJSON` ("json"). The `ResponseFormat.Type` field SHALL be typed as `ResponseFormatType`.

#### Scenario: Output package uses typed constant
- **WHEN** the `output.Text()` function constructs a `ResponseFormat`
- **THEN** it SHALL use `ResponseFormatText` instead of the bare string `"text"`

#### Scenario: ResponseFormatType JSON round-trip
- **WHEN** a `ResponseFormat{Type: ResponseFormatJSON}` is marshaled to JSON
- **THEN** it SHALL produce `{"type":"json"}`, identical to the previous bare string behavior

### Requirement: StepType typed string enum

The `aisdk` package SHALL define a `StepType` typed string with constants: `StepTypeInitial` ("initial"), `StepTypeToolResult` ("tool-result"). The `StepResult.StepType` field SHALL be typed as `StepType`.

#### Scenario: StreamText assigns typed StepType
- **WHEN** the orchestration loop creates a new step
- **THEN** it SHALL assign `StepTypeInitial` for the first call and `StepTypeToolResult` for subsequent tool-result steps

#### Scenario: StepType test assertions
- **WHEN** a test asserts on `StepResult.StepType`
- **THEN** it SHALL compare against `StepTypeInitial` or `StepTypeToolResult` constants

### Requirement: ToolInvocationState typed string enum

The `aisdk` package SHALL define a `ToolInvocationState` typed string with constants: `ToolStateInputStreaming` ("input-streaming"), `ToolStateInputAvailable` ("input-available"), `ToolStateApprovalRequested` ("approval-requested"), `ToolStateApprovalResponded` ("approval-responded"), `ToolStateOutputAvailable` ("output-available"), `ToolStateOutputError` ("output-error"), `ToolStateOutputDenied` ("output-denied"). The `ToolInvocationPart.State` and `DynamicToolUIPart.State` fields SHALL be typed as `ToolInvocationState`. The `ChunkToolInputError` chunk type SHALL map to `ToolStateOutputError` on the part, matching upstream behavior where `tool-input-error` chunks produce `output-error` state.

#### Scenario: Stream assigns typed state
- **WHEN** the stream processor creates a `ToolInvocationPart` with a completed tool call
- **THEN** it SHALL assign `ToolStateInputAvailable` as the State

#### Scenario: Convert checks typed state
- **WHEN** `isToolInvocationComplete` checks a part's state
- **THEN** it SHALL compare against `ToolStateOutputAvailable`, `ToolStateOutputError`, and `ToolStateOutputDenied` constants

#### Scenario: ToolInvocationState JSON round-trip
- **WHEN** a `ToolInvocationPart{State: ToolStateOutputAvailable}` is marshaled to JSON
- **THEN** it SHALL produce `"state":"output-available"`, identical to the previous bare string behavior

### Requirement: SourceType typed string enum

The `provider` package SHALL define a `SourceType` typed string with constants: `SourceTypeURL` ("url"), `SourceTypeDocument` ("document"). The `SourceInfo.SourceType` field (in `provider/stream_part.go`), `GenerateContentPart.SourceType` field (in `provider/language_model.go`), and `Source.SourceType` field (in `types.go`) SHALL all be typed as `provider.SourceType`.

#### Scenario: Anthropic sets typed source type
- **WHEN** the Anthropic provider creates a source part for a web search citation
- **THEN** it SHALL assign `provider.SourceTypeURL` instead of the bare string `"url"`

#### Scenario: SourceType comparison in streamtext
- **WHEN** the orchestration checks `src.SourceType == "url"` to build a SourceURLPart
- **THEN** it SHALL compare against `provider.SourceTypeURL`

### Requirement: ToolResultContentType typed string enum

The `provider` package SHALL define a `ToolResultContentType` typed string with canonical constants: `ToolContentText` ("text"), `ToolContentFile` ("file"), and `ToolContentCustom` ("custom"). The `ToolResultContentValue.Type` field SHALL be typed as `ToolResultContentType`, and file values SHALL carry `Data *DataContent` using the LanguageModelV4 tagged data union.

The legacy constants `ToolContentFileData` ("file-data"), `ToolContentFileURL` ("file-url"), and `ToolContentFileReference` ("file-reference") SHALL remain available to recognize legacy wire input. Decoding SHALL normalize them to `ToolContentFile`, and marshaling SHALL emit `"file"`.

#### Scenario: ToolResultContentValue uses typed constant
- **WHEN** a test constructs a canonical file content value
- **THEN** it SHALL use `ToolContentFile` instead of the bare string `"file"`

#### Scenario: legacy discriminator normalization
- **WHEN** a legacy `"file-data"`, `"file-url"`, or `"file-reference"` value is decoded
- **THEN** its `Type` SHALL normalize to `ToolContentFile`

### Requirement: GenerateContentType typed string enum

The `provider` package SHALL define a `GenerateContentType` typed string with constants: `ContentText` ("text"), `ContentReasoning` ("reasoning"), `ContentToolCall` ("tool-call"), `ContentToolResult` ("tool-result"), `ContentSource` ("source"), `ContentFile` ("file"), `ContentReasoningFile` ("reasoning-file"), `ContentCustom` ("custom"), `ContentToolApprovalRequest` ("tool-approval-request"). The `GenerateContentPart.Type` field SHALL be typed as `GenerateContentType`.

#### Scenario: Anthropic constructs typed content parts
- **WHEN** the Anthropic provider creates a `GenerateContentPart` for a text block
- **THEN** it SHALL use `ContentText` instead of the bare string `"text"`

#### Scenario: Middleware switches on typed content type
- **WHEN** the simulate-streaming middleware switches on `part.Type`
- **THEN** the case values SHALL use `ContentText`, `ContentReasoning`, etc.

### Requirement: ReasoningEffort typed string enum

The `provider` package SHALL define a `ReasoningEffort` typed string with constants `ReasoningProviderDefault`, `ReasoningNone` (`"none"`), `ReasoningMinimal` (`"minimal"`), `ReasoningLow` (`"low"`), `ReasoningMedium` (`"medium"`), `ReasoningHigh` (`"high"`), and `ReasoningXHigh` (`"xhigh"`). `ReasoningProviderDefault` SHALL have the empty-string value so it is the Go zero value. The `CallOptions.Reasoning` field SHALL be typed as `ReasoningEffort`, and the existing constant names SHALL be preserved.

#### Scenario: Reasoning field uses typed value
- **WHEN** a caller sets `CallOptions.Reasoning`
- **THEN** it SHALL assign a `ReasoningEffort` value, for example `opts.Reasoning = ReasoningHigh`

#### Scenario: Anthropic reasoning maps use typed keys
- **WHEN** the Anthropic provider's reasoning maps are defined
- **THEN** their key type SHALL be `ReasoningEffort`, not bare `string`

#### Scenario: Explicit reasoning JSON round-trip
- **WHEN** `CallOptions{Reasoning: ReasoningHigh}` is marshaled to JSON
- **THEN** it SHALL produce `"reasoning":"high"`

#### Scenario: Provider-default is omitted from provider-domain JSON
- **WHEN** a zero-valued `CallOptions` is marshaled to JSON
- **THEN** its `reasoning` member SHALL be omitted
- **AND** a strict wire adapter SHALL remain responsible for normalizing explicit wire `"provider-default"` to that zero value
