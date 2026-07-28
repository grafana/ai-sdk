## MODIFIED Requirements

### Requirement: FunctionTool struct

`FunctionTool` SHALL have the following fields:
- `Type ToolType` (json `"type"`) -- always `ToolTypeFunction` (`"function"`)
- `Name string` (json `"name"`)
- `Description string` (json `"description,omitempty"`)
- `InputSchema json.RawMessage` (json `"inputSchema,omitempty"`)
- `InputExamples []InputExample` (json `"inputExamples,omitempty"`)
- `Strict bool` (json `"strict,omitempty"`)
- `ProviderOptions map[string]json.RawMessage` (json `"providerOptions,omitempty"`)

`FunctionTool` SHALL implement the `Tool` sealed interface.

#### Scenario: FunctionTool with all fields
- **WHEN** a `FunctionTool` is constructed with Name `"get_weather"`, Description `"Gets weather"`, InputSchema containing a JSON schema, InputExamples with one example, and Strict `true`
- **THEN** it SHALL be a valid `Tool` with Type `ToolTypeFunction` and all fields accessible

#### Scenario: FunctionTool retains ProviderOptions
- **WHEN** a `FunctionTool` is constructed with `ProviderOptions` containing an `"anthropic"` key
- **THEN** the ProviderOptions SHALL be accessible on the struct

#### Scenario: FunctionTool.Type is ToolType not string
- **WHEN** `FunctionTool` is defined in `provider/language_model.go`
- **THEN** its `Type` field SHALL be typed as `ToolType`, not bare `string`

### Requirement: ProviderTool struct

`ProviderTool` SHALL have the following fields:
- `Type ToolType` (json `"type"`) -- always `ToolTypeProvider` (`"provider"`)
- `Name string` (json `"name"`)
- `ID string` (json `"id"`)
- `Args map[string]json.RawMessage` (json `"args,omitempty"`)

`ProviderTool` SHALL NOT have a `ProviderOptions` field. `ProviderTool` SHALL implement the `Tool` sealed interface.

#### Scenario: ProviderTool with ID and Args
- **WHEN** a `ProviderTool` is constructed with Name `"web_search"`, ID `"anthropic.web_search_20250305"`, and Args containing `"maxUses"`
- **THEN** it SHALL be a valid `Tool` with Type `ToolTypeProvider` and all fields accessible

#### Scenario: ProviderTool has no ProviderOptions
- **WHEN** a `ProviderTool` struct is inspected
- **THEN** it SHALL NOT have a `ProviderOptions` field

#### Scenario: ProviderTool.Type is ToolType not string
- **WHEN** `ProviderTool` is defined in `provider/language_model.go`
- **THEN** its `Type` field SHALL be typed as `ToolType`, not bare `string`

### Requirement: Orchestration layer tool conversion

The `toolSetToProviderTools` function SHALL convert `aisdk.Tool` entries into the appropriate `provider.Tool` interface values:
- Tools with Type `""`, `"function"`, or `"dynamic"` SHALL produce `provider.FunctionTool` values with `Type: provider.ToolTypeFunction`
- Tools with Type `"provider"` SHALL produce `provider.ProviderTool` values with `Type: provider.ToolTypeProvider`
- Tools with unrecognized types SHALL produce a warning and be skipped

#### Scenario: Function tool conversion
- **WHEN** `toolSetToProviderTools` receives a tool with Type `""` and Name `"get_weather"`
- **THEN** it SHALL produce a `provider.FunctionTool` with Type `provider.ToolTypeFunction`, the tool's Name, Description, InputSchema, InputExamples (wrapped in InputExample), Strict, and ProviderOptions

#### Scenario: Provider tool conversion
- **WHEN** `toolSetToProviderTools` receives a tool with Type `"provider"` and ID `"anthropic.web_search_20250305"`
- **THEN** it SHALL produce a `provider.ProviderTool` with Type `provider.ToolTypeProvider`, the tool's Name, ID, and Args

#### Scenario: InputExamples wrapping during conversion
- **WHEN** `toolSetToProviderTools` converts a function tool with `InputExamples` containing raw JSON values
- **THEN** each raw JSON value SHALL be wrapped in an `InputExample{Input: <raw>}` in the resulting `FunctionTool.InputExamples`
