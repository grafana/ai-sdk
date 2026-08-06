## Why

The current provider-wire implementation makes `provider` domain JSON methods simultaneously own canonical LanguageModelV4 encoding, legacy Go compatibility, and transport validation, while its HTTP resolver bypasses canonical catalog identity and gateway-wide middleware policy. Adding more public gateway façades on that foundation would compound hidden protocol coupling, unsafe error exposure, and inconsistent execution behavior.

## What Changes

- Add a transport-neutral LanguageModel execution runtime whose public input is a normalized `GatewayCall`: originating protocol, requested public model ID, provider `CallOptions`, parsed gateway control options, and trusted request metadata. An ordered pre-resolution policy/transform seam validates provider-bound headers/options and routing controls before a call-aware resolver adapts to `gateway/catalog`. The runtime preserves requested, canonical, and model-reported resolved identity, injects stable gateway metadata into model middleware context, invokes unary or streaming model calls, and owns shared cancellation and timeout behavior without importing `net/http` or façade DTOs.
- Add stable gateway failure categories backed by sentinels and wrapped private causes, plus a derived non-error classification value containing kind, retryability, private cause, and allowlisted safe parameters. Retryability is computed at the active boundary rather than inherited as an `errors.Join` marker. Public adapters independently map classifications to their own HTTP status and envelope shape.
- Add a new strict bidirectional LanguageModelV4 provider-wire codec and HTTP service alongside `gateway/providerwire`. It preserves the pinned `@ai-sdk/gateway@4.0.33` `/language-model` route, headers, JSON/SSE shapes, ordered repeatable error parts, and clean EOF behavior, but uses explicit V4 DTOs and validated conversions instead of `json.Marshal(provider.*)`.
- Define the new codec contract as semantic round-trip compatibility for valid supported LanguageModelV4 request, generate-result, and stream-part values. Contradictory unions, unknown discriminators, missing required fields, malformed tool input JSON, and unsupported values fail closed; strict decoders do not accept the legacy Go-only dialect.
- Keep the existing `gateway/providerwire` package, its HTTP-aware resolver, legacy-tolerant codecs, provider JSON methods, and the existing Grafana-provider default transport available for backward compatibility. Adoption is explicit and side-by-side in this change. The documented migration end state is strict V4 as canonical, Grafana no longer depending on legacy codecs, provider custom JSON no longer owning provider-wire serialization, and eventual legacy deprecation/removal in follow-up changes.
- Correct new-service HTTP behavior: standards-consistent content negotiation, bounded request reads and encoded-response transport limits, `http.ResponseController` flushing, no `Connection: keep-alive` application header, adapter-local 405/406/413/415 errors, pre-commit execution errors, and post-commit façade-specific stream errors. Encoding before checking limits prevents partial commitment but is not described as bounding encoder allocation.
- Keep provider request bodies, backend response headers/bodies, setup metadata, and backend `modelId` private. Strict V4 responses omit those values and allowlist only protocol-correct public metadata; a future Chat adapter may use canonical public identity for its `model` field.
- Strengthen executable compatibility evidence with successful unary stock-TypeScript interop covering the fields the pinned client preserves; strict-codec and Grafana assertions for allowlisted response ID/timestamp and omission of backend transport/model detail; `PartError` followed by `PartFinish`; malformed variant rejection; policy-before-resolution and middleware-context checks; resource-limit tests; and parity validation against the registered upstream baseline.
- Add an explicit strict-codec mode and response-size limits to the existing Grafana remote provider without changing its default legacy mode or existing constructor calls, and complete its normalized error vocabulary for strict-service `forbidden` and `failed_dependency` responses.
- Do not add Chat Completions, Responses, Anthropic Messages, a generic SSE framework, model discovery, host authentication, concrete routing/fallback policy, or removal of legacy provider JSON behavior in this change.

## Capabilities

### New Capabilities

- `gateway-runtime`: Normalized gateway calls, pre-resolution policy/transformation, call-aware catalog adaptation, trusted middleware context, LanguageModel invocation identity, and minimal lifecycle management reusable by façades that map losslessly to the provider LanguageModelV4 domain.
- `gateway-failure-classification`: Stable internal failure categories, derived retryability, private-cause preservation, allowlisted safe parameters, and protocol-owned projection rules.
- `gateway-providerwire-v4`: Strict bidirectional LanguageModelV4 DTO codecs and the new upstream-compatible HTTP/SSE service, including privacy allowlists, encoded transport limits, and interoperability evidence.

### Modified Capabilities

- `gateway-model-catalog`: New gateway services use the runtime's call-aware resolver seam with a default catalog adapter rather than requiring a host-written HTTP resolver; legacy composition remains supported.
- `grafana-provider`: Remote unary, error, and SSE reads become bounded and an explicit strict bidirectional codec mode is added while existing constructors and default legacy mode remain compatible.
- `gateway-error-normalization`: Grafana recognizes the strict service's `forbidden` and `failed_dependency` categories while preserving normalized causes and retry metadata.

## Impact

- New public packages under `gateway/` for the shared runtime and strict LanguageModelV4 service; package names and dependency direction are finalized in the design.
- Existing `gateway/providerwire`, `provider`, `gateway/catalog`, and `providers/grafana` APIs remain available. The Grafana provider gains bounded-read behavior and an opt-in strict codec mode; changing the default, deprecating legacy APIs, and deleting provider JSON wire ownership are follow-up migrations.
- Hosts can run the legacy and new handlers side by side and migrate explicitly while stock `@ai-sdk/gateway@4.0.33` and canonical Grafana requests retain the same external LanguageModelV4 wire contract. Shared success behavior is tested against both handlers; strict sanitization and legacy error-detail preservation are tested separately.
- Parity-sensitive work affects the provider contract, provider transport, frontend interop harness, and conformance governance. The registered baseline remains `@ai-sdk/provider@4.0.4`, `@ai-sdk/gateway@4.0.33`, and `ai@7.0.44`; there is no upstream server implementation for local lifecycle choices.
