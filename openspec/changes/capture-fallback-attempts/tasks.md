## 1. Shared observation

- [x] 1.1 Add focused failing tests for request isolation, independent panic recovery and source-error preservation.
- [x] 1.2 Add context-scoped observation and preserve SourceErr without changing fallback decisions or ownership.

## 2. Validation and documentation

- [x] 2.1 Document request-scoped capture, callback synchronization and native-error publication boundaries.
- [x] 2.2 Run fallback/root tests, races, vet/lint, Go 1.26 compatibility, parity and strict OpenSpec validation; record the evidence.
