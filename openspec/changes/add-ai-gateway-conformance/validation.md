# Validation evidence

## Implemented scope

All 25 implementation tasks are complete. The existing provider corpus drives both gateway clients without capability skips or expected-failure passes. No production gateway implementation, provider captures, golden snapshots, or registered upstream versions were changed.

The container harness uses a Linux-local Docker internal bridge, replay backends, and an ephemeral signing key/JWKS service. Unsafe authentication was not used because its production guard requires a loopback listener. Missing replay/configuration adapters fail independently of fixture discovery.

## Commands

Passed:

- `mise run test-conformance-gateway-harness`: existing Go conformance cases and 150 TypeScript tests, including read-only replay/comparison of all 136 provider fixtures through the extracted direct TypeScript execution path.
- `mise run parity-check`: baseline validation, fixture provenance/coverage, provider shape, ProviderWire contract tests, and full direct conformance.
- `mise run verify-ai-gateway-boundary`: module/license boundary builds and tests.
- `cd test/conformance && GOWORK=off go vet -tags conformance ./...`.
- `cd test/conformance && GOWORK=off golangci-lint run --build-tags conformance ./...`: zero issues.
- Final TypeScript typecheck and 13 focused runtime/replay/inventory/CI tests after reporting changes.
- `openspec validate add-ai-gateway-conformance --strict` and `git diff --check`.

## Full gateway matrix

`mise run test-conformance-gateway` built the production image and exited nonzero as intended:

| Result | Rows |
| --- | ---: |
| Passed | 8 |
| Failed comparison | 122 |
| Failed unclassified startup | 142 |
| Total | 272 |
| Client executor invoked | 130 |

The eight passes are seven Anthropic Go-client cases and one OpenAI-compatible Go-client case. OpenAI Responses and Bedrock attempts account for the 142 pre-readiness failures. The inspected service lacks these backend constructors, but its production startup logs expose only `process_failure`; the report therefore does not claim a verified provider-rejection cause. It retains each attempted configuration, Docker state, and sanitized logs as harness failure evidence.

Execution failures include rejected high-level tool-choice defaults and unsupported request/response families. These are compatibility gaps, not expected passes. Intentional gateway privacy/metadata differences are not suppressed.

There were no global report errors or missing/duplicate rows. Docker container/network inventories contained no `conformance-*` resources after execution. Provider input files and expected artifacts remained unchanged.

Local artifacts: `test/conformance/gateway-results/report.json`, `summary.md`, and `row-*.json` (ignored, not committed). CI uploads the same directory and links it from its summary.

## CI policy and review

The independent `Gateway conformance (advisory)` job preserves a failing conclusion. It is not added to required direct checks or publication/deployment dependencies. GitHub's effective main-branch rules were inspected with `gh api repos/grafana/ai-sdk/rules/branches/main`; the new check was absent and the existing `conformance-test` check remained required. No repository settings were changed.

An independent read-only review identified resource-creation cleanup, lost startup diagnostics, unsupported-provider attribution, and missing-adapter attribution issues. All were addressed with focused regression tests. A subsequent independent review could not start because the local subagent runner module was unavailable; final verification was performed by the implementing agent. No claim of a clean second independent review is made.

Remaining limitations are explicit: advisory parity is incomplete; opaque startup failures cannot establish a specific rejection cause; image networking currently requires Linux with a local Docker daemon. A later change can promote gateway conformance to a required check once the complete matrix passes.
