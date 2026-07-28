## ADDED Requirements

### Requirement: TypedToolDef struct provides typed tool definition

The system SHALL provide a generic struct `TypedToolDef[I, O any]` that defines a tool using Go types instead of raw JSON. The struct SHALL include:

- `Name` (string): tool identifier
- `Description` (string): tool description for the LLM
- `Title` (string, optional): display title
- `Execute` (func): typed execution function `func(ctx context.Context, input I, opts ToolExecutionOptions) (O, error)`
- `OutputSchema` (schema.Schema, optional): explicit output schema override
- `InputExamples` ([]I, optional): typed input examples
- `Strict` (bool, optional): strict mode flag
- `ProviderOptions` (map, optional): provider-specific options
- `ValidateInput` (func, optional): typed input validation `func(input I) error`
- `ToModelOutput` (func, optional): custom model output conversion

#### Scenario: Minimal typed tool definition
- **WHEN** a user creates a `TypedToolDef` with only `Description` and `Execute` set
- **THEN** the definition is valid and sufficient for constructing a tool

#### Scenario: Full typed tool definition
- **WHEN** a user creates a `TypedToolDef` with all optional fields populated
- **THEN** all fields are preserved through construction into the resulting `Tool`

### Requirement: TypedTool function constructs Tool from typed definition

The system SHALL provide a generic function `TypedTool[I, O any](def TypedToolDef[I, O]) (Tool, error)` that constructs a standard `Tool` from a typed definition.

#### Scenario: Successful tool construction
- **WHEN** `TypedTool` is called with a valid `TypedToolDef`
- **THEN** it returns a `Tool` with `InputSchema` derived from type `I` via `schema.SchemaFor[I]()` and a nil error

#### Scenario: Schema derivation failure
- **WHEN** `TypedTool` is called and `schema.SchemaFor[I]()` returns an error
- **THEN** it returns a zero `Tool` and a wrapped error

### Requirement: Input schema auto-derived from Go type

The system SHALL derive the input JSON Schema from the Go type parameter `I` using `schema.SchemaFor[I]()` at construction time. The resulting schema SHALL be set as `Tool.InputSchema`.

#### Scenario: Struct with JSON tags produces correct schema
- **WHEN** type `I` is a struct with `json` and `jsonschema` struct tags
- **THEN** the derived schema reflects the field names, types, and constraints from those tags

#### Scenario: Schema includes required fields
- **WHEN** type `I` is a struct with non-pointer fields
- **THEN** the derived schema marks those fields as required in the JSON Schema

### Requirement: Execute function wraps marshal/unmarshal

The resulting `Tool.Execute` function SHALL unmarshal the `json.RawMessage` input into type `I`, call the typed execute function, and marshal the `O` output back to `json.RawMessage`.

#### Scenario: Successful execution round-trip
- **WHEN** `Tool.Execute` is called with valid JSON matching type `I`
- **THEN** the input is unmarshaled to `I`, the typed execute function is called, and the output `O` is marshaled to `json.RawMessage`

#### Scenario: Input unmarshal failure
- **WHEN** `Tool.Execute` is called with JSON that cannot be unmarshaled to type `I`
- **THEN** it returns a nil result and a wrapped unmarshal error

#### Scenario: Execute function error propagation
- **WHEN** the typed execute function returns an error
- **THEN** `Tool.Execute` returns nil result and that same error without wrapping

#### Scenario: Output marshal failure
- **WHEN** the typed execute function returns a value `O` that cannot be marshaled to JSON
- **THEN** `Tool.Execute` returns a nil result and a marshal error

### Requirement: ValidateInput wraps typed validation

When `TypedToolDef.ValidateInput` is provided, the resulting `Tool.ValidateInput` SHALL unmarshal the `json.RawMessage` input into type `I` and then call the typed validation function.

#### Scenario: Valid input passes typed validation
- **WHEN** `Tool.ValidateInput` is called with valid JSON and the typed validator returns nil
- **THEN** it returns nil

#### Scenario: Invalid input fails typed validation
- **WHEN** `Tool.ValidateInput` is called and the typed validator returns an error
- **THEN** it returns that error

#### Scenario: ValidateInput not provided
- **WHEN** `TypedToolDef.ValidateInput` is nil
- **THEN** the resulting `Tool.ValidateInput` is nil

### Requirement: InputExamples marshaled at construction

When `TypedToolDef.InputExamples` is provided, each typed example SHALL be marshaled to `json.RawMessage` during `TypedTool` construction and set on `Tool.InputExamples`.

#### Scenario: Examples successfully marshaled
- **WHEN** `TypedToolDef.InputExamples` contains valid values of type `I`
- **THEN** each is marshaled to JSON and stored in `Tool.InputExamples`

#### Scenario: Example marshal failure
- **WHEN** an input example cannot be marshaled to JSON
- **THEN** `TypedTool` returns a zero `Tool` and a wrapped error

### Requirement: Optional fields passed through

The `TypedTool` function SHALL pass through optional fields from `TypedToolDef` to the resulting `Tool`: `Description`, `Title`, `OutputSchema`, `Strict`, `ProviderOptions`, and `ToModelOutput`.

#### Scenario: All optional fields preserved
- **WHEN** `TypedToolDef` has `Description`, `Title`, `OutputSchema`, `Strict`, `ProviderOptions`, and `ToModelOutput` set
- **THEN** the resulting `Tool` has identical values for those fields

### Requirement: Low-level Tool unchanged

The existing `Tool` struct, `ToolExecuteFunc`, `ToolExecutionOptions`, and `ToolSet` types SHALL remain unchanged. `TypedTool` is additive only.

#### Scenario: Existing tool code compiles unchanged
- **WHEN** existing code using `Tool` struct directly with `json.RawMessage` schema and execute
- **THEN** it continues to compile and work without modification
