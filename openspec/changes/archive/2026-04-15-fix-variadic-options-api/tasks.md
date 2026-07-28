## 1. Change Function Signatures

- [x] 1.1 Update `ConvertToModelMessages` in `convert.go` to accept `opts *ConvertOptions` instead of `opts ...ConvertOptions`; dereference pointer or use zero-value when `nil`
- [x] 1.2 Update `WriteUIMessageStream` in `http.go` to accept `opts *UIMessageStreamOptions` instead of `opts ...UIMessageStreamOptions`; dereference pointer or use zero-value when `nil`
- [x] 1.3 Update `ReadUIMessageStream` in `http.go` to accept `opts *ReadStreamOption` instead of `opts ...ReadStreamOption`; dereference pointer or use zero-value when `nil`

## 2. Update Internal Call Sites

- [x] 2.1 Update `ConvertToModelMessages` call in `streamtext.go` to pass `nil` instead of omitting the variadic argument
- [x] 2.2 Update `ConvertToModelMessages` calls in `convert_test.go` to pass `nil` or `&ConvertOptions{...}` as appropriate

## 3. Update Tests

- [x] 3.1 Update `TestWriteUIMessageStream` in `http_test.go` to pass `nil` for the opts parameter
- [x] 3.2 Update `TestReadUIMessageStream` in `http_test.go` to pass `nil` for the opts parameter

## 4. Update Documentation

- [x] 4.1 Update `README.md` examples that reference `ConvertToModelMessages`, `WriteUIMessageStream`, or `ReadUIMessageStream` to use the new signatures

## 5. Verify

- [x] 5.1 Run `make build` to confirm compilation
- [x] 5.2 Run `make test` to confirm all tests pass
- [x] 5.3 Run `make lint` to confirm no lint errors
