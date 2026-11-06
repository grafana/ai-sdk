## Context

The registered reference is `@ai-sdk/amazon-bedrock@5.0.130` and `@ai-sdk/anthropic@4.0.94`, source revision `028a025398782f4938303f48fbeee45c79af544b`. The issue originally assessed Bedrock 5.0.125; the warning behavior remains present at the current pin. `amazon-bedrock-prepare-tools.ts` first filters unsupported web tools, appends shared Anthropic warnings, then warns for provider tools without a factory. Non-Anthropic models produce only the Bedrock warning, without details.

## Goals / Non-Goals

Goals: exact warning lists, grouped ordering, unchanged filtering, and equal unary/stream delivery. Non-goals: changing tool-choice request routing or recording live provider traffic.

## Decisions

Keep unknown tools excluded from request construction. Collect their diagnostics after web-tool filtering, respecting the upstream named-choice selection boundary. Shared Anthropic warnings precede its forced-choice warning and the Bedrock warnings. Forced choices using only functions retain the Bedrock-specific warning wording.

Use focused synthetic request tests; no new provider recordings or imported fixtures are claimed. Existing Bedrock conformance snapshots remain unchanged. The coverage map already lists this provider implementation surface, so it needs no inventory edit.

## Risks / Trade-offs

Unknown tools combined with explicit tool choices can expose existing request-routing differences unrelated to warning content. Preserve current requests and report that limit rather than silently expand the fix.
