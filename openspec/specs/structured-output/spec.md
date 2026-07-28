# structured-output Specification

## Purpose

Define structured output configuration, parsing and validation behavior, partial snapshot and array element streaming guarantees, typed result access, and object-generation convenience APIs for `StreamText` and `GenerateText`.

## Requirements

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

### Requirement: Object output mode

The system SHALL provide an `ObjectOutput[T]` implementation that accepts a `schema.Schema` value, uses its `.JSON()` for the provider response format, and uses its `.Validate()` for validating the LLM response. The parsed result SHALL be accessible as a typed value. Since the `Schema` is pre-compiled, `output.Object[T]()` SHALL NOT return an error -- construction cannot fail when given a valid `Schema`.

#### Scenario: Generate a typed object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` where `schema` is a `schema.Schema` and the LLM returns valid JSON matching the schema
- **THEN** `output.Value[Recipe](result)` SHALL return the parsed `Recipe` value with no error

#### Scenario: LLM returns invalid JSON for object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns JSON that does not match the schema
- **THEN** `output.Value[Recipe](result)` SHALL return an error wrapping `ErrNoObjectGenerated`
- **AND** `result.Text()` SHALL still contain the raw LLM response

#### Scenario: LLM returns unparseable text for object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns text that is not valid JSON
- **THEN** `output.Value[Recipe](result)` SHALL return an error wrapping `ErrNoObjectGenerated` with a JSON parse error as cause

### Requirement: Array output mode

The system SHALL provide an `ArrayOutput[T]` implementation that accepts a `schema.Schema` for the element type. The element schema's `.JSON()` SHALL be wrapped in an outer object (`{"elements": [...]}`) for the provider. The wrapper schema SHALL be constructed as a new `schema.Schema` internally via `schema.SchemaFromJSON` for validation. Each element SHALL be validated against the element schema.

#### Scenario: Generate an array of typed elements

- **WHEN** `Output` is set to `output.Array[City](elementSchema)` where `elementSchema` is a `schema.Schema` and the LLM returns valid JSON with a wrapped array
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

### Requirement: Final output parsing follows operation semantics

When `Output` is set, `StreamText` SHALL run `Output.ParseComplete()` on the final step's accumulated text independently of its finish reason. Successful parsing SHALL populate `OutputValue`; parse or validation failure SHALL populate `OutputError`. `GenerateText` SHALL run final output parsing only when the final step's unified finish reason is `stop`, matching the upstream non-streaming operation.

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

#### Scenario: GenerateText parses output after a stop finish

- **WHEN** the final `GenerateText` step completes with `FinishReasonStop` and `Output` is set
- **THEN** the system SHALL call `ParseComplete` on the accumulated text and store the result

#### Scenario: GenerateText does not parse output after a non-stop finish

- **WHEN** the final `GenerateText` step completes with a unified finish reason other than `stop` and `Output` is set
- **THEN** `Output` and `OutputError` on `GenerateTextResult` SHALL both be nil

### Requirement: Structured output with tools

The system SHALL support combining structured output with tool calling in the same request. Structured output generation counts as a step in the multi-step execution model.

#### Scenario: Tools and structured output together

- **WHEN** `StreamTextParams` has both `Tools` and `Output` set, and `StopWhen` allows multiple steps
- **THEN** the system SHALL execute tool calls in earlier steps and produce structured output in the final step when the LLM finishes with `FinishReason == "stop"`

### Requirement: Partial output streaming

The system SHALL provide a `PartialOutputStream()` method on `StreamTextResult` that returns a channel of `json.RawMessage` values representing partial snapshots produced by the configured output mode. Partial parsing failures SHALL be silently skipped. Every distinct successfully parsed snapshot SHALL be delivered exactly once and in production order. The stream SHALL remain lossless when consumption starts after generation, and SHALL close only after generation finishes and all queued snapshots are delivered.

#### Scenario: Receive partial output during streaming

- **WHEN** `Output` is set and the LLM streams text deltas that form partial JSON
- **THEN** `PartialOutputStream()` SHALL emit `json.RawMessage` values for each successfully parsed partial snapshot
- **AND** only emit when the parsed result differs from the previous emission
- **AND** deliver each emitted snapshot exactly once and in production order

#### Scenario: Partial output consumption starts after the channel buffer is exceeded

- **WHEN** generation produces more distinct partial snapshots than the public channel buffer can hold before the consumer starts reading
- **THEN** `PartialOutputStream()` SHALL deliver every snapshot exactly once and in production order
- **AND** structured output generation and result completion SHALL NOT block on the delayed consumer

#### Scenario: No Output set

- **WHEN** `Output` is nil
- **THEN** `PartialOutputStream()` SHALL return a closed channel immediately

### Requirement: Array element streaming

The system SHALL provide an `ElementStream()` method on `StreamTextResult` that returns a channel of `json.RawMessage` values, where each value is a complete validated array element. This SHALL only emit elements for array output mode. Every completed element SHALL be delivered exactly once and in array order. The stream SHALL remain lossless when consumption starts after generation, and SHALL close only after generation finishes and all queued elements are delivered.

#### Scenario: Receive validated elements during array streaming

- **WHEN** `Output` is set to array mode and the LLM streams array elements
- **THEN** `ElementStream()` SHALL emit each complete element as a `json.RawMessage` after it passes schema validation
- **AND** incomplete trailing elements SHALL NOT be emitted
- **AND** each completed element SHALL be delivered exactly once and in array order

#### Scenario: Element consumption starts after the channel buffer is exceeded

- **WHEN** generation completes more array elements than the public channel buffer can hold before the consumer starts reading
- **THEN** `ElementStream()` SHALL deliver every completed element exactly once and in array order
- **AND** structured output generation and result completion SHALL NOT block on the delayed consumer

#### Scenario: Non-array output mode

- **WHEN** `Output` is set to object, choice, or json mode
- **THEN** `ElementStream()` SHALL emit no values
- **AND** the channel SHALL close when generation finishes

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
