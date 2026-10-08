## 1. Isolate client retention

- [x] 1.1 Verify the pinned Gateway cause-data and status-derived retry behavior and extract only client decoders and regression tests from the approved implementation.
- [x] 1.2 Document HTTP versus SSE data carriers without claiming Gateway evidence is already produced.

## 2. Validate the standalone slice

- [x] 2.1 Run complete client tests/races, vet/lint and declared-dependency tests with Go 1.26.8; preserve module and license isolation.
- [x] 2.2 Run ProviderWire/parity and docs checks against the stack base; verify exact retained data, bounds, classification and ordered consumption.
- [x] 2.3 Record branch-local validation and limitations, validate OpenSpec strictly and commit the client-only slice.

## 3. Simplify error decoding after review

- [x] 3.1 Replace exact-name error-field helpers with standard typed JSON decoding, accepting Go case-insensitive member matching while retaining raw HTTP/SSE data.
- [x] 3.2 Update casing, opaque-data and malformed-envelope regression coverage without relaxing classification, retryability or bounds.
- [x] 3.3 Document the approved Go decoding adaptation and rerun client, ProviderWire/parity, docs and module-boundary checks; commit and push the reviewed PR update.
