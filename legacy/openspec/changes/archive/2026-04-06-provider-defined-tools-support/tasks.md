## 1. Add Fields to `aisdk.Tool`

- [x] 1.1 Add `Type string`, `ID string`, and `Args map[string]json.RawMessage` fields to the `Tool` struct in `tool.go`

## 2. Update `toolSetToProviderTools()` Conversion

- [x] 2.1 Add a switch on `t.Type` in `toolSetToProviderTools()` (`convert.go:261`):
  - `"provider"`: construct `provider.Tool` with `Type`, `Name` (map key), `ID`, `Args`
  - `""` or `"function"`: existing function tool path
  - `default`: emit unsupported warning and skip
- [x] 2.2 Write tests for `toolSetToProviderTools()`: provider-defined tool passes through ID/Args, function tool unchanged, empty Type defaults to function, mixed ToolSet with both types

## 3. Integration Verification

- [x] 3.1 Write a test that exercises `StreamText` with a provider-defined tool in the ToolSet, using a mock model that verifies the tool arrives at `DoStream` with `Type: "provider"` and correct `ID`/`Args`
- [x] 3.2 Write a test that verifies callbacks (`OnInputStart`, `OnInputAvailable`) fire for provider-defined tools when the mock model emits matching stream parts
- [x] 3.3 Run `make test` to verify all existing tests pass
- [x] 3.4 Run `make vet` and `make lint`
