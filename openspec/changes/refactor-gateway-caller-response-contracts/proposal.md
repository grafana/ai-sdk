## Why

Gateway response restrictions currently treat useful provider metadata, warnings, sources and identity as shared Grafana-account secrets. Issue #303 revises that assumption: customers retain their provider relationship and billing, while tenant authorization, credential protection, execution safety and telemetry privacy remain independent obligations; the current startup-configured credentials are not BYOK provisioning.

## What Changes

- Establish an explicit restriction inventory with retained, removed and deferred policies and concrete rationales, separating authorized caller data from telemetry and operator configuration.
- Preserve unary warnings and meaningful stream warnings; stop rewriting native source IDs/display fields and streaming response model identity. Emit unary identity only at registered response fields, documenting the pinned client's transport overwrite and typed-identity boundary.
- Treat supported opaque provider metadata as ordinary bounded response data. **#280 exclusively owns metadata codecs, client filters and continuation implementation**; this proposal coordinates the revised contract and dependent acceptance, not a duplicate implementation.
- Replace blanket provider-error concealment with adapter-reviewed bounded messages/status/code/type/details in the registered envelope; bind diagnostics to a trusted AGPL-only per-candidate provenance wrapper before fallback/logical identity composition. Project only the authoritative candidate failure, retain fixed Gateway-internal errors and protect credential-bearing transport, pinned retry and stream termination semantics.
- Define debugging as Gateway-owned client request/response transport plus registered bounded response identity and reviewed error diagnostics—not native transport passthrough.
- **BREAKING** for consumers relying on normalized `source-N` IDs, numeric-only `citation` metadata, fixed warning prose, canonical stream model IDs or fixed provider-error code/status combinations; independent Go `GatewayError.Code` becomes JSON-valued to preserve string/number/null codes, with bounded JSON `Param` details. Retain strict protocol unions and existing lifecycle/security guards; no compatibility dialect.
- Deliver focused independently green behavior packages with normative specs, schemas, client mappings, docs and two-client evidence in each implementation PR. Do not rewrite historical milestone acceptance.

## Capabilities

### New Capabilities
- `gateway-caller-response-policy`: Restriction dispositions, credential/tenant provenance, bounded actionable provider errors, debugging and response/telemetry separation.

### Modified Capabilities
- `providerwire-v4-unary-runtime`: Supported warnings, metadata integration, registered response identity and provider-versus-internal failures.
- `providerwire-v4-streaming-runtime`: Value-preserving warnings, actual response identity, metadata integration and bounded non-terminal provider errors.
- `grafana-gateway-client`: Independent bounded decoding without concealment filters, registered error details and explicit unary transport/identity boundary.
- `gateway-sources`: Native identity/display and bounded metadata instead of response-local citation projection.
- `gateway-reasoning-content`: Replace the closed continuation allowlist under #280, retaining replacement and lifecycle semantics.
- `gateway-unary-function-tools`: Supported tool-call metadata under #280 without changing execution ownership.
- `gateway-streaming-function-tools`: Supported tool-event metadata under #280 without enabling provider execution.
- `providerwire-v4-http-contract`: Correct unary warning combination evidence and registered-client response-identity/error consumption proof.
- `gateway-ordered-text-fallback`: Preserve caller response identity/reviewed diagnostics and authoritative exhausted-fallback provenance while retaining ordering, execution/commitment, private attempt observation and topology protections.
- `gateway-provider-configuration`: Replace blanket caller backend/error-body concealment with source-specific credential/configuration/telemetry protections and trusted construction-time error policy.

## Impact

Server: `ai-gateway/providerwire/v4/{response,stream,errors,sources,reasoning}.go` and response schemas; an AGPL-only internal providererrors wrapper/projection seam is installed per physical candidate in `service/catalog.go` before fallback and logical composition. Catalog/provider/core interfaces remain unchanged. Active service/fallback contracts are revised alongside runtime behavior; option/discovery and telemetry policies are audited, not blanket opened. Client: private codecs in Apache `providers/grafana`, never importing AGPL server DTOs. Tests: ProviderWire TypeScript workspace, independent Go client, authenticated command, authentic two-client conformance, provider request continuation and UI integration where affected. Docs and stable parity boundaries update with implementation. Full authentic two-client matrix acceptance depends on delivery of #201's currently open draft harness; current main's focused ProviderWire/command checks and direct conformance are not that matrix.

The reference stays commit `4e8c387622ee1bb0d55841664416d38754d5c9a3` (`ai@7.0.109`, `@ai-sdk/gateway@4.0.88`, `@ai-sdk/provider@4.0.17`). #299's later error target is separate. No credential provisioning/storage, native API adapters, unsupported content/tools, catalog configuration dump, core termination redesign or baseline upgrade is included. This plan is not implementation or acceptance proof.
