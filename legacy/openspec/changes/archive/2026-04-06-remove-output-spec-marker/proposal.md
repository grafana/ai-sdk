## Why

The `Output` interface includes an exported `OutputSpec()` marker method intended to prevent accidental interface satisfaction. This marker is an anomaly: every other sealed interface in the codebase uses unexported markers (`textStreamPart()`, `contentPart()`, `message()`, etc.) which genuinely seal. `OutputSpec()` is exported because the interface lives in `aisdk` while implementations live in `output/`, making an unexported marker impossible -- but this means it provides no sealing benefit. The remaining three methods have signatures specific enough (referencing `provider.ResponseFormat`) that accidental implementation is not a realistic concern.

## What Changes

- Remove the `OutputSpec()` method from the `Output` interface definition in `output.go`
- Remove the `OutputSpec()` implementation from all five types: `ObjectOutput[T]`, `ArrayOutput[T]`, `ChoiceOutput`, `JSONOutput`, `TextOutput`
- Remove associated doc comments about the marker method

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `structured-output`: The requirement that the Output interface be "sealed via an unexported marker method" must be updated to reflect that the interface relies on method signature specificity rather than a marker method.

## Impact

- **Code**: `output.go` (interface definition), `output/object.go`, `output/array.go`, `output/choice.go`, `output/json.go`, `output/text.go` (implementations)
- **Tests**: `output/output_test.go` compile-time interface checks remain valid (they check the three functional methods, not the marker)
- **API**: Removing a method from an interface is a narrowing change -- existing implementations remain valid. Any external code calling `OutputSpec()` directly would break, but the method is documented as "not intended to be called directly" and grep confirms zero call sites.
- **Wire format**: No impact. This is purely an internal interface change.
