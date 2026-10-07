# Service activation evidence

## Validation

Validated on this branch after rebasing onto main at `d863b86e` (#326/#328).
All listed gates were rerun on each stack branch; the final command suite passes
60 cases, including main's mapped fallback and high-level default-token coverage:

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

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

## Stack preservation

- Original split base: `69bdfa02503c96256c0f23594ad396542de3c374`.
- Rebased stack base: `d863b86e` (main, including #326/#328).
- Backup: `nrbrd/byok-before-stack` (`19cb6a09`), including the public
  test-only TLS certificate/key that were previously hidden by the PEM ignore rule.
- Stack: `nrbrd/byok-client-safety` → `nrbrd/byok-runtime` → `nrbrd/byok`.
- Before rebasing, the final non-OpenSpec tree matched the backup exactly.
  The rebase preserves main's opaque native options and mapped fallback behavior
  rather than restoring deleted policy inventories or text-only wrappers.
- Original requirements are accounted for across the three changes. Caller-owned
  capture guidance moves to grafana-gateway-client; the transport selector bound
  is explicit. The obsolete selected-backend policy delta is dropped in favor of
  main's gateway-native-provider-options contract.
- Client and engine changes are complete. Deployment task 4.1 remains externally
  owned; no environment activation or deployed network proof is claimed.

## Rebase evidence

The operator authorized rebasing before draft PR publication. Conflict resolution
preserves main's native-option/fallback tests, documentation and coverage map,
including unwrapped high-level TypeScript calls and native token defaults.

BYOK Anthropic composition reuses the existing consumption guard for MCP
servers/history, container skills and provider-side fallback. A failing regression
first demonstrated native I/O without that guard; unary/streaming tests now prove
zero attempts for consumed bypasses and retain ordinary-field negative controls.
Configured guard implementations and native converters remain unchanged.

Local gates passed; GitHub CI is separate evidence and is not claimed here.
