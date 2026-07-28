## 1. Core Implementation

- [x] 1.1 Create `typed_tool.go` with `TypedToolDef[I, O any]` struct and `TypedTool[I, O any]` function: schema derivation via `schema.SchemaFor[I]()`, execute wrapper (unmarshal input, call typed execute, marshal output), ValidateInput wrapper, InputExamples marshaling, and passthrough of optional fields (Description, Title, OutputSchema, Strict, ProviderOptions, ToModelOutput)
- [x] 1.2 Create `typed_tool_test.go` with tests covering: successful construction with minimal and full definitions, schema derivation from struct tags, execute round-trip (success, input unmarshal failure, execute error propagation, output marshal failure), ValidateInput wrapping (valid, invalid, nil), InputExamples marshaling (success, failure), optional field passthrough, and schema derivation failure

## 2. Verification

- [x] 2.1 Run `make check` (fmt + vet + test) and fix any issues
