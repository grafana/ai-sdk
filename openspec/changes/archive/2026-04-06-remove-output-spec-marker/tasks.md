## 1. Remove marker from interface

- [x] 1.1 Remove `OutputSpec()` method and its doc comments from the `Output` interface in `output.go`
- [x] 1.2 Remove the explanatory comment block about why the marker is exported (lines 19-22)

## 2. Remove marker from implementations

- [x] 2.1 Remove `OutputSpec()` from `ObjectOutput[T]` in `output/object.go`
- [x] 2.2 Remove `OutputSpec()` from `ArrayOutput[T]` in `output/array.go`
- [x] 2.3 Remove `OutputSpec()` from `ChoiceOutput` in `output/choice.go`
- [x] 2.4 Remove `OutputSpec()` from `JSONOutput` in `output/json.go`
- [x] 2.5 Remove `OutputSpec()` from `TextOutput` in `output/text.go`

## 3. Verify

- [x] 3.1 Run `go vet ./...` to confirm no compilation errors
- [x] 3.2 Run `make test` to confirm all tests pass
