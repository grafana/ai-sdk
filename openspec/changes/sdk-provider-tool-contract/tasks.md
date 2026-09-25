## 1. Validate provider tools and normalize markers

- [x] 1.1 Reject incompatible function/provider fields before direct provider or Go Gateway client I/O; normalize nil Go provider args without relaxing HTTP input rules.
- [x] 1.2 Cover provider-tool request conversion in native adapters and preserve supported unsupported-feature handling.
- [x] 1.3 Normalize unary dynamic and preliminary markers; preserve absent, false and true streaming input-start values through provider and client decoding.
- [x] 1.4 Test text-stream and UI classification against the registered upstream behavior, including cross-language input-start cases.

## 2. Extend independent client and Anthropic behavior

- [x] 2.1 Decode bounded unary/SSE provider calls, results, deferred results and reviewed metadata without trusting server-owned identity fields.
- [x] 2.2 Preserve omitted versus explicit Anthropic MCP token/enabled values and their native request projection.
- [x] 2.3 Use future caller deadlines for unary Anthropic SDK timeout defaults without reducing the token budget or overriding explicit timeouts.

## 3. Preserve service boundaries and verify

- [x] 3.1 Migrate existing Gateway consumers without activating provider tools or MCP in the service; keep downloadable internal pins on revisions already merged to main and retain rejection tests.
- [x] 3.2 Scope SDK documentation and parity evidence to supported behavior without changing authentic provider fixtures.
- [x] 3.3 Review the implementation against the registered upstream source and tests; preserve inherited warning, authentication and provider continuation behavior.
- [x] 3.4 Run tests, build, vet, lint, parity, frontend and Gateway candidate-source integration, merged-pin ancestry, isolation and Gateway-boundary checks; validate this change.
