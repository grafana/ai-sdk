## Context

This final WP13 slice layers over SDK readiness (#238) and bounded provider-tool transport (#239). Pinned `packages/anthropic/src/anthropic-language-model-options.ts`, `anthropic-language-model.ts` and `convert-to-anthropic-prompt.ts` define request server options, native MCP blocks and type/serverName replay. `packages/gateway/src/gateway-language-model.ts` serializes those root options. All references use registered commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, not upstream HEAD. Public client source cannot establish Vercel's private hosted-service policy.

## Goals / Non-Goals

Goals: preserve request-level hosted MCP semantics on direct Anthropic routes; enforce bounded destination/options and configured-name validation; keep credentials isolated from normalized output and telemetry.

Non-goals: a Gateway MCP executor/proxy, host-managed aliases, general root provider options, tool fallback/replay, live egress attestation, persisted conversations or later result content families.

## Decisions

- Add a narrow parser for ordered `mcpServers` instead of general option passthrough. Preserve optional token/enabled/allowed-tools semantics using the corrected Apache prerequisite. Enforce count/field budgets, unique names, HTTPS, no embedded credentials and no URL fragments.
- Gate effects at service construction, where configured physical provider and fallback topology are known. The wrapped model reports `grafana`, so runtime Provider() identity is not routing authority. A construction-time capability bit allows only single direct Anthropic routes; no public discovery field or broad registry is needed.
- Derive allowed MCP names from validated options each request. Require provider ownership for MCP call continuation and matching caller-configured names. Return only `anthropic.type='mcp-tool-use'` and serverName, omitting caller/private fields for this shape. Unknown names fail rather than silently degrading to ordinary tools.
- Reuse the predecessor's bounded history and result lifecycle. Server options never create session state, a tool runner or fallback eligibility.
- Keep provider-only captures/scenarios and add MCP-specific captures plus table variants. Re-run both-client deferred success/error and authenticated code-execution/MCP native continuation. Fake endpoints establish transport evidence, not recorded provider provenance.

## Risks / Trade-offs

- Caller-supplied credentials/destinations → authenticate first, constrain URLs, gate configured routes, forward only to the selected Anthropic transport and assert no URL/token in normalized errors/output or telemetry.
- Remote connection policy → Anthropic initiates MCP egress; Gateway HTTP restrictions cannot prove its remote network policy. Deployment requires explicit destination/token and egress review.
- Metadata needed for replay → expose only the caller-chosen configured name, never endpoint or private backend identity.
- False parity claim → classify HTTPS and metadata restrictions as Grafana safety policy, not Vercel hosted-server equivalence. Compound outputs remain deferred and fail safely.

## Migration Plan

Merge after #238 and #239 using committed published module pins with `GOWORK=off`. Keep this and predecessor OpenSpec changes unarchived until owner review. Production activation is not claimed; review egress and image notices/corresponding source before rollout. Rollback deploys the prior runtime, which rejects MCP, with no persisted-state migration.

## Open Questions

Production approval of caller-selected destinations/tokens and Anthropic remote egress remains an operational prerequisite outside this PR.
