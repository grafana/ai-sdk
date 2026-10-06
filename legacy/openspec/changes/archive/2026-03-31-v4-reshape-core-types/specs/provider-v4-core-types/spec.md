## ADDED Requirements

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
The `provider.ResponseMetadata` struct SHALL contain only `ID string`, `Timestamp time.Time`, and `ModelID string`. The `Headers` and `Body` fields SHALL NOT exist on `ResponseMetadata`.

#### Scenario: ResponseMetadata struct shape
- **WHEN** `ResponseMetadata` is defined in `provider/language_model.go`
- **THEN** it SHALL have exactly three fields: `ID`, `ModelID`, `Timestamp`

#### Scenario: Provider constructs ResponseMetadata
- **WHEN** a provider returns response metadata
- **THEN** it SHALL construct `ResponseMetadata{ID: ..., ModelID: ...}` without Headers or Body

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
