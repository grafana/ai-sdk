## ADDED Requirements

### Requirement: ProviderOptions named map type

The `provider` package SHALL define `ProviderOptions` as a named type alias for `map[string]ProviderOption`:

```go
type ProviderOptions map[string]ProviderOption
```

This named type SHALL carry custom `MarshalJSON` and `UnmarshalJSON` methods (see next requirement) so it round-trips losslessly through `encoding/json`. Every `ProviderOptions` field across the provider package and the root `aisdk` package SHALL use this named type instead of inline `map[string]provider.ProviderOption`.

#### Scenario: Named type definition
- **WHEN** the `provider` package is inspected
- **THEN** `ProviderOptions` SHALL be defined as `type ProviderOptions map[string]ProviderOption`

#### Scenario: Field types use the named alias
- **WHEN** any struct in the `provider` or `aisdk` package has a `ProviderOptions` field
- **THEN** the field type SHALL be `provider.ProviderOptions` (the named alias), not the inline `map[string]provider.ProviderOption`

### Requirement: Lossless JSON round-trip via RawProviderOption

The `ProviderOptions` named type SHALL implement `MarshalJSON` and `UnmarshalJSON` such that:

- **Marshal**: each entry is serialized by calling `json.Marshal` on the concrete `ProviderOption` value. Typed providers (e.g., `AnthropicOptions`) serialize their concrete struct; `RawProviderOption` writes its `Raw` bytes directly. The resulting JSON is `{key1: <value1JSON>, key2: <value2JSON>, ...}`.
- **Unmarshal**: the wire JSON is decoded as `map[string]json.RawMessage`. Each entry is wrapped as `RawProviderOption{Key: k, Raw: v}` regardless of the original concrete type that produced it. Consumers reach typed values via the existing `ResolveOption[T]` helper.

This intentional asymmetry — typed values out, `RawProviderOption` values back — is the existing pattern; this requirement just extends it to apply at every wire boundary uniformly.

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

### Requirement: ProviderOptions JSON tags everywhere

Every `ProviderOptions` field across the provider package and the root `aisdk` package SHALL be tagged `json:"providerOptions,omitempty"` (replacing the previous `json:"-"`). Affected fields include but are not limited to:

- `provider.CallOptions.ProviderOptions`
- `provider.Message.ProviderOptions`
- `provider.ContentPart.ProviderOptions`
- `provider.Tool.ProviderOptions`
- `provider.ToolResultOutput.ProviderOptions`
- `provider.ToolResultContentValue.ProviderOptions`
- `aisdk.StreamTextParams.ProviderOptions`
- `aisdk.PrepareStepResult.ProviderOptions`
- `aisdk.SystemModelMessage.ProviderOptions`
- `aisdk.Tool.ProviderOptions`

#### Scenario: No json:"-" on ProviderOptions
- **WHEN** the codebase is searched for ``json:"-"`` on `ProviderOptions` fields
- **THEN** no match SHALL be found

#### Scenario: All ProviderOptions JSON-encode under "providerOptions" key
- **WHEN** any struct with a non-nil `ProviderOptions` is marshaled to JSON
- **THEN** the output SHALL contain a `"providerOptions"` key with the encoded map

## MODIFIED Requirements

### Requirement: ProviderOption interface

The `provider` package SHALL define a `ProviderOption` interface with a single method `ProviderKey() string` that returns the provider namespace key (e.g., `"anthropic"`). The named map alias `ProviderOptions` (`= map[string]ProviderOption`) SHALL be the canonical type used at every `ProviderOptions` field site, and that map type MUST carry lossless `MarshalJSON` and `UnmarshalJSON` implementations.

#### Scenario: Provider option type implements interface
- **WHEN** `AnthropicOptions` implements `ProviderKey()` returning `"anthropic"`
- **THEN** it SHALL satisfy the `ProviderOption` interface at compile time

#### Scenario: ProviderOptions field type
- **WHEN** any struct in the `provider` or `aisdk` package has a `ProviderOptions` field
- **THEN** the field type SHALL be `provider.ProviderOptions` (the named alias)

### Requirement: ProviderOptions field type across all structs

Every `ProviderOptions` field in the `provider` and `aisdk` packages SHALL use type `provider.ProviderOptions` (the named alias of `map[string]provider.ProviderOption`). This MUST include the unified `Message`, the unified `ContentPart`, the unified `Tool`, plus `CallOptions`, `ToolResultOutput`, `ToolResultContentValue`, `StreamTextParams`, `PrepareStepResult`, `SystemModelMessage`, and `aisdk.Tool`. The previous list referenced now-removed concrete types (`SystemMessage`, `UserMessage`, `AssistantMessage`, `ToolMessage`, all `*ContentPart` variants, `FunctionTool`); these are superseded by the unified flat structs introduced in `provider-v4-content-model` and `v4-tool-type-split`.

#### Scenario: Unified Message uses ProviderOptions alias
- **WHEN** the `provider.Message` struct is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

#### Scenario: Unified ContentPart uses ProviderOptions alias
- **WHEN** the `provider.ContentPart` struct is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

#### Scenario: Unified Tool uses ProviderOptions alias
- **WHEN** the `provider.Tool` struct is inspected
- **THEN** its `ProviderOptions` field SHALL be of type `provider.ProviderOptions`

### Requirement: ConvertToModelMessages metadata bridge

The `ConvertToModelMessages` function SHALL wrap `ProviderMetadata` JSON values in `RawProviderOption` when converting UI-layer parts to provider-layer messages. The result type MUST be the named `provider.ProviderOptions` (no longer `map[string]provider.ProviderOption` inline). Each entry SHALL be wrapped as `RawProviderOption{Key: k, Raw: v}`. Downstream JSON serialization through any wire MUST be lossless because the named alias carries custom marshalers.

#### Scenario: ProviderMetadata to ProviderOptions conversion
- **WHEN** a UI text part with `ProviderMetadata{"anthropic": <json>}` is converted to a flat assistant-role `ContentPart{Type: ContentPartTypeText, ...}`
- **THEN** the resulting `ContentPart.ProviderOptions["anthropic"]` SHALL be a `RawProviderOption` wrapping the original JSON

#### Scenario: Empty ProviderMetadata
- **WHEN** a UI part has no `ProviderMetadata` (nil or empty map)
- **THEN** the resulting `ContentPart.ProviderOptions` SHALL be nil
