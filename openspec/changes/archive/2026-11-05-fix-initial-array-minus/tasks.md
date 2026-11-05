# Tasks

## 1. Repair and regression contract

- [x] 1.1 Inspect the registered upstream implementation and tests, add both helper tables and reader/output regressions, and confirm they fail before the fix.
- [x] 1.2 Generate provider-independent partial-output fixtures with registered upstream, verify the Go replay fails, and remove the two premature index assignments so focused regressions pass.

## 2. Integration verification

- [x] 2.1 Run root tests, focused race tests, frontend integration and available parity constituents; document any environment limits with exact results.
- [x] 2.2 Validate the change artifacts and verify both capability deltas match the implemented behavior.

## Workflow follow-up

- Synchronize the specs and archive the completed change before publication.
