# typed-provider-options Specification

## Purpose

Define typed provider options and lossless JSON behavior for provider-specific metadata across the provider and orchestration packages.

## Requirements

### Requirement: ProviderOptions named map type

The `provider` package SHALL define `ProviderOptions` as a named type alias for `map[string]ProviderOption`:

```go
type ProviderOptions map[string]ProviderOption
```

This named type SHALL carry custom `MarshalJSON` and `UnmarshalJSON` methods so it round-trips losslessly through `encoding/json`. Every `ProviderOptions` field across the provider package and the root `aisdk` package SHALL use this named type instead of inline `map[string]provider.ProviderOption`.

#### Scenario: Named type definition
- **WHEN** the `provider` package is inspected
- **THEN** `ProviderOptions` SHALL be defined as `type ProviderOptions map[string]ProviderOption`

#### Scenario: Field types use the named alias
- **WHEN** any struct in the `provider` or `aisdk` package has a `ProviderOptions` field
- **THEN** the field type SHALL be `provider.ProviderOptions` (the named alias), not the inline `map[string]provider.ProviderOption`

### Requirement: ProviderOption interface

The `provider` package SHALL define a `ProviderOption` interface with a single method `ProviderKey() string` that returns the provider namespace key (e.g., `"anthropic"`). The named map alias `ProviderOptions` (`= map[string]ProviderOption`) SHALL be the canonical type used at every `ProviderOptions` field site, and that map type MUST carry lossless `MarshalJSON` and `UnmarshalJSON` implementations.

#### Scenario: Provider option type implements interface
- **WHEN** `AnthropicOptions` implements `ProviderKey()` returning `"anthropic"`
- **THEN** it SHALL satisfy the `ProviderOption` interface at compile time

#### Scenario: ProviderOptions field type
- **WHEN** any struct in the `provider` or `aisdk` package has a `ProviderOptions` field
- **THEN** the field type SHALL be `provider.ProviderOptions` (the named alias)

### Requirement: RawProviderOption wrapper

The `provider` package SHALL define a `RawProviderOption` struct with fields `Key string` and `Raw json.RawMessage`. `RawProviderOption` SHALL implement the `ProviderOption` interface with `ProviderKey()` returning its `Key` field. This type SHALL be used to wrap genuine JSON data from round-tripped provider metadata and decoded provider options.

#### Scenario: Round-tripped metadata wrapped as RawProviderOption
- **WHEN** `ConvertToModelMessages` converts a `ProviderMetadata` map entry with key `"anthropic"` and JSON value `{"cacheControl": {"type": "ephemeral"}}`
- **THEN** the resulting `ProviderOptions["anthropic"]` SHALL be a `RawProviderOption{Key: "anthropic", Raw: <the JSON bytes>}`

#### Scenario: RawProviderOption satisfies interface
- **WHEN** a `RawProviderOption` is placed in a `ProviderOptions` map
- **THEN** it SHALL satisfy the `ProviderOption` interface at compile time

### Requirement: Lossless JSON round-trip via RawProviderOption

`ProviderOptions` SHALL implement lossless `MarshalJSON`/`UnmarshalJSON` uniformly at every wire boundary. Marshaling SHALL call `json.Marshal` on concrete values: typed options serialize their structs, `RawProviderOption` writes `Raw` bytes directly, yielding `{key: <valueJSON>, ...}`. Unmarshaling SHALL decode `map[string]json.RawMessage`, wrapping every entry as `RawProviderOption{Key: k, Raw: v}` regardless of original type. `ResolveOption[T]` SHALL recover typed views.

#### Scenario: Typed option marshals to its concrete JSON
- **WHEN** `ProviderOptions{"anthropic": AnthropicOptions{...}}` is marshaled to JSON
- **THEN** the output SHALL contain `"anthropic": <JSON of AnthropicOptions struct>`

#### Scenario: RawProviderOption marshals to its raw bytes
- **WHEN** `ProviderOptions{"anthropic": RawProviderOption{Key: "anthropic", Raw: []byte(`{"x":1}`)}}` is marshaled
- **THEN** the output SHALL contain `"anthropic": {"x":1}` (the raw bytes inlined)

#### Scenario: Unmarshal wraps every entry as RawProviderOption
- **WHEN** the JSON `{"anthropic": {"x": 1}, "openai": {"y": 2}}` is unmarshaled into `ProviderOptions`
- **THEN** the resulting map SHALL contain both keys, and each value SHALL be a `RawProviderOption` carrying the corresponding raw JSON

#### Scenario: ResolveOption recovers the typed view
- **WHEN** `ResolveOption[AnthropicOptions]` is called on a `ProviderOptions` map decoded from the wire
- **THEN** it SHALL successfully unmarshal the `RawProviderOption` into the typed `AnthropicOptions`

### Requirement: BuildProviderOptions helper

The `provider` package SHALL define a `BuildProviderOptions(opts ...ProviderOption) ProviderOptions` function that constructs a `ProviderOptions` map from variadic typed values, using each value's `ProviderKey()` as the map key. When multiple values share the same key, the last value SHALL win.

#### Scenario: Single provider option
- **WHEN** `BuildProviderOptions` is called with a single `AnthropicOptions` value
- **THEN** the result SHALL be `ProviderOptions{"anthropic": <the AnthropicOptions value>}`

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

The provider package SHALL define `ResolveOption[T any](opts ProviderOptions, key string) (T, bool, error)`: absent key returns zero/false/nil; value of `T` returns direct assertion/true/nil; `RawProviderOption` returns `json.Unmarshal` into `T`/true/error-or-nil; unexpected type returns zero/true/error.

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

Every `ProviderOptions` field in provider/aisdk SHALL use named `provider.ProviderOptions` (`map[string]provider.ProviderOption`), including unified `Message`, `ContentPart`, `Tool`, `CallOptions`, `ToolResultOutput`, `ToolResultContentValue`, `StreamTextParams`, `PrepareStepResult`, `SystemModelMessage` and `aisdk.Tool`. Unified structs supersede removed message/content variants and `FunctionTool` from the previous list under provider-v4-content-model and v4-tool-type-split.

#### Scenario: Unified Message uses ProviderOptions alias
- **WHEN** the `provider.Message` struct is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

#### Scenario: Unified ContentPart uses ProviderOptions alias
- **WHEN** the `provider.ContentPart` struct is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

#### Scenario: Unified Tool uses ProviderOptions alias
- **WHEN** the `provider.Tool` struct is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

#### Scenario: Orchestration layer types
- **WHEN** `StreamTextParams`, `PrepareStepResult`, `SystemModelMessage`, or `Tool` is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

### Requirement: ProviderOptions JSON tags everywhere

Every `ProviderOptions` field in provider and root aisdk SHALL use `json:"providerOptions,omitempty"`, replacing `json:"-"`.

#### Scenario: ProviderOptions tag coverage
- **WHEN** JSON tags are inspected on known provider-options-bearing fields
- **THEN** the tag requirement SHALL include, without being limited to, `provider.CallOptions.ProviderOptions`, `provider.Message.ProviderOptions`, `provider.ContentPart.ProviderOptions`, `provider.Tool.ProviderOptions`, `provider.ToolResultOutput.ProviderOptions`, `provider.ToolResultContentValue.ProviderOptions`, `aisdk.StreamTextParams.ProviderOptions`, `aisdk.PrepareStepResult.ProviderOptions`, `aisdk.SystemModelMessage.ProviderOptions` and `aisdk.Tool.ProviderOptions`

#### Scenario: No json:"-" on ProviderOptions
- **WHEN** the codebase is searched for ``json:"-"`` on `ProviderOptions` fields
- **THEN** no match SHALL be found

#### Scenario: All ProviderOptions JSON-encode under "providerOptions" key
- **WHEN** any struct with a non-nil `ProviderOptions` is marshaled to JSON
- **THEN** the output SHALL contain a `"providerOptions"` key with the encoded map

### Requirement: Anthropic option types implement ProviderOption

The `providers/anthropic` module's `anthropic` package SHALL define an `AnthropicOptions` struct that implements `ProviderOption` with `ProviderKey()` returning `"anthropic"`. The `AnthropicToolOptions` struct SHALL implement `ProviderOption` with `ProviderKey()` returning `"anthropic"`.

#### Scenario: AnthropicOptions implements ProviderOption
- **WHEN** an `AnthropicOptions` value is used
- **THEN** it SHALL satisfy the `provider.ProviderOption` interface with `ProviderKey() == "anthropic"`

#### Scenario: AnthropicToolOptions implements ProviderOption
- **WHEN** an `AnthropicToolOptions` value is used
- **THEN** it SHALL satisfy the `provider.ProviderOption` interface with `ProviderKey() == "anthropic"`

### Requirement: CacheControl convenience helper

The `providers/anthropic` module's `anthropic` package SHALL define a `CacheControl(cacheType string) ProviderOption` function that returns a typed provider option for per-part cache control configuration. The returned value SHALL implement `ProviderOption` with `ProviderKey()` returning `"anthropic"`.

#### Scenario: CacheControl ephemeral
- **WHEN** `CacheControl("ephemeral")` is called
- **THEN** it SHALL return a `ProviderOption` that providers can resolve to extract cache control type `"ephemeral"`

#### Scenario: CacheControl used in BuildProviderOptions
- **WHEN** `BuildProviderOptions(anthropic.CacheControl("ephemeral"))` is called
- **THEN** the result SHALL contain an `"anthropic"` key with the cache control option

### Requirement: ConvertToModelMessages metadata bridge

The `ConvertToModelMessages` function SHALL wrap `ProviderMetadata` JSON values in `RawProviderOption` when converting UI-layer parts to provider-layer messages. The result type MUST be the named `provider.ProviderOptions` (no longer `map[string]provider.ProviderOption` inline). Each entry SHALL be wrapped as `RawProviderOption{Key: k, Raw: v}`. Downstream JSON serialization through any wire MUST be lossless because the named alias carries custom marshalers.

#### Scenario: ProviderMetadata to ProviderOptions conversion
- **WHEN** a UI text part with `ProviderMetadata{"anthropic": <json>}` is converted to a flat assistant-role `ContentPart{Type: ContentPartTypeText, ...}`
- **THEN** the resulting `ContentPart.ProviderOptions["anthropic"]` SHALL be a `RawProviderOption` wrapping the original JSON

#### Scenario: Empty ProviderMetadata
- **WHEN** a UI part has no `ProviderMetadata` (nil or empty map)
- **THEN** the resulting `ContentPart.ProviderOptions` SHALL be nil
