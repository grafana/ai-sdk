# Proposal

## Why

Callers that send Anthropic server-side fallback (`providerOptions.anthropic.fallbacks`, an explicit chain or `"default"`) through the Gateway get `400 invalid request` ([#562](https://github.com/grafana/ai-sdk/issues/562)). The refusal exists because a caller could make the operator's Anthropic account serve models the catalog never granted. The Go provider already forwards the option like `@ai-sdk/anthropic@4.0.94`, so the only gap is an operator policy. The live recordings `anthropic/recorded/server-side-fallback-{chain,default}` replay that request, and the Gateway refuses both before any backend call.

## What Changes

- A configured candidate (`primary` or a `fallback` entry) can carry an optional `serverFallbacks` policy: the fallback models it allows and whether `"default"` is allowed. Without a policy, the candidate refuses as it does today.
- Configuration loading accepts the policy only on `anthropic` candidates. The policy must allow `"default"` or at least one model, and its models must be distinct, nonempty and within the existing text bounds. Programmatic catalog construction, which skips configuration validation, enforces only the `anthropic`-type check.
- Native Anthropic option validation resolves the same `AnthropicOptions` the adapter consumes and checks them against the candidate's policy, before native I/O and after per-attempt translation:
  - each chain entry's model must be allowlisted (exact bytes), and entries must be distinct. Upstream forwards any array; these are intentional Gateway support boundaries, because the operator account pays;
  - `"default"` requires `allowDefault`.
  - Other per-entry fields are forwarded unchanged.
  - A refusal uses the existing unsupported-request error without echoing model names.
- BYOK keeps refusing native server-side fallback (both chain and `"default"`) before any account attempt. Configured policy never applies to BYOK.
- Ordered Gateway fallback (#121) is unchanged:
  - Server-side fallback runs inside one Gateway attempt.
  - Each candidate applies its own policy.
  - A refused candidate fails before its native request, and the existing decider advances to the next candidate.
  - If no candidate is left, the call fails with the invalid-request error.
- Request routing (#316, delivered by #596) reuses each route's startup candidates, so every routed attempt applies its own candidate's policy. When routes share a destination, routing attempts it once, under the policy of the candidate composed first. BYOK fallback-model accounts keep refusing.
- The execution overview's selected attempt gains an optional `servedModelId`. It is the `to.model` of the latest valid `anthropic.fallback` marker in the selected output so far, across a stream's response segments, and is absent when no hop occurred. `modelId` stays the configured or requested model, and the `gateway.routing` summary is unchanged. Every carrier of the selected attempt publishes it, including unary invalid-output errors and stream errors after the first part: committed error parts, terminal waits (closed, deadline, canceled, idle timeout, invalid output) and the adapter-failure terminal error.
- Docs are updated. A command-level test replays the two recorded fixtures, unchanged, through the real Gateway binary under a fixed allowing policy, with both registered clients.

## Capabilities

### New Capabilities

- `gateway-anthropic-server-fallbacks`: operator-policy-gated Anthropic server-side fallback at native consumption. Covers the policy shape, load and programmatic-construction validation, chain/default enforcement, Gateway support boundaries, combination with ordered fallback and request routing, and evidence boundaries. It cross-references the BYOK refusal instead of restating it.

### Modified Capabilities

- `gateway-request-byok`: the BYOK refusal of native server-side fallback stays, but the spec stops requiring the same refusal for configured mode. Configured mode is governed by `gateway-anthropic-server-fallbacks`. This capability stays the single normative home of the BYOK refusal.
- `gateway-ordered-text-fallback`: a candidate reference may carry the optional `serverFallbacks` native-option policy. It is not a candidate reference and not a nested fallback route. Duplicate detection stays keyed by provider/backend model.
- `gateway-execution-overview`: the selected attempt may carry `servedModelId` when server-side fallback moved the response to another model. Every selected-attempt carrier publishes it. The owned schema accepts it.
- `gateway-execution-runtime`: request-local attempt capture records the served model after sealing without changing the attempts list or its check against expected attempts, and the test adapter reports it like ProviderWire.

`gateway-request-routing` is not modified. Its "Candidate-native request semantics across routed attempts" already runs each routed attempt through its own candidate's native validation, and "Repeated models and candidates" already attempts a shared destination once. The new capability cross-references both.

`gateway-native-provider-options` is not modified. Its spec has no fallbacks text, the MCP precedent (`gateway-anthropic-mcp`) used a new capability, and design D6's audit row satisfies its consumption audit.

## Impact

- Code:
  - `ai-gateway/pkg/internal/{config,nativeoptions,service,byok}`;
  - `ai-gateway/internal/testadapter`;
  - `ai-gateway/internal/execution` (overview, capture, stream);
  - `ai-gateway/providerwire/v4` (carriers and `schema/gateway_execution.json`).
- Tests: Go unit and service tests, ProviderWire schema tests (`ai-gateway/test/providerwire-v4`) and command tests.
- Docs: `ai-gateway/README.md`, `docs/providers/grafana-gateway.md`, `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md`.
- Recorded-fixture acceptance: the issue names `mise run test-conformance-gateway`, which does not exist on main. Task group 5 proves the same two fixtures through the real Gateway command with both clients (design D8). Recorded fixtures stay untouched. The implementation PR closes #562 and states this substitution.
- Baseline: registered `ai` 7.0.159, `@ai-sdk/anthropic` 4.0.94 and `@ai-sdk/gateway` 4.0.132 (commit `028a0253`). No pin, provider-module or UI-chunk change.
- Client-visible changes:
  - allowed fallbacks now succeed where they were refused;
  - an additive optional overview field.
