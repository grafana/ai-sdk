## ADDED Requirements

### Requirement: RequestMetadata uses provider type directly
The `aisdk` package SHALL NOT define its own `RequestMetadata` type. All references to request metadata in `aisdk` SHALL use `provider.RequestMetadata` directly.

#### Scenario: StepResult uses provider.RequestMetadata
- **WHEN** a `StepResult` struct is constructed
- **THEN** its `Request` field SHALL be of type `provider.RequestMetadata`

#### Scenario: No aisdk.RequestMetadata type exists
- **WHEN** the `aisdk` package is compiled
- **THEN** no `RequestMetadata` type definition SHALL exist in the root package

### Requirement: ResponseMetadata embeds provider.ResponseMetadata
The `aisdk.ResponseMetadata` type SHALL embed `provider.ResponseMetadata` instead of duplicating its fields. The only additional field SHALL be `Headers map[string]string`.

#### Scenario: Field access through embedding
- **WHEN** accessing `ID`, `ModelID`, or `Timestamp` on `aisdk.ResponseMetadata`
- **THEN** the values SHALL be promoted from the embedded `provider.ResponseMetadata`

#### Scenario: JSON serialization unchanged
- **WHEN** `aisdk.ResponseMetadata` is marshaled to JSON
- **THEN** the output SHALL contain the same fields (`id`, `modelId`, `timestamp`, `headers`) with the same structure as before the refactoring

#### Scenario: No Messages field
- **WHEN** inspecting `aisdk.ResponseMetadata`
- **THEN** no `Messages` field SHALL exist on the type

### Requirement: ConvertToModelMessages does not accept context
The `ConvertToModelMessages` function SHALL NOT accept a `context.Context` parameter. Its signature SHALL be `func ConvertToModelMessages(messages []UIMessage, opts ...ConvertOptions) ([]provider.Message, error)`.

#### Scenario: Direct call without context
- **WHEN** calling `ConvertToModelMessages`
- **THEN** the caller SHALL pass `messages` as the first argument (not a context)

#### Scenario: All callers updated
- **WHEN** any code in the repository calls `ConvertToModelMessages`
- **THEN** no `context.Context` argument SHALL be passed
