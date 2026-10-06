## ADDED Requirements

### Requirement: Output interface on StreamTextParams

The system SHALL provide an `Output` interface in the root `aisdk` package that can be set on `StreamTextParams`. When `Output` is set, `StreamText`/`GenerateText` SHALL use its `ResponseFormat()` to configure the provider call and its `ParseComplete()` to validate the final response. The `Output` interface SHALL be sealed via an unexported marker method.

#### Scenario: Output sets provider ResponseFormat

- **WHEN** `StreamTextParams.Output` is set to an `Output` implementation
- **THEN** the `CallOptions.ResponseFormat` sent to `model.DoStream` SHALL be the value returned by `Output.ResponseFormat()`

#### Scenario: Output takes precedence over explicit ResponseFormat

- **WHEN** both `StreamTextParams.Output` and `StreamTextParams.ResponseFormat` are set
- **THEN** the `Output.ResponseFormat()` value SHALL take precedence

#### Scenario: No Output specified

- **WHEN** `StreamTextParams.Output` is nil
- **THEN** behavior SHALL be identical to current `StreamText`/`GenerateText` with no structured output

### Requirement: Object output mode

The system SHALL provide an `ObjectOutput[T]` implementation that generates a JSON schema from the type parameter, sends it to the provider as a JSON response format, and validates the LLM response against the schema. The parsed result SHALL be accessible as a typed value.

#### Scenario: Generate a typed object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns valid JSON matching the schema
- **THEN** `output.Value[Recipe](result)` SHALL return the parsed `Recipe` value with no error

#### Scenario: LLM returns invalid JSON for object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns JSON that does not match the schema
- **THEN** `output.Value[Recipe](result)` SHALL return an error wrapping `ErrNoObjectGenerated`
- **AND** `result.Text()` SHALL still contain the raw LLM response

#### Scenario: LLM returns unparseable text for object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns text that is not valid JSON
- **THEN** `output.Value[Recipe](result)` SHALL return an error wrapping `ErrNoObjectGenerated` with a JSON parse error as cause

### Requirement: Array output mode

The system SHALL provide an `ArrayOutput[T]` implementation that wraps the element schema in an outer object (`{"elements": [...]}`) for the provider, and unwraps the response transparently. Each element SHALL be validated against the element schema.

#### Scenario: Generate an array of typed elements

- **WHEN** `Output` is set to `output.Array[City](elementSchema)` and the LLM returns valid JSON with a wrapped array
- **THEN** `output.Value[[]City](result)` SHALL return the parsed slice of `City` values

#### Scenario: LLM returns array with invalid element

- **WHEN** the LLM returns JSON where one element does not match the element schema
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

### Requirement: Choice output mode

The system SHALL provide a `ChoiceOutput` implementation that wraps the options in an outer object (`{"result": "..."}`) with an enum constraint, and unwraps the response to return the selected string.

#### Scenario: Generate a choice from options

- **WHEN** `Output` is set to `output.Choice("sunny", "rainy", "snowy")` and the LLM returns `{"result": "sunny"}`
- **THEN** `output.Value[string](result)` SHALL return `"sunny"`

#### Scenario: LLM returns value not in options

- **WHEN** the LLM returns `{"result": "cloudy"}` which is not in the option set
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

### Requirement: JSON output mode

The system SHALL provide a `JSONOutput` implementation that requests JSON mode from the provider without a schema constraint. The response SHALL be validated as parseable JSON but not against any schema.

#### Scenario: Generate unstructured JSON

- **WHEN** `Output` is set to `output.JSON()` and the LLM returns valid JSON
- **THEN** `output.Value[any](result)` SHALL return the parsed JSON value (map, slice, string, number, bool, or nil)

#### Scenario: LLM returns invalid JSON in JSON mode

- **WHEN** `Output` is set to `output.JSON()` and the LLM returns text that is not valid JSON
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

### Requirement: Output validation on final step only

The system SHALL run `Output.ParseComplete()` only when the final step's `FinishReason` is `"stop"`. If the LLM stops for any other reason (length, content filter, tool calls), no output SHALL be produced.

#### Scenario: Finish reason is stop

- **WHEN** the final step completes with `FinishReason == "stop"` and `Output` is set
- **THEN** the system SHALL call `ParseComplete` on the accumulated text and store the result

#### Scenario: Finish reason is not stop

- **WHEN** the final step completes with `FinishReason != "stop"` and `Output` is set
- **THEN** no output SHALL be produced and `result.Output()` SHALL return nil

### Requirement: Structured output with tools

The system SHALL support combining structured output with tool calling in the same request. Structured output generation counts as a step in the multi-step execution model.

#### Scenario: Tools and structured output together

- **WHEN** `StreamTextParams` has both `Tools` and `Output` set, and `StopWhen` allows multiple steps
- **THEN** the system SHALL execute tool calls in earlier steps and produce structured output in the final step when the LLM finishes with `FinishReason == "stop"`

### Requirement: Partial object streaming

The system SHALL provide a `PartialOutputStream()` method on `StreamTextResult` that returns a channel of `json.RawMessage` values representing partial JSON snapshots of the object being generated. Partial parsing failures SHALL be silently skipped.

#### Scenario: Receive partial objects during streaming

- **WHEN** `Output` is set and the LLM streams text deltas that form partial JSON
- **THEN** `PartialOutputStream()` SHALL emit `json.RawMessage` values for each successfully parsed partial snapshot
- **AND** only emit when the parsed result differs from the previous emission

#### Scenario: No Output set

- **WHEN** `Output` is nil
- **THEN** `PartialOutputStream()` SHALL return a closed channel immediately

### Requirement: Array element streaming

The system SHALL provide an `ElementStream()` method on `StreamTextResult` that returns a channel of `json.RawMessage` values, where each value is a complete validated array element. This SHALL only emit elements for array output mode.

#### Scenario: Receive validated elements during array streaming

- **WHEN** `Output` is set to array mode and the LLM streams array elements
- **THEN** `ElementStream()` SHALL emit each complete element as a `json.RawMessage` after it passes schema validation
- **AND** incomplete trailing elements SHALL NOT be emitted

#### Scenario: Non-array output mode

- **WHEN** `Output` is set to object, choice, or json mode
- **THEN** `ElementStream()` SHALL return a closed channel immediately

### Requirement: Typed result access via generic functions

The system SHALL provide generic free functions in the `output` package for type-safe access to structured output results: `Value[T]` for final results, and `TypedElementStream[T]` for array elements.

#### Scenario: Value with correct type

- **WHEN** `output.Value[Recipe](result)` is called and the stored output is a `Recipe`
- **THEN** it SHALL return the typed value with no error

#### Scenario: Value with type mismatch

- **WHEN** `output.Value[User](result)` is called but the stored output is a `Recipe`
- **THEN** it SHALL return an error indicating type mismatch

#### Scenario: TypedElementStream

- **WHEN** `output.TypedElementStream[City](result)` is called with array output mode
- **THEN** it SHALL return a channel that emits each element unmarshaled into `City`

### Requirement: Convenience wrappers GenerateObject and StreamObject

The system SHALL provide `GenerateObject[T]()` and `StreamObject[T]()` generic functions that wrap `GenerateText`/`StreamText` respectively. These SHALL set the `Output` field internally and return typed result wrappers.

#### Scenario: GenerateObject returns typed result

- **WHEN** `output.GenerateObject[Recipe](ctx, params, objectOutput)` is called
- **THEN** it SHALL call `GenerateText` with `Output` set and return an `ObjectResult[T]` with a typed `Object()` accessor

#### Scenario: StreamObject returns typed streaming result

- **WHEN** `output.StreamObject[Recipe](ctx, params, objectOutput)` is called
- **THEN** it SHALL call `StreamText` with `Output` set and return a `StreamObjectResult[T]` with typed partial and element stream accessors

### Requirement: ErrNoObjectGenerated sentinel error

The system SHALL define `ErrNoObjectGenerated` as a sentinel error in the root `aisdk` package. All output validation failures SHALL wrap this error. The error context SHALL preserve the raw text, response metadata, and usage information.

#### Scenario: Check error type

- **WHEN** structured output validation fails
- **THEN** `errors.Is(err, aisdk.ErrNoObjectGenerated)` SHALL return true
- **AND** the raw LLM text SHALL be accessible from the result via `result.Text()`
