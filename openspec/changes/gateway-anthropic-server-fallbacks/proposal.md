# Proposal

## Why

Callers that send Anthropic server-side fallback (`providerOptions.anthropic.fallbacks`, an explicit chain or `"default"`) through the Gateway get `400 invalid request` ([#562](https://github.com/grafana/ai-sdk/issues/562)). The refusal exists because a caller could make the operator's Anthropic account serve models the catalog never granted. The Go provider already forwards the option like `@ai-sdk/anthropic@4.0.82`, so the only gap is an operator policy. The #201 matrix rows `anthropic/recorded/server-side-fallback-{chain,default}` fail on this refusal.

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
- Request routing (#316) is not designed here. Whatever candidates a future router selects, each applies its own policy.
- The execution overview's selected attempt gains an optional `servedModelId`. It is the `to.model` of the last `anthropic.fallback` marker in the selected output, and is absent when no hop occurred. `modelId` stays the configured or requested model. Every carrier of the selected attempt publishes it, including stream errors after the first part: committed error parts, terminal waits (closed, deadline, canceled, idle timeout) and the adapter-failure terminal error.
- Docs and the #201 harness are updated. The harness grants a fixed policy to the two matrix rows only. That harness work is gated on #201 merging.

## Capabilities

### New Capabilities

- `gateway-anthropic-server-fallbacks`: operator-policy-gated Anthropic server-side fallback at native consumption. Covers the policy shape, load and programmatic-construction validation, chain/default enforcement, Gateway support boundaries, combination with ordered fallback and routing, and evidence boundaries. It cross-references the BYOK refusal instead of restating it.

### Modified Capabilities

- `gateway-request-byok`: the BYOK refusal of native server-side fallback stays, but the spec stops requiring the same refusal for configured mode. Configured mode is governed by `gateway-anthropic-server-fallbacks`. This capability stays the single normative home of the BYOK refusal.
- `gateway-ordered-text-fallback`: a candidate reference may carry the optional `serverFallbacks` native-option policy. It is not a candidate reference and not a nested fallback route. Duplicate detection stays keyed by provider/backend model.
- `gateway-execution-overview`: the selected attempt may carry `servedModelId` when server-side fallback moved the response to another model. Every selected-attempt carrier publishes it. The owned schema accepts it.
- `gateway-execution-runtime`: request-local attempt capture records the served model after sealing without changing the attempts list, and the test adapter reports it like ProviderWire.

`gateway-native-provider-options` is not modified. Its spec has no fallbacks text, the MCP precedent (`gateway-anthropic-mcp`) used a new capability, and design D6's audit row satisfies its consumption audit.

## Impact

- Code:
  - `ai-gateway/cmd/grafana-ai-gateway/internal/{config,nativeoptions,service,byok}`;
  - `ai-gateway/internal/testadapter`;
  - `ai-gateway/internal/execution` (overview, capture, stream);
  - `ai-gateway/providerwire/v4` (carriers and `schema/gateway_execution.json`).
- Tests: Go unit and service tests, ProviderWire schema tests (`ai-gateway/test/providerwire-v4`) and command tests.
- Docs: `ai-gateway/README.md`, `docs/providers/grafana-gateway.md`, `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md`.
- Matrix acceptance: depends on unmerged PR #201 (head `6cc5480d`; `test/conformance/tools/gateway-runtime.mts`; main has no `test-conformance-gateway` task). Recorded fixtures stay untouched. The implementation PR uses `Refs #562`, not a closing keyword, unless task group 5 passes in the same PR. The change is not archived and #562 is not closed until task 5.2 passes.
- Baseline: registered `@ai-sdk/anthropic` 4.0.82 and `@ai-sdk/gateway` 4.0.116 (commit `b7f6255a`). No pin, provider-module or UI-chunk change.
- Client-visible changes:
  - allowed fallbacks now succeed where they were refused;
  - an additive optional overview field.
