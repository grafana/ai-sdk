## Why

The existing `gateway/providerwire` server and `providers/grafana` client expose a tolerant Go-to-Go transport that conflicts with the strict ProviderWire V4 architecture planned against the registered `@ai-sdk/gateway@4.0.52` baseline. Retiring that legacy surface first prevents new releases from implying compatibility and creates a clean boundary for the strict protocol and client to be introduced in later work packages.

## What Changes

- **BREAKING** Remove the exported `gateway/providerwire` package, including its handler, codecs, routes, errors, and SSE helpers.
- **BREAKING** Remove the `github.com/grafana/ai-sdk/providers/grafana` module, including its legacy authentication, options, gateway-error normalization, and client implementation.
- Document root `v0.1.0-alpha.1` as the rollback point for root-module/server deployments only; preserve Grafana-client source at that repository tag and in Git history without claiming an independently fetchable nested-module version.
- Remove legacy-only `test/interop` coverage and Grafana provider-wire conformance while retaining provider conformance for Anthropic, Bedrock, OpenAI, and OpenAI-compatible implementations.
- Remove the retired module and test workspace from `go.work`, `mise` tasks, dependency manifests, baseline validation inputs, and CI-facing checks.
- Preserve provider-domain JSON behavior and tests, reword only comments or godoc that present provider structs as ProviderWire HTTP authority, and remove the two legacy transport rows from parity coverage.
- Remove or revise user documentation that advertises the retired server and Grafana client, while preserving documentation for transport-independent catalog, fallback, middleware, provider, and UI SSE capabilities.
- Update all active OpenSpec capabilities and parity coverage so no normative requirement or compatibility row still requires the deleted packages or legacy harness.
- Do not add compatibility shims, aliases, a replacement transport, or a new nested-module release in this change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `provider-wire`: Retire the legacy wire contract and retain provider-domain JSON only as non-HTTP representation behavior.
- `gateway-providerwire-server`: Remove the public legacy HTTP server contract and handler requirements.
- `grafana-provider`: Remove the nested legacy client module and all of its transport, authentication, registry, and test requirements.
- `grafana-provider-options`: Remove the option types owned by the deleted Grafana client module.
- `gateway-error-normalization`: Remove the normalized error types owned by the deleted Grafana client module.
- `conformance-testing`: Remove Grafana transport conformance and Grafana replay requirements while retaining direct-provider and provider-independent coverage.
- `upstream-parity-governance`: Remove the deleted interop workspace from the set of parity TypeScript consumers.
- `api-call-error`: Reframe JSON reconstruction as a provider-domain invariant rather than reconstruction through the retired transport.
- `docs-structure`: Remove the requirement for the deleted Grafana Cloud provider page.
- `gateway-model-catalog`: Keep the catalog transport-neutral without requiring composition with the retired handler.
- `prometheus-middleware`: Remove documentation requirements that depend on deleted `providers/grafana` option types.
- `context-enrichment-middleware`: Remove the stale Grafana-controls scenario and deleted-package references while preserving generic provider-option merging and middleware ordering.

## Impact

- Removes public Go import paths `github.com/grafana/ai-sdk/gateway/providerwire` and `github.com/grafana/ai-sdk/providers/grafana`.
- Removes the legacy `/language-model` handler implementation and its tolerant request, response, error, and SSE formats.
- Changes the repository workspace, task graph, TypeScript test workspace, conformance module dependencies, active OpenSpec capabilities, documentation navigation, parity coverage map, and rollback notes.
- Leaves `provider`, concrete provider modules, `gateway/catalog`, `fallback`, `registry`, middleware, root UI-message SSE support, retained TypeScript integration tests, and non-Grafana conformance behavior unchanged.
