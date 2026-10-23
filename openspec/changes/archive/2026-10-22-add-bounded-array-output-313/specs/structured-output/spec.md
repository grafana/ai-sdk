## MODIFIED Requirements

### Requirement: Array output mode

The system SHALL provide ArrayOutput[T] accepting element schema.Schema. Its JSON SHALL be wrapped as {"elements": [...]} in a new schema.Schema via schema.SchemaFromJSON. This SHALL remain the strict provider format with additionalProperties false and elements required. `WithMinItems` and `WithMaxItems` SHALL be `ArrayOption`s that only `Array` accepts, so passing them to `Object`, `ChoiceWithOptions` or `JSON` fails to compile; every `ObjectOption` SHALL also be an `ArrayOption`. They SHALL set optional `minItems` and `maxItems` keywords on the `elements` property; an unset bound SHALL be omitted and zero SHALL be sent as an explicit bound. `Array` SHALL return an error, before any model call, when a bound is negative or the minimum exceeds the maximum.

#### Scenario: Generate an array of typed elements

- **WHEN** `Output` is set to `output.Array[City](elementSchema)` where `elementSchema` is a `schema.Schema` and the LLM returns valid JSON with a wrapped array
- **THEN** `output.Value[[]City](result)` SHALL return the parsed slice of `City` values

#### Scenario: LLM returns array with invalid element

- **WHEN** the LLM returns JSON where one element does not match the element schema
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

#### Scenario: LLM adds an unrelated wrapper property

- **WHEN** the LLM returns `{"elements":[{"name":"Paris","population":2161000}],"extra":true}`
- **THEN** parsing SHALL succeed and return the single `City` element

#### Scenario: LLM omits the elements array

- **WHEN** the LLM returns `{}`, `{"elements":null}`, `{"elements":{"name":"Paris"}}` or a document that is not a JSON object
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

#### Scenario: Bounds reach the request schema

- **WHEN** `output.Array[City](elementSchema, output.WithMinItems(1), output.WithMaxItems(2))` is used
- **THEN** the response format's `elements` property SHALL contain `"minItems":1` and `"maxItems":2`

#### Scenario: Invalid bounds

- **WHEN** a bound is negative or `WithMinItems` exceeds `WithMaxItems`
- **THEN** `output.Array` SHALL return a nil output and an error

#### Scenario: Final array outside the bounds

- **WHEN** the LLM returns an elements array shorter than the minimum or longer than the maximum
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated` and `ErrInvalidOutputText` that names the violated bound
- **AND** the length SHALL be checked before element validation

### Requirement: Array element streaming

The system SHALL provide an `ElementStream()` method on `StreamTextResult` that returns a channel of `json.RawMessage` values, where each value is a complete validated array element. This SHALL only emit elements for array output mode. Every completed element SHALL be delivered exactly once and in array order. The stream SHALL remain lossless when consumption starts after generation, and SHALL close only after generation finishes and all queued elements are delivered, except that for a `BoundedArrayOutput` with a maximum the stream SHALL deliver at most that many elements and SHALL close as soon as an element past the maximum is parsed in any step, and `ElementStreamError()` SHALL then return an error wrapping `ErrInvalidOutputText`. A negative maximum reported by a custom `BoundedArrayOutput` SHALL be treated as zero.

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

#### Scenario: Model streams more elements than the maximum

- **WHEN** the array output sets `WithMaxItems(2)` and the LLM streams four elements
- **THEN** `ElementStream()` SHALL deliver only the first two elements
- **AND** the channel SHALL close once the third element is parsed, before generation finishes
- **AND** `ElementStreamError()` SHALL report the overflow once the channel is closed
- **AND** `PartialOutputStream()` SHALL still include all four elements
- **AND** `OutputError()` SHALL wrap `ErrNoObjectGenerated` and report the maximum when this is the final step

#### Scenario: Later step after an overflow

- **WHEN** an earlier step overflowed the maximum and a later step streams a new array
- **THEN** `ElementStream()` SHALL deliver no further elements
- **AND** `ElementStreamError()` SHALL report the overflow even when `OutputError()` is nil

#### Scenario: Repair accepts an array within the bounds

- **WHEN** the LLM streams more elements than the maximum and `WithRepairText` accepts text whose array is within the bounds
- **THEN** `OutputError()` SHALL be nil and the output SHALL be the repaired array
- **AND** `ElementStream()` SHALL still have delivered only elements from the original text, up to the maximum
- **AND** `ElementStreamError()` SHALL report the overflow
