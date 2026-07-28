## MODIFIED Requirements

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

## ADDED Requirements

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
