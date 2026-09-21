## Why

Go's shared text orchestration defaults omitted tool choice to `auto` only when tools are active, unlike the registered upstream SDK. The Gateway already accepts that default on direct routes, but its fallback guard still rejects it for ordinary text-only calls. Issue #202 requires aligning core defaulting and allowing that harmless default through fallback routes.

## What Changes

- Default omitted effective tool choice to `auto` independently of tools across `StreamText`, `GenerateText`, and Agent entry points, preserving explicit and per-step choices.
- Permit only no-tools pure automatic choice through the fallback text guard, forwarding original options without changing effect rejection, retry eligibility, or commitment.
- Add core/shared-caller, Gateway HTTP, fallback/failover, and actual high-level Go/TypeScript command regressions. Observe SDK-generated requests without rewriting them; build the Go high-level probe against local core/client sources.
- Update compatibility evidence and the existing spec statements affected by core defaulting, fallback admission, and proven high-level text streaming.

Direct-route function-tool mapping, client serialization, authentication, dependency pins, and UI/SSE protocols are unchanged. The mapper and exact `ai@7.0.65` test dependency already exist on main; they are not implementation work in this PR.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `functional-options`: Specify tools-independent automatic choice preparation and preserve explicit/per-step precedence.
- `gateway-ordered-text-fallback`: Exempt only harmless no-tools automatic choice from the effect guard and preserve it across candidate calls.
- `gateway-unary-function-tools`: Narrow its existing blanket fallback-choice prohibition to agree with the pure-auto exception.
- `ai-gateway-cloud-authentication`: Add actual high-level text-only streaming evidence through the existing authenticated command composition and correct the unsupported-streaming claim.

## Impact

Production changes are limited to `streamtext.go` and `ai-gateway/cmd/grafana-ai-gateway/internal/service/fallback_route.go`. Regression tests additionally cover the unchanged Gateway mapper and existing client composition. Custom models and middleware now observe nonnil automatic choice for text-only high-level calls.

The registered baseline remains `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` (`ai@7.0.65`, `@ai-sdk/gateway@4.0.52`). No provider inputs, fixture expectations, production dependencies, or protocol schemas change. High-level TypeScript `generateText` body headers and default unary token limits remain outside scope; deferred validation is recorded in `tasks.md`.
