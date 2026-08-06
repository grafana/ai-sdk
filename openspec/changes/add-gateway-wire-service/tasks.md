## 1. Freeze Existing Canonical and Legacy Contracts

- [x] 1.1 Add a successful stock `@ai-sdk/gateway@4.0.33` unary scenario against the existing handler that asserts ordered content, tool calls/results, files, sources, finish reason, provider metadata, and usage plus the pinned client's transport-owned request/response/warnings behavior.
- [x] 1.2 Add stock-client request coverage for canonical tool-result file data, including `Uint8Array` conversion and already-encoded data, and confirm the existing handler decodes the exact file semantics.
- [x] 1.3 Preserve and extend the canonical stream scenario that emits `PartError` followed by later content and `PartFinish`, verifying both TypeScript and Grafana clients receive every part in order.
- [x] 1.4 Add an external-package compile fixture covering legacy constants, resolvers, handler constructor/options/defaults, codecs, SSE reader/writers, and error helpers so exported names and call shapes cannot change accidentally.
- [x] 1.5 Freeze legacy-only tolerant payload decoding, omitted `Content-Type`, permissive `Accept` including `q=0`, preserved `APICallError` fields, and the existing `Connection: keep-alive` stream header.
- [x] 1.6 Run `go test ./gateway/providerwire ./provider`, `mise run test-grafana`, and `mise run test-interop` to establish the pre-rewrite baseline.

## 2. Implement Gateway Call Control and Failure Classification

- [x] 2.1 Define typed `Protocol`, `GatewayCall`, `GatewayOptions`, immutable trusted `CallMetadata`, and separate policy-derived metadata in `gateway/runtime`; cover every registered gateway field and retain unknown valid gateway keys as opaque extension JSON without HTTP or façade DTO dependencies.
- [x] 2.2 Implement ordered pre-resolution call policy that may transform provider-bound options, gateway controls, and policy-derived metadata while keeping protocol, original requested model ID, request ID, and host-authenticated attributes immutable.
- [x] 2.3 Add policy tests for prohibited downstream `Authorization`, disallowed provider options, raw-chunk exposure, transformed routing controls, unknown extension retention, immutable trusted attributes, defensive copies, and rejection before resolver/model middleware invocation.
- [x] 2.4 Implement a call-aware resolver interface plus a default `catalog.ModelResolver` adapter that rejects unsupported non-empty gateway controls rather than ignoring or forwarding them.
- [x] 2.5 Create `gateway/failure` category sentinels and cause-preserving wrapping without a custom error type or retryability marker in the error chain.
- [x] 2.6 Implement the non-error `Classification` value with deterministic kind precedence, freshly derived retryability, private cause, and typed allowlisted safe parameters.
- [x] 2.7 Test outer-boundary retryability override, catalog unknown models, policy failures, cancellation/deadlines, rate limits, backend authentication/model lookup, unattributed provider 4xx, transient dependencies, deterministic internal failures, and private-cause traversal.
- [x] 2.8 Implement and test the V4 runtime-failure projection table, including 424/502 dependency mapping, the pinned TypeScript HTTP-500 retry asymmetry, and disclosure tests for provider/internal diagnostics.
- [x] 2.9 Keep method/media/negotiation/body-size/DTO failures adapter-local and test exact safe 405/406/413/415 responses without routing them through one invalid-call HTTP mapping.

## 3. Implement the Shared LanguageModel Runtime

- [x] 3.1 Create `runtime.New` with call policy, call-aware resolver, a 120-second default and positive total-timeout option, constructor validation, and transport dependency guards.
- [x] 3.2 Implement immutable requested/canonical/resolved identity, document resolved values as model-reported rather than actual fallback attempts, and retain available identity on every failure after policy/resolution.
- [x] 3.3 Add typed context accessors for protocol, non-empty gateway request ID, requested/canonical public model IDs, immutable host-authenticated attributes, and distinct policy-derived metadata; return defensive copies and prove caller-controlled headers never become authenticated context.
- [x] 3.4 Add ordered model middleware configuration and tests proving the chain is attached once, entered once, and receives enriched gateway context for generate and stream.
- [x] 3.5 Implement synchronous unary invocation with post-resolution timeout, classified failures, identity-bearing outcomes, and success/timeout/cancellation/nil-result tests.
- [x] 3.6 Add a blocked `DoGenerate` test proving context expiry without an extra runtime-owned invocation goroutine.
- [x] 3.7 Implement the minimal stream session with identity, single-consumer ordered `Parts`, lifecycle `Wait`, and idempotent `Cancel`, without exposing provider setup request/response metadata or protocol-specific state.
- [x] 3.8 Add stream tests for clean close, repeatable `PartError` data followed by finish, identity on setup failure, nil stream, caller/adapter cancellation, total timeout, backpressure, established channels that ignore cancellation, and blocked synchronous `DoStream` context expiry.
- [x] 3.9 Review the public `GatewayCall`, policy, resolver, identity, and stream seams with the Chat Completions owner before protocol adapter implementation; record accepted changes without adding speculative Chat DTOs.

## 4. Implement the Strict Bidirectional V4 Codec

- [x] 4.1 Create independent `gateway/providerwire/v4` codec and service package boundaries, route/header constants, package documentation, and a dependency test rejecting imports of legacy `gateway/providerwire`.
- [x] 4.2 Implement private wire-native request DTOs and field-by-field encode/decode for prompts, content, tagged data unions, tools, tool choice, response format, generation settings, provider options, and headers.
- [x] 4.3 Extract and validate `providerOptions.gateway` into `runtime.GatewayOptions`, retain unknown valid keys byte-equivalently in the extension map, remove the namespace from provider-bound options, and add goldens for every registered field plus unsupported-control failure through the default resolver.
- [x] 4.4 Implement canonical tool-call, tool-result, tool-approval, multipart result, and tool-result file conversions with complete JSON validation.
- [x] 4.5 Implement private generate-result DTOs with strict encode and decode for every registered generated-content variant, finish reason, usage, warnings, provider metadata, and canonical response metadata.
- [x] 4.6 Implement private stream DTOs with strict encode and decode for every registered stream discriminator, required empty deltas, sources, file data, metadata/raw parts, repeatable errors, and terminal usage.
- [x] 4.7 Add exact registered-baseline request/result/stream goldens and strict rejection tests for unknown discriminators, contradictory unions, missing fields, malformed JSON, legacy request/response shapes, and harmless additive fields.
- [x] 4.8 Add safe error conversion with a structured category copy for Grafana normalization while discarding original provider data and diagnostics.
- [x] 4.9 Add encoded unary and complete framed-SSE-event limit helpers that count `data: `, canonical JSON, and `\n\n` exactly; document that encoding may allocate the rejected value.

## 5. Implement the Runtime-Backed Strict V4 Service

- [x] 5.1 Implement `providerwirev4.NewHandler` over a concrete runtime with strict method/header/content-type validation, quality-aware `Accept`, bounded request reads, and runtime-bypass assertions for adapter-local failures.
- [x] 5.2 Add a host metadata extractor plus configurable default request-ID generator; make authenticated attributes immutable, guarantee a non-empty ID before policy, and prove request-body/header claims are not promoted automatically.
- [x] 5.3 Construct `GatewayCall` for unary and stream dispatch and prove policy runs before call-aware resolution and model middleware.
- [x] 5.4 Implement unary dispatch that applies the V4 public metadata allowlist, omits private backend `modelId` without substituting the canonical alias, encodes/size-checks before HTTP 200 commitment, and maps runtime failures through safe V4 envelopes.
- [x] 5.5 Implement streaming dispatch with a 60-second idle timer active only while waiting for the next runtime part, canonical SSE framing, clean EOF, safe repeatable provider errors, and best-effort post-commit lifecycle errors.
- [x] 5.6 Omit provider request bodies, backend response headers/bodies, stream setup headers, and resolved provider/model identity; permit raw chunks only when explicitly requested and accepted by call policy.
- [x] 5.7 Use `http.ResponseController` for initial/per-event flushes, omit `Connection: keep-alive`, and test wrapped writers, unsupported/failed flush, write failure, runtime cancellation, and exclusion of synchronous write time from idle accounting.
- [x] 5.8 Add exact request/unary/event limit tests, including server/client-identical framed-event accounting and explicit evidence that limits prevent partial commitment but do not claim bounded encoder allocation.

## 6. Migrate Grafana Incrementally

- [x] 6.1 Add a typed provider-wire mode and explicit strict-mode option while preserving existing constructors, `Option`, `WithHTTPClient`, client precedence, and default legacy mode.
- [x] 6.2 Route strict-mode request encoding and unary/SSE response decoding exclusively through `gateway/providerwire/v4`, with dependency tests proving no legacy codec or provider custom JSON path is used.
- [x] 6.3 Add strict-mode tests for canonical request/result/event behavior, legacy response rejection, allowlisted response ID/timestamp, backend transport/model-ID omission, safe stream category data, and all normalized error categories.
- [x] 6.4 Add named defaults and positive options for 16 MiB unary success, 1 MiB error body, and 8 MiB complete framed SSE event limits to both constructors.
- [x] 6.5 Replace unary/error `io.ReadAll` and unbounded SSE line reading with limit-plus-one and incremental complete-event reads preserving multiline events and final-line-plus-EOF behavior.
- [x] 6.6 Add exact-limit, limit-plus-one, long-line, multiline, invalid-content-type, cancellation, unchanged legacy request, constructor compatibility, and strict server/client accounting tests.
- [x] 6.7 Add `GatewayErrorForbidden` and `GatewayErrorFailedDependency`; normalize strict unary errors automatically and preserve streamed category data for explicit `NormalizeAPICallError` without changing the stream contract.

## 7. Prove Dual Deployment and Record the End State

- [x] 7.1 Parameterize the interop harness with distinct legacy and strict base URLs for the required dual-handler coverage rather than one ambiguous `/language-model` endpoint.
- [x] 7.2 Run shared canonical stock TypeScript and Grafana success scenarios against both handlers, using Grafana legacy mode for legacy and strict mode for strict.
- [x] 7.3 Add handler-specific error suites: strict typed/redacted errors and legacy preservation of existing details/shapes.
- [x] 7.4 Add client-observed error matrices for every public category, including 424/502 dependencies, forbidden errors, and the TypeScript-versus-Grafana internal retry difference.
- [x] 7.5 Verify strict public warnings and allowlisted response ID/timestamp while asserting omission of provider request body, backend headers/body, and backend `modelId` without alias substitution.
- [x] 7.6 Document staged mounting, explicit codec/base-URL cutover, rollback, trusted metadata configuration, gateway-option handling, transport-limit semantics, and absence of automatic negotiation or stream replay.
- [x] 7.7 Document the intended follow-up end state: strict V4 canonical, Grafana default switched only after adoption evidence, legacy provider wire deprecated/removed through a breaking change, and provider custom JSON no longer owning provider-wire serialization.
- [x] 7.8 Update `test/conformance/PARITY.md` for pinned-client unary replacement, retry behavior, strict metadata privacy, and durable coverage changes without inventing provider fixtures.
- [x] 7.9 Run formatting, `go test ./gateway/... ./provider`, `mise run test-grafana`, `mise run test-interop`, `mise run validate-parity-baseline`, `mise run parity-check`, `mise run vet`, and `mise run lint`; record residual policy, provider-boundary, client-retry, allocation, and deployment risks.
