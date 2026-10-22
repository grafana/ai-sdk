## 1. Implementation

- [x] 1.1 Add bound options, constructor validation and request schema keywords to `output.Array`.
- [x] 1.2 Check complete array length before element validation.
- [x] 1.3 Cap `ElementStream` at the maximum, close it at overflow and leave partial snapshots uncapped.
- [x] 1.4 Add `ElementStreamErr` and treat a negative custom maximum as zero.
- [x] 1.5 Record the overflow and repair differences in `test/conformance/upstream.yaml`.

## 2. Validation

- [x] 2.1 Unit tests for unset, zero, minimum-only, maximum-only and paired request schemas, invalid bounds, boundary lengths and the length-first check.
- [x] 2.2 StreamText tests for the element cap, the close at overflow while generation continues, uncapped partial output, the overflow `OutputError`, an accepted repair and the bound on the provider request.
- [x] 2.3 Update the structured output guide.
