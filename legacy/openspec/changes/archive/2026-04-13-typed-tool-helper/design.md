## Context

Every user-defined tool currently requires manual JSON schema authoring, input unmarshaling, and output marshaling. `schema.SchemaFor[T]()` already derives JSON Schema from Go types via struct tags but is not wired into the tool definition path. The upstream TS SDK's `tool()` helper is an identity function used purely for TypeScript type inference -- in Go, the equivalent must do real work: derive the schema and wrap the execute function with marshal/unmarshal logic.

Current tool definition requires the user to keep a JSON schema string in sync with a Go struct, unmarshal `json.RawMessage` input in every execute body, and marshal the output back. This is error-prone and repetitive.

## Goals / Non-Goals

**Goals:**
- Eliminate marshal/unmarshal boilerplate from tool definitions
- Auto-derive input schema from Go types at construction time
- Provide a typed execute function signature `func(ctx, I, opts) (O, error)`
- Keep the low-level `Tool` struct unchanged for dynamic/runtime schema use cases
- Stay additive -- no breaking changes to existing API

**Non-Goals:**
- Adding `Name` field to `Tool` struct (that's #98)
- Auto-deriving output schema from `O` (output schema is rarely needed and the type parameter `O` may not always produce a useful schema, e.g. `any` or interface types)
- Supporting streaming tool results (async iterable pattern from upstream)
- Provider tool support (`TypedTool` is for function tools only)

## Decisions

### 1. Return `(Tool, error)` not just `Tool`

`schema.SchemaFor[I]()` can fail (e.g. if the type contains unsupported constructs). Panicking violates the codebase's error handling conventions. Returning error lets callers handle schema derivation failures explicitly.

**Alternative**: Panic on schema error (simpler API, matches the "called once at init" pattern). Rejected because the codebase convention is `(value, error)` returns and never panicking except for CSPRNG failure.

### 2. Include `Name` on `TypedToolDef` despite `Tool` lacking it

`TypedToolDef` is a definition struct, not `Tool` itself. Including `Name` serves as self-documentation and enables the pattern `ToolSet{def.Name: tool}`. When #98 adds `Name` to `Tool`, `TypedTool` can start populating it without API changes to `TypedToolDef`.

**Alternative**: Omit `Name` and only use map keys. Rejected because the definition should carry its full identity, and this is forward-compatible with #98.

### 3. Input schema auto-derived, output schema manual

Input schema is always derived from `I` via `schema.SchemaFor[I]()` -- this is the primary value proposition. Output schema is an optional override field (`schema.Schema`) because most tools don't need it and the output type `O` may not produce a useful schema (e.g. `map[string]any`, interface types, or `json.RawMessage` passthrough).

### 4. Typed `ValidateInput` wrapping

`TypedToolDef.ValidateInput` is `func(input I) error` -- receives the already-unmarshaled typed input. The wrapper unmarshals `json.RawMessage` to `I` first, then calls the typed validator. This keeps validation logic type-safe and consistent with the typed execute pattern.

### 5. `InputExamples` as `[]I` with marshal at construction

`TypedToolDef.InputExamples` is `[]I` (typed examples). Each is marshaled to `json.RawMessage` during `TypedTool` construction. This keeps the definition fully typed while producing the `[]json.RawMessage` that `Tool` expects. Marshal errors during construction are returned as part of the `(Tool, error)` return.

### 6. File placement in root package

`TypedTool` and `TypedToolDef` go in a new file `typed_tool.go` in the root `aisdk` package, alongside `tool.go`. Tests in `typed_tool_test.go`. No new package needed -- this is a convenience wrapper over existing types in the same package.

## Risks / Trade-offs

- [Risk: Schema derivation limitations] `schema.SchemaFor[T]()` may not support all Go type constructs (e.g. deeply nested generics, interface fields). -> Mitigation: Users can fall back to the low-level `Tool` struct with manual schema for unsupported types.
- [Risk: Error handling at init time] Returning `(Tool, error)` adds friction at tool registration time (typically `init()` or `main()`). -> Mitigation: This is standard Go convention. Users who prefer panicking can wrap with `must()` helpers.
- [Trade-off: No output schema derivation] Users who want output schema validation must provide it manually. -> Acceptable because output schema is rarely used and auto-deriving from O would fail for common output types like `map[string]any`.
