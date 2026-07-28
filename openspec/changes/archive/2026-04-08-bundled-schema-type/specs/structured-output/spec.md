## MODIFIED Requirements

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
