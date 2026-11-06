# Tasks

## 1. Delivery authority

- [x] 1.1 Limit retiring validity with deterministic regressions that fail on the base and pass after the fix; document the limit.
- [x] 1.2 Implement ordered cache publication and typed controller acquisition, with deterministic background revocation, isolation, ordering, expiry, capacity, idle and shutdown tests; document ownership and omission semantics.

## 2. Integration evidence

- [x] 2.1 Verify queued/current/retiring revocation through real local OTLP export and preserve unaffected delivery; do not claim Cloud ingestion.
- [x] 2.2 Run focused race tests, Gateway source checks, baseline/spec/docs validation and review the final diff against the accepted scope; provide an external before/after walkthrough.

## Workflow follow-up

- Archive the verified change before publishing the draft in the existing stack.
