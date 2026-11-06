## 1. Host composition

- [x] 1.1 Construct Chat on the same runtime as ProviderWire and register its
  route entry with the Chat host-error writer.
- [x] 1.2 Update CUE/generated manifests and route-table/manifest tests.
- [x] 1.3 Preserve shared selection, translation and access policies without
  protocol-specific catalog or access dispatch.

## 2. Protocol clients and evidence

- [x] 2.1 Prepare official Go/TypeScript clients against the real plugin through
  platformhost with synthetic native backends.
- [x] 2.2 Pass route/manifest and Go client suites on the final source.
- [x] 2.3 Pass TypeScript typecheck and client suite on the final source.
- [x] 2.4 Verify focused runtime/Chat tests including sequential-segment refusal
  and record exact evidence and remaining deployment limits.

## 3. Spec integration

- [x] 3.1 Restate the three Chat cuts over Runtime/Call/Stream, Translator and
  host.ErrorWriter, with #483 explicitly pending.
- [x] 3.2 Validate all Chat changes and synced specs strictly, sync this route
  change and archive after its scoped checks pass.

#483 lifecycle integration, broad merge/release gates and deployed Grafana
verification remain separate prerequisites; these tasks do not complete them.
