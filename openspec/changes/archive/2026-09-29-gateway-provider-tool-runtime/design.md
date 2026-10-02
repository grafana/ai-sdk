## Context

This second WP13 slice depends on `sdk-provider-tool-contract` (#238). Source/tests at registered commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4` define the behavior: V4 provider tool/call/result/prompt types; Gateway 4.0.94 serialization; ai 7.0.116 deferred tool results; Anthropic 4.0.65 code-execution alias/replay; OpenAI 4.0.78 item/caller continuation. Public client source does not define Vercel's private hosted-service validation policy.

## Goals / Non-Goals

Goals: enable ordinary provider-tool definitions and results with bounded, stateless unary/SSE transport, both-client evidence and private-safe observation.

Non-goals: MCP root options or MCP metadata, later content families, local tool execution, persistent sessions, a new fallback retry/dispatch contract or an upstream baseline upgrade.

## Decisions

- Extend the existing discriminator-specific private mappers and DTOs. The complete request schema rejects inactive provider-definition fields before model resolution; native providers retain tool ID/argument/alias authority.
- Track only call identity and lifecycle. Seed eligible unresolved provider-owned calls from bounded request history, exclude completed/client-owned history, and do not charge historical calls against current provider-part count. Do not copy payloads or persist state.
- Permit complete calls to finish without results, but once previews start require a final result. Preserve input-start dynamic absence/false/true and omit the unregistered result-level providerExecuted wire field.
- Reuse the parent's bounded opaque metadata transport, preserving namespace objects, nested extensions and omitted/empty presence. Keep generic ownership/ID/name/history/lifecycle validation at the codec and provider-specific interpretation at actual native consumption. Returned namespace metadata does not authorize execution or require configured-server membership. Ordinary request-part options stay opaque; native-consuming adapters guard only their actual authority-changing fields/scopes.
- Keep hosted MCP activation out of this PR. Native Anthropic consumption of non-empty MCP configuration remains unsupported; foreign Anthropic settings are inert for a compatible adapter whose configured namespace is different. The successor owns bounded MCP validation at actual native consumption. Opaque response metadata is not an authority source and does not need codec-level configured-server classification.
- Split combined tests into ordinary code-execution and later MCP scenarios. Capture provider-only goldens with the pinned client instead of editing captured payloads. Keep native fake-provider evidence distinct from authentic provider fixtures.

## Risks / Trade-offs

- Incorrect history eligibility or preview closure → shared unary/stream history reconstruction and invalid-ID/name/completion tests.
- Unsupported generated media/pre-call image previews → fail safely; media remains WP16, while inherited source and reasoning-file transport retains its existing behavior.
- Hidden native effects before selection → retain the inherited fallback mechanics and document possible duplicate effects. Developers choose fallback-safe workflows; application/SDK retries are separate and no exactly-once effect claim is made.
- Returning metadata does not authorize capture → independent caller transport and real-command metadata-only telemetry assertions. Credential/account/destination authority remains host-owned.
- Incomplete intermediate evidence → both registered clients exercise native code execution and deferred-result handler scenarios without MCP; isolated module and full parity gates run at this branch.

## Migration Plan

Merge after SDK prerequisites, then merge `gateway-anthropic-mcp`. This runtime OpenSpec change is archived; the successor owns its separate MCP change and scoped docs. Existing published module refs remain unchanged. There is no persisted-state migration; rollback uses the prior image/module set. Deployment activation is unverified.
