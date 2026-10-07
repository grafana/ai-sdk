# Service activation evidence

## Validation

Validated on this branch, independently of later stack changes:

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

- Base: `69bdfa02503c96256c0f23594ad396542de3c374`.
- Backup: `nrbrd/byok-before-stack` (`19cb6a09`), including the public
  test-only TLS certificate/key that were previously hidden by the PEM ignore rule.
- Stack: `nrbrd/byok-client-safety` → `nrbrd/byok-runtime` → `nrbrd/byok`.
- The final non-OpenSpec tree matches the backup exactly.
- Every original requirement is retained across the three changes. Caller-owned
  capture guidance moves from gateway-request-byok to grafana-gateway-client;
  the client's transport selector bound is now an explicit requirement.
- Client and engine changes are complete. Deployment task 4.1 remains externally
  owned; no environment activation or deployed network proof is claimed.

## Publishing status

The split was validated on its original base, not a merge with newer main.
Main has since advanced with #326 (native provider options) and #328 (fallback
capabilities). A non-mutating merge check of the first branch against origin/main
reports conflicts in gateway-command.test.ts, docs/providers/grafana-gateway.md
and test/conformance/PARITY.md. Publishing/rebasing is paused for operator direction;
no merged-candidate CI success is claimed. A rebase must preserve those newer
contracts and rerun each slice's gates.
