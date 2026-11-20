# Proposal

## Why

Claude Code and other Anthropic clients speak the Anthropic Messages API, and
the Gateway serves only ProviderWire and OpenAI Chat Completions. Teams that
want to point Claude Code at a Grafana-configured model have no route to call.
Issue #427 tracks this; the first draft (#488) was written for the standalone
Gateway command, which no longer exists. The Gateway is now a Grafana app plugin
whose platform authenticates callers and whose adapters register through a
route table, so the adapter has to be rebuilt on that contract.

## What Changes

- Add `POST namespaces/{namespace}/v1/messages` to the manifest and the
  production route table, with the `anthropic_messages` telemetry label and an
  Anthropic-shaped host-error writer.
- Add an Anthropic Messages adapter over the shared execution runtime: unary
  responses and incremental SSE; system text; ordered text turns;
  client-executed function tools, tool choice and tool-result continuations;
  stop reasons and provider-reported usage including cache read and write
  counts.
- Accept the request fields and headers Claude Code sends (`cache_control`,
  `metadata`, `thinking`, `context_management`, `output_config`,
  `anthropic-beta`, `?beta=true`, a `system`-role message after the first user
  message) without failing the request. Fields that tune only the Anthropic
  wire are accepted and not yet applied; the spec records each as a known
  limitation instead of claiming support.
- Refuse server tools, images and documents with a fixed Anthropic error
  document before model resolution.
- Require bearer authentication. The platform rejects `x-api-key` before the
  plugin runs, so Anthropic clients and Claude Code must send the Grafana
  token as a bearer token.
- Supersede #488. Its standalone listener, private authentication shim and
  adapter-local limits are not carried over.

Not in this change: thinking and signature replay, cache-control preservation,
`count_tokens`, compaction round trips (#296), model discovery, BYOK
administration, subscription or OAuth credential pass-through, and the Claude
Agent SDK. Each stays a documented limitation or a follow-up under #427.

## Capabilities

### New Capabilities
- `gateway-anthropic-messages-adapter`: the Anthropic Messages request subset,
  response and SSE semantics, protocol errors, authentication contract and
  client evidence for the Gateway's `v1/messages` route.

### Modified Capabilities
- `gateway-plugin-host`: the manifest and the production route table gain the
  `POST v1/messages` namespaced route, so the declared route count and the
  route and manifest agreement scenarios change.

## Impact

- New package `ai-gateway/anthropic/messages`; edits to the Gateway route table
  and construction in `ai-gateway/pkg/internal/gateway`.
- `ai-gateway/kinds/manifest.cue` and the generated manifest
  (`src/app-sdk-manifest.json`, `pkg/internal/manifestdata`).
- Route and manifest tests; adapter tests using the official
  `anthropic-sdk-go` client.
- Documentation: a client guide, the Gateway architecture walkthrough and
  glossary, and the README's local-development section.
- No change to ProviderWire, Chat Completions, configuration settings or
  operator-facing limits.
