## MODIFIED Requirements

### Requirement: Tool is a flat discriminated struct

The `provider` package SHALL define `Tool` as a single flat struct discriminated by a typed `Type` field:

```go
type Tool struct {
    Type            ToolType                   `json:"type"`
    Name            string                     `json:"name"`
    Description     *string                    `json:"description,omitempty"`
    InputSchema     json.RawMessage            `json:"inputSchema,omitempty"`
    InputExamples   []InputExample             `json:"inputExamples,omitempty"`
    Strict          *bool                      `json:"strict,omitempty"`
    ID              string                     `json:"id,omitempty"`
    Args            map[string]json.RawMessage `json:"args,omitempty"`
    ProviderOptions ProviderOptions            `json:"providerOptions,omitempty"`
}
```

The `Type` field SHALL be either `ToolTypeFunction` or `ToolTypeProvider`. `Description` SHALL be used only by the function arm; nil means absent and a non-nil pointer to `""` means explicitly present and empty. The previous sealed `Tool` interface, `tool()` marker method, and concrete `FunctionTool` and `ProviderTool` types SHALL remain removed.

Generic JSON round-trip behavior SHALL remain compatibility behavior and SHALL preserve description absence, explicit empty, and non-empty values. It SHALL NOT define strict protocol validity.

#### Scenario: Tool is a struct
- **WHEN** the `provider.Tool` type is inspected
- **THEN** it SHALL be a Go struct exported as `provider.Tool` with public fields including `Type`, `Name`, and the function/provider variant fields

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** `provider.FunctionTool` and `provider.ProviderTool` SHALL NOT exist as identifiers

#### Scenario: Tool round-trips via encoding/json
- **WHEN** any valid function or provider `Tool` is compatibility-marshaled and unmarshaled
- **THEN** the decoded value SHALL equal the original with no supported field loss

#### Scenario: Function tool compatibility round-trip
- **WHEN** a function-typed `Tool` with a non-empty description is compatibility-marshaled and unmarshaled
- **THEN** the decoded value SHALL equal the original with no field loss

#### Scenario: Description preserves all optional states
- **WHEN** function tools set `Description` to nil, a pointer to `""`, and a pointer to a non-empty value
- **THEN** compatibility encoding and decoding SHALL preserve all three states distinctly

#### Scenario: Strict preserves all optional states
- **WHEN** function-typed tools set `Strict` to nil, a pointer to true, and a pointer to false
- **THEN** compatibility encoding and decoding SHALL preserve absent, true, and false distinctly

#### Scenario: Provider tool round-trips
- **WHEN** a provider-typed `Tool` with `ID` and `Args` is compatibility-marshaled and unmarshaled
- **THEN** the decoded value SHALL equal the original

### Requirement: FunctionTool struct

The `FunctionTool` struct SHALL remain absent as a distinct type. Function-typed tools MUST be constructed as `Tool{Type: ToolTypeFunction, Name: ..., Description: <optional pointer>, InputSchema: ..., InputExamples: ..., Strict: ..., ProviderOptions: ...}`. The same field names live on the unified `Tool` struct; provider-tool-only fields (`ID`, `Args`) SHALL be left unset for function tools.

#### Scenario: Function tool with all fields
- **WHEN** a function-typed `Tool` is constructed with `Type: ToolTypeFunction`, `Name: "get_weather"`, `Description` pointing to `"Gets weather"`, an `InputSchema`, one `InputExamples` entry, and `Strict` pointing to true
- **THEN** the resulting `Tool` SHALL be valid for use as a function tool

#### Scenario: Explicit empty description is valid
- **WHEN** a function-typed `Tool` has `Description` pointing to `""`
- **THEN** the provider request value SHALL remain distinct from a tool with `Description == nil`

#### Scenario: Function tool retains ProviderOptions
- **WHEN** a function-typed `Tool` carries `ProviderOptions` with key `"anthropic"`
- **THEN** the options SHALL compatibility-round-trip and remain readable by `ResolveOption[T]`

#### Scenario: Function tool Type field is ToolType
- **WHEN** a function-typed `Tool` is inspected
- **THEN** its `Type` field SHALL be of type `ToolType`, not bare `string`
