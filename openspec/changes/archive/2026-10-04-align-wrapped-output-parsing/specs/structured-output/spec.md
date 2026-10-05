## MODIFIED Requirements

### Requirement: Array output mode

The system SHALL provide an `ArrayOutput[T]` implementation that accepts a `schema.Schema` for the element type. The element schema's `.JSON()` SHALL be wrapped in an outer object (`{"elements": [...]}`) for the provider. The wrapper schema SHALL be constructed as a new `schema.Schema` internally via `schema.SchemaFromJSON` and SHALL remain the strict response format, with `additionalProperties` false and `elements` required. Complete parsing SHALL extract the value at runtime instead of validating the wrapper schema: the response SHALL be a JSON object that contains an `elements` array, and each element SHALL be validated against the element schema. A wrapper property beyond `elements` SHALL NOT fail parsing. A response that is not an object, that omits `elements`, or whose `elements` value is null or not an array SHALL return an error wrapping `ErrNoObjectGenerated`.

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

### Requirement: Choice output mode

The system SHALL provide a `ChoiceOutput` implementation that wraps the options in an outer object (`{"result": "..."}`) with an enum constraint, and unwraps the response to return the selected string. The wrapper schema SHALL remain the strict response format, with `additionalProperties` false and `result` required. Complete parsing SHALL extract the value at runtime instead of validating the wrapper schema: the response SHALL be a JSON object whose `result` is a string within the option set, and a wrapper property beyond `result` SHALL NOT fail parsing. Partial parsing SHALL publish nothing unless the parsed snapshot is an object whose `result` is present and holds a string; a missing, null or non-string `result` SHALL NOT be read as the empty string. Given a present string, a successfully parsed snapshot SHALL publish only an exact option and a repaired snapshot SHALL publish only a unique prefix match.

#### Scenario: Generate a choice from options

- **WHEN** `Output` is set to `output.Choice("sunny", "rainy", "snowy")` and the LLM returns `{"result": "sunny"}`
- **THEN** `output.Value[string](result)` SHALL return `"sunny"`

#### Scenario: LLM returns value not in options

- **WHEN** the LLM returns `{"result": "cloudy"}` which is not in the option set
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

#### Scenario: LLM adds an unrelated wrapper property

- **WHEN** the LLM returns `{"result":"sunny","extra":true}`
- **THEN** parsing SHALL succeed and return `"sunny"`

#### Scenario: Partial snapshot carries no choice yet

- **WHEN** a single-option choice receives the partial snapshots `{`, `{"other":`, `{"result":null,` or `{"result":1}`
- **THEN** partial parsing SHALL publish no value

#### Scenario: Partial snapshot carries a unique prefix

- **WHEN** `output.Choice("sunny", "rainy", "snowy")` receives the partial snapshot `{"result":"rai`
- **THEN** partial parsing SHALL publish `"rainy"`
