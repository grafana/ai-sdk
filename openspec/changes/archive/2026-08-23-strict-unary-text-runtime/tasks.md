## 1. Normalize Reasoning Semantics

- [x] 1.1 Make `ReasoningProviderDefault` the empty-string zero value and make provider call reasoning value-typed.
- [x] 1.2 Update orchestration, middleware, conformance, and all providers while retaining pointers only in configuration layers that need presence semantics.
- [x] 1.3 Verify provider request behavior and reasoning tests across modules.

## 2. Build the Unary Handler

- [x] 2.1 Add immutable construction with resolver, request/response byte limits, and model duration.
- [x] 2.2 Validate the exact unary HTTP envelope and bounded UTF-8 request body.
- [x] 2.3 Compile and apply the complete registered request schema before mapping.
- [x] 2.4 Add raw HTTP tests for envelope, body boundaries, malformed input, duplicate-member last-value behavior, escaped-lone-surrogate normalization, and downstream-call suppression.

## 3. Map Text and Scalars

- [x] 3.1 Add private typed request DTOs for text messages, scalar controls, stop sequences, response format, and reasoning.
- [x] 3.2 Preserve message order, empty strings, scalar presence and zero values, integer lexical constraints, and reasoning normalization.
- [x] 3.3 Reject each deferred capability family with a stable fixed document without recursively revalidating schema-owned unsupported unions.
- [x] 3.4 Replay every committed ProviderWire request golden without modifying it.

## 4. Resolve and Invoke Models

- [x] 4.1 Resolve the exact requested ID and validate canonical ID, model presence, and V4 specification.
- [x] 4.2 Invoke `DoGenerate` once under request cancellation and total duration.
- [x] 4.3 Recover model panics and use a buffered handoff so non-cooperative models cannot hold handler latency open.
- [x] 4.4 Add resolution, cancellation, timeout, panic, nil-result, and late-return tests.

## 5. Emit Fixed Errors and Minimal Success

- [x] 5.1 Precompute finite privacy-safe error documents for runtime categories and unsupported families.
- [x] 5.2 Reduce provider, transport, catalog, timeout, cancellation, and internal failures without serializing causes.
- [x] 5.3 Map only unary text content, finish reason, and JavaScript-safe usage.
- [x] 5.4 Omit warnings, response metadata, canonical and backend identity, provider metadata, bodies, headers, and raw usage.
- [x] 5.5 Preflight content count and aggregate raw string bytes before UTF-8 validation, encode the private DTO with standard Go JSON, and enforce the final response byte limit before HTTP 200.
- [x] 5.6 Add hostile escaping, invalid UTF-8, content-count, aggregate-byte, and precommit boundary tests; validate success and error schemas in tests rather than at runtime.

## 6. Add Compatibility Evidence

- [x] 6.1 Add the production handler to the integration test server.
- [x] 6.2 Exercise minimal success, representative errors, and cancellation through `@ai-sdk/gateway@4.0.52`.
- [x] 6.3 Keep synthetic Gateway client-class tests for every runtime status plus supported host errors, including class/type/status/retryability assertions, and remove the duplicate Go-server lifecycle.
- [x] 6.4 Update the parity map while retaining strict streaming as deferred.

## 7. Validate

- [x] 7.1 Run formatting, vet, lint, build, full module tests, and race tests.
- [x] 7.2 Run ProviderWire, integration, parity-baseline, and parity checks.
- [x] 7.3 Validate canonical OpenSpec specs strictly and verify no schemas, goldens, conformance inputs, or module files are rewritten by normal checks.
