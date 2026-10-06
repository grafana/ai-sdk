## 1. Provider-tool runtime and native conversion

- [x] 1.1 Preserve strict provider definitions, private unary/SSE encoders, history correlation and preview lifecycle from the reviewed WP13 implementation.
- [x] 1.2 Preserve bounded opaque tool metadata and generic tool correlation; forward actual native MCP configuration, definitions and history without imposing foreign-namespace inventories or capability gates.
- [x] 1.3 Retain bounds, malformed output, cancellation, writer failure, fallback and privacy regression tests.

## 2. Independent evidence and documentation

- [x] 2.1 Capture a provider-only request golden with the registered Gateway client; leave provider fixture inputs unchanged.
- [x] 2.2 Exercise deferred results through both clients and the real handler using non-MCP metadata.
- [x] 2.3 Split authenticated native code-execution acceptance from MCP; verify aliases, large default unary token budget, continuation and metadata-only observation in both modes/clients.
- [x] 2.4 Document native hosted-MCP behavior and request-boundary evidence without claiming live effects or complete hosted-result continuation coverage.
- [x] 2.5 Run full tests, build, vet, lint, parity, integration, module-resolution and Gateway-boundary gates on this intermediate branch.
- [x] 2.6 Validate the change and publish the second draft PR against the SDK prerequisite; keep both changes unarchived.
