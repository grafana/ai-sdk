## 1. Split runtime behavior from MCP

- [x] 1.1 Preserve strict provider definitions, private unary/SSE encoders, history correlation and preview lifecycle from the reviewed WP13 implementation.
- [x] 1.2 Limit metadata to ordinary reviewed fields; keep MCP root options and continuation/output metadata rejected.
- [x] 1.3 Retain bounds, malformed output, cancellation, writer failure, fallback and privacy regression tests.

## 2. Independent evidence and documentation

- [x] 2.1 Capture a provider-only request golden with the registered Gateway client; leave provider fixture inputs unchanged.
- [x] 2.2 Exercise deferred results through both clients and the real handler using non-MCP metadata.
- [x] 2.3 Split authenticated native code-execution acceptance from MCP; verify aliases, large default unary token budget, continuation and metadata-only observation in both modes/clients.
- [x] 2.4 Scope docs and parity coverage to provider tools, with MCP explicitly deferred to its own change.
- [x] 2.5 Run full tests, build, vet, lint, parity, integration, candidate-source, merged-pin and Gateway-boundary gates on the provider-tool tree.
- [x] 2.6 Validate the change and publish the second draft PR against the SDK prerequisite; keep both changes unarchived.

## 3. Revalidate the updated prerequisite

- [x] 3.1 Integrate SDK prerequisites while retaining Cloud CAP and OpenAI Responses service coverage.
- [x] 3.2 Review the registered ai 7.0.107/provider 4.0.17/Gateway 4.0.87/Anthropic 4.0.58/OpenAI 4.0.71 source and preserve the provider-only boundary.
- [x] 3.3 Re-run full tests, build, vet, lint, parity, frontend integration, authenticated command, candidate-source and Gateway-boundary checks.

## 4. Preserve existing option policy

- [x] 4.1 Retain main's root, message and part provider-option forwarding and call-header protections alongside provider-tool request mapping.
- [x] 4.2 Prove tool call/result parts cannot bypass protected-field checks and deferred MCP remains rejected, while ordinary configured options continue to work.
- [x] 4.3 Validate the provider-tool tree under candidate-source CI and canonical merged-pin ancestry.
