## MODIFIED Requirements

### Requirement: Output interface on StreamTextParams

The system SHALL provide an `Output` interface in the root `aisdk` package that can be set on `StreamTextParams`. When `Output` is set, `StreamText`/`GenerateText` SHALL use its `ResponseFormat()` to configure the provider call and its `ParseComplete()` to validate the final response. The interface SHALL consist of three methods: `ResponseFormat() *provider.ResponseFormat`, `ParseComplete(text string) (any, error)`, and `ParsePartial(text string) (any, bool)`. No marker method SHALL be included.

#### Scenario: Output sets provider ResponseFormat

- **WHEN** `StreamTextParams.Output` is set to an `Output` implementation
- **THEN** the `CallOptions.ResponseFormat` sent to `model.DoStream` SHALL be the value returned by `Output.ResponseFormat()`

#### Scenario: Output takes precedence over explicit ResponseFormat

- **WHEN** both `StreamTextParams.Output` and `StreamTextParams.ResponseFormat` are set
- **THEN** the `Output.ResponseFormat()` value SHALL take precedence

#### Scenario: No Output specified

- **WHEN** `StreamTextParams.Output` is nil
- **THEN** behavior SHALL be identical to current `StreamText`/`GenerateText` with no structured output
