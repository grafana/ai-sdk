## 1. Shared SDK capture

- [x] 1.1 Add request-scoped observation and original source errors without changing fallback policy, returned errors or cleanup.
- [x] 1.2 Test independent observers/requests, cancellation, commitment, unowned late results and high-level SDK access; document synchronization and publication boundaries.
- [x] 1.3 Validate root tests/races/vet/lint, Go 1.26, parity and module boundaries; consolidate the SDK scope into this single PR-owned OpenSpec change.

## 2. Compact Gateway projection

- [x] 2.1 Replace the dormant collector/wrappers and diagnostic allocations with pure ordered-attempt projection and shallow protected summaries.
- [x] 2.2 Add best-effort namespace enrichment using caller-owned complete-envelope limits, preserving original metadata on no-room collisions.
- [x] 2.3 Replace the namespace schema and focused regressions; prove full arrays, no duplicated stream history, input isolation and exact envelope-limit behavior.

## 3. Validation and delivery

- [x] 3.1 Run Gateway/root tests, races, Go 1.26, vet/lint, ProviderWire/parity, authenticated command and module/docs checks.
- [x] 3.2 Update PARITY/validation and PR migration notes, validate the single OpenSpec change, commit/push without runtime activation or unrelated checksums.
