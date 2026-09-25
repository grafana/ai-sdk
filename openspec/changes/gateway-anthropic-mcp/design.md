## Context

This final WP13 slice layers over SDK readiness (#238) and bounded provider-tool transport (#239). Pinned `packages/anthropic/src/anthropic-language-model-options.ts`, `anthropic-language-model.ts` and `convert-to-anthropic-prompt.ts` define request server options, native MCP blocks and type/serverName replay. `packages/gateway/src/gateway-language-model.ts` serializes those root options. All references use registered commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, not upstream HEAD. Public client source cannot establish Vercel's private hosted-service policy.

## Goals / Non-Goals

Goals: preserve request-level hosted MCP semantics on direct Anthropic routes; enforce bounded destination/options and configured-name validation; keep credentials isolated from normalized output and telemetry.

Non-goals: a Gateway MCP executor/proxy, host-managed aliases, weakening existing provider-option/header protections, tool fallback/replay, live egress attestation, persisted conversations or later result content families.

## Decisions

- Validate exact root `anthropic.mcpServers` definitions before the general provider-option mapper. Allow only that member through the host-owned-field guard at root level, retaining #191's byte-preserving forwarding of other safe options and its protected checks at every other level. Preserve optional token/enabled/allowed-tools semantics. Enforce count/field budgets, unique names, HTTPS, no embedded credentials and no URL fragments.
- Gate effects using service-configured physical provider and fallback topology, not wrapped `Provider()` identity. Only a single direct Anthropic route receives an MCP-enabled provider-option policy; the handler rejects nonempty server lists when that capability is absent before filtering or invocation. A model wrapper independently refuses MCP on noneligible routes without rejecting ordinary allowed options. No public discovery field or broad registry is needed.
- Derive allowed MCP names from validated options each request. Require provider ownership for MCP call continuation and matching caller-configured names. Return only `anthropic.type='mcp-tool-use'` and serverName, omitting caller/private fields for this shape. Unknown names fail rather than silently degrading to ordinary tools.
- Reuse the predecessor's bounded history and result lifecycle. Server options never create session state, a tool runner or fallback eligibility.
- Keep provider-only captures/scenarios and add MCP-specific captures plus table variants. Re-run both-client deferred success/error and authenticated code-execution/MCP native continuation. Fake endpoints establish transport evidence, not recorded provider provenance.

## Risks / Trade-offs

- Caller-supplied credentials/destinations → authenticate first, constrain URLs, gate configured routes, forward only to the selected Anthropic transport and assert no URL/token in normalized errors/output or telemetry.
- Remote connection policy → Anthropic initiates MCP egress; Gateway HTTP restrictions cannot prove its remote network policy. Deployment requires explicit destination/token and egress review.
- Metadata needed for replay → expose only the caller-chosen configured name, never endpoint or private backend identity.
- False parity claim → classify HTTPS and metadata restrictions as Grafana safety policy, not Vercel hosted-server equivalence. Compound outputs remain deferred and fail safely.

## Migration Plan

Integrate after the SDK and provider-tool capabilities, using candidate-source checks and real internal pins already merged to canonical main. Keep all three OpenSpec changes unarchived until owner review. Production activation is not claimed; review egress and image notices/corresponding source before rollout. Rollback deploys the prior runtime, which rejects MCP, with no persisted-state migration.

## Open Questions

Production approval of caller-selected destinations/tokens and Anthropic remote egress remains an operational prerequisite outside this PR.
