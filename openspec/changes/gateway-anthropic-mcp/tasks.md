## 1. Restore the narrow MCP extension

- [x] 1.1 Preserve registered root server option parsing, field/count/destination limits and native omission/empty/false semantics.
- [x] 1.2 Gate MCP in service composition using configured direct Anthropic topology; reject other backends and fallback before invocation.
- [x] 1.3 Match MCP continuation/output metadata against configured names while exposing no URL/token or unrelated caller fields.

## 2. Separate MCP evidence from provider-only evidence

- [x] 2.1 Capture a separate pinned-client MCP request golden and production replay; preserve the provider-only golden.
- [x] 2.2 Add MCP unary/stream metadata and deferred-result tests; retain predecessor non-MCP tests.
- [x] 2.3 Exercise both-client deferred success/error and authenticated native code-execution/MCP scenarios independently of provider-only scenarios.
- [x] 2.4 Verify malformed options, unconfigured names, fallback-without-definitions, non-Anthropic routes and private-safe observation.
- [x] 2.5 Scope MCP operator/client docs and parity coverage, explicitly leaving live egress and deployment activation unverified.
- [x] 2.6 Run tests, build, vet, lint, parity, integration, focused race, module-resolution and Gateway-boundary checks; compare the final runtime with the original reviewed implementation.
- [ ] 2.7 Validate all three unarchived changes, publish the final draft PR and link the stack; supersede #237 without rewriting published prerequisite refs.

## 3. Integrate the current main baseline

- [x] 3.1 Merge #239's current-reference integration and retain combined published Apache dependencies.
- [x] 3.2 Review MCP projection/replay against Anthropic 4.0.58 and Gateway 4.0.87 at the registered commit; add denial coverage for main's OpenAI Responses route.
- [x] 3.3 Re-capture both request goldens with Gateway 4.0.87 (no content changes) and re-run tests, build, vet, lint, parity, frontend integration, authenticated command, focused race, module-resolution and Gateway-boundary checks.
