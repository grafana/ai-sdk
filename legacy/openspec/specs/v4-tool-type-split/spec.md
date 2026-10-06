# v4-tool-type-split Specification

## Purpose

Define the provider-level tool type system using a flat discriminated `Tool` struct that round-trips losslessly through JSON while preserving V4 function-tool and provider-tool semantics.

## Requirements

### Requirement: Tool is a flat discriminated struct

The `provider` package SHALL define `Tool` as a single flat struct discriminated by a typed `Type` field, mirroring how `provider.StreamPart` and `provider.ContentPart` are modeled:

```go
type Tool struct {
    Type            ToolType                   `json:"type"`
    Name            string                     `json:"name"`
    Description     string                     `json:"description,omitempty"`
    InputSchema     json.RawMessage            `json:"inputSchema,omitempty"`
    InputExamples   []InputExample             `json:"inputExamples,omitempty"`
    Strict          *bool                      `json:"strict,omitempty"`
    ID              string                     `json:"id,omitempty"`
    Args            map[string]json.RawMessage `json:"args,omitempty"`
    ProviderOptions ProviderOptions            `json:"providerOptions,omitempty"`
}
```

The `Type` field SHALL be either `ToolTypeFunction` or `ToolTypeProvider`. The previous sealed `Tool` interface, the `tool()` marker method, and the concrete `FunctionTool` and `ProviderTool` types SHALL be removed.

#### Scenario: Tool is a struct
- **WHEN** the `provider.Tool` type is inspected
- **THEN** it SHALL be a Go struct exported as `provider.Tool` with public fields including `Type`, `Name`, and the function/provider variant fields

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** `provider.FunctionTool` and `provider.ProviderTool` SHALL NOT exist as identifiers

#### Scenario: Tool round-trips via encoding/json
- **WHEN** a function-typed `Tool` is marshaled and unmarshaled
- **THEN** the decoded value SHALL equal the original (using `reflect.DeepEqual`) with no field loss

#### Scenario: Strict preserves all optional states
- **WHEN** function-typed tools set `Strict` to nil, a pointer to true, and a pointer to false
- **THEN** JSON encoding and decoding SHALL preserve absent, true, and false as distinct states

#### Scenario: Provider tool round-trips
- **WHEN** a provider-typed `Tool` with `ID` and `Args` is marshaled and unmarshaled
- **THEN** the decoded value SHALL equal the original

### Requirement: CallOptions.Tools uses the flat Tool struct

`CallOptions.Tools` SHALL be typed as `[]Tool` (the flat struct, not an interface) and SHALL be tagged `json:"tools,omitempty"`.

#### Scenario: Tools field type
- **WHEN** the `CallOptions` struct is inspected
- **THEN** the `Tools` field SHALL be `[]Tool` with json tag `"tools,omitempty"`

#### Scenario: Tools round-trip
- **WHEN** a `CallOptions` with a function tool and a provider tool is marshaled and unmarshaled
- **THEN** both tools SHALL be preserved in order with full field fidelity

### Requirement: Tool sealed interface

The `Tool` sealed interface SHALL no longer exist. `Tool` MUST be a flat struct discriminated by its `Type` field. Type switches over the previous `FunctionTool`/`ProviderTool` concrete types SHALL be replaced with switches on `tool.Type` (`ToolTypeFunction` or `ToolTypeProvider`). `CallOptions.Tools` SHALL be typed as `[]Tool` (the flat struct, not an interface).

#### Scenario: Function tool dispatch via Type
- **WHEN** a consumer iterates over `CallOptions.Tools`
- **THEN** it SHALL dispatch on `tool.Type == ToolTypeFunction` or `tool.Type == ToolTypeProvider` instead of a type switch on concrete types

### Requirement: FunctionTool struct

The `FunctionTool` struct SHALL no longer exist as a distinct type. Function-typed tools MUST be constructed as `Tool{Type: ToolTypeFunction, Name: ..., Description: ..., InputSchema: ..., InputExamples: ..., Strict: ..., ProviderOptions: ...}`. The same field names live on the unified `Tool` struct; provider-tool-only fields (`ID`, `Args`) SHALL be left unset for function tools.

#### Scenario: Function tool with all fields
- **WHEN** a function-typed `Tool` is constructed with `Type: ToolTypeFunction`, `Name: "get_weather"`, `Description: "Gets weather"`, an `InputSchema`, one `InputExamples` entry, and `Strict` pointing to true
- **THEN** the resulting `Tool` SHALL be valid for use as a function tool

#### Scenario: Function tool retains ProviderOptions
- **WHEN** a function-typed `Tool` carries `ProviderOptions` with key `"anthropic"`
- **THEN** the options SHALL round-trip via JSON and be readable by `ResolveOption[T]`

#### Scenario: Function tool Type field is ToolType
- **WHEN** a function-typed `Tool` is inspected
- **THEN** its `Type` field SHALL be of type `ToolType` (typed string), not bare `string`

### Requirement: ProviderTool struct

The `ProviderTool` struct SHALL no longer exist as a distinct type. Provider-typed tools MUST be constructed as `Tool{Type: ToolTypeProvider, Name: ..., ID: ..., Args: ..., ProviderOptions: ...}`. The unified `Tool` struct exposes `ProviderOptions` for both variants; this is a deliberate widening from the previous `ProviderTool`-without-`ProviderOptions` design to keep the schema flat. Producers MAY leave `ProviderOptions` nil for provider tools.

#### Scenario: Provider tool with ID and Args
- **WHEN** a provider-typed `Tool` is constructed with `Type: ToolTypeProvider`, `Name: "web_search"`, `ID: "anthropic.web_search_20250305"`, and `Args` containing `"maxUses"`
- **THEN** the resulting `Tool` SHALL be valid for use as a provider tool

#### Scenario: Provider tool Type field is ToolType
- **WHEN** a provider-typed `Tool` is inspected
- **THEN** its `Type` field SHALL be of type `ToolType` (typed string), not bare `string`

### Requirement: InputExample wrapper type

The `provider` package SHALL continue to define an `InputExample` struct with a single field `Input json.RawMessage` (`json:"input"`). The unified `Tool.InputExamples` (formerly `FunctionTool.InputExamples`) MUST use `[]InputExample`.

#### Scenario: InputExample wraps input
- **WHEN** an `InputExample` is marshaled to JSON
- **THEN** the output SHALL be `{"input": <value>}` wrapping the raw JSON input

### Requirement: Orchestration layer tool conversion

The `toolSetToProviderTools` function SHALL convert `aisdk.Tool` entries into the unified flat `provider.Tool` struct as follows:

- Tools with Type `""`, `"function"`, or `"dynamic"` SHALL produce `provider.Tool{Type: provider.ToolTypeFunction, ...}` carrying Name, Description, InputSchema, InputExamples, Strict, and ProviderOptions.
- Tools with Type `"provider"` SHALL produce `provider.Tool{Type: provider.ToolTypeProvider, ...}` carrying Name, ID, and Args.
- Tools with unrecognized types SHALL produce a warning and be skipped.

#### Scenario: Function tool conversion
- **WHEN** `toolSetToProviderTools` receives a tool with Type `""` and Name `"get_weather"`
- **THEN** it SHALL produce a `provider.Tool` with `Type: ToolTypeFunction`, the tool's Name, Description, InputSchema, InputExamples (wrapped in InputExample), Strict, and ProviderOptions

#### Scenario: Provider tool conversion
- **WHEN** `toolSetToProviderTools` receives a tool with Type `"provider"` and ID `"anthropic.web_search_20250305"`
- **THEN** it SHALL produce a `provider.Tool` with `Type: ToolTypeProvider`, the tool's Name, ID, and Args

#### Scenario: InputExamples wrapping during conversion
- **WHEN** `toolSetToProviderTools` converts a function tool with `InputExamples` containing raw JSON values
- **THEN** each raw JSON value SHALL be wrapped in an `InputExample{Input: <raw>}` in the resulting `Tool.InputExamples`
