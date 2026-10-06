## ADDED Requirements

### Requirement: Schema type bundles definition with validation

The `schema` package (`github.com/grafana/ai-sdk/schema`) SHALL provide a `Schema` struct that bundles a JSON Schema definition (`json.RawMessage`) with a pre-compiled validator. `Schema` SHALL be a value type (not a pointer) that is safe to copy and store. A zero-value `Schema` SHALL have nil raw bytes and no validator.

#### Scenario: Schema carries both definition and validator

- **WHEN** a `Schema` is created via any constructor (`SchemaFor[T]`, `SchemaFromJSON`, `SchemaFromFile`)
- **THEN** `schema.JSON()` SHALL return the raw JSON Schema bytes
- **AND** `schema.Validate(data)` SHALL validate data against the compiled schema

#### Scenario: Schema implements json.Marshaler

- **WHEN** a `Schema` value is marshaled via `json.Marshal`
- **THEN** the result SHALL be the raw JSON Schema bytes (identical to `schema.JSON()`)

#### Scenario: Schema implements json.Unmarshaler

- **WHEN** raw JSON Schema bytes are unmarshaled into a `Schema` value via `json.Unmarshal`
- **THEN** the `Schema` SHALL have the raw bytes set and a compiled validator ready for use
- **AND** `schema.Validate(data)` SHALL work identically to a `Schema` created via `SchemaFromJSON`

#### Scenario: Schema JSON round-trip

- **WHEN** a `Schema` is marshaled via `json.Marshal` and the result is unmarshaled back into a new `Schema`
- **THEN** the restored `Schema` SHALL have identical raw bytes and a working validator

#### Scenario: Zero-value Schema

- **WHEN** a zero-value `Schema{}` is used
- **THEN** `schema.JSON()` SHALL return nil
- **AND** `schema.Validate(data)` SHALL return an error indicating no schema is compiled

### Requirement: SchemaFor derives Schema from Go types

The `schema.SchemaFor[T]()` function SHALL return `(Schema, error)`. It SHALL generate the JSON Schema from the Go type using `invopop/jsonschema`, clean it (strip `$schema`, `$id`, inline simple `$defs`), and pre-compile it for validation. All existing struct tag support (enum, pattern, title, description, minimum, maximum, etc.) SHALL be preserved.

#### Scenario: SchemaFor returns a bundled Schema

- **WHEN** `SchemaFor[T]()` is called with a struct type
- **THEN** it SHALL return a `Schema` whose `.JSON()` matches the previously generated `json.RawMessage`
- **AND** whose `.Validate()` correctly validates conforming data

#### Scenario: SchemaFor compilation failure

- **WHEN** the generated schema cannot be compiled (invalid schema structure)
- **THEN** `SchemaFor[T]()` SHALL return a non-nil error

### Requirement: SchemaFromJSON creates Schema from raw bytes

The `schema` package SHALL provide `SchemaFromJSON(raw json.RawMessage) (Schema, error)` as the constructor for hand-written, file-loaded, or dynamically generated JSON schemas. It SHALL compile the schema immediately and return an error if the schema is invalid.

#### Scenario: Valid JSON Schema bytes

- **WHEN** `SchemaFromJSON` is called with valid JSON Schema bytes
- **THEN** it SHALL return a `Schema` with the same raw bytes and a working validator

#### Scenario: Invalid JSON Schema bytes

- **WHEN** `SchemaFromJSON` is called with bytes that are not a valid JSON Schema
- **THEN** it SHALL return a non-nil error

#### Scenario: Invalid JSON bytes

- **WHEN** `SchemaFromJSON` is called with bytes that are not valid JSON
- **THEN** it SHALL return a non-nil error

### Requirement: SchemaFromFile loads Schema from disk

The `schema.SchemaFromFile(path string)` function SHALL return `(Schema, error)`. It SHALL read the file, validate it is valid JSON, and compile it into a `Schema`.

#### Scenario: Load and compile schema from file

- **WHEN** `SchemaFromFile` is called with a path to a valid JSON Schema file
- **THEN** it SHALL return a `Schema` whose `.JSON()` contains the file contents
- **AND** whose `.Validate()` correctly validates conforming data

#### Scenario: File does not exist

- **WHEN** `SchemaFromFile` is called with a non-existent path
- **THEN** it SHALL return a non-nil error

### Requirement: Schema.JSON extracts raw bytes for provider boundary

The `Schema.JSON()` method SHALL return `json.RawMessage` containing the raw JSON Schema bytes. This is the extraction point for passing schemas across package boundaries that use `json.RawMessage` (e.g., `provider.Tool.InputSchema`).

#### Scenario: Extract bytes for provider tool

- **WHEN** a `Schema` is created and `.JSON()` is called
- **THEN** the returned `json.RawMessage` SHALL be identical to the bytes originally used to create the schema

### Requirement: Schema.Validate validates data against the schema

The `Schema.Validate(data json.RawMessage) error` method SHALL validate JSON data against the pre-compiled schema. Validation errors SHALL include JSON pointer locations.

#### Scenario: Valid data passes

- **WHEN** `schema.Validate(data)` is called with data that conforms to the schema
- **THEN** it SHALL return nil

#### Scenario: Invalid data fails with details

- **WHEN** `schema.Validate(data)` is called with non-conforming data
- **THEN** it SHALL return an error containing the JSON pointer to the failing location

#### Scenario: Concurrent validation is safe

- **WHEN** multiple goroutines call `Validate` on the same `Schema` concurrently
- **THEN** all calls SHALL produce correct results without data races

### Requirement: schema package is a leaf with no internal module dependencies

The `schema` package SHALL NOT import `github.com/grafana/ai-sdk` (root) or `github.com/grafana/ai-sdk/provider`. Its only external dependencies SHALL be `invopop/jsonschema` (generation) and `santhosh-tekuri/jsonschema` (validation). This ensures it can be imported by all other packages in the module without creating import cycles.

#### Scenario: Import from root package

- **WHEN** the root `aisdk` package imports `github.com/grafana/ai-sdk/schema`
- **THEN** the build SHALL succeed with no import cycle

#### Scenario: Import from output package

- **WHEN** the `output` package imports `github.com/grafana/ai-sdk/schema`
- **THEN** the build SHALL succeed with no import cycle
