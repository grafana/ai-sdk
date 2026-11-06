# Proposal

## Why

Callers that send Anthropic server-side fallback (`providerOptions.anthropic.fallbacks`, an explicit chain or `"default"`) through the Gateway get `400 invalid request` ([#562](https://github.com/grafana/ai-sdk/issues/562)). The refusal exists because a caller could make the operator's Anthropic account serve models the catalog never granted. BYOK refuses too, although the caller's own account pays. The Go provider already forwards the option like `@ai-sdk/anthropic@4.0.94`, so the gaps are an operator policy for configured candidates and forwarding for BYOK. The live recordings `anthropic/recorded/server-side-fallback-{chain,default}` replay that request, and the Gateway refuses both before any backend call.

## What Changes

- A configured candidate (`primary` or a `fallback` entry) can carry an optional `serverFallbacks` policy: the fallback models it allows and whether `"default"` is allowed. Without a policy, the candidate refuses as it does today.
- Configuration loading accepts the policy only on `anthropic` candidates. The policy must allow `"default"` or at least one model, and its models must be distinct, nonempty and within the existing text bounds. Programmatic catalog construction, which skips configuration validation, enforces only the `anthropic`-type check.
- Native Anthropic option validation resolves the same `AnthropicOptions` the adapter consumes and checks them against the candidate's policy, before native I/O and after per-attempt translation:
  - each chain entry's model must be allowlisted (exact bytes), and entries must be distinct. Upstream forwards any array; these are intentional Gateway support boundaries, because the operator account pays;
  - `"default"` requires `allowDefault`.
  - Other per-entry fields are forwarded unchanged.
  - A refusal uses the existing unsupported-request error without echoing model names.
- BYOK forwards native server-side fallback (a chain, including repeated models, and `"default"`) without a policy, matching `@ai-sdk/anthropic@4.0.94`, for plain and routed calls and for translation-added options. The caller's own account pays, and BYOK already reaches any native model of the caller's accounts through the selector and routed `models`. Configured policies never apply to BYOK, and no client control grants one. Container skills refusal and MCP checks are unchanged in both modes.
- Ordered Gateway fallback (#121) is unchanged:
  - Server-side fallback runs inside one Gateway attempt.
  - Each candidate applies its own policy.
  - A refused candidate fails before its native request, and the existing decider advances to the next candidate.
  - If no candidate is left, the call fails with the invalid-request error.
- Request routing (#316, delivered by #596) reuses each route's startup candidates, so every routed attempt applies its own candidate's policy. When routes share a destination, routing attempts it once, under the policy of the candidate composed first. BYOK fallback-model accounts forward server-side fallback on each Anthropic account.
- The execution overview's selected attempt, configured or BYOK, gains an optional `servedModelId`. It is the `to.model` of the latest valid `anthropic.fallback` marker in the selected output so far, across a stream's response segments, and is absent when no hop occurred. `modelId` stays the configured or requested model, and the `gateway.routing` summary is unchanged. Every carrier of the selected attempt publishes it, including unary invalid-output errors and stream errors after the first part: committed error parts, terminal waits (closed, deadline, canceled, idle timeout, invalid output) and the adapter-failure terminal error.
- Docs are updated. A command-level test replays the two recorded fixtures, unchanged, through the real Gateway binary under a fixed allowing policy, with both registered clients.

## Capabilities

### New Capabilities

- `gateway-anthropic-server-fallbacks`: operator-policy-gated Anthropic server-side fallback for configured candidates at native consumption. Covers the policy shape, load and programmatic-construction validation, chain/default enforcement, Gateway support boundaries, combination with ordered fallback and request routing, and evidence boundaries. It cross-references BYOK forwarding in `gateway-request-byok` instead of restating it.

### Modified Capabilities

- `gateway-request-byok`: BYOK stops refusing native server-side fallback and forwards it without a policy, plain and routed, including translation-added options. "Unsupported controls fail explicitly" states that server-side fallback within the selected account and destination is not an override. The spec stops requiring a refusal for configured mode, which `gateway-anthropic-server-fallbacks` governs. "Shared native execution protections" is aligned with the existing mixed-chain behavior: one pre-check before any account attempt only when every destination is Anthropic, otherwise per Anthropic account. This capability stays the single normative home of BYOK behavior.
- `gateway-ordered-text-fallback`: a candidate reference may carry the optional `serverFallbacks` native-option policy. It is not a candidate reference and not a nested fallback route. Duplicate detection stays keyed by provider/backend model.
- `gateway-execution-overview`: the selected attempt, configured or BYOK, may carry `servedModelId` when server-side fallback moved the response to another model. Every selected-attempt carrier publishes it. The owned schema accepts it.
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
- Issue acceptance "Disallowed models or default are still refused before native I/O, with tests for configured and BYOK modes": by nara's decision BYOK forwards (design Resolved decisions). Configured tests prove refusal; BYOK tests prove forwarding and that configured policies do not apply. The implementation PR states this.
- Recorded-fixture acceptance: the issue names `mise run test-conformance-gateway`, which does not exist on main. Task group 5 proves the same two fixtures through the real Gateway command with both clients (design D8). Recorded fixtures stay untouched. The implementation PR closes #562 and states this substitution.
- Baseline: registered `ai` 7.0.159, `@ai-sdk/anthropic` 4.0.94 and `@ai-sdk/gateway` 4.0.132 (commit `028a0253`). No pin, provider-module or UI-chunk change.
- Client-visible changes:
  - allowed configured fallbacks, and all BYOK fallbacks, now succeed where they were refused;
  - an additive optional overview field.
