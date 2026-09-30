## MODIFIED Requirements

### Requirement: JSON output mode

The system SHALL provide a `JSONOutput` implementation that requests JSON mode from the provider without a schema constraint. The response SHALL be validated as parseable JSON but not against any schema.

#### Scenario: Generate unstructured non-null JSON

- **WHEN** `Output` is set to `output.JSON()` and the LLM returns valid non-null JSON
- **THEN** `output.Value[any](result)` through an `OutputAccessor` wrapper SHALL return the parsed JSON value (map, slice, string, number, or bool)

#### Scenario: Generate JSON null

- **WHEN** `Output` is set to `output.JSON()` and the final generated response is `null` with a `stop` finish
- **THEN** `ParseComplete("null")` SHALL succeed with a nil value and the generated result SHALL have nil `Output` and nil `OutputError`
- **AND** `output.Value[any]` through an `OutputAccessor` wrapper of the generated result SHALL report missing output

#### Scenario: LLM returns invalid JSON in JSON mode

- **WHEN** `Output` is set to `output.JSON()` and the LLM returns text that is not valid JSON
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

### Requirement: Final output parsing follows operation semantics

When `Output` is set, `StreamText` SHALL run `Output.ParseComplete()` on the final step's accumulated text independently of its finish reason. Successful parsing SHALL populate `OutputValue`; parse or validation failure SHALL populate `OutputError`. `GenerateText` and `ToolLoopAgent.Generate` SHALL run `Output.ParseComplete()` on the final step's accumulated text if the unified finish reason is `stop`, or if it is not `tool-calls` and the accumulated text is nonempty. Generated calls SHALL NOT parse an empty non-`stop` response or any `tool-calls` response. For generated calls, successful parsing SHALL populate `GenerateTextResult.Output` with the parsed value and leave `OutputError` nil; parse or validation failure SHALL leave `Output` nil and populate `OutputError`, without failing the generated call itself. For the built-in object, array, choice, and JSON modes, that failure SHALL wrap `ErrNoObjectGenerated`. If parsing is skipped, `Output` and `OutputError` SHALL both remain nil. The final text and unified finish reason SHALL remain available in all cases. Through an `output.OutputAccessor` wrapper of the generated result (such as `output.ObjectResult[T]`), `output.Value[T]` SHALL return a non-nil typed output when one is available and an explicit error on parse failure or missing output; `GenerateTextResult` itself has fields, not the accessor methods. Raw JSON `null` is a successful parse with nil `Output` and nil `OutputError`, but the existing typed accessor reports it as missing output. This applies to object, array, choice, and JSON output modes.

#### Scenario: StreamText parses valid output after a length finish

- **WHEN** the final `StreamText` step completes with `FinishReasonLength` and accumulated text that is valid for the configured `Output`
- **THEN** the system SHALL call `ParseComplete` on the accumulated text
- **AND** `OutputValue()` SHALL return the parsed value
- **AND** `OutputError()` SHALL return nil

#### Scenario: StreamText exposes invalid output after a length finish

- **WHEN** the final `StreamText` step completes with `FinishReasonLength` and truncated or invalid text for the configured `Output`
- **THEN** `OutputValue()` SHALL return nil
- **AND** `OutputError()` SHALL return an error wrapping `ErrNoObjectGenerated`
- **AND** `Text()` SHALL retain the raw accumulated response

#### Scenario: Generated stop finish attempts complete parsing even with empty text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `stop` with built-in object, array, choice, or JSON output configured, whether its text is valid, invalid, or empty
- **THEN** the system SHALL call `ParseComplete` on that text
- **AND** valid text (non-null for JSON mode) SHALL yield the complete output with nil `OutputError`, while invalid or empty text SHALL yield nil output and an `OutputError` wrapping `ErrNoObjectGenerated`

#### Scenario: Generated non-tool-call non-stop finish parses nonempty valid text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `length`, `content-filter`, `error`, or `other` with nonempty valid object, array, or choice output text, or non-null valid JSON output text
- **THEN** the system SHALL call `ParseComplete` on that text
- **AND** `Output` SHALL contain the validated value and `OutputError` SHALL be nil
- **AND** `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL return the value without error

#### Scenario: Generated non-tool-call non-stop finish reports invalid text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `length`, `content-filter`, `error`, or `other` with nonempty malformed JSON or JSON invalid for the configured object, array, or choice schema
- **THEN** the system SHALL call `ParseComplete` on that text
- **AND** `Output` SHALL be nil and both `OutputError` and the error from `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL wrap `ErrNoObjectGenerated`
- **AND** the generated call SHALL still return its result, including its raw text and finish reason

#### Scenario: Generated non-stop finish with empty text skips complete parsing

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `length`, `content-filter`, `error`, or `other` with empty text
- **THEN** the system SHALL NOT call `ParseComplete`
- **AND** `Output` and `OutputError` SHALL both be nil
- **AND** `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL report missing output

#### Scenario: Generated tool-calls finish skips complete parsing regardless of text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `tool-calls` with valid, invalid, or empty text
- **THEN** the system SHALL NOT call `ParseComplete`
- **AND** `Output` and `OutputError` SHALL both be nil
- **AND** `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL report missing output

#### Scenario: Generated output comes from the final step

- **WHEN** a generated call continues after an earlier tool step and its final step contains structured output
- **THEN** complete output parsing SHALL use the final step's accumulated text and finish reason, not the earlier step's
