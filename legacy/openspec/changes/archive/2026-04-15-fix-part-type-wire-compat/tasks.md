## 1. Update PartType() Methods

- [x] 1.1 Change `DataPart.PartType()` in `message.go` from `(DataPart) PartType() string { return "data" }` to `(p DataPart) PartType() string { return "data-" + p.DataName }` (value receiver with field access)
- [x] 1.2 Change `ToolInvocationPart.PartType()` in `message.go` from `(ToolInvocationPart) PartType() string { return "tool-invocation" }` to `(p ToolInvocationPart) PartType() string { return "tool-" + p.ToolName }` (value receiver with field access)

## 2. Update Tests

- [x] 2.1 Update `TestAllPartTypes` in `message_json_test.go` to expect wire-compatible `PartType()` values for `DataPart` (e.g., `"data-usage"`) and `ToolInvocationPart` (e.g., `"tool-searchWeb"`)
- [x] 2.2 Update assertion in `http_test.go` that checks `PartType() == "tool-invocation"` to use the wire-compatible form `"tool-{toolName}"`
- [x] 2.3 Add explicit `PartType()` unit tests: verify `DataPart{DataName: "usage"}.PartType() == "data-usage"` and `ToolInvocationPart{ToolName: "search"}.PartType() == "tool-search"`

## 3. Verify

- [x] 3.1 Run `make test` to confirm all tests pass
- [x] 3.2 Run `make lint` (or `make vet`) to confirm no lint issues
