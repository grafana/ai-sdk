# Client retention validation

## Scope and reference

First PR of #322, based on #332 at c32f3f6d. This change owns only independent client error retention and its documentation/tests. Collection/schema and production emission belong to the next two changes; this PR does not claim those are implemented.

Reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18, Provider Utils 5.0.49, upstream 5d12eaa6caa193d3901cbab98a734403eb6bf622. Pinned cause-data extraction and status-derived retry semantics were rechecked during the split.

## Regression evidence

- Complete valid HTTP error envelopes survive in Data and ResponseBody; malformed/bounded responses remain protocol errors.
- Committed data is byte-exact, including whitespace, escapes and absent/null/empty distinctions; exact-case member selection and retryability mismatch rejection remain covered.
- Existing injected error/content/finish and high-level access tests pass without Gateway production changes.
- Client production and test files were extracted unchanged from reviewed commit 39354e21. Original pre-fix failures and review receipts belong to the combined implementation; this split reruns tests on its own base rather than claiming a new independent review.

## Passed on this branch

- Full Grafana client race tests, vet and golangci-lint (zero issues).
- Complete client tests under Go 1.26.8, GOTOOLCHAIN=local, GOWORK=off and readonly declared dependencies.
- MODULE=providers/grafana mise run verify-published-module.
- mise run parity-check, including ProviderWire contract/runtime checks.
- mise run test-ai-gateway-command: 66 tests, no skips; existing server behavior remains unchanged.
- Documentation lint, Gateway boundary, SDK/Gateway isolation and candidate-workspace checks.
- Strict OpenSpec validation and git diff --check.

## Limits

Injected responses prove client retention, not live-provider or private-service parity. The optional provider-shape report skips unavailable registered package source. No provider fixture inputs were modified. No Gateway producer, SDK retry policy, public error accessor or module baseline change is included. OpenSpec sync/archive remains pending review and separate approval.
