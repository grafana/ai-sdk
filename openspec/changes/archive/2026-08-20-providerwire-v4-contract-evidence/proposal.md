## Why

The repository lacks an independently verified, presence-correct contract for the exact ProviderWire V4 requests emitted by the registered `@ai-sdk/gateway` client. Establishing that contract and its evidence corpus now is required before changing the Go provider request model or implementing a strict V4 runtime, while preserving the deployed tolerant ProviderWire transport unchanged.

## What Changes

- Add a dedicated exact-pinned `test/providerwire-v4` workspace that classifies the complete LanguageModelV4 request surface and Gateway serializer behavior against the registered upstream baseline.
- Add deterministic unary, streaming, and multi-step request scenarios with a local recording server, strict raw-body parsing before normalization, stable semantic body and outer-header captures, exact-key composition and pre-Fetch lower-case last-value normalization assertions, observation assertions, and schema validation.
- Add a normative strict ProviderWire V4 request JSON Schema and shared definitions covering request envelope, presence, nullability, empty values, ordering, and tagged unions.
- Add an evidence-backed Phase 2 delta table comparing the complete captured surface with the current Go provider model, with executable witnesses for every demonstrated lost distinction.
- Add focused, locally authored black-box client-consumption probes for unary JSON, SSE framing with clean EOF, and non-2xx errors, with the pinned Gateway error path included in source-equivalence evidence, without claiming exhaustive response compatibility.
- Register the workspace as an exact upstream-baseline consumer in validation and upgrade tooling.
- Add non-mutating `mise run check-providerwire-v4` and explicit artifact-writing `mise run update-providerwire-v4-artifacts` workflows.
- Update the parity coverage map to state the exact request-contract and client-consumption evidence provided by this phase and the remaining runtime gaps.
- Keep the existing `gateway/providerwire` implementation and all production provider APIs unchanged.

## Capabilities

### New Capabilities

- `providerwire-v4-request-contract`: Defines the strict, pinned ProviderWire V4 language-model request envelope and JSON payload contract, including presence and union semantics, without introducing a production runtime.
- `providerwire-v4-contract-evidence`: Defines the authoritative request-surface inventory, deterministic pinned-client evidence, Phase 2 delta evidence, smoke-level response probes, and maintenance workflows.

### Modified Capabilities

- `upstream-parity-governance`: Extends exact baseline-consumer validation and coordinated upgrade coverage to the ProviderWire V4 evidence workspace and its generated artifacts.

## Impact

- Adds a new TypeScript workspace and committed request-contract evidence under `test/providerwire-v4`.
- Extends `test/conformance/tools` baseline validation and upgrade consumer lists, focused tooling tests, the test workspace configuration and lockfile, and `mise.toml` workflows.
- Adds strict request schemas, request-surface classification, semantic captures, Go-model delta evidence, and focused client-consumption probes.
- Updates `test/conformance/PARITY.md` and OpenSpec contracts for the new parity surface.
- Does not change `provider.CallOptions`, provider implementations, legacy ProviderWire bytes, production handlers or clients, frontend wire output, authentication, policy, or service deployment.
