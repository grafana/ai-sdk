## Purpose

Generic helper for type-safe tool definition with automatic JSON Schema derivation from Go types, input unmarshaling, and output marshaling.

## Requirements

### Requirement: TypedToolDef struct provides typed tool definition

The system SHALL provide a generic struct `TypedToolDef[I, O any]` that defines a tool using Go types instead of raw JSON. The struct SHALL include:

- `Name` (string): tool identifier
- `Description` (string): tool description for the LLM
- `Title` (string, optional): display title
- `Execute` (func): typed execution function `func(ctx context.Context, input I, opts ToolExecutionOptions) (O, error)`
- `OutputSchema` (schema.Schema, optional): explicit output schema override
- `InputExamples` ([]I, optional): typed input examples
- `Strict` (*bool, optional): strict mode flag; nil, true, and false remain distinct
- `ProviderOptions` (map, optional): provider-specific options
- `ValidateInput` (func, optional): typed input validation `func(input I) error`
- `ToModelOutput` (func, optional): typed model output conversion `func(toolCallID string, input I, output O) (*provider.ToolResultOutput, error)`
- `OnInputStart` (func, optional): callback when argument streaming starts `func(ToolExecutionOptions)`
- `OnInputDelta` (func, optional): callback for argument streaming deltas `func(inputTextDelta string, opts ToolExecutionOptions)`
- `OnInputAvailable` (func, optional): typed callback when full input is available `func(input I, err error, opts ToolExecutionOptions)`

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
- **WHEN** `TypedTool` is called and `schema.SchemaFor[I]()` returns an error or panics
- **THEN** it returns a zero `Tool` and a wrapped error (panics are recovered and converted to errors)

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
- **THEN** `Tool.Execute` returns a nil result and a wrapped marshal error with context

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

### Requirement: ToModelOutput wraps typed model output conversion

When `TypedToolDef.ToModelOutput` is provided, the resulting `Tool.ToModelOutput` SHALL unmarshal both the input (`json.RawMessage` to `I`) and output (`json.RawMessage` to `O`) from the `ToolOutputContext`, then call the typed callback with the tool call ID, typed input, and typed output.

#### Scenario: Successful typed model output conversion
- **WHEN** `Tool.ToModelOutput` is called with valid JSON for both input and output
- **THEN** both are unmarshaled to their typed forms and the typed callback is called

#### Scenario: Input unmarshal failure in ToModelOutput
- **WHEN** `Tool.ToModelOutput` is called and the input JSON cannot be unmarshaled to `I`
- **THEN** it returns a nil result and a wrapped error

#### Scenario: Output unmarshal failure in ToModelOutput
- **WHEN** `Tool.ToModelOutput` is called and the output JSON cannot be unmarshaled to `O`
- **THEN** it returns a nil result and a wrapped error

#### Scenario: ToModelOutput not provided
- **WHEN** `TypedToolDef.ToModelOutput` is nil
- **THEN** the resulting `Tool.ToModelOutput` is nil

### Requirement: OnInputAvailable wraps typed callback with error reporting

When `TypedToolDef.OnInputAvailable` is provided, the resulting `Tool.OnInputAvailable` SHALL unmarshal the `json.RawMessage` input into type `I` and call the typed callback. On unmarshal failure, the callback is still called with the zero value of `I` and a non-nil error, allowing the caller to observe the failure.

#### Scenario: Successful input available callback
- **WHEN** `Tool.OnInputAvailable` is called with valid JSON matching type `I`
- **THEN** the typed callback receives the unmarshaled input and a nil error

#### Scenario: Input unmarshal failure in OnInputAvailable
- **WHEN** `Tool.OnInputAvailable` is called with invalid JSON
- **THEN** the typed callback receives a zero-value `I` and a non-nil wrapped error

#### Scenario: OnInputAvailable not provided
- **WHEN** `TypedToolDef.OnInputAvailable` is nil
- **THEN** the resulting `Tool.OnInputAvailable` is nil

### Requirement: InputExamples marshaled at construction

When `TypedToolDef.InputExamples` is provided, each typed example SHALL be marshaled to `json.RawMessage` during `TypedTool` construction and set on `Tool.InputExamples`.

#### Scenario: Examples successfully marshaled
- **WHEN** `TypedToolDef.InputExamples` contains valid values of type `I`
- **THEN** each is marshaled to JSON and stored in `Tool.InputExamples`

#### Scenario: Example marshal failure
- **WHEN** an input example cannot be marshaled to JSON
- **THEN** `TypedTool` returns a zero `Tool` and a wrapped error

### Requirement: Optional fields passed through

The `TypedTool` function SHALL pass through optional fields from `TypedToolDef` to the resulting `Tool` without wrapping: `Description`, `Title`, `OutputSchema`, `Strict`, `ProviderOptions`, `OnInputStart`, and `OnInputDelta`.

#### Scenario: All passthrough fields preserved
- **WHEN** `TypedToolDef` has `Description`, `Title`, `OutputSchema`, `Strict`, `ProviderOptions`, `OnInputStart`, and `OnInputDelta` set
- **THEN** the resulting `Tool` has identical values for those fields

### Requirement: Low-level Tool integration

`TypedTool` SHALL construct the existing `Tool` representation without changing its execution, schema, callback, or provider-option behavior. The optional `Strict` pointer SHALL be passed through unchanged.

#### Scenario: Existing tool behavior is preserved
- **WHEN** code uses `Tool` directly with a schema, execute function, and optional strict pointer
- **THEN** its execution and schema behavior remains unchanged and all strict states remain representable
