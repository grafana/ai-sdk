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

Strict OpenSpec validation also passed with exact CLI version 1.14.0. Active
changes still require archiving before merge.

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

The command still uses the pre-existing authentication modes and configured
catalog. Two command-test expectations move with the client change: reserved
headers fail locally, and Cloud calls use the Cloud constructor rather than
an access-token constructor with an overriding Authorization header.
