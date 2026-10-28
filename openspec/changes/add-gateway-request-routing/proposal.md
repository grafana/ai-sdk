# Proposal

## Why

Vercel Gateway clients route each request with `providerOptions.gateway.models`, `order` and `only`. Our Gateway refuses these controls: configured callers get the reserved-provider-options error, and BYOK callers get the unsupported-control error. Consumers can only use the chain each route was configured with. #316 asks the Gateway to accept these controls within each caller's authorized accounts, alongside configured routes, and to report how a request was routed. The runtime (#555), BYOK attempt overviews (#420) and the host contract (#581) are now in place, so routing can reuse their selection, attempt capture and per-attempt option translation.

## What Changes

- **Request routing controls.** Both access modes accept `models`, `order` and `only` under call-level `providerOptions.gateway`. The pinned client already forwards them unchanged.
  - `models`: ordered fallback model IDs, tried after the requested model. Configured callers name public route IDs or aliases. BYOK callers name `provider/model` selectors backed by their `gateway.byok` accounts.
  - `only`: an exclusive provider filter, applied to each model's candidates before ordering.
  - `order`: a provider preference. Matching candidates move to the front of each model's candidates, and the rest keep their configured order. It does not filter.
  - Provider slugs are the `provider` values that discovery and `gateway.execution` already report.
- **Deterministic composition.** The effective chain is the requested model's candidates, then each `models` entry's candidates, with `only` and then `order` applied per model. Without controls, the prebuilt route is used unchanged. With controls, one request-scoped fallback chain is composed from the already-constructed physical candidates, beneath one logical middleware chain. Controls can reorder, filter or append authorized models. They can never add credentials, endpoints or another account population.
- **Upfront diagnostics.** These fail before any provider call, with fixed messages that never echo values: malformed controls, a conditional `models` entry (upstream allows these only on decision requests), an unknown `models` entry, BYOK entries without accounts, `only` excluding every candidate, and a composed chain above the 8-attempt bound.
- **`providerTimeouts.byok`.** BYOK callers can set a per-provider wait, in milliseconds, for a provider to start responding. When it expires, that attempt fails as an eligible timeout and the chain moves to the next account. It has no effect once the provider has started responding, and it does not change commitment.
- **Routing metadata.** A `gateway.routing` object sits next to `gateway.execution` wherever an execution overview is published. It is a subset of Vercel's documented routing fields: requested model, canonical slug, first and final provider, and per-model attempts with per-provider outcomes, projected from the same capture. `gateway.execution` is unchanged.
- **Go client (`providers/grafana`):**
  - a typed `GatewayOptions` provider option with `Models`, `Order`, `Only`, `BYOK` and `ProviderTimeouts.BYOK`. It mirrors the supported subset of upstream's exported `GatewayProviderOptions` type, encodes the same JSON the pinned TypeScript client sends, and replaces raw-JSON `gateway` options in the docs;
  - a typed reader for `providerMetadata.gateway.execution` and `gateway.routing`, on results, stream parts and error-carried metadata;
  - no client-side validation: the Gateway stays the only authority.
- **BREAKING (wire):** the unsupported-control message changes from "only gateway.byok is supported" to one listing the supported controls. Configured callers that send `byok`, `providerTimeouts` or other unsupported controls now get the unsupported-control category instead of reserved-provider-options.
- Controls outside this scope, such as `sort`, `has`, `zeroDataRetention`, `disallowPromptTraining`, `user`, `tags`, `caching`, `serviceTier` and `quotaEntityId`, are still refused explicitly rather than silently ignored.

## Capabilities

### New Capabilities

- `gateway-request-routing`: request routing controls, their grammar and precedence, the composed chain, authorization boundaries, routing diagnostics, `providerTimeouts.byok`, and the evidence that proves them.

### Modified Capabilities

- `gateway-execution-runtime`: configured selection accepts routing controls, and the classification gains a routing-invalid category and a fallback-model reason. Attempt capture carries ordered model groups.
- `gateway-execution-overview`: the Gateway namespace gains the `gateway.routing` projection next to `gateway.execution`.
- `gateway-request-byok`: `byok` is no longer the only accepted control. Fallback models span providers with supplied accounts, and the attempt bound applies to the composed chain.
- `grafana-gateway-client`: the typed `GatewayOptions` request option and the typed Gateway metadata reader.
- `gateway-ordered-text-fallback`: request routing controls may now reorder, filter and extend configured candidate order.
- `gateway-resource-policy`: the 8-attempt bound applies to the composed request chain.
- `providerwire-v4-unary-runtime`: the configured reserved-options refusal is replaced by routing-control handling, and the fixed routing error documents are added.

## Impact

- **Code:**
  - `ai-gateway/internal/execution`: selection attribution as ordered model groups, routing projection and metadata placement, new classification;
  - `ai-gateway/cmd/grafana-ai-gateway/internal/service`: control decoding, the physical-candidate route table and per-request composition;
  - `byok`: multi-model decoding, composition and the provider timeout wrapper;
  - `providerwire/v4`: error documents and the `gateway_execution.json` schema.
- **Clients:**
  - The Go client gains `GatewayOptions` and the metadata reader, both additive public API in the `providers/grafana` module. It still forwards `providerOptions.gateway` and decodes `providerMetadata` opaquely.
  - The pinned TypeScript client is unchanged. Its docs use the exported `GatewayProviderOptions` type.
- **Parity:** the pinned `@ai-sdk/gateway@4.0.116` client (`test/conformance/upstream.yaml`) only forwards these controls; routing behavior belongs to the Gateway service. Vercel's service docs are used as design reference, not as pinned evidence. `GatewayOptions` is a parity-preserving Go adaptation of the exported upstream type. The metadata reader is a Go extension, because upstream types `providerMetadata.gateway` as an open record. No conformance fixtures change, and the support boundary lives in the new capability spec.
- **Docs:** `docs/providers/grafana-gateway.md`, the Gateway architecture walkthrough and the glossary.
- **Delivery:** branch `nrbrd/model-routing-support`, based on `main` after #579 (#420) and #581 merged. One `feat!:` PR with a `BREAKING CHANGE:` note for the unsupported-control change. It gives the Gateway plugin and `providers/grafana` major releases, although the client change itself is additive.
- **Out of scope:** `sort`, health ranking, new native adapters or codecs, credential storage, billing and quota, conditional decision fallbacks, and system-credential fallback for BYOK callers.
