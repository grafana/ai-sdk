# Client retention validation

## Scope and reference

First PR of #322, initially based on #332 at c32f3f6d and subsequently rebased onto main at 1be616c3 after #332 merged. This change owns only independent client error retention and its documentation/tests. Collection/schema and production emission belong to the next two changes; this PR does not claim those are implemented.

Reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18, Provider Utils 5.0.49, upstream 5d12eaa6caa193d3901cbab98a734403eb6bf622. Pinned cause-data extraction and status-derived retry semantics were rechecked during the split.

## Regression evidence

- Complete valid HTTP error envelopes survive in Data and ResponseBody; malformed/bounded responses remain protocol errors.
- Committed data is byte-exact, including whitespace, escapes, numeric lexemes and absent/null/empty distinctions; standard Go error-member casing and retryability mismatch rejection are covered.
- Existing injected error/content/finish and high-level access tests pass without Gateway production changes.
- Client production and test files were initially extracted unchanged from reviewed commit 39354e21. Original pre-fix failures and review receipts belong to that combined implementation, not the later simplification. Retention tests were consolidated into errors_test.go during PR review.

## Passed on this branch

- Full Grafana client race tests, vet and golangci-lint (zero issues).
- Complete client tests under Go 1.26.8, GOTOOLCHAIN=local, GOWORK=off and readonly declared dependencies.
- MODULE=providers/grafana mise run verify-published-module.
- mise run parity-check, including ProviderWire contract/runtime checks.
- mise run test-ai-gateway-command: 66 tests, no skips; existing server behavior remains unchanged.
- Documentation lint, Gateway boundary, SDK/Gateway isolation and candidate-workspace checks.
- Strict OpenSpec validation and git diff --check.

## Owner-approved error decoder simplification

The owner approved standard Go JSON member matching instead of maintaining exact-name error filtering. HTTP and SSE errors now decode directly into private typed envelopes; SSE data remains json.RawMessage from the original event. Removed decodeWireError, jsonField and decodeObjectMembers without changing the other response-family decoders.

HTTP and SSE noncanonical-casing regressions failed against the previous decoder and pass after the change. Raw retained data, malformed/missing/null envelopes, classification, retry consistency, bounds and ordered consumption remain covered. Case-insensitive error member acceptance is documented as an intentional Go adaptation in PARITY.md and the client delta; it does not change the server's canonical output or claim identical handling of noncanonical input by the pinned TS client.

All checks above were rerun after simplification: full client races/vet/lint, Go 1.26.8 and published-dependency checks, parity (108 schema / 96 client-runtime tests), 66 command tests, docs and isolation/boundary/workspace checks. No provider input fixture changed.

## Main merge and archive verification

Merged origin/main at 8d36b82d after provider-tool/MCP delivery. Resolved stream.go by retaining main's variant-local decoding and new tool markers alongside this change's original-byte error decoder. Workspace root/Gateway/client tests, client races/vet/lint, source integration, parity and frontend checks pass: 120 schema tests, 110 client/runtime tests, 74 command tests and 23 frontend files / 132 tests.

The merge exposed an inherited standalone dependency failure: provider.ValidateTools was referenced by the client but absent from its old SDK pin. With owner approval, providers/grafana now requires the merged, remotely resolved SDK version v0.1.0-alpha.1.0.20261007173843-8d36b82dfd52. Go 1.26.8 standalone race tests, standalone vet, the published-client gate and merged-pin ancestry checks pass with the updated go.mod/go.sum. No replace directive or Go baseline increase was introduced.

The owner authorized syncing the client delta into openspec/specs/grafana-gateway-client/spec.md and archiving this completed change on 2026-10-07. This archives only the client-retention change, not the downstream collection/runtime changes.

## Limits

Injected responses prove client retention, not live-provider or private-service parity. The optional provider-shape report skips unavailable registered package source. No provider fixture inputs were modified. No Gateway producer, SDK retry policy, public error accessor or module baseline change is included. OpenSpec sync/archive is owner-approved; downstream PR delivery and release remain separate.
