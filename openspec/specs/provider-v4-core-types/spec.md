# provider-v4-core-types Specification

## Purpose
TBD - created by archiving change v4-reshape-core-types. Update Purpose after archive.
## Requirements
### Requirement: V4 Usage type with nested token structs
The `Usage` type SHALL have two nested struct fields `InputTokens` (type `InputTokenUsage`) and `OutputTokens` (type `OutputTokenUsage`), plus an optional `Raw` field (`json.RawMessage`). The `TotalTokens` top-level field SHALL NOT exist. The `InputTokenDetails` and `OutputTokenDetails` types SHALL NOT exist.

#### Scenario: Usage struct shape matches V4
- **WHEN** a provider constructs a `Usage` value
- **THEN** it SHALL set `InputTokens.Total` and `OutputTokens.Total` for the token counts, and optionally set detail fields (`InputTokens.NoCache`, `InputTokens.CacheRead`, `InputTokens.CacheWrite`, `OutputTokens.Text`, `OutputTokens.Reasoning`)

#### Scenario: Usage JSON serialization uses camelCase nested keys
- **WHEN** a `Usage` value is serialized to JSON
- **THEN** it SHALL produce `{"inputTokens":{"total":...},"outputTokens":{"total":...}}` with camelCase keys and `omitempty` on all optional `*int` fields

#### Scenario: Usage with no detail fields
- **WHEN** a provider only knows total input and output token counts
- **THEN** it SHALL set only `InputTokens.Total` and `OutputTokens.Total`, and all other `*int` fields SHALL be nil

#### Scenario: Usage with cache token details
- **WHEN** a provider returns cache usage information (e.g. Anthropic cache_read/cache_creation)
- **THEN** it SHALL set `InputTokens.CacheRead` and `InputTokens.CacheWrite` on the `InputTokenUsage` struct

### Requirement: InputTokenUsage struct
The `InputTokenUsage` type SHALL have fields: `Total *int`, `NoCache *int`, `CacheRead *int`, `CacheWrite *int`. All fields SHALL be `*int` with `omitempty` JSON tags.

#### Scenario: InputTokenUsage fields
- **WHEN** `InputTokenUsage` is defined
- **THEN** it SHALL have exactly four `*int` fields: `Total` (json:"total"), `NoCache` (json:"noCache"), `CacheRead` (json:"cacheRead"), `CacheWrite` (json:"cacheWrite")

### Requirement: OutputTokenUsage struct
The `OutputTokenUsage` type SHALL have fields: `Total *int`, `Text *int`, `Reasoning *int`. All fields SHALL be `*int` with `omitempty` JSON tags.

#### Scenario: OutputTokenUsage fields
- **WHEN** `OutputTokenUsage` is defined
- **THEN** it SHALL have exactly three `*int` fields: `Total` (json:"total"), `Text` (json:"text"), `Reasoning` (json:"reasoning")

### Requirement: Usage aggregation sums nested totals
The `aggregateUsage` function SHALL sum `InputTokens.Total` and `OutputTokens.Total` across steps. Detail fields (cache, text, reasoning) SHALL NOT be aggregated.

#### Scenario: Aggregate usage across multiple steps
- **WHEN** two steps have `InputTokens.Total` of 100 and 200, and `OutputTokens.Total` of 50 and 75
- **THEN** the aggregated usage SHALL have `InputTokens.Total` of 300 and `OutputTokens.Total` of 125

#### Scenario: Aggregate usage with nil totals
- **WHEN** a step has nil `InputTokens.Total`
- **THEN** it SHALL be treated as zero in the aggregation sum

### Requirement: FinishReason as struct with Unified and Raw
The `FinishReason` type SHALL be a struct with two fields: `Unified` (type `UnifiedFinishReason`) and `Raw` (type `string`). The `UnifiedFinishReason` type SHALL be a `string` type alias with constants: `stop`, `length`, `content-filter`, `tool-calls`, `error`, `other`.

#### Scenario: FinishReason struct shape
- **WHEN** a provider returns a finish reason
- **THEN** it SHALL construct `FinishReason{Unified: <mapped-value>, Raw: <provider-native-string>}`

#### Scenario: FinishReason constants
- **WHEN** code references finish reason constants
- **THEN** it SHALL use `UnifiedFinishReason` typed constants: `FinishReasonStop`, `FinishReasonLength`, `FinishReasonContentFilter`, `FinishReasonToolCalls`, `FinishReasonError`, `FinishReasonOther`

#### Scenario: FinishReason JSON serialization
- **WHEN** a `FinishReason` is serialized to JSON
- **THEN** it SHALL produce `{"unified":"stop","raw":"end_turn"}` with the raw field omitted when empty

#### Scenario: ToolChoice field uses ToolChoiceType
- **WHEN** `ToolChoice` is defined in `provider/types.go`
- **THEN** its `Type` field SHALL be typed as `ToolChoiceType`, not bare `string`

#### Scenario: Warning field uses WarningType
- **WHEN** `Warning` is defined in `provider/types.go`
- **THEN** its `Type` field SHALL be typed as `WarningType`, not bare `string`

#### Scenario: ToolResultContentValue field uses ToolResultContentType
- **WHEN** `ToolResultContentValue` is defined in `provider/types.go`
- **THEN** its `Type` field SHALL be typed as `ToolResultContentType`, not bare `string`

#### Scenario: ReasoningEffort typed constants
- **WHEN** reasoning effort constants are defined in `provider/types.go`
- **THEN** they SHALL be typed as `ReasoningEffort`, not untyped `string`

#### Scenario: CallOptions.Reasoning uses ReasoningEffort pointer
- **WHEN** `CallOptions` is defined in `provider/language_model.go`
- **THEN** its `Reasoning` field SHALL be `*ReasoningEffort`, not `*string`

#### Scenario: ResponseFormat field uses ResponseFormatType
- **WHEN** `ResponseFormat` is defined in `provider/language_model.go`
- **THEN** its `Type` field SHALL be typed as `ResponseFormatType`, not bare `string`

#### Scenario: GenerateContentPart field uses GenerateContentType
- **WHEN** `GenerateContentPart` is defined in `provider/language_model.go`
- **THEN** its `Type` field SHALL be typed as `GenerateContentType`, not bare `string`

#### Scenario: GenerateContentPart.SourceType uses SourceType
- **WHEN** `GenerateContentPart` is defined in `provider/language_model.go`
- **THEN** its `SourceType` field SHALL be typed as `SourceType`, not bare `string`

### Requirement: RawFinishReason removed from all types
No struct in the codebase SHALL have a standalone `RawFinishReason` field. This applies to `StreamPart` (provider), `StepResult`, `StreamFinishStep`, and `StreamFinish` (orchestration). The raw finish reason SHALL be carried exclusively inside `FinishReason.Raw`.

#### Scenario: StreamPart finish with raw reason
- **WHEN** a provider emits a PartFinish stream part
- **THEN** it SHALL set `StreamPart.FinishReason` to a `FinishReason` struct containing both the unified and raw values

#### Scenario: Orchestration types carry FinishReason struct
- **WHEN** `StepResult`, `StreamFinishStep`, or `StreamFinish` report a finish reason
- **THEN** each SHALL have a `FinishReason` field of type `provider.FinishReason` (struct) and SHALL NOT have a separate `RawFinishReason` field

#### Scenario: Orchestration reads raw finish reason
- **WHEN** the orchestration layer needs the raw finish reason
- **THEN** it SHALL read `FinishReason.Raw` instead of a standalone `RawFinishReason` field

### Requirement: SSE wire format preserves string finish reason
The `UIMessageChunk.FinishReason` field SHALL remain a `string` type. It SHALL be populated with the `Unified` value from the `FinishReason` struct.

#### Scenario: SSE finish chunk serialization
- **WHEN** a `StreamFinish` event is converted to a `UIMessageChunk`
- **THEN** the chunk's `FinishReason` field SHALL be `string(finishReason.Unified)`

### Requirement: ProviderMetadata type replaces Metadata
The type `Metadata` SHALL be renamed to `ProviderMetadata`. All fields currently typed as `Metadata` SHALL use `ProviderMetadata` as their type. The `GenerateResult.Metadata` field SHALL be renamed to `GenerateResult.ProviderMetadata`.

#### Scenario: Type definition
- **WHEN** `ProviderMetadata` is defined in `provider/types.go`
- **THEN** it SHALL be `type ProviderMetadata map[string]json.RawMessage`

#### Scenario: Field naming consistency
- **WHEN** any struct has a field for provider metadata
- **THEN** the field SHALL be named `ProviderMetadata` and typed `ProviderMetadata` (or `provider.ProviderMetadata` from external packages)

### Requirement: ResponseMetadata slimmed to ID, Timestamp, ModelID

The `provider.ResponseMetadata` struct SHALL contain `ID string`, `Timestamp time.Time`, `ModelID string`, and `Provider string`. The `Headers` and `Body` fields SHALL NOT exist on `ResponseMetadata`. The `Provider` field SHALL be optional (`omitempty`) and carry the identifier of the provider that served the request (e.g. `anthropic`, `anthropic.vertex`).

#### Scenario: ResponseMetadata struct shape
- **WHEN** `ResponseMetadata` is defined in `provider/language_model.go`
- **THEN** it SHALL have exactly four fields: `ID`, `ModelID`, `Provider`, `Timestamp`

#### Scenario: Provider constructs ResponseMetadata
- **WHEN** a provider returns response metadata
- **THEN** it SHALL construct `ResponseMetadata{ID: ..., ModelID: ..., Provider: ...}` without Headers or Body

#### Scenario: ResponseMetadata JSON omits empty provider
- **WHEN** a `ResponseMetadata` value with an empty `Provider` is serialized to JSON
- **THEN** the `provider` key SHALL be omitted (`omitempty`), preserving backward compatibility with existing payloads

### Requirement: GenerateResponse type for result-level response
A `GenerateResponse` type SHALL exist that embeds `ResponseMetadata` and adds `Headers map[string]string` and `Body json.RawMessage`. The `GenerateResult.Response` field SHALL be `*GenerateResponse`.

#### Scenario: GenerateResponse struct shape
- **WHEN** `GenerateResponse` is defined
- **THEN** it SHALL embed `ResponseMetadata` and add `Headers map[string]string` and `Body json.RawMessage` fields

#### Scenario: GenerateResult uses GenerateResponse
- **WHEN** `GenerateResult` is defined
- **THEN** its `Response` field SHALL be `*GenerateResponse` (not `*ResponseMetadata`)

### Requirement: StreamResult response unchanged
The `StreamResult.Response` field SHALL remain `*ResponseHeaders` (containing only `Headers map[string]string`). This is already V4-aligned.

#### Scenario: StreamResult response type
- **WHEN** `StreamResult` is defined
- **THEN** its `Response` field SHALL be `*ResponseHeaders`

### Requirement: specificationVersion returns v4
The `SpecificationVersion()` method on all `LanguageModel` implementations SHALL return `"v4"`.

#### Scenario: Anthropic provider version
- **WHEN** `SpecificationVersion()` is called on the Anthropic model
- **THEN** it SHALL return `"v4"`

#### Scenario: Fallback model version
- **WHEN** `SpecificationVersion()` is called on a fallback model
- **THEN** it SHALL delegate to the first candidate and return its version

### Requirement: Served provider is exposed on response metadata

Providers SHALL set the served provider identifier on the response metadata for both the generate and stream paths so that consumers (e.g. metrics) can attribute a call to the provider that actually handled it, including after a fallback switches candidates.

#### Scenario: Generate path carries provider
- **WHEN** a provider returns a `GenerateResult`
- **THEN** `GenerateResult.Response.Provider` SHALL be set to the provider that served the request

#### Scenario: Stream path carries provider
- **WHEN** a provider emits a `PartResponseMeta` stream part
- **THEN** `StreamPart.Provider` SHALL be set to the provider that served the request

#### Scenario: Orchestration exposes served provider
- **WHEN** `StreamText` processes a `PartResponseMeta` stream part
- **THEN** it SHALL copy `StreamPart.Provider` into the step's `ResponseMetadata.Provider`, and `StreamTextResult.Response()` SHALL report that provider

#### Scenario: Fallback forwards served provider without modification
- **WHEN** a `fallback.Model` fails over to a non-primary candidate and that candidate serves the request
- **THEN** the response/stream metadata SHALL carry the serving candidate's provider, because the fallback wrapper forwards the candidate's output verbatim

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

