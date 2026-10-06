## Purpose

Define JSON Schema generation, provider-oriented cleanup, validation, compilation, file loading, and use as the shared schema representation across the SDK.

## Requirements

### Requirement: Schema generation from Go struct tags

The system SHALL provide a generic `SchemaFor[T]()` function in the `schema` package that generates a JSON Schema from a Go struct type using `invopop/jsonschema`. The function SHALL return `Schema` (bundled definition + validator) suitable for use in `output.Object[T]()`, `output.Array[T]()`, `aisdk.Tool` fields, and for validation via `Schema.Validate()`.

#### Scenario: Generate schema from simple struct

- **WHEN** `SchemaFor[T]()` is called with a struct type that has `json` tags
- **THEN** it SHALL return a `Schema` whose `.JSON()` contains a valid JSON Schema with properties matching the struct fields

#### Scenario: Struct tags with enum constraints

- **WHEN** a struct field has `jsonschema:"enum=red,enum=green,enum=blue"`
- **THEN** the generated schema's `.JSON()` property SHALL include `"enum": ["red", "green", "blue"]`

#### Scenario: Struct tags with description

- **WHEN** a struct field has `jsonschema:"title=Name,description=The user name"`
- **THEN** the generated schema's `.JSON()` property SHALL include the title and description

#### Scenario: Struct tags with numeric constraints

- **WHEN** a struct field has `jsonschema:"minimum=0,maximum=100"`
- **THEN** the generated schema's `.JSON()` property SHALL include `"minimum": 0, "maximum": 100`

#### Scenario: Struct tags with string constraints

- **WHEN** a struct field has `jsonschema:"minLength=1,maxLength=200,pattern=^[a-z]+$"`
- **THEN** the generated schema's `.JSON()` property SHALL include the minLength, maxLength, and pattern constraints

#### Scenario: Custom type schema via JSONSchema interface

- **WHEN** a type implements invopop's `JSONSchema() *jsonschema.Schema` interface
- **THEN** `SchemaFor[T]()` SHALL use the custom schema definition instead of reflection

### Requirement: Schema cleanup for LLM providers

The system SHALL clean the generated schema to maximize compatibility with LLM providers. Cleanup SHALL strip `$schema`, `$id` fields and handle `$defs`/`$ref` for simple non-recursive schemas by inlining definitions.

#### Scenario: Strip metadata fields

- **WHEN** `SchemaFor[T]()` generates a schema with `$schema` and `$id` fields
- **THEN** the returned `Schema.JSON()` SHALL NOT contain `$schema` or `$id`

#### Scenario: Inline simple $defs references

- **WHEN** the generated schema has a `$defs` section with a single `$ref` at the root
- **THEN** the returned schema SHALL inline the referenced definition, removing the `$defs`/`$ref` indirection

### Requirement: Schema validation of LLM responses

The system SHALL provide a `Validate(schema json.RawMessage, data json.RawMessage) error` function that validates JSON data against a JSON Schema using `santhosh-tekuri/jsonschema`. Validation errors SHALL include JSON pointer locations identifying which part of the data failed.

#### Scenario: Valid data passes validation

- **WHEN** JSON data matches the schema
- **THEN** `Validate` SHALL return nil

#### Scenario: Invalid data fails validation with details

- **WHEN** JSON data does not match the schema (e.g., wrong type, missing required field, invalid enum value)
- **THEN** `Validate` SHALL return an error containing the JSON pointer to the failing location and a description of the violation

#### Scenario: Invalid schema fails compilation

- **WHEN** the schema itself is not valid JSON Schema
- **THEN** `Validate` SHALL return an error indicating schema compilation failure

### Requirement: Compiled schema for repeated validation

The system SHALL provide a `CompileSchema(schema json.RawMessage) (*CompiledSchema, error)` function that pre-compiles a schema for repeated validation. `CompiledSchema.Validate(data json.RawMessage) error` SHALL validate data against the pre-compiled schema. Compiled schemas SHALL be thread-safe.

#### Scenario: Compile once, validate many

- **WHEN** a schema is compiled via `CompileSchema` and then used to validate multiple JSON documents
- **THEN** each validation call SHALL produce correct results without re-compiling

#### Scenario: Concurrent validation

- **WHEN** multiple goroutines call `Validate` on the same `CompiledSchema` concurrently
- **THEN** all calls SHALL produce correct results without data races

### Requirement: Schema loading from files

The `schema` package SHALL support loading JSON Schema from files via `SchemaFromFile(path string) (Schema, error)`. The returned `Schema` SHALL be usable for `output.Object[T]()`, `aisdk.Tool` fields, and validation.

#### Scenario: Load schema from JSON file

- **WHEN** a JSON Schema file is read via `SchemaFromFile`
- **THEN** the returned `Schema.JSON()` SHALL be usable as the schema parameter in `output.Object[T]()` and SHALL pass through to `provider.ResponseFormat.Schema`

#### Scenario: Loaded schema used for validation

- **WHEN** a schema loaded from a file via `SchemaFromFile` is used
- **THEN** `schema.Validate()` SHALL validate LLM responses identically to schemas generated from struct tags

### Requirement: Schema as schema currency

All schema operations SHALL use `Schema` as the primary type for bundling definition and validation. `Schema.JSON()` SHALL return `json.RawMessage` for interoperability with provider interfaces and other code that requires raw bytes. The `output` implementations SHALL accept and store `Schema` values. `aisdk.Tool` SHALL use `Schema` for its schema fields.

#### Scenario: Generated schema used end-to-end

- **WHEN** `SchemaFor[T]()` produces a `Schema`
- **THEN** `schema.JSON()` SHALL be usable for `provider.ResponseFormat.Schema`
- **AND** `schema.Validate()` SHALL validate data against the schema
- **AND** the `Schema` SHALL be directly assignable to `aisdk.Tool.InputSchema`

#### Scenario: User-provided raw schema

- **WHEN** a user provides their own `json.RawMessage` schema via `SchemaFromJSON()`
- **THEN** the resulting `Schema` SHALL work identically with `output.Object[T]()`, `aisdk.Tool` fields, validation, and the provider
