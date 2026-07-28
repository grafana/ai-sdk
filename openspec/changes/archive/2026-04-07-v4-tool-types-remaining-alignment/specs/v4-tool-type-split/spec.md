## ADDED Requirements

### Requirement: Tool sealed interface

The `provider` package SHALL define a `Tool` interface with an unexported `tool()` marker method. Exactly two concrete types SHALL implement this interface: `FunctionTool` and `ProviderTool`.

`CallOptions.Tools` SHALL be typed as `[]Tool` (interface).

#### Scenario: FunctionTool satisfies Tool interface
- **WHEN** a `FunctionTool` value is used
- **THEN** it SHALL satisfy the `Tool` interface at compile time

#### Scenario: ProviderTool satisfies Tool interface
- **WHEN** a `ProviderTool` value is used
- **THEN** it SHALL satisfy the `Tool` interface at compile time

#### Scenario: Type switch dispatch on Tool
- **WHEN** a consumer iterates over `CallOptions.Tools`
- **THEN** it SHALL be able to use a Go type switch to dispatch on `FunctionTool` and `ProviderTool`

### Requirement: FunctionTool struct

`FunctionTool` SHALL have the following fields:
- `Type string` (json `"type"`) -- always `"function"`
- `Name string` (json `"name"`)
- `Description string` (json `"description,omitempty"`)
- `InputSchema json.RawMessage` (json `"inputSchema,omitempty"`)
- `InputExamples []InputExample` (json `"inputExamples,omitempty"`)
- `Strict bool` (json `"strict,omitempty"`)
- `ProviderOptions map[string]json.RawMessage` (json `"providerOptions,omitempty"`)

`FunctionTool` SHALL implement the `Tool` sealed interface.

#### Scenario: FunctionTool with all fields
- **WHEN** a `FunctionTool` is constructed with Name `"get_weather"`, Description `"Gets weather"`, InputSchema containing a JSON schema, InputExamples with one example, and Strict `true`
- **THEN** it SHALL be a valid `Tool` with Type `"function"` and all fields accessible

#### Scenario: FunctionTool retains ProviderOptions
- **WHEN** a `FunctionTool` is constructed with `ProviderOptions` containing an `"anthropic"` key
- **THEN** the ProviderOptions SHALL be accessible on the struct

### Requirement: ProviderTool struct

`ProviderTool` SHALL have the following fields:
- `Type string` (json `"type"`) -- always `"provider"`
- `Name string` (json `"name"`)
- `ID string` (json `"id"`)
- `Args map[string]json.RawMessage` (json `"args,omitempty"`)

`ProviderTool` SHALL NOT have a `ProviderOptions` field. `ProviderTool` SHALL implement the `Tool` sealed interface.

#### Scenario: ProviderTool with ID and Args
- **WHEN** a `ProviderTool` is constructed with Name `"web_search"`, ID `"anthropic.web_search_20250305"`, and Args containing `"maxUses"`
- **THEN** it SHALL be a valid `Tool` with Type `"provider"` and all fields accessible

#### Scenario: ProviderTool has no ProviderOptions
- **WHEN** a `ProviderTool` struct is inspected
- **THEN** it SHALL NOT have a `ProviderOptions` field

### Requirement: InputExample wrapper type

The `provider` package SHALL define an `InputExample` struct with a single field:
- `Input json.RawMessage` (json `"input"`)

`FunctionTool.InputExamples` SHALL use `[]InputExample` instead of `[]json.RawMessage`.

#### Scenario: InputExample wraps input
- **WHEN** an `InputExample` is marshaled to JSON
- **THEN** the output SHALL be `{"input": <value>}` wrapping the raw JSON input

### Requirement: Orchestration layer tool conversion

The `toolSetToProviderTools` function SHALL convert `aisdk.Tool` entries into the appropriate `provider.Tool` interface values:
- Tools with Type `""`, `"function"`, or `"dynamic"` SHALL produce `provider.FunctionTool` values
- Tools with Type `"provider"` SHALL produce `provider.ProviderTool` values
- Tools with unrecognized types SHALL produce a warning and be skipped

#### Scenario: Function tool conversion
- **WHEN** `toolSetToProviderTools` receives a tool with Type `""` and Name `"get_weather"`
- **THEN** it SHALL produce a `provider.FunctionTool` with Type `"function"`, the tool's Name, Description, InputSchema, InputExamples (wrapped in InputExample), Strict, and ProviderOptions

#### Scenario: Provider tool conversion
- **WHEN** `toolSetToProviderTools` receives a tool with Type `"provider"` and ID `"anthropic.web_search_20250305"`
- **THEN** it SHALL produce a `provider.ProviderTool` with Type `"provider"`, the tool's Name, ID, and Args

#### Scenario: InputExamples wrapping during conversion
- **WHEN** `toolSetToProviderTools` converts a function tool with `InputExamples` containing raw JSON values
- **THEN** each raw JSON value SHALL be wrapped in an `InputExample{Input: <raw>}` in the resulting `FunctionTool.InputExamples`
