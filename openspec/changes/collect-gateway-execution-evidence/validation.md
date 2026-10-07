# Collection foundation validation

## Scope and prerequisites

Second PR of #322, stacked above retain-grafana-error-extensions and #332. This change introduces the internal collector/projection and schema without service or handler activation. All implementation and schema/test files match the corresponding files in 39354e21. Public carrier delivery is deliberately deferred to expose-gateway-attempts-and-failures.

Reference remains ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. The namespace is a Grafana extension, not a claim about Vercel private-service output.

## Proof

- Collector tests cover 16/17 attempts, selected identity beyond the attempt cap, cancellation/sealing, immutable namespace relocation, exact optional allocations and concurrent snapshots.
- Projection tests cover known credential fields and echoes, escaped codes, duplicate members, complete redacted-component bounds, disposition states and useful ordinary detail.
- Raw projection tests cover string/key/surrogate/numeric fidelity, HTML expansion and decoder-buffer refill.
- Strict schema tests validate routing/evidence, complete attempts, overflow and native-error dispositions and reject invalid shapes.
- No production Go file imports the collector at this stack entry; the existing command suite still exercises the old public failure projection.

## Passed on this branch

- mise run test-ai-gateway.
- Collector, ProviderWire and service race tests with go.gateway.work.
- Gateway vet and golangci-lint: zero issues.
- mise run parity-check, including TypeScript typecheck, 121 schema tests and existing client/runtime tests.
- mise run test-ai-gateway-command: 66 tests, no skips.
- Documentation lint, Gateway boundary, SDK/Gateway isolation and candidate-workspace checks.
- Strict OpenSpec validation and git diff --check.

## Limits

Package/schema tests do not establish real-handler carrier assembly, operator-sink independence or frontend interoperability; those proofs ship with activation. Original combined review receipts are historical, not a new review of this split. The optional provider-shape report still lacks registered package source. No provider fixture inputs changed. The approved Gateway Go minimum is 1.27; client Go remains 1.26.3. Sync/archive and release remain outside this operation.
