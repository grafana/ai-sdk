## Context

The Anthropic provider currently appends the beta header `effort-2025-11-24` whenever it sets `output_config.effort` on the request body. There are two code paths:

1. **Provider-options path** (`convert_request.go:1764-1767` in `applyProviderOptions`): when the caller sets `AnthropicOptions.Effort` directly.
2. **Reasoning-mapping path** (`reasoning.go:156-159` in `applyReasoningConfigWithProviderHints`): when `CallOptions.Reasoning` is mapped to an effort value for an adaptive-thinking-capable model.

This header was correct during the Anthropic effort-parameter preview (added 2025-11-24) but was retired when the feature went GA. The Anthropic API now silently ignores the header on direct requests, but Vertex AI's strict beta header validator rejects requests carrying it with `HTTP 400: Unexpected value(s) `effort-2025-11-24` for the `anthropic-beta` header`.

Upstream removed this in `vercel/ai@e5c4f40` (anthropic canary.46). The Go port must follow.

## Goals / Non-Goals

**Goals:**
- Remove the `effort-2025-11-24` beta header from both code paths.
- Keep `output_config.effort` body field wiring unchanged — it remains the mechanism that drives the feature.
- Update tests and the `effort-level` spec to reflect the new behavior.
- Restore Vertex AI compatibility for any request that uses effort or maps reasoning to effort.

**Non-Goals:**
- Changing the `AnthropicOptions.Effort` provider option surface or the `CallOptions.Reasoning` mapping behavior.
- Removing other beta headers (e.g., `interleaved-thinking-2025-05-14`) that are still required.
- Adding a compatibility flag to re-enable the header — direct Anthropic ignores it harmlessly, and Vertex actively rejects it, so there is no environment where keeping it is correct.

## Decisions

**Decision: Remove the beta header in both code paths, not just one.**
Both call sites add the same header for the same reason (effort being set). Leaving one in would still trigger Vertex 400s for whichever path is unfixed. Match upstream's single-line removal in `anthropic-language-model.ts`.

**Decision: Update the `effort-level` spec instead of leaving the requirement orphaned.**
The current spec has two requirements that mandate the header (`Effort beta header` and `Reasoning effort beta header`). Both must be removed so the spec describes actual behavior. Adjacent scenarios (e.g., `No beta headers for none`) keep their wording — they already match the new reality.

**Decision: No fallback / no transition period.**
Direct Anthropic silently ignores the stale header (no harm in removing it). Vertex AI rejects requests carrying it (active harm in keeping it). Removal is strictly an improvement on both backends; there is no caller for whom the old behavior is preferable.

## Risks / Trade-offs

- **Risk**: If a future Anthropic version reintroduces a beta gate for effort, we would need to re-add it. → Mitigation: Same pattern as upstream; trivial to revert.
- **Risk**: Tests in the wild that asserted the header was present will fail. → Mitigation: We own all the relevant tests in this repo; external callers asserting on request bodies are out of scope for the SDK contract.
