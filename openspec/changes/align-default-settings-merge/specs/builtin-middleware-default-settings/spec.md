## MODIFIED Requirements

### Requirement: Apply default call options

`DefaultSettings` SHALL accept a settings struct and return a `Middleware` whose `TransformParams` merges defaults into `provider.CallOptions`. Caller values SHALL take precedence. A pointer or slice default SHALL fill only a nil caller field; a non-nil empty slice SHALL be kept. Supported fields: `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `StopSequences`, `ResponseFormat`, `Seed`, `Reasoning`, `Headers`, `ProviderOptions`, `Tools`, `ToolChoice`.

#### Scenario: Default temperature applied
- **WHEN** `DefaultSettings` is configured with `Temperature: ptr(0.7)`
- **AND** the caller does not set `Temperature` in `CallOptions`
- **THEN** `CallOptions.Temperature` SHALL be `0.7` when it reaches the model

#### Scenario: Caller value takes precedence
- **WHEN** `DefaultSettings` is configured with `Temperature: ptr(0.7)`
- **AND** the caller sets `Temperature: ptr(0.3)` in `CallOptions`
- **THEN** `CallOptions.Temperature` SHALL be `0.3` when it reaches the model

#### Scenario: Explicit zero is kept
- **WHEN** `DefaultSettings` is configured with `Temperature: ptr(0.7)`
- **AND** the caller sets `Temperature: ptr(0.0)`
- **THEN** `CallOptions.Temperature` SHALL be `0` when it reaches the model

#### Scenario: Multiple default fields
- **WHEN** `DefaultSettings` is configured with `Temperature`, `MaxOutputTokens`, and `StopSequences`
- **AND** the caller only sets `Temperature`
- **THEN** `MaxOutputTokens` and `StopSequences` SHALL use the defaults
- **AND** `Temperature` SHALL use the caller's value

#### Scenario: Empty caller slices are kept
- **WHEN** `DefaultSettings` is configured with default `StopSequences` and `Tools`
- **AND** the caller sets `StopSequences` and `Tools` to non-nil empty slices
- **THEN** the model SHALL receive empty `StopSequences` and empty `Tools`

#### Scenario: ProviderOptions merge
- **WHEN** `DefaultSettings` is configured with `ProviderOptions` `{"anthropic": {"a": 1}}`
- **AND** the caller provides `ProviderOptions` `{"anthropic": {"b": 2}, "openai": {"c": 3}}`
- **THEN** the model SHALL receive `{"anthropic": {"a": 1, "b": 2}, "openai": {"c": 3}}`

#### Scenario: Headers merge
- **WHEN** `DefaultSettings` is configured with default `Headers`
- **AND** the caller provides additional `Headers`
- **THEN** caller headers SHALL override default headers for matching keys
- **AND** non-overlapping default headers SHALL be preserved

## ADDED Requirements

### Requirement: Deep-merge default provider options

When both the defaults and the caller supply `ProviderOptions`, they SHALL merge recursively by JSON representation: objects under the same key SHALL merge key by key, and for any other value the caller's SHALL replace the default's, including arrays and JSON `null`. Caller keys `__proto__`, `constructor` and `prototype` SHALL be ignored while merging. When only one side supplies provider options, its value SHALL pass through unchanged.

#### Scenario: Nested provider options merge
- **WHEN** the default `anthropic` namespace is `{"tools": {"retrieval": {"enabled": true}, "math": {"enabled": true}}}`
- **AND** the caller's is `{"tools": {"retrieval": {"enabled": false}, "code": {"enabled": true}}}`
- **THEN** the model SHALL receive `{"tools": {"retrieval": {"enabled": false}, "math": {"enabled": true}, "code": {"enabled": true}}}`

#### Scenario: Arrays and null replace
- **WHEN** a default namespace member is an array or object and the caller's same member is a different array or JSON `null`
- **THEN** the model SHALL receive the caller's array or `null` for that member

#### Scenario: Empty provider options on one side
- **WHEN** either side supplies empty `ProviderOptions` and the other supplies a namespace
- **THEN** the model SHALL receive that namespace
- **WHEN** neither side supplies provider options
- **THEN** the model SHALL receive nil `ProviderOptions`

#### Scenario: Prototype keys are ignored
- **WHEN** the caller's namespace contains `__proto__`, `constructor` or `prototype` keys alongside ordinary keys and both sides supply provider options
- **THEN** the merged namespace SHALL contain the default keys and the caller's ordinary keys only

#### Scenario: Typed options remain resolvable
- **WHEN** the default is a typed provider option and the caller supplies raw JSON for the same namespace
- **THEN** the provider SHALL resolve the merged namespace with `provider.ResolveOption` into its typed options containing both sides' values

#### Scenario: Inputs are not mutated
- **WHEN** a merge combines default and caller provider options
- **THEN** the settings and the caller's `CallOptions` values SHALL be unchanged after the call

### Requirement: Merge default response format by type

A default `ResponseFormat` SHALL apply when the caller's is nil. When both are set, the caller's type SHALL win, or the default's if the caller's is empty. For a JSON result, an unset caller schema, name or description SHALL take the default's. A text result SHALL carry no schema, name or description.

#### Scenario: JSON caller inherits default schema
- **WHEN** the default is JSON with a schema and name
- **AND** the caller's is JSON with no schema or name
- **THEN** the model SHALL receive JSON with the default schema and name

#### Scenario: Caller schema wins
- **WHEN** both the default and the caller supply JSON formats with schemas
- **THEN** the model SHALL receive the caller's schema

#### Scenario: Text caller over JSON default
- **WHEN** the default is JSON with a schema, name and description
- **AND** the caller's is text
- **THEN** the model SHALL receive a text format with no schema, name or description

### Requirement: Replace default tool choice

A default `ToolChoice` SHALL apply only when the caller's is nil. A caller `ToolChoice` SHALL replace the default whole, so a non-named caller choice SHALL carry no tool name.

#### Scenario: Automatic caller choice over named default
- **WHEN** the default `ToolChoice` is a named tool choice
- **AND** the caller's is automatic
- **THEN** the model SHALL receive an automatic choice with no tool name

### Requirement: Default reasoning effort

`Reasoning` SHALL be a Go addition to the upstream settings. A default reasoning effort SHALL apply when the caller's `Reasoning` is the zero `ReasoningProviderDefault`, which also represents an explicit provider-default request.

#### Scenario: Default reasoning applied
- **WHEN** `DefaultSettings` is configured with `Reasoning: ReasoningMedium`
- **AND** the caller leaves `Reasoning` at its zero value
- **THEN** the model SHALL receive `ReasoningMedium`
- **WHEN** the caller sets `Reasoning: ReasoningHigh`
- **THEN** the model SHALL receive `ReasoningHigh`
