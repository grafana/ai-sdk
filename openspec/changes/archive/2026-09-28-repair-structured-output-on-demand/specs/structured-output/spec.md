## ADDED Requirements

### Requirement: Opt-in repair of invalid complete structured output

The system SHALL expose an optional `aisdk.WithRepairText` option usable with both `GenerateText` and `StreamText`, including their `output.GenerateObject` and `output.StreamObject` wrappers. The callback SHALL receive the original generated text and the error returned by `Output.ParseComplete` and SHALL return repaired text, an acceptance indicator, and an error. Only when an eligible final complete parse returns an error wrapping both `aisdk.ErrNoObjectGenerated` and `aisdk.ErrInvalidOutputText` SHALL the system invoke a configured callback, exactly once. `ErrInvalidOutputText` SHALL indicate JSON syntax or schema-validation failure of generated text, not a Go typed-conversion failure after schema validation succeeds. If accepted, it SHALL call the configured `Output.ParseComplete` again on the returned text, once, without invoking repair again. This SHALL apply to schema and JSON validation of Object, Array, Choice, and JSON output modes, as well as custom `Output` implementations that wrap both sentinels for repairable text failures; it SHALL NOT require changing the three-method `Output` interface or the existing stop/non-stop parse eligibility.

#### Scenario: Repair malformed JSON through GenerateObject
- **WHEN** `output.GenerateObject[T]` is passed `aisdk.WithRepairText` and the model returns malformed JSON for an `output.Object[T]` that the callback repairs to schema-valid JSON
- **THEN** the callback SHALL receive exactly the original generated text and the original parse error wrapping both `ErrNoObjectGenerated` and `ErrInvalidOutputText`
- **AND** `Object()` SHALL return the typed value from validated repaired text without an output error

#### Scenario: Repair schema-invalid JSON through StreamObject
- **WHEN** `output.StreamObject[T]` is passed `aisdk.WithRepairText` and the model returns valid JSON that fails the configured object schema but the callback returns schema-valid JSON
- **THEN** the callback SHALL receive exactly the original text and schema validation error wrapping both `ErrNoObjectGenerated` and `ErrInvalidOutputText`
- **AND** `Object()` SHALL return the typed validated repaired value without an output error

#### Scenario: Shared option on direct calls and other output modes
- **WHEN** direct `GenerateText` or `StreamText` configures `WithOutput` with Object, Array, Choice, JSON, or a custom `Output` wrapping both `ErrNoObjectGenerated` and `ErrInvalidOutputText`, along with `WithRepairText`, and the callback accepts repaired text valid for that output
- **THEN** `Output` or `OutputValue()` SHALL contain the parsed and validated repaired value
- **AND** the same output's `ResponseFormat` and normal parse/validation rules SHALL be used for the original and repaired text

#### Scenario: No repair configured
- **WHEN** `output.GenerateObject` or `output.StreamObject` is called without `WithRepairText` and its configured output rejects the model text
- **THEN** `Object()` SHALL return the original output error wrapping `ErrNoObjectGenerated` without an extra parse attempt

#### Scenario: Declined repair
- **WHEN** the callback returns `accepted == false` with no callback error after a validation failure
- **THEN** `OutputError` and the typed accessor SHALL return the original parse/validation error
- **AND** the system SHALL NOT parse returned text

#### Scenario: Repair callback fails
- **WHEN** the callback returns an error after an eligible parse/validation failure
- **THEN** `OutputError` and the typed accessor SHALL return the callback error
- **AND** the system SHALL NOT parse returned text again

#### Scenario: Accepted repaired output remains invalid
- **WHEN** the callback accepts text that fails the configured output's complete parser or schema validation
- **THEN** `OutputError` and the typed accessor SHALL return the repaired text's parse/validation error wrapping `ErrNoObjectGenerated`
- **AND** the callback SHALL NOT run a second time

#### Scenario: Schema-valid text fails Go typed conversion
- **WHEN** Object or Array schema validation succeeds but typed `json.Unmarshal` into `T` fails (including a custom unmarshaler failure), with `WithRepairText` configured
- **THEN** `OutputError` SHALL still wrap `ErrNoObjectGenerated` but SHALL NOT wrap `ErrInvalidOutputText`
- **AND** the callback SHALL NOT run and the original typed-conversion error SHALL be returned

#### Scenario: No eligible validation error
- **WHEN** original complete parsing succeeds, the output is nil, a custom output returns an error lacking either required sentinel, or complete parsing is not eligible under existing finish-reason rules
- **THEN** the callback SHALL NOT run
- **AND** original result behavior SHALL remain unchanged

### Requirement: Repair preserves original model response and inspectable errors

The system SHALL preserve original generated model text, content, full/UI message stream chunks, partial and array-element streams, response metadata, and usage when attempting repair; only final structured `OutputValue` and `OutputError` SHALL be affected. Builtin output parse/validation failures SHALL retain the underlying parse or schema error in the Go error chain while still wrapping `ErrNoObjectGenerated`; repairable JSON/schema failures SHALL also wrap `ErrInvalidOutputText`, so callers can inspect the failure cause and eligibility separately.

#### Scenario: Repaired output does not rewrite raw result
- **WHEN** a callback repairs generated text to a valid final value
- **THEN** the final typed value SHALL reflect the repaired text
- **AND** `Text`, `Content`, `Steps`, `Response`, `Usage` and `TotalUsage` SHALL reflect the original provider response
- **AND** text, partial-output and UI chunks SHALL NOT be replaced with repaired text or synthesized from it

#### Scenario: Inspect original failure cause
- **WHEN** a builtin structured output rejects malformed JSON or schema-invalid JSON
- **THEN** the callback's error SHALL satisfy `errors.Is(err, aisdk.ErrNoObjectGenerated)` and `errors.Is(err, aisdk.ErrInvalidOutputText)`
- **AND** the underlying JSON parse or schema validation error SHALL be discoverable with `errors.As`
