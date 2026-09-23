## Why

Issue [#107](https://github.com/grafana/ai-sdk/issues/107), work package 13 of `~/ai-sdk-ai-gateway-plan.md`, extends the merged unary and streaming function-tool capabilities (#105 and #106). The Gateway currently rejects provider definitions, provider-executed history and enabled execution markers, preventing clients from consuming native provider tools even where Apache provider implementations already support them.

## What Changes

- Accept the registered provider-tool definition (`type`, `id`, `name`, object `args`) without widening it to function-tool fields or definition-level `providerOptions`.
- Preserve execution ownership, dynamic calls, preliminary/final results, and reviewed tool metadata through unary, streaming and stateless continuation flows in both clients, including deferred results correlated with unresolved provider calls in supplied history and Anthropic MCP tool replay.
- **BREAKING**: normalize provider-domain `Preliminary` and unary `Dynamic` fields to booleans where absent and false mean disabled. Retain `StreamPart.Dynamic *bool` because input-start absence triggers tool-definition inference while explicit false suppresses it; preserve that distinction end to end and align this input-start inference with pinned upstream. Function `Strict` stays presence-aware.
- Reject invalid mixed direct-Go tool definitions before backend I/O; retain native provider-specific conversion, alias mapping and unsupported-tool behavior.
- Accept the registered request-level `providerOptions.anthropic.mcpServers` for direct Anthropic routes, forwarding its endpoint and optional token only to that provider, and expose the minimum MCP `serverName` metadata necessary for continuation. Keep other root provider options deferred and prohibit MCP secrets/URLs in public output or operational telemetry.
- Extend bounded private Gateway encoders/state, native request evidence, the independent Go client and logical observability while preserving metadata-only export privacy.
- Keep effectful fallback disabled. Do not introduce a Gateway tool executor, tool registry, cross-request state, approvals, new host-managed MCP configuration, general root provider-option passthrough, or later content capabilities.

## Capabilities

### New Capabilities

- `gateway-provider-tools`: provider definitions, execution ownership, provider results/continuation including Anthropic MCP, a narrow request-level MCP configuration option, reviewed metadata, direct validation, privacy and end-to-end acceptance.
- `anthropic-unary-context-deadline`: let bounded unary calls use the caller's context deadline to avoid the Anthropic Go SDK's non-streaming token-estimate refusal, while explicit request timeouts retain precedence.

### Modified Capabilities

- `gateway-unary-function-tools`: extend history/output beyond client-executed calls while preserving the function subset and its bounds.
- `gateway-streaming-function-tools`: replace the WP13 deferral with correlated provider calls, dynamic markers and preliminary-to-final results.
- `grafana-gateway-client`: extend closed unary/SSE consumption to the provider-tool families, retaining independent codecs and client-owned normalization.
- `v4-tool-type-split`: prohibit function-only fields, including definition-level provider options, on provider-typed tools instead of the previous deliberate widening.
- `v4-tool-result-alignment`: normalize semantically equivalent markers while retaining presence-sensitive input-start dynamic behavior.
- `mcp-server-tools`: make native Anthropic MCP `enabled` and `authorizationToken` presence-aware so omitted and explicit false/empty retain their registered request semantics.

## Impact

Apache work affects `provider/`, root consumers of changed fields, `providers/grafana`, existing native provider conversions and unary deadline handling in `providers/anthropic`, and `middleware/agentobservability`. AGPL work affects `ai-gateway/providerwire/v4`, service acceptance/privacy tests and `ai-gateway/test/providerwire-v4`. Planning documents remain in `openspec/`.

The authority remains `test/conformance/upstream.yaml`: provider 4.0.7, gateway 4.0.52, ai 7.0.65 and the registered provider packages. No baseline upgrade is proposed. The public Vercel Gateway client serializes request-level `mcpServers` and consumes tool metadata, but its private service's acceptance/egress policy is not available; Grafana's explicit safety policy is not claimed as Vercel-hosted parity. Provider contract/implementation evidence needs native request snapshots; Gateway evidence needs actual pinned-client differential/handler tests; affected UI behavior needs cross-language integration. Existing recorded/upstream provider inputs remain provenance-controlled.

Apache prerequisite changes must precede dependent Gateway commits and be published at immutable, proxy-resolvable versions. Gateway verification uses committed module pins with `GOWORK=off`, without committed replacements or root-workspace membership.
