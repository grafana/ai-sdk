## Context

This second WP13 slice depends on `sdk-provider-tool-contract` (#238). Source/tests at registered commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` define the behavior: V4 provider tool/call/result/prompt types; Gateway 4.0.52 serialization; ai 7.0.65 deferred tool results; Anthropic 4.0.38 code-execution alias/replay; OpenAI 4.0.41 item/caller continuation. Public client source does not define Vercel's private hosted-service validation policy.

## Goals / Non-Goals

Goals: enable ordinary provider-tool definitions and results with bounded, stateless unary/SSE transport, both-client evidence and private-safe observation.

Non-goals: MCP root options or MCP metadata, later content families, local tool execution, persistent sessions, fallback effects or an upstream baseline upgrade.

## Decisions

- Extend the existing discriminator-specific private mappers and DTOs. The complete request schema rejects inactive provider-definition fields before model resolution; native providers retain tool ID/argument/alias authority.
- Track only call identity and lifecycle. Seed eligible unresolved provider-owned calls from bounded request history, exclude completed/client-owned history, and do not charge historical calls against current provider-part count. Do not copy payloads or persist state.
- Permit complete calls to finish without results, but once previews start require a final result. Preserve input-start dynamic absence/false/true and omit the unregistered result-level providerExecuted wire field.
- Bound metadata before projection. Anthropic caller type/toolId and OpenAI/Azure itemId, namespace and caller type/callerId are reviewed producer/replay fields; unknown private fields are omitted. Explicit MCP metadata fails rather than becoming an ordinary tool. Ordinary request-part options remain opaque namespace objects except host-reserved namespaces.
- Keep MCP acceptance out of this PR rather than adding a disabled feature switch. Nonempty root options remain unsupported. The successor introduces options, configured-server matching and service-owned route eligibility together.
- Split combined tests into ordinary code-execution and later MCP scenarios. Capture provider-only goldens with the pinned client instead of editing captured payloads. Keep native fake-provider evidence distinct from authentic provider fixtures.

## Risks / Trade-offs

- Incorrect history eligibility or preview closure → shared unary/stream history reconstruction and invalid-ID/name/completion tests.
- Provider tools emitting sources/files/pre-call image previews → fail safely; media remains WP16 and other families retain their owners.
- Metadata leaking deployment information → bounded explicit projection and real-command metadata-only telemetry assertions.
- Incomplete intermediate evidence → both registered clients exercise native code execution and deferred-result handler scenarios without MCP; isolated module and full parity gates run at this branch.

## Migration Plan

Merge after SDK prerequisites, then merge `gateway-anthropic-mcp`. Each PR carries only its own unarchived OpenSpec change and scoped docs. Existing published module refs remain unchanged. There is no persisted-state migration; rollback uses the prior image/module set. Deployment activation is unverified.
