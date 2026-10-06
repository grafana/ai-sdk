## 1. Create `schema` Package

- [x] 1.1 Create `schema/` directory with `doc.go` (package documentation)
- [x] 1.2 Move `SchemaFor[T]`, `SchemaForType`, `cleanSchema`, `inlineDefs` from `output/schema.go` to `schema/schema.go` -- update package name and error prefixes
- [x] 1.3 Move `CompiledSchema`, `CompileSchema`, `Validate` from `output/schema.go` to `schema/schema.go`
- [x] 1.4 Move `SchemaFromFile` from `output/schema.go` to `schema/schema.go`
- [x] 1.5 Add `Schema` struct with `raw json.RawMessage` and `compiled *CompiledSchema` fields, plus `JSON()`, `Validate()`, and `MarshalJSON()` methods
- [x] 1.6 Add `SchemaFromJSON(raw json.RawMessage) (Schema, error)` constructor
- [x] 1.7 Update `SchemaFor[T]()` to return `(Schema, error)` -- build Schema from generated+cleaned bytes
- [x] 1.8 Update `SchemaFromFile(path)` to return `(Schema, error)` -- delegate to `SchemaFromJSON`
- [x] 1.9 Move schema tests from `output/schema_test.go` to `schema/schema_test.go`, add tests for `Schema` type, `SchemaFromJSON`, zero-value, `MarshalJSON`, concurrent `Validate`

## 2. Update `output` Package

- [x] 2.1 Remove `output/schema.go` (all contents moved to `schema/`)
- [x] 2.2 Remove `output/schema_test.go` (all contents moved to `schema/`)
- [x] 2.3 Change `Object[T](schema json.RawMessage, ...)` to `Object[T](s schema.Schema, ...)` -- remove internal `CompileSchema` call, use schema's compiled validator
- [x] 2.4 Change `ObjectOutput[T]` struct to store `schema.Schema` instead of separate `json.RawMessage` + `*CompiledSchema` fields
- [x] 2.5 Change `Array[T](elementSchema json.RawMessage)` to `Array[T](elementSchema schema.Schema)` -- use `.JSON()` to build wrapper, construct wrapper as `Schema` via `SchemaFromJSON`
- [x] 2.6 Change `ArrayOutput[T]` struct to store `schema.Schema` values instead of separate fields
- [x] 2.7 Update `output_test.go` to construct schemas via `schema.SchemaFor[T]()` or `schema.SchemaFromJSON()` instead of raw `json.RawMessage`

## 3. Update Root `aisdk` Package

- [x] 3.1 Change `aisdk.Tool.InputSchema` and `OutputSchema` from `json.RawMessage` to `schema.Schema` in `tool.go`
- [x] 3.2 Update `convert.go` (`toolSetToProviderTools`) to call `.JSON()` when building `provider.FunctionTool.InputSchema`
- [x] 3.3 Update any root package tests that construct `Tool` values with raw schema bytes

## 4. Update Anthropic Module (if needed)

- [x] 4.1 Check `anthropic/convert_request.go` for any direct use of `aisdk.Tool` schema fields and update accordingly
- [x] 4.2 Check anthropic tests for `aisdk.Tool` construction patterns

## 5. Validation and Build

- [x] 5.1 Run `make build` to verify compilation across both modules
- [x] 5.2 Run `make test` to verify all tests pass
- [x] 5.3 Run `make lint` to verify no lint issues
