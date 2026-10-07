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

## Review cleanup

All validation gates above passed again after simplifying selection/cancellation
and sharing the native-option guard. The command suite passed all 57 cases and
the image-source regression ran without skips. Strict OpenSpec 1.14.0 validation
also passed after updating the selection contract and removing a stray read marker.

Before the main integration below, a failing BYOK regression demonstrated native I/O for Anthropic MCP
servers/history, skills and server-side fallback. Unary/streaming tests now prove
zero native calls for those controls, with ordinary-option acceptance. The
configured validator/wrapper moved unchanged apart from package/exported names
into internal/nativeoptions; configured compatibility checks remain covered by
the existing service suite. BYOK applies the Anthropic guard outside fallback.

Selection now returns only model and identity; the handler retains mapped native
options. Unary invocation has one cancellation authority while retaining child
cancellation, buffered completion and panic containment. Unused account-access
and BYOK-discovery errors are deferred to activation.

PR #369 was not changed. Its eventual rebase must own those activation errors,
stop returning Selection.Options and remove its duplicate Anthropic guard now
provided by the engine.

## Main integration

Rebased onto PR #367 at `6f00c0e8`, which merged origin/main `8d36b82d`.
Provider-tool unresolved-history mapping now runs before selection and remains
available to unary and streaming output, alongside the shared execution deadline.
Opaque metadata and native MCP support from main remain intact.

The shared native-option implementation matches main byte-for-byte apart from
package/exported names. Its private MCP bounds test moved unchanged into the
nativeoptions package rather than exposing validation internals. New main test
call sites use CatalogSelector.

BYOK tests now follow main's MCP contract instead of the earlier blanket refusal:
valid configuration and matching provider-owned history reach native I/O, local
MCP-looking markers remain inert, and invalid destinations/unconfigured history,
skills and native fallback fail before attempts. Unary and streaming tests
continue asserting the fixed inference host/model and request-supplied key.

All listed build, test, race, vet/lint, parity, integration, boundary, workflow,
pin, provider-shape and image gates passed again; the command suite passed 74
tests. Strict OpenSpec 1.14.0 validation passed for this active change. The two
unchanged main-spec Purpose warnings recorded in PR #367 remain outside this
merge/rebase. Registered upstream package versions are unchanged.

PR #369 remains at `4e5bd486`; it has not been rebased or pushed.

No module pins, upstream baseline or authentic provider fixture inputs changed.
Local fake services and image tests do not establish live native acceptance,
Vercel hosted-service behavior or deployed Cloud/network authorization.

The command and config validator use `CatalogSelector`; no `NewBYOKSelector`
is installed. Existing command cases still exercise the old modes and catalog.
The new engine has focused native HTTP/race tests, while selector tests prove
one logical invocation and a budget shared with selection. Service activation,
operator BYOK capture and deployed policy remain the next change's responsibility.
