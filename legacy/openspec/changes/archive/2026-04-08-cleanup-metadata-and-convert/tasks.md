## 1. Remove aisdk.RequestMetadata

- [x] 1.1 Delete `RequestMetadata` type definition from `text.go`
- [x] 1.2 Change `StepResult.Request` field type from `RequestMetadata` to `provider.RequestMetadata` in `text.go`
- [x] 1.3 Update the request metadata assignment in `streamtext.go` (~line 595) to use `provider.RequestMetadata` directly
- [x] 1.4 Update `generatetext.go` if it references `RequestMetadata`

## 2. Refactor aisdk.ResponseMetadata to embed provider type

- [x] 2.1 Replace `aisdk.ResponseMetadata` definition in `text.go` to embed `provider.ResponseMetadata` and keep only `Headers map[string]string`
- [x] 2.2 Remove the `Messages` field from `ResponseMetadata`
- [x] 2.3 Update `streamtext.go` field-by-field copy (~lines 588-593) to assign embedded struct directly
- [x] 2.4 Update `StreamTextResult.Response()` method if it constructs or returns `ResponseMetadata`
- [x] 2.5 Update `generatetext.go` for new `ResponseMetadata` shape (struct literals, field assignments)

## 3. Remove context.Context from ConvertToModelMessages

- [x] 3.1 Remove `context.Context` parameter from `ConvertToModelMessages` signature in `convert.go`
- [x] 3.2 Update the caller in `streamtext.go` (~line 212) to drop the `ctx` argument
- [x] 3.3 Update test helper and all test callers in `convert_test.go` to drop `context.Background()` argument

## 4. Verify

- [x] 4.1 Run `make build` to confirm compilation
- [x] 4.2 Run `make test` to confirm all tests pass
- [x] 4.3 Run `make lint` to confirm no lint issues
