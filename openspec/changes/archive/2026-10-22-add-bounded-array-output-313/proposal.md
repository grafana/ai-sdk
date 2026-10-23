## Why

Issue #313: the registered `ai@7.0.127` `Output.array` accepts `minItems` and `maxItems`, sends them on the wrapper's `elements` schema, rejects invalid bounds, checks the final array length and stops element delivery at `maxItems`. `output.Array[T]` has no way to bound the enclosing array.

## What Changes

- Add `output.WithMinItems` and `output.WithMaxItems` options. Unset keeps the array unbounded; zero is an explicit bound.
- Add `output.ArrayOption`, which `Array` now takes. Every `ObjectOption` is an `ArrayOption`, and the bounds are `ArrayOption`s only, so passing one to `Object`, `ChoiceWithOptions` or `JSON` fails to compile, as upstream's types reject `minItems` on `Output.object`. Code that spreads a `[]ObjectOption` into `Array` must convert it; the module is alpha, so this is a marked breaking change.
- `output.Array` returns an error for a negative bound or a minimum above the maximum, so no model call is made. Go integers already exclude the fractional values upstream rejects.
- The bounds appear on the request schema's `elements` property. Complete parsing checks the length before validating elements, as upstream does, and reports a violation wrapping `ErrNoObjectGenerated` and `ErrInvalidOutputText`.
- `ElementStream` delivers at most `maxItems` elements. Upstream errors its element stream as soon as an element past the maximum appears; a Go channel has no error slot, so the channel closes at that point and stays closed for later steps, and a new `StreamTextResult.ElementStreamError` reports the overflow and blocks until the element stream closes. `OutputError` still validates only the final output. Partial output snapshots are not capped, matching upstream. An accepted `WithRepairText` result within the bounds leaves `OutputError` nil while `ElementStreamError` still reports the overflow; upstream `streamText` has no output repair. Both are recorded as the `array-element-stream-overflow` documented deviation.
- A root `BoundedArrayOutput` interface, implemented by `(*ArrayOutput[T]).MaxItems`, lets the root element stream read the maximum without importing `output`.

## Capabilities

### Modified Capabilities
- `structured-output`: array bounds on the request schema, complete parsing and element streaming.

## Impact

`output` package API (two options, one method), root `BoundedArrayOutput` and `StreamTextResult.ElementStreamError`, the root element stream, the structured output guide and a `knownGaps` entry. No wire, UI chunk or provider change.
