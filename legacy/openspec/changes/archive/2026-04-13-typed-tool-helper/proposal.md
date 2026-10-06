## Why

Every tool definition requires identical boilerplate: manually writing a JSON schema string, unmarshaling input from `json.RawMessage`, doing work, and marshaling the output back. `schema.SchemaFor[T]()` already derives JSON Schema from Go types but is not wired into the tool path. A generic helper would eliminate this repetition, match the upstream TS SDK's `tool()` ergonomics, and let users work with Go types end-to-end.

## What Changes

- Add a `TypedTool[I, O]` generic function that accepts a typed definition and returns a standard `Tool`
- Add a `TypedToolDef[I, O]` struct for the typed definition input (description, typed execute func, optional overrides)
- Schema derivation via `schema.SchemaFor[I]()` happens at construction time; marshal/unmarshal is handled internally
- The low-level `Tool` struct and `ToolExecuteFunc` remain unchanged for dynamic/runtime schema use cases

## Capabilities

### New Capabilities

- `typed-tool`: Generic helper for type-safe tool definition with automatic schema derivation, input unmarshaling, and output marshaling

### Modified Capabilities

(none)

## Impact

- **Code**: New file(s) in root `aisdk` package (`typed_tool.go`, `typed_tool_test.go`)
- **APIs**: Additive only -- new public function `TypedTool[I, O]` and struct `TypedToolDef[I, O]`; no changes to existing `Tool`/`ToolSet` types
- **Dependencies**: Uses existing `schema.SchemaFor[T]()` from the `schema` package; no new external dependencies
- **Related issues**: #104 (this issue), builds on the same pattern as #97 (`ObjectFor[T]`), complementary to #98 (Tool Name field -- not yet implemented, so `TypedToolDef` carries `Name` but current `Tool` struct does not; the name is used only as the map key when adding to a `ToolSet`)
