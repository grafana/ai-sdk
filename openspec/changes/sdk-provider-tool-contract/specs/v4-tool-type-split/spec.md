## MODIFIED Requirements

### Requirement: ProviderTool struct
The `ProviderTool` struct SHALL no longer exist as a distinct type. Provider-typed tools MUST be constructed as `Tool{Type: ToolTypeProvider, Name: ..., ID: ..., Args: ...}`. Function-only fields Description, InputSchema, InputExamples, Strict and ProviderOptions SHALL remain unset on this variant. The flat struct SHALL retain ProviderOptions for function tools only; direct entry points SHALL reject representably populated incompatible fields before backend I/O. Private wire validation SHALL also reject explicitly empty forbidden members that the Go flat struct cannot distinguish from absence. The registered HTTP projection SHALL require a JSON object for provider args, including `{}`. For direct Go calls, a nil `Args` map SHALL normalize to `{}` at request conversion; this is a Go adaptation to the default empty args supplied by registered upstream provider-tool factories, not a relaxation of HTTP validation. Non-nil argument maps SHALL contain valid JSON values.

#### Scenario: Provider tool with ID and Args
- **WHEN** a provider-typed `Tool` is constructed with `Type: ToolTypeProvider`, `Name: "web_search"`, `ID: "anthropic.web_search_20250305"`, and `Args` containing `"maxUses"`
- **THEN** the resulting `Tool` SHALL be valid for use as a provider tool

#### Scenario: Provider tool Type field is ToolType
- **WHEN** a provider-typed `Tool` is inspected
- **THEN** its `Type` field SHALL be of type `ToolType` (typed string), not bare `string`

#### Scenario: Provider options are not a provider-definition field
- **WHEN** a provider-typed tool carries non-nil ProviderOptions, even an empty map
- **THEN** direct validation SHALL reject it rather than forwarding or silently dropping the options

#### Scenario: Empty args is selected
- **WHEN** a provider tool carries an explicitly empty Args object
- **THEN** registered request projection SHALL retain `args: {}`

#### Scenario: Nil Go args are a default empty object
- **WHEN** a direct Go caller omits `Args` for a provider tool that needs no configuration
- **THEN** native conversion and the Go Gateway client SHALL treat the args as `{}` without altering strict HTTP acceptance of absent or null `args`

#### Scenario: Malformed Go args are rejected
- **WHEN** a non-nil Go `Args` map contains empty or invalid `json.RawMessage` values
- **THEN** direct validation SHALL reject the tool before native I/O
