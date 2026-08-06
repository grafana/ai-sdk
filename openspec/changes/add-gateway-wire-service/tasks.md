## 1. Specification and Package Shape

- [x] 1.1 Rewrite the proposal, design, tasks, and affected deltas around direct `catalog.ModelResolver` composition and handler-owned lifecycle.
- [x] 1.2 Delete the proposed `gateway-runtime` and `gateway-failure-classification` deltas and remove the `gateway/runtime` and `gateway/failure` packages.
- [x] 1.3 Preserve the pinned baseline, strict private-DTO independence, legacy compatibility, Grafana strict mode, dual deployment, privacy, limits, SSE semantics, and representative interop requirements.
- [x] 1.4 Remove Chat-fit, public runtime, policy, identity, metadata/request-ID, call-aware resolver, middleware, stream-session, and typed gateway-control requirements; use OpenSpec 1.8-compatible scenario names.

## 2. Strict V4 Implementation

- [x] 2.1 Change `providerwirev4.NewHandler` to require a non-nil `catalog.ModelResolver`, resolve the exact requested ID with request context, reject nil models, and invoke the resolved model directly.
- [x] 2.2 Move the positive 120-second default total timeout beside the existing idle timeout and select directly over provider parts, request/total context, and idle expiry without invocation goroutines or proxy streams.
- [x] 2.3 Preserve unary/stream privacy filtering, ordering, clean EOF, repeatable `PartError` continuation, response-controller flushing, write failure behavior, and pre/post-commit error handling.
- [x] 2.4 Replace public failure classification with private V4 helpers for unknown model, rate limit, timeout/cancel, permanent/transient dependency, generic invocation dependency, and internal adapter defect mappings.
- [x] 2.5 Make strict request decoding return `provider.CallOptions`, remove absent or empty top-level `providerOptions.gateway`, reject non-empty keys and nested reserved namespaces, and reject raw-chunk requests before resolution.
- [x] 2.6 Preserve the codec matrices and fix valid empty inline-text file data round-tripping without changing prerequisite provider APIs.

## 3. Clients, Tests, and Documentation

- [x] 3.1 Rewrite V4 handler, Grafana server, and interop test composition around catalog resolvers and direct models; remove runtime metadata, request-ID, middleware, policy, identity, and stream-session cases.
- [x] 3.2 Remove speculative policy-only interop producers while retaining cheap strict-decoder compatibility for registered Grafana error categories.
- [x] 3.3 Update the provider-wire guide and parity coverage text for direct catalog composition and handler-owned lifecycle.
- [x] 3.4 Keep legacy tests/endpoints unchanged and preserve distinct legacy and strict deployment paths.

## 4. Balanced Simplification

- [x] 4.1 Let canonical discriminators select union arms, ignore inactive sibling fields, retain active/type/reference/null/privacy/legacy validation, and delete the mirrored union-validation matrices.
- [x] 4.2 Keep ambiguous discriminator-less provider-domain encoding fail-closed and preserve empty inline-text prompt and tool-result file round-trips.
- [x] 4.3 Reduce the unreleased V4 public codec API to handler construction/options/constants and strict Grafana client operations; adapt server fixtures to handler-backed encoding.
- [x] 4.4 Replace Grafana's general mode enum with binary `WithStrictProviderWire()` selection and remove defensive model fallbacks.
- [x] 4.5 Apply new Grafana response limits only in strict mode and freeze original legacy readers and precedence.
- [x] 4.6 Remove duplicate file-data, SSE-boundary, repeated error-matrix, legacy public-fixture, and interop evidence where a closer boundary already proves behavior.
- [x] 4.7 Update the proposal, design, requirements, guide, and parity claims for the simplified validation, public API, binary opt-in, and strict-only limits.

## 5. Evidence

- [x] 5.1 Run gofmt and focused gateway/provider tests, including resolver-success context precedence, ready-channel cancellation, exact request/unary limits, wrapped flushing, and post-commit failure evidence.
- [x] 5.2 Run Grafana tests with strict exact/limit-plus-one unary, error, and SSE reads, plus all 17 pinned TypeScript interop cases.
- [x] 5.3 Run baseline/parity validation, integration tests, vet, lint, docs lint, and strict OpenSpec validation.
- [x] 5.4 Run final diff checks, record production/test/doc deltas versus HEAD and the PR base, and verify no staged files or commits.
