# Tasks

## 1. Evidence

- [x] 1.1 Add the core UI fixture `test/conformance/ui/finish-then-error` (`terminal`, `next-segment`) with an upstream generator, and confirm the Go replay of `terminal` fails
- [x] 1.2 Add a `StreamText` unit test for unified and raw finish reasons across terminal orders
- [x] 1.3 Add the `ui-stream-error-after-finish` cross-language scenario and Vitest assertion

## 2. Implementation

- [x] 2.1 Set the unified finish reason to `error` on a provider error part in `StreamText`

## 3. Validation

- [x] 3.1 Run `go test ./...`, the conformance suite, `mise run test-integration` scenario tests, `mise run lint` and `mise run parity-check`
