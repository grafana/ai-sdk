## ADDED Requirements

### Requirement: LanguageModelV4 request numbers preserve historical integers and finite JavaScript numbers

The provider package SHALL define this focused public request-number API:

```go
type LanguageModelNumber struct {
    // private representation
}

func LanguageModelNumberFromInt(value int) LanguageModelNumber
func LanguageModelNumberFromInt64(value int64) LanguageModelNumber
func LanguageModelNumberFromFloat64(value float64) (LanguageModelNumber, error)

func (n LanguageModelNumber) Int64() (int64, bool)
func (n LanguageModelNumber) Float64() (float64, bool)
```

`CallOptions.MaxOutputTokens`, `CallOptions.TopK`, and `CallOptions.Seed` SHALL be `*LanguageModelNumber`. The type SHALL use private integer and floating variants and SHALL NOT be a generic union framework.

Integer constructors SHALL preserve the exact signed integer. The float constructor SHALL reject NaN and positive or negative infinity. It SHALL canonicalize a finite float to the integer variant only when conversion to `int64` is in range and round-trips exactly, including canonicalizing negative zero to integer zero; every other finite float SHALL preserve its IEEE-754 value.

`Int64` SHALL succeed only for the integer variant. `Float64` SHALL succeed for the floating variant and for an integer exactly representable as `float64`; it SHALL fail rather than round a large exact integer. The zero value of `LanguageModelNumber` SHALL be invalid.

#### Scenario: Historical large integer is exact
- **WHEN** `LanguageModelNumberFromInt64(9007199254740993)` is called
- **THEN** `Int64` SHALL return `9007199254740993` exactly
- **AND** `Float64` SHALL report that lossless conversion is unavailable

#### Scenario: Fractional pinned value is exact
- **WHEN** `LanguageModelNumberFromFloat64(1.5)` is called
- **THEN** construction SHALL succeed and `Float64` SHALL return `1.5`

#### Scenario: Integral float is canonicalized
- **WHEN** `LanguageModelNumberFromFloat64(42.0)` is called
- **THEN** the value SHALL use the integer representation and `Int64` SHALL return `42`

#### Scenario: Non-finite numbers are rejected
- **WHEN** the float constructor receives NaN, positive infinity, or negative infinity
- **THEN** it SHALL return an error and no valid `LanguageModelNumber`

#### Scenario: Invalid zero value is rejected
- **WHEN** a protocol or provider adapter receives the zero value of `LanguageModelNumber`
- **THEN** it SHALL return an invalid-request or conversion error before encoding or provider invocation

### Requirement: LanguageModelNumber compatibility JSON is exact but non-normative

`LanguageModelNumber` SHALL retain `MarshalJSON` and `UnmarshalJSON` only as provider generic-JSON compatibility behavior. The integer variant SHALL encode as its exact decimal integer; the floating variant SHALL encode as a finite JSON number. Decoding SHALL first preserve a plain decimal integer token exactly when it fits `int64`; otherwise it SHALL parse the token as a finite `float64` and apply the constructor's canonicalization. It SHALL reject null, strings, malformed numbers, and non-finite results. Protocol encoders SHALL inspect `Int64` and `Float64` and SHALL NOT treat these generic methods as protocol authority.

#### Scenario: Historical integer JSON bytes are preserved
- **WHEN** the integer variant containing `9007199254740993` is compatibility-marshaled
- **THEN** the result SHALL be the exact bytes `9007199254740993`

#### Scenario: Fractional compatibility JSON remains numeric
- **WHEN** the floating variant containing `2.5` is compatibility-marshaled
- **THEN** the result SHALL be a JSON number semantically equal to `2.5`

#### Scenario: Finite number outside int64 remains representable
- **WHEN** compatibility decoding receives a valid finite JSON number outside `int64`
- **THEN** it SHALL preserve the corresponding finite `float64` value

#### Scenario: Invalid compatibility input fails
- **WHEN** compatibility decoding receives null, a quoted number, a non-finite result, or malformed JSON
- **THEN** decoding SHALL fail without producing a valid number

### Requirement: LanguageModelV4 optional request scalars preserve presence

The transport-neutral provider request contract SHALL preserve the optional scalar distinctions demonstrated by the registered `@ai-sdk/provider@4.0.7` and `@ai-sdk/gateway@4.0.52` evidence. `CallOptions.IncludeRawChunks` SHALL be `*bool`. `ResponseFormat.Name`, `ResponseFormat.Description`, and function `Tool.Description` SHALL be `*string`.

These changes are source-breaking. The provider package SHALL NOT add a generic optional-value abstraction.

#### Scenario: Explicit false raw-chunk request is representable
- **WHEN** `IncludeRawChunks` points to false
- **THEN** the value SHALL differ from `IncludeRawChunks == nil`

#### Scenario: Explicit empty response-format strings are representable
- **WHEN** a response-format name or description points to `""`
- **THEN** it SHALL differ from the corresponding nil field

#### Scenario: Explicit empty function description is representable
- **WHEN** a function tool description points to `""`
- **THEN** it SHALL differ from a nil description

#### Scenario: Existing pointer scalars remain presence-aware
- **WHEN** temperature, top-p, penalties, strict mode, approval decisions, or reasoning are absent or explicitly set
- **THEN** their existing pointer representation and behavior SHALL remain unchanged

### Requirement: Provider numeric conversion matches the registered implementation

Each supported provider SHALL preserve the exact registered provider behavior for `maxOutputTokens`, `topK`, and `seed`. A provider MUST NOT silently round, truncate, wrap, clamp, warn, or omit a number unless the exact pinned provider does so for that field and condition.

The required initial behavior SHALL be:

- Anthropic and Vertex Anthropic forward `maxOutputTokens` and `topK` exactly when supported, while retaining pinned model-cap and reasoning-budget adjustment/clamping, unsupported-model `topK` handling, thinking-mode sampling omission, and seed warning behavior.
- Bedrock forwards `maxOutputTokens` and `topK` exactly when supported, while retaining pinned Anthropic-thinking budget arithmetic and `topK` omission/warning plus seed warning behavior.
- OpenAI Responses forwards `maxOutputTokens` exactly and retains the pinned unsupported warning/omission for `topK` and `seed`.
- OpenAI-compatible forwards `maxOutputTokens` and `seed` exactly and retains the pinned unsupported warning/omission for `topK`.

Generated SDK integer fields SHALL be overridden through their supported extra-field mechanism when necessary. Repository-owned JSON request representations SHALL carry `LanguageModelNumber` directly. If final request evidence cannot match the registered implementation, work SHALL stop for an explicit intentional-deviation decision.

#### Scenario: Anthropic supported fractional topK reaches the final request
- **WHEN** Anthropic or Vertex Anthropic receives fractional `topK` on a supported model with thinking disabled
- **THEN** its final serialized backend request SHALL carry the same fraction exactly once through the SDK override

#### Scenario: Anthropic supported fractional max output is forwarded
- **WHEN** Anthropic or Vertex Anthropic receives fractional `maxOutputTokens` on a model and reasoning path where the pinned provider forwards that field
- **THEN** its final semantic backend request SHALL carry the same number subject only to the pinned max-token arithmetic and model cap

#### Scenario: Anthropic thinking removes fractional topK override
- **WHEN** fractional `topK` was installed through an SDK extra-field override and thinking becomes enabled or adaptive
- **THEN** the final serialized backend request SHALL omit `top_k`
- **AND** the provider SHALL emit the pinned unsupported warning

#### Scenario: Anthropic sampling-rejecting model removes fractional topK override
- **WHEN** fractional `topK` was installed through an SDK extra-field override and the selected model rejects sampling parameters
- **THEN** the final serialized backend request SHALL omit `top_k`
- **AND** the provider SHALL emit the pinned model-specific warning

#### Scenario: Anthropic max-token clamping matches pinned behavior
- **WHEN** max-token reasoning arithmetic exceeds a pinned model cap
- **THEN** the Go provider SHALL reproduce the pinned arithmetic, warning, and clamping behavior

#### Scenario: Bedrock supported fractions are forwarded
- **WHEN** Bedrock receives fractional `maxOutputTokens` or `topK` on a model and reasoning path where the pinned provider forwards that field
- **THEN** its final semantic backend request SHALL carry the same number subject only to pinned reasoning-budget arithmetic

#### Scenario: Bedrock thinking omission matches pinned behavior
- **WHEN** Anthropic thinking is enabled and the pinned Bedrock provider removes `topK`
- **THEN** the Go provider SHALL omit `topK` and emit the pinned warning

#### Scenario: OpenAI Responses max output is forwarded
- **WHEN** OpenAI Responses receives fractional `maxOutputTokens`
- **THEN** its final semantic backend request SHALL carry the same number

#### Scenario: OpenAI-compatible supported fractions are forwarded
- **WHEN** OpenAI-compatible receives fractional `maxOutputTokens` or `seed`
- **THEN** its final semantic backend request SHALL carry the same number

#### Scenario: Pinned unsupported settings remain unsupported
- **WHEN** a provider receives `seed` or `topK` for a field that its exact pinned implementation classifies as unsupported
- **THEN** the Go provider SHALL emit the same unsupported warning and omit that field

#### Scenario: Large historical integer is not rounded
- **WHEN** a provider receives an integer variant outside the exact `float64` integer range
- **THEN** it SHALL preserve the exact integer through an integer-capable request path or fail explicitly
- **AND** it SHALL NOT convert the value through `float64`
