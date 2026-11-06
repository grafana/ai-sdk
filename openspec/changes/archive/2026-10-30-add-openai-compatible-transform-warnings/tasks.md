## 1. Reference and regression

- [x] 1.1 Compare the registered upstream transform implementation/tests and identify current Go call sites.
- [x] 1.2 Add the warning-aware regression test and record its failure before implementation.

## 2. Implementation

- [x] 2.1 Add the additive option, shared callback adaptation, and warning propagation.
- [x] 2.2 Cover unary/stream metadata, current and returned warnings, legacy/nil/precedence, errors, core consumers and isolation.
- [x] 2.3 Document the callback contract and guide usage.

## 3. Validation and archive

- [x] 3.1 Run focused/full provider tests, race, vet and lint.
- [x] 3.2 Run docs, OpenSpec and applicable registered parity checks; document evidence limits.
- [x] 3.3 Verify implementation against the requirements, sync the spec and archive the change.
