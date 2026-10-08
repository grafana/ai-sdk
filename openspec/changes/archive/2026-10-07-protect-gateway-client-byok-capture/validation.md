# Client/capture evidence

## Validation

Validated on this branch, independently of later stack changes. All gates were
rerun after rebasing onto main at `d863b86e` (#326/#328); the newer high-level
client, native-option and mapped-fallback coverage remains intact:

- `mise run test-short`, `build`, `vet`, `lint`, `parity-check`.
- `mise run test-ai-gateway-source-integration` and `test-integration`,
  including pinned-client typechecks and real-command tests.
- `mise run lint-docs`, `verify-ai-gateway-boundary`,
  `verify-sdk-gateway-isolation`, `verify-merged-pins`, `test-ci-workflow`.
- `mise run test-ai-gateway-image-source` (not skipped) and
  `build-ai-gateway-image`.
- SDK race tests for Grafana, logger, Agent Observability, enrichment and fallback;
  Gateway race tests for command internals and ProviderWire.
- Explicit provider-shape comparison against installed registered
  `@ai-sdk/provider@4.0.18` source; all discriminator families match.
- Strict OpenSpec validation and `git diff --check`.

## Review cleanup

All gates above were rerun after the simplification. A failing regression first
demonstrated over-redaction of ordinary tool-output gateway/byok fields and
gateway.providerTimeouts.byok; it now passes. Typed and nested provider-option
capture, unary/streaming request and error bodies, custom-redactor ordering,
truncation and original-data ownership remain covered. The command and frontend
integration suites each passed 57 tests; the image-source test ran without skips.

The reflection normalizer and TypeScript helper were removed. Capture now uses
JSON copies and explicit request-body object validation; it does not promise
arbitrary-object normalization or recursive byte decoding. Unused high-level
BYOK test-harness plumbing is deferred to the activation slice. Other stack
branches were not changed or rebased.

All seven new test files were subsequently consolidated into existing suites;
test bodies remain byte-for-byte unchanged. Grafana/logger/Agent Observability
race tests, vet, lint, parity, source integration and CI workflow tests passed
after consolidation. No production code or other stack branches changed.

Strict OpenSpec validation passed with exact CLI version 1.14.0. The completed
change was synchronized into the client and logger main specs and archived on
2026-10-07 after review approval.

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

The command still uses the pre-existing authentication modes and configured
catalog. Two command-test expectations move with the client change: reserved
headers fail locally, and Cloud calls use the Cloud constructor rather than
an access-token constructor with an overriding Authorization header.

## Current-main conflict resolution

Merged origin/main ddaa841a, preserving additive Gateway HTTP/SSE error data,
aggregate usage and Anthropic tool-choice behavior alongside client authentication
and BYOK capture protections. The sole conflict was in the client guide; it now
retains both bounded error-extension guidance and reserved authentication-header
ownership. The registered upstream package versions remain unchanged.

All gates listed above passed again, including fresh SDK/Gateway races, explicit
provider-shape comparison and the source/image checks. The command suite passed
74 tests with no skips. Logs are under /tmp/byok-stack-pr1-restack-20261008/.
The client/capture archive remains complete; later stack slices retain their own
validation and service-activation ownership.

## Standalone client test dependency repair

Readonly standalone Grafana tests exposed an undeclared logger import in the
client/logger composition witness. Moved all six capture cases and assertions to
`test/integration/testserver/grafana_logger_capture_test.go`, already exercised by
`mise run test-integration`. The local-only SDK harness uses explicit source
replacements; published Grafana/logger manifests and runtime code are unchanged.
The synthetic unary fixture and test body are identical apart from public client
qualification and fixture scope. No capture coverage or provider input was removed.

Grafana standalone readonly races pass on Go 1.26.8 and the current toolchain;
`MODULE=providers/grafana mise run verify-published-module` passes from a fresh
public-proxy cache. The local-only harness passed ten readonly race repetitions,
and workspace Grafana/logger/harness races and all full gates above passed.
Logs: `/tmp/pr367-{grafana-standalone-green,grafana-go126-standalone,grafana-published-module,capture-local-module,capture-workspace}.log`
and `/tmp/byok-stack-pr1-test-module-repair-20261008/`.
