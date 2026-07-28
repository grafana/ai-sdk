## 1. Dependencies and Package Scaffolding

- [x] 1.1 Add `invopop/jsonschema` and `santhosh-tekuri/jsonschema/v5` to `go.mod` and run `go mod tidy`
- [x] 1.2 Create `output/` package directory with `doc.go` package documentation

## 2. JSON Schema Infrastructure (`output/schema.go`)

- [x] 2.1 Implement `SchemaFor[T]() (json.RawMessage, error)` wrapping `invopop/jsonschema.Reflector` with struct tag support (enum, pattern, title, description, format, default, min/max)
- [x] 2.2 Implement schema cleanup: strip `$schema`, `$id`, and inline simple `$defs`/`$ref` for LLM provider compatibility
- [x] 2.3 Implement `CompileSchema(schema json.RawMessage) (*CompiledSchema, error)` wrapping `santhosh-tekuri/jsonschema` compiler
- [x] 2.4 Implement `CompiledSchema.Validate(data json.RawMessage) error` with JSON pointer error details
- [x] 2.5 Implement convenience `Validate(schema json.RawMessage, data json.RawMessage) error` that compiles and validates in one call
- [x] 2.6 Write tests for `SchemaFor` with various struct tag combinations (enum, pattern, min/max, description, custom `JSONSchema()` interface)
- [x] 2.7 Write tests for `CompileSchema` and `Validate` covering valid data, invalid data, invalid schema, and concurrent validation

## 3. Output Interface (root `aisdk` package)

- [x] 3.1 Define the `Output` interface in the root package with sealed marker method `outputSpec()`, `ResponseFormat()`, `ParseComplete()`, and `ParsePartial()`
- [x] 3.2 Define `ErrNoObjectGenerated` sentinel error
- [x] 3.3 Add `Output` field to `StreamTextParams`
- [x] 3.4 Add `output` field (type `any`), `outputErr` field, `Output() any`, and `OutputError() error` methods to `StreamTextResult`
- [x] 3.5 Add `Output` and `OutputError` fields to `GenerateTextResult`, populate them in `GenerateText()`

## 4. Output Implementations (`output/` package)

- [x] 4.1 Implement `ObjectOutput[T]` -- accepts `json.RawMessage` schema, compiles it, sets `ResponseFormat` with type "json" and schema, `ParseComplete` validates and unmarshals into `T`, `ParsePartial` attempts best-effort JSON parse
- [x] 4.2 Implement `ArrayOutput[T]` -- wraps element schema in `{"elements": [...]}` outer object, `ParseComplete` unwraps and validates each element, `ParsePartial` extracts complete elements seen so far
- [x] 4.3 Implement `ChoiceOutput` -- wraps options in `{"result": "..."}` with enum constraint, `ParseComplete` unwraps and returns the selected string
- [x] 4.4 Implement `JSONOutput` -- sets `ResponseFormat` with type "json" and no schema, `ParseComplete` validates JSON parseability only
- [x] 4.5 Implement `TextOutput` -- no-op Output that passes through text unchanged (for completeness / explicit "no structured output")
- [x] 4.6 Write tests for each Output implementation covering `ResponseFormat()`, `ParseComplete()` with valid/invalid input, and `ParsePartial()`

## 5. StreamText Integration

- [x] 5.1 In `streamtext.go` `run()`, before `DoStream` call: if `params.Output` is set, override `callOpts.ResponseFormat` with `params.Output.ResponseFormat()`
- [x] 5.2 In `streamtext.go` `run()`, at stream finish: if `params.Output` is set and `FinishReason == "stop"`, call `ParseComplete` on accumulated text and store result/error
- [x] 5.3 Add `partialOutputStream` channel to `StreamTextResult`, implement `PartialOutputStream() <-chan json.RawMessage` method
- [x] 5.4 Launch partial streaming goroutine that observes text deltas, accumulates, calls `ParsePartial`, and emits changed partials to the channel
- [x] 5.5 Add `elementStream` channel to `StreamTextResult`, implement `ElementStream() <-chan json.RawMessage` method for array mode
- [x] 5.6 Close partial/element streams when the main stream finishes, handle nil Output (return closed channels)
- [x] 5.7 Write integration tests for StreamText with Output: object mode end-to-end, array mode end-to-end, choice mode, nil Output (no regression)

## 6. Typed Accessors and Convenience Wrappers (`output/` package)

- [x] 6.1 Implement `Value[T](result) (T, error)` -- type-asserts from `result.Output()`, returns clear error on mismatch or nil output
- [x] 6.2 Implement `TypedElementStream[T](result) <-chan T` -- wraps `ElementStream()` with per-element `json.Unmarshal`
- [x] 6.3 Implement `GenerateObject[T](ctx, params, output) (*ObjectResult[T], error)` -- sets `Output` on params, calls `GenerateText`, returns typed wrapper
- [x] 6.4 Implement `StreamObject[T](ctx, params, output) *StreamObjectResult[T]` -- sets `Output` on params, calls `StreamText`, returns typed wrapper
- [x] 6.5 Define `ObjectResult[T]` with `Object() (T, error)` method and `StreamObjectResult[T]` with typed stream accessors
- [x] 6.6 Write tests for `Value`, `TypedElementStream`, `GenerateObject`, and `StreamObject` including type mismatch cases

## 7. GenerateText Integration

- [x] 7.1 In `generatetext.go`, populate `GenerateTextResult.Output` and `GenerateTextResult.OutputError` from `StreamTextResult` after drain
- [x] 7.2 Write test for `GenerateText` with Output: verify `Output` and `OutputError` fields are correctly populated

## 8. Verification

- [x] 8.1 Run `make check` (fmt + vet + test) and fix any failures
- [x] 8.2 Verify `make build` succeeds with new dependencies
- [x] 8.3 Verify no circular dependency between `aisdk` and `output` packages (Output interface in root, implementations in `output/`)
