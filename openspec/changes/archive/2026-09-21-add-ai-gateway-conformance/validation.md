# Validation evidence

## Implemented scope

The existing provider corpus drives both gateway clients without capability skips or expected-failure passes. Both clients call the AI Gateway plugin through one Grafana stack with deterministic replay backends. No production gateway implementation, provider captures, golden snapshots, or registered upstream versions were changed.

The runtime was redesigned after the gateway moved to the Grafana App Platform plugin. A spike against the pinned Grafana nightly confirmed that Grafana service account tokens reach the plugin route, SSE events arrive incrementally, the plugin's startup reason appears in Grafana's logs, and Grafana reaches host replay servers through `host-gateway`. An unsupported provider type makes Grafana exit, which led to progressive provider admission.

## Commands

Passed:

- `mise run test-conformance-gateway-harness`: Go conformance tests and 159 TypeScript tests, including read-only replay and comparison of all 141 provider fixtures through the extracted direct TypeScript path, now also checking provider stream parts.
- `mise run generate-conformance`: regenerated every provider fixture with the extracted execution path; all provider fixture expectations were byte-identical. One synthetic file outside this change, `testdata/anthropic-prompt-conversion/expected-results.jsonl`, regenerates differently on the merged main baseline and was left unchanged.
- `mise run parity-check`, `mise run verify-ai-gateway-boundary`, `mise run test-ci-workflow`, `mise run lint-docs`.
- `cd test/conformance && GOWORK=off go vet -tags conformance ./...` and `golangci-lint run --build-tags conformance ./...`: zero issues.
- `openspec validate conformance-testing --type spec` and `git diff --check`.

## Full gateway matrix

`mise run test-conformance-gateway` ran in 102 seconds and exited nonzero as intended. The plugin package combined the backend built from this revision with the frontend from main's CI artifact for the merged commit, because the local sandbox could not load the frontend build's native SWC binding; `ai-gateway/` is identical between the two revisions.

| Result | Rows |
| --- | ---: |
| Passed | 2 |
| Failed comparison | 218 |
| Failed provider setup (Bedrock) | 62 |
| Total | 282 |
| Client executor invoked | 220 |

The stack booted twice: the first boot's plugin rejected `providers.row-N.type "bedrock"`, and the second boot without Bedrock became ready. There were no harness errors, missing or duplicate rows, or leftover containers.

Of the 218 comparison failures, 192 differ only where the gateway adds `providerMetadata.gateway.execution` to the finish part, as observed through the provider-parts comparison. This count is an upper bound because that comparison reports the first mismatch per model call. The other 26 rows differ in UI chunks, backend requests, provider parts or execution errors. Intentional gateway metadata is not normalized away.

Local artifacts: `test/conformance/gateway-results/report.json`, `summary.md`, and `row-*.json` (ignored, not committed). CI uploads the same directory and links it from its summary.

## CI policy

The `Gateway conformance` job builds the plugin package and uses the Grafana image pinned for the plugin end-to-end tests; a workflow test keeps them equal. The required `conformance-test` status succeeds only when both direct and gateway conformance succeed. Plugin package validation does not depend on the gateway job.

Remaining limitations: the matrix blocks merging until it passes; whether the gateway execution metadata becomes a reviewed equivalence is open; the harness requires Linux with a local Docker daemon.
