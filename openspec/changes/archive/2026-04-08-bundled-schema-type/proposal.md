## Why

Schema handling uses `json.RawMessage` as the public type, forcing users to manually coordinate generation (`SchemaFor[T]`) and validation (`CompileSchema` + `Validate`) across two separate libraries. The upstream TS SDK bundles these into a single `Schema` type that carries both the schema definition and a validate function, reflecting the insight that definition and validation are always used together.

Additionally, schema utilities currently live in the `output` package, which is conceptually about structured output modes (Object, Array, Choice). Schema generation and validation are general-purpose concerns that don't belong there -- tools need schemas too, and importing `output` for schema types is misleading.

This change introduces a new `schema` sub-package with a bundled `Schema` type, extracts schema utilities from `output`, and updates all consumers.

## What Changes

- New `schema` sub-package (`github.com/grafana/ai-sdk/schema`) with `Schema` type bundling raw JSON Schema bytes with a pre-compiled validator
- `schema.SchemaFor[T]()` (moved from `output`) returns `Schema` instead of `json.RawMessage`
- New `schema.SchemaFromJSON(json.RawMessage)` constructor for hand-written or dynamically generated schemas
- `schema.SchemaFromFile(path)` (moved from `output`) returns `Schema` instead of `json.RawMessage`
- `schema.CompileSchema()` and `schema.Validate()` (moved from `output`) retained as standalone utilities
- `output.Object[T]`, `output.Array[T]` accept `schema.Schema` instead of `json.RawMessage`
- `aisdk.Tool.InputSchema` and `OutputSchema` change from `json.RawMessage` to `schema.Schema`
- `convert.go` calls `.JSON()` at the provider boundary to extract raw bytes for `provider.Tool`
- `provider.Tool.InputSchema` stays `json.RawMessage` (provider package remains a zero-dependency leaf)
- **BREAKING**: All public APIs that accepted or returned `json.RawMessage` for schemas change to `schema.Schema`

## Capabilities

### New Capabilities

- `bundled-schema`: New `schema` sub-package with unified Schema type that bundles JSON Schema definition with pre-compiled validation, providing constructors from Go types, raw JSON, and files

### Modified Capabilities

- `json-schema`: Schema generation and validation functions move from `output` to `schema` package and change signatures to use `Schema` instead of `json.RawMessage`
- `structured-output`: Output constructors (`Object`, `Array`) accept `schema.Schema` instead of `json.RawMessage`

## Impact

- **New `schema` package**: Contains `Schema` type, `SchemaFor[T]`, `SchemaFromJSON`, `SchemaFromFile`, `CompileSchema`, `CompiledSchema`, `Validate` -- all moved from `output`
- **output package**: Removes `schema.go`; `object.go`, `array.go` change constructor signatures to accept `schema.Schema`; imports `schema` package
- **root package**: `tool.go` changes `InputSchema`/`OutputSchema` field types to `schema.Schema`; `convert.go` calls `.JSON()` at provider boundary
- **provider package**: No changes (stays `json.RawMessage`)
- **anthropic module**: `convert_request.go` may need updates if it reads `aisdk.Tool` schema fields
- **Tests**: All tests constructing schemas as `json.RawMessage` need migration to `schema.SchemaFromJSON` or `schema.SchemaFor`
- **Downstream consumers**: Breaking change for any code creating `Tool` or `Output` instances with raw schema bytes
