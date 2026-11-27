## Context

At `vercel/ai@0e8e0543` (`ai@7.0.173`), `createAgentUIStream` validates history, passes `convertDataPart` and the Agent's tools to `convertToModelMessages`, and then starts the Agent. The response and HTTP pipe helpers forward that option. Upstream helper tests cover converted user text and omission; direct conversion tests also cover user and assistant file output and part order. Go already has the direct converter but currently passes only the tools from `agent.go`.

## Goals / Non-Goals

Keep option-free and individual emission-option calls unchanged. Allow explicit text/file model projection without making the converter an emission option. Do not expose generic conversion options or change history validation, SSE, or any provider adapter.

## Decisions

- Use a sealed `AgentUIStreamOption` for both helpers. Existing `UIMessageStreamOption` values satisfy it; `WithAgentUIStreamConvertDataPart` does not satisfy the emission-only interface. This avoids duplicate public helpers and silent no-ops on emission-only paths.
- Collect emission options in order and configure conversion separately. Reuse `WithConvertDataPart` after validation, with a copy of the raw bytes for each callback. This prevents in-place mutation and later writes through retained slices without duplicating entire message histories. Use the validated originals for response assembly, not the input slices or converted content.
- Compare both public helpers at their prompt and frontend assembly boundaries against the pinned TypeScript helpers using synthetic model streams. Protocol and provider adapter fixtures are unchanged.

## Risks / Trade-offs

Go variadic slices are not covariant. Callers spreading `[]UIMessageStreamOption` must build `[]AgentUIStreamOption` or copy each option into a slice of that type. Exact function-type assignments must use the new variadic type. Passing existing individual options, or no options, still works. Document this source break in the PR and use breaking-change release intent.
