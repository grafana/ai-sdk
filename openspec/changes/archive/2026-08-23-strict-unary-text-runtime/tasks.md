## 1. Migrate Reasoning to Provider-Default Zero Semantics

- [x] 1.1 Change `provider.ReasoningProviderDefault` to the empty-string zero value and change `provider.CallOptions.Reasoning` from pointer to value, with JSON and typed-enum tests for omitted, explicit operational, and strict-wire provider-default behavior.
- [x] 1.2 Update Anthropic, Bedrock, OpenAI Responses, and OpenAI-compatible request conversion to treat zero-valued reasoning as the existing no-op while preserving every explicit level and provider-option precedence.
- [x] 1.3 Update root option merging, prepared-step values, middleware defaults/logging, conformance configuration, mocks, and tests so pointer presence remains only in configuration layers that need merge semantics and provider calls receive a value.
- [x] 1.4 Run focused root and provider-module reasoning tests plus provider request snapshots to confirm the migration does not change requests for omitted or explicit reasoning.

## 2. Establish Unary Schemas and Handler Construction

- [x] 2.1 Add closed hand-authored draft 2020-12 unary-success and safe-error schemas under `gateway/providerwire/v4/schema`, with focused positive and negative schema tests for text, warnings including required empty strings, metadata, usage, finish reasons, and the exact nested error members.
- [x] 2.2 Add protocol constants, named unary limits, policy and handler configuration, embedded schema compilation, dependency validation, overflow-safe `limit+1` validation for byte limits, and canonical internal-error fit validation for only the error-response limit.
- [x] 2.3 Add constructor tests for nil resolver, invalid durations and numeric limits, unsafe increments, too-small error bounds, small positive unary bounds, schema compilation, immutable configuration, and valid construction.
- [x] 2.4 Implement exact method, relative-path, JSON media-type, specification-version, model-ID, and unary-mode envelope validation, with raw HTTP tests for missing, empty, repeated, collision-normalized, and invalid values plus unrelated host headers.
- [x] 2.5 Implement request body close and bounded `limit+1` reading, with below/at/above request-byte tests and zero downstream-call assertions.

## 3. Implement Bounded Lexical JSON Processing

- [x] 3.1 Add table-driven failing tests for duplicate members at every depth, excessive depth/tokens/number bytes, invalid UTF-8, malformed escapes, lone surrogates, malformed JSON, and trailing values.
- [x] 3.2 Implement the iterative protocol-local JSON scanner with stack frames, duplicate-member tracking, token/depth accounting, bounded number lexemes, UTF-8 validation, escape validation, and surrogate-pair validation without building a semantic tree.
- [x] 3.3 Add below/at/above tests for depth, token, and numeric-token limits plus differential valid-JSON cases covering escaped text, opaque nested JSON, and every phase 2 request golden.
- [x] 3.4 Apply the unchanged precompiled complete request schema after lexical validation and add sequencing tests proving schema-invalid or schema-instance-decoding failures never reach mapping, policy, resolution, or invocation; include a short huge-exponent case that fails safely without arbitrary-precision processing.

## 4. Map the Supported Request Subset Explicitly

- [x] 4.1 Before implementing the mapper, add a recording policy, resolver, and V4 model plus failing Go replay expectations for the specified per-record stage matrix: streaming records reject at the unary envelope; sequence record 1 executes; scalar/header unary records reach `body-headers`; and the comprehensive record reaches first `provider-options`.
- [x] 4.2 Add private request DTOs that retain scalar numeric lexemes and nested union values as `json.RawMessage`, using explicit role/discriminator switches rather than provider-domain JSON unmarshaling.
- [x] 4.3 Map ordered system messages, user and assistant text parts, required empty text, scalar pointer presence, `strconv.ParseInt` plus checked Go-int conversion, `strconv.ParseFloat` plus finite checks, stop sequences, text response format, and typed reasoning values; use a dedicated supported scalar request to prove exact execution rather than `scalar-presence.json`.
- [x] 4.4 Add typed stable unsupported-capability constants and explicit branches for files, reasoning/custom content, tools/approvals, structured output, provider options at every registered scope, body headers, and raw output.
- [x] 4.5 Normalize empty tools, provider-options maps containing only empty namespace objects, exactly `headers: {}`, `includeRawChunks: false`, and text response format as no-ops; prove a header member with value `""` activates `body-headers`.
- [x] 4.6 Add focused schema-valid requests that activate each unsupported capability independently, define fixed traversal/first-capability order for multi-capability input, and complete golden stage/count assertions without modifying phase 2 goldens.

## 5. Enforce Policy, Resolution, and Model Execution Order

- [x] 5.1 Implement the no-op/default host policy and exact-once policy application after supported mapping, including categorized policy-failure tests.
- [x] 5.2 Resolve the exact untrimmed requested ID once, preserve `catalog.ErrUnknownModel`, and validate non-empty canonical ID, non-nil model, and V4 specification before invocation.
- [x] 5.3 Invoke `DoGenerate` once with the policy-approved options under request cancellation and configured total duration, recover panics inside the child model goroutine, reject `nil, nil`, and use a buffered result handoff for late returns.
- [x] 5.4 Add ordered call-recording tests for supported execution, aliases, policy failure, unknown/invalid catalog results, model panic, `nil, nil`, normal and late return, caller cancellation, timeout, and ignored cancellation; assert bounded handler latency while documenting that a forever-blocked provider can retain its goroutine.

## 6. Add Closed Safe Failure Normalization

- [x] 6.1 Define the authoritative safe-error table with exact HTTP status, `error.type`, `error.code`, `param: null`, status-derived retryability, approved message, and pinned-client class; use `internal_server_error` for overload, upstream, timeout, and cancellation fallback classification.
- [x] 6.2 Implement context cancellation/deadline precedence, exact reduction for `APICallError` 408, 429, 503, 504, 529, zero, other 4xx/5xx and remaining statuses, timeout-capable `net.Error`/`*url.Error`, other network transport failures, arbitrary non-transport errors, model panic, and `nil, nil`.
- [x] 6.3 Implement bounded encoding and schema validation for exactly `{"error":{"message":...,"type":...,"param":null,"code":...}}`, with no serialized retryability field and with prevalidated canonical internal-error fallback bytes.
- [x] 6.4 Add hostile-cause privacy tests containing credentials, URLs, headers, request/response bodies, provider identities, backend model IDs, and metadata; assert none cross the raw HTTP boundary.
- [x] 6.5 Add raw exact-byte and registered-client tests for every safe category, asserting the table's HTTP/type/code/param, status-derived retryability, and one of the seven recognized pinned-client classes; include focused connection-refused/DNS and transport-timeout cases.

## 7. Map, Bound, and Commit Unary Success

- [x] 7.1 Add private success DTOs and explicit mapping for ordered text, all four registered warning variants, registered finish reasons, JavaScript-safe non-negative usage, and allowlisted response ID/timestamp; always emit required warning keys even when their value is empty.
- [x] 7.2 Normalize every successful `response.modelId` to the canonical public catalog ID and omit provider request/response bodies, headers, provider name, backend model ID, raw usage, provider metadata, and part metadata.
- [x] 7.3 Add mapping tests proving known warnings with empty required strings remain valid, while non-text content, unknown warning discriminators, invalid finish reasons, negative usage, and usage above `9007199254740991` fail before HTTP 200.
- [x] 7.4 Implement incremental bounded JSON document writing, including bounded string escaping, and validate complete bounded success bytes against the unary schema before writing status or headers.
- [x] 7.5 Add below/at/above unary-response and error-response boundary tests, oversized provider text tests, success-schema failure tests, and writer tests proving no complete oversized encoded copy is retained.
- [x] 7.6 Add raw privacy and canonical-alias tests with a deliberately hostile provider result and verify required empty text, warnings, usage, finish reason, response metadata, content type, status, and exact schema validity.

## 8. Add Pinned Cross-Language Runtime Evidence

- [x] 8.1 Add a deterministic Go test server scenario backed by the production handler and recording model for supported unary text and representative safe non-success responses.
- [x] 8.2 Extend the exact-pinned `test/providerwire-v4` workspace to call the real Go handler through `@ai-sdk/gateway@4.0.52` and assert client-observable content, usage, finish behavior, cancellation signal handling, each recognized error class, and intentional `GatewayInternalServerError` fallback categories.
- [x] 8.3 Keep raw Go HTTP assertions authoritative for server warnings, canonical response identity, response schemas, privacy, and bounds that the registered client masks or accepts permissively.
- [x] 8.4 Register the runtime integration in the non-mutating ProviderWire/parity workflow without adding or altering provider conformance input fixtures.

## 9. Update Parity Evidence and Verify the Change

- [x] 9.1 Update `test/conformance/PARITY.md` to classify strict Go envelope/lexical/schema decoding, unsupported mapping, unary DTO/privacy/bounds, golden replay, and pinned-client runtime evidence while retaining streaming as deferred.
- [x] 9.2 Run focused package tests throughout implementation, then run `gofmt`, `mise run vet`, `mise run lint`, and `mise run test` across the root and all provider modules.
- [x] 9.3 Run `mise run test-providerwire-v4`, the new runtime integration, `mise run validate-parity-baseline`, and `mise run parity-check`; verify normal checks do not rewrite schemas, goldens, or tracked files.
- [x] 9.4 Run `mise run build`, inspect module dependency direction, and search for restored legacy transport imports, provider-domain wire serialization, unbounded request/response/error paths, debug code, and unintended streaming implementation.
- [x] 9.5 Run `openspec validate strict-unary-text-runtime --strict` and review the final diff against phase 3 acceptance, the registered package versions, upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, and the modified reasoning specifications.
