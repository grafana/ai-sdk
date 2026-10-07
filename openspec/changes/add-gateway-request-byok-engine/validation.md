# Engine evidence

## Validation

Validated on this branch, independently of later stack changes. All gates were
rerun after rebasing onto main at `d863b86e` (#326/#328). Catalog selection now
retains opaque native options rather than restoring the deleted policy inventories.
All gates below also passed after rebasing onto reviewed PR #367 at `58487635`,
including its capture simplification, consolidated tests and synchronized/archived
client specs. Range-diff confirmed the three engine commits reapplied unchanged.
This engine change remains active for review; the activation branch is untouched:

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

Review consolidation keeps the new BYOK package's tests in model_test.go and
moves selection cases into the existing handler suite. Test bodies are unchanged.
BYOK/ProviderWire race tests, vet, lint, parity and source integration passed after
consolidation; production code and other stack branches were untouched.

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

The command and config validator use `CatalogSelector`; no `NewBYOKSelector`
is installed. Existing command cases still exercise the old modes and catalog.
The new engine has focused native HTTP/race tests, while selector tests prove
one logical invocation and a budget shared with selection. Service activation,
operator BYOK capture and deployed policy remain the next change's responsibility.
