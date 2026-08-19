## 1. Baseline, Evidence, and Failure Contracts

- [x] 1.1 Verify the parent contains the archived H1 contract and evidence refactor, the working tree is clean apart from this change, and the exact registered source commit contains `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider-utils@5.0.27`, and the selected provider source paths; stop on drift or failed source equivalence.
- [x] 1.2 Inventory the registered unary fixture surface, record Bedrock `json-tool-with-answer` as the only provenance-valid H2 provider fixture, and document the remaining provider-boundary gap without adding or changing provider inputs.
- [x] 1.3 Reconfirm every request, result, error, and provider-domain distinction used by H2 against the checked-in schemas, exact pinned Gateway/provider sources and tests, and current Go provider types; classify any discrepancy before implementation.
- [x] 1.4 Add focused failing handler tests for ordered envelope/body/syntax/schema/policy gates, resolver bypass, at-most-once invocation, timeout/cancellation, privacy, bounds, and safe errors before implementing the corresponding runtime behavior.

## 2. Independent Unary Conformance Oracles

- [x] 2.1 Extend the exact-package TypeScript conformance tooling to observe the full raw unary `LanguageModelV4GenerateResult` from the unchanged selected Bedrock input, validate it against H1, and compare pinned Gateway consumption with the existing `expected-generate.json` semantic outcome.
- [x] 2.2 Implement the independent TypeScript H2 unary policy projector that strips exactly provider metadata, raw usage, request details, response headers/body, provider identity, and backend model ID while preserving safe contract fields.
- [x] 2.3 Add a generated policy-normalized unary expectation with source fixture, registered baseline, policy profile, and generation authority recorded; validate staged output before atomic replacement and prove no Go V4 runtime code generates it.
- [x] 2.4 Add non-mutating raw and normalized expectation checks, baseline-consumer validation, and provenance tests that fail if provider inputs change or derived projections are mislabeled.

## 3. Production Syntax and Schema Runtime

- [x] 3.1 Move the proven `jsontext.Decoder` wrapper into focused production code and make the existing duplicate-name, trailing-value, UTF-8, surrogate, malformed-escape, truncation, and original-byte tests exercise that implementation.
- [x] 3.2 Embed all checked-in ProviderWire V4 schemas, refactor the contract registry into production code, compile the complete Draft 2020-12 graph once without filesystem/network access, and reuse it safely across concurrent handlers.
- [x] 3.3 Make handler construction surface embedded contract compilation failures and add tests for offline built-binary use, concurrent validation, unknown schemas, and safe normalized instance paths.
- [x] 3.4 Keep OpenAPI and JSON Schema authoritative by verifying no production schema generation, reflected provider DTO, or exported wire value is introduced.

## 4. Public Handler Surface and HTTP Gates

- [x] 4.1 Add the V4 route/header/version/media constants, total-timeout sentinel, request-aware resolver interface and function adapter, handler type, named defaults, and positive-valued functional options without changing legacy constants or APIs.
- [x] 4.2 Implement construction checks for nil and typed-nil resolvers, nil options, non-positive values, and error limits below the fixed fallback envelope size.
- [x] 4.3 Implement exact unary envelope validation for method, `/language-model` path, required unpadded routing values, mandatory JSON content type, H1 positive-quality exact/type/full-wildcard Accept rules, and non-retryable rejection of streaming `true`.
- [x] 4.4 Implement bounded request-body reading and close behavior with exact limit-plus-one classification, read-error/cancellation handling, and resolver bypass.
- [x] 4.5 Wire the fixed envelope, body, strict syntax, and request-schema order together and prove each failed gate prevents decoding, policy, resolution, and invocation.

## 5. Private Request Values, Policy, and Adaptation

- [x] 5.1 Define private exact request wire values that preserve optional scalar presence, absent versus explicit-empty arrays/maps, exact selected arms, and copied opaque JSON without exporting DTOs or codecs.
- [x] 5.2 Implement request adapters for system/user/assistant/tool messages, tagged file data, reasoning/custom parts, tool calls/results/approvals, function/provider tools, tool choice, response format, scalar settings, and provider options, with the approved canonicalization of optional false/empty-string presence that the provider domain cannot expose.
- [x] 5.3 Decode base64 inline files with aggregate decoded-byte accounting and reject malformed or over-limit resources before resolution while preserving empty inline data.
- [x] 5.4 Implement the fixed request policy: remove empty top-level Gateway options, reject any non-empty Gateway controls, reject nested reserved `gateway` members, remove empty body headers and the exact pinned `user-agent: ai/7.0.65` orchestration marker, reject every other non-empty body-header map, and reject raw-chunk intent.
- [x] 5.5 Preserve non-reserved provider namespaces as `provider.RawProviderOption`, opaque JSON semantically, and explicit empty tools/stop sequences/maps; add complete positive and negative adapter tests for every request union arm.
- [x] 5.6 Enforce the approved numeric adaptation by bounding lexeme/exponent work; rejecting fractional or out-of-range `maxOutputTokens`, `topK`, and `seed`; and rejecting standard floats that overflow, underflow non-zero to zero, or fail canonical decimal round-tripping before resolution; do not round, truncate, or weaken the schema.

## 6. Resolver, Timeout, and Unary Invocation

- [x] 6.1 Start one total-timeout context after request/policy acceptance and before resolution, carry host request context into the resolver, and pass the same context to `DoGenerate`.
- [x] 6.2 Invoke the resolver at most once, reject a nil model safely, invoke `DoGenerate` exactly once, and prove `DoStream` is never called by the unary handler.
- [x] 6.3 Normalize total-timeout cause to retryable 504 and observable consumer cancellation to non-retryable 499 while proving cancellation reaches resolver/model work promptly.
- [x] 6.4 Add race-safe tests for resolver errors, nil models, provider errors, completion at timeout boundaries, request cancellation, and exact invocation counts.

## 7. Unary Result Policy, Encoding, and Safe Errors

- [x] 7.1 Define private exact generate-result and safe-error wire values and adapters for every H1 unary content arm, required empty arrays, finish reason, usage, warnings, response ID, and non-zero timestamp.
- [x] 7.2 Apply disclosure policy during result adaptation by removing top-level/per-content provider metadata, raw usage, request details, response headers/body, provider identity, and backend model ID while preserving all other representable public fields.
- [x] 7.3 Encode successful results into the configured bounded pre-commit buffer, validate the complete semantic value against `generate-result.json`, and commit HTTP 200 JSON only after adaptation, encoding, size, and schema checks pass.
- [x] 7.4 Implement stage-aware safe error normalization for request, policy, resolver, provider, timeout, cancellation, nil-result, adaptation, encoding, and size failures without serializing backend messages or `APICallError` diagnostic fields.
- [x] 7.5 Preserve usable explicit retryability locally, normalize invalid statuses, enforce status correlation and the configured error limit, and fall back to the fixed validated HTTP 500 envelope when needed.
- [x] 7.6 Add exhaustive tests for metadata/raw/backend privacy, every safe error arm used by H2, 429 and 5xx mapping, retryability overrides, nil/unrepresentable/invalid/oversized results, pre-commit guarantees, and write failures without a second response.

## 8. Real Provider and Pinned Client Runtime Evidence

- [x] 8.1 Extend the conformance Go lane to place the real Bedrock provider for the unchanged selected unary fixture behind the real V4 handler and compare its response with the independently generated normalized expectation.
- [x] 8.2 Preserve the existing downstream `expected-requests.jsonl` assertion and prove the V4 resolver and provider `DoGenerate` each run exactly once in the selected conformance case.
- [x] 8.3 Add V4 unary scenarios to the Go interop test server without modifying its legacy route, then exercise exact pinned stock-Gateway `doGenerate` and `ai.generateText` against the real handler.
- [x] 8.4 Add cross-language assertions for request adaptation, explicit empties, opaque values, structured output/tools as applicable, response privacy, safe 429/5xx categories and stock status-derived retryability.
- [x] 8.5 Add HTTP-level policy rejection scenarios proving non-empty body headers, Gateway controls, raw-chunk intent, and streaming mode bypass resolver/model invocation.

## 9. Documentation, Parity, and Aggregate Checks

- [x] 9.1 Update the ProviderWire V4 interop index to distinguish generated request captures, raw unary transport, independent policy projection, normalized expectation, Go runtime evidence, authorities, commands, claims, and non-claims.
- [x] 9.2 Update `test/conformance/PARITY.md` from contract-only V4 coverage to strict unary runtime coverage and retain explicit gaps for streaming, Go client, Grafana adoption, frontend runtime, private-server behavior, and providers without provenance-valid unary inputs.
- [x] 9.3 Update the provider-wire server guide and docs navigation as needed to distinguish the complete legacy handler from the strict unary-only V4 handler, host-wrapper responsibilities, options, policy restrictions, and streaming deferral without duplicating godoc API reference.
- [x] 9.4 Expand `mise run test-interop-contract` and `mise run check-providerwire-v4` with non-mutating unary runtime and raw/policy conformance checks while keeping artifact replacement explicit and atomic.
- [x] 9.5 Run legacy provider-wire, Grafana, interop, integration, and frontend checks and confirm no legacy API, wire bytes, SSE framing, UI chunks, or default transport changed.

## 10. Verification and OpenSpec Completion

- [x] 10.1 Run `gofmt`, focused package tests with race detection, `go vet ./gateway/providerwire/...`, TypeScript type checking, docs lint, `mise run validate-providerwire-v4-contract`, `mise run test-interop-contract`, and `mise run check-providerwire-v4`.
- [x] 10.2 Run `mise run test-conformance`, `mise run test-integration`, `mise run test-interop`, `mise run validate-parity-baseline`, and `mise run parity-check`; classify every observed difference as pinned-client behavior, local projection, host restriction, Go adaptation, defect, or coverage gap.
- [x] 10.3 Run `git diff --check` and scan changed artifacts for secrets, credentials, provider input modifications, machine-local paths, unrelated branch/plan references, mislabeled provider recordings, exported wire DTOs, and accidental streaming/client/Grafana implementation.
- [x] 10.4 Strictly validate the change, verify implementation against proposal/design/specs/tasks, synchronize both modified canonical capability specs, archive the change, confirm zero active changes, and run `openspec validate --all --strict`.
