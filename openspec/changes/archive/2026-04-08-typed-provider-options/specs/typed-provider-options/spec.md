## ADDED Requirements

### Requirement: ProviderOption interface
The `provider` package SHALL define a `ProviderOption` interface with a single method `ProviderKey() string` that returns the provider namespace key (e.g., `"anthropic"`). This interface SHALL be used as the value type in all `ProviderOptions` map fields across the codebase, replacing `json.RawMessage`.

#### Scenario: Provider option type implements interface
- **WHEN** `AnthropicOptions` implements `ProviderKey()` returning `"anthropic"`
- **THEN** it SHALL satisfy the `ProviderOption` interface at compile time

#### Scenario: ProviderOptions field type
- **WHEN** any struct in the `provider` or `aisdk` package has a `ProviderOptions` field
- **THEN** the field type SHALL be `map[string]provider.ProviderOption`

### Requirement: RawProviderOption wrapper
The `provider` package SHALL define a `RawProviderOption` struct with fields `Key string` and `Raw json.RawMessage`. `RawProviderOption` SHALL implement the `ProviderOption` interface with `ProviderKey()` returning its `Key` field. This type SHALL be used to wrap genuine JSON data from round-tripped provider metadata (e.g., when `ProviderMetadata` from a previous SSE response is converted back to `ProviderOptions` via `ConvertToModelMessages`).

#### Scenario: Round-tripped metadata wrapped as RawProviderOption
- **WHEN** `ConvertToModelMessages` converts a `ProviderMetadata` map entry with key `"anthropic"` and JSON value `{"cacheControl": {"type": "ephemeral"}}`
- **THEN** the resulting `ProviderOptions["anthropic"]` SHALL be a `RawProviderOption{Key: "anthropic", Raw: <the JSON bytes>}`

#### Scenario: RawProviderOption satisfies interface
- **WHEN** a `RawProviderOption` is placed in a `map[string]ProviderOption`
- **THEN** it SHALL satisfy the `ProviderOption` interface at compile time

### Requirement: BuildProviderOptions helper
The `provider` package SHALL define a `BuildProviderOptions(opts ...ProviderOption) map[string]ProviderOption` function that constructs a `ProviderOptions` map from variadic typed values, using each value's `ProviderKey()` as the map key. When multiple values share the same key, the last value SHALL win.

#### Scenario: Single provider option
- **WHEN** `BuildProviderOptions` is called with a single `AnthropicOptions` value
- **THEN** the result SHALL be `map[string]ProviderOption{"anthropic": <the AnthropicOptions value>}`

#### Scenario: Multiple provider options with different keys
- **WHEN** `BuildProviderOptions` is called with options for providers `"anthropic"` and `"openai"`
- **THEN** the result SHALL contain both entries keyed by their respective provider names

#### Scenario: Duplicate keys use last value
- **WHEN** `BuildProviderOptions` is called with two values both returning `ProviderKey() == "anthropic"`
- **THEN** the result SHALL contain only the last value for the `"anthropic"` key

#### Scenario: Empty variadic call
- **WHEN** `BuildProviderOptions` is called with no arguments
- **THEN** the result SHALL be an empty non-nil map

### Requirement: ResolveOption generic helper
The `provider` package SHALL define a generic function `ResolveOption[T any](opts map[string]ProviderOption, key string) (T, bool, error)` that resolves a typed provider option from the map. The function SHALL handle three cases: (1) key not present returns zero value, false, nil; (2) value is type `T` returns the value via direct type assertion, true, nil; (3) value is `RawProviderOption` returns the result of `json.Unmarshal` into `T`, true, error-or-nil; (4) value is an unexpected type returns zero value, true, and an error.

#### Scenario: Typed option resolved directly
- **WHEN** `ResolveOption[AnthropicOptions]` is called with a map containing a fresh `AnthropicOptions` value at key `"anthropic"`
- **THEN** it SHALL return the value directly via type assertion with `true, nil`

#### Scenario: Raw option resolved via JSON unmarshal
- **WHEN** `ResolveOption[AnthropicOptions]` is called with a map containing a `RawProviderOption` at key `"anthropic"` with valid JSON
- **THEN** it SHALL return the unmarshaled `AnthropicOptions` with `true, nil`

#### Scenario: Key not present
- **WHEN** `ResolveOption[AnthropicOptions]` is called with a map that does not contain key `"anthropic"`
- **THEN** it SHALL return zero value, `false`, `nil`

#### Scenario: Malformed JSON in RawProviderOption
- **WHEN** `ResolveOption[AnthropicOptions]` is called with a `RawProviderOption` containing invalid JSON
- **THEN** it SHALL return zero value, `true`, and a non-nil error

#### Scenario: Unexpected type
- **WHEN** `ResolveOption[AnthropicOptions]` is called with a map containing a value of an unrelated type at key `"anthropic"`
- **THEN** it SHALL return zero value, `true`, and an error describing the unexpected type

### Requirement: ProviderOptions field type across all structs
All `ProviderOptions` fields in the `provider` and `aisdk` packages SHALL use type `map[string]provider.ProviderOption`. This includes `CallOptions`, `FunctionTool`, `SystemMessage`, `UserMessage`, `AssistantMessage`, `ToolMessage`, all content part types (`TextContentPart`, `FileContentPart`, `ReasoningContentPart`, `ToolCallContentPart`, `ToolResultContentPart`, `CustomContentPart`, `ReasoningFileContentPart`, `ToolApprovalResponseContentPart`), `ToolResultOutput`, `ToolResultContentValue`, `StreamTextParams`, `PrepareStepResult`, `SystemModelMessage`, and `Tool`.

#### Scenario: Provider package message types
- **WHEN** `SystemMessage`, `UserMessage`, `AssistantMessage`, or `ToolMessage` is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `map[string]provider.ProviderOption`

#### Scenario: Provider package content part types
- **WHEN** any content part type (`TextContentPart`, `FileContentPart`, `ReasoningContentPart`, `ToolCallContentPart`, `ToolResultContentPart`, `CustomContentPart`, `ReasoningFileContentPart`, `ToolApprovalResponseContentPart`) is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `map[string]provider.ProviderOption`

#### Scenario: Orchestration layer types
- **WHEN** `StreamTextParams`, `PrepareStepResult`, `SystemModelMessage`, or `Tool` is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `map[string]provider.ProviderOption`

### Requirement: Anthropic option types implement ProviderOption
The `anthropic` package's `AnthropicOptions` struct SHALL implement `ProviderOption` with `ProviderKey()` returning `"anthropic"`. The `AnthropicToolOptions` struct SHALL implement `ProviderOption` with `ProviderKey()` returning `"anthropic"`.

#### Scenario: AnthropicOptions implements ProviderOption
- **WHEN** an `AnthropicOptions` value is used
- **THEN** it SHALL satisfy the `provider.ProviderOption` interface with `ProviderKey() == "anthropic"`

#### Scenario: AnthropicToolOptions implements ProviderOption
- **WHEN** an `AnthropicToolOptions` value is used
- **THEN** it SHALL satisfy the `provider.ProviderOption` interface with `ProviderKey() == "anthropic"`

### Requirement: CacheControl convenience helper
The `anthropic` package SHALL define a `CacheControl(cacheType string) ProviderOption` function that returns a typed provider option for per-part cache control configuration. The returned value SHALL implement `ProviderOption` with `ProviderKey()` returning `"anthropic"`.

#### Scenario: CacheControl ephemeral
- **WHEN** `CacheControl("ephemeral")` is called
- **THEN** it SHALL return a `ProviderOption` that providers can resolve to extract cache control type `"ephemeral"`

#### Scenario: CacheControl used in BuildProviderOptions
- **WHEN** `BuildProviderOptions(anthropic.CacheControl("ephemeral"))` is called
- **THEN** the result SHALL contain an `"anthropic"` key with the cache control option

### Requirement: ConvertToModelMessages metadata bridge
The `ConvertToModelMessages` function SHALL wrap `ProviderMetadata` JSON values in `RawProviderOption` when converting UI-layer parts to provider-layer messages. The `providerMetadataToOptions` helper SHALL produce `map[string]provider.ProviderOption` with each entry wrapped as `RawProviderOption{Key: k, Raw: v}`.

#### Scenario: ProviderMetadata to ProviderOptions conversion
- **WHEN** a `TextPart` with `ProviderMetadata{"anthropic": <json>}` is converted to a `TextContentPart`
- **THEN** the resulting `TextContentPart.ProviderOptions["anthropic"]` SHALL be a `RawProviderOption` wrapping the original JSON

#### Scenario: Empty ProviderMetadata
- **WHEN** a part has no `ProviderMetadata` (nil or empty map)
- **THEN** the resulting `ProviderOptions` SHALL be nil
