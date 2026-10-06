## Context

The Anthropic API has an `output_config` object on message requests that carries both `effort` (reasoning depth control) and `format` (structured output schema). The Anthropic Go SDK (v1.26.0) already exposes `BetaOutputConfigParam` with `Effort BetaOutputConfigEffort` and `Format BetaJSONOutputFormatParam` fields on `BetaMessageNewParams.OutputConfig`.

Our `applyProviderOptions` in `anthropic/convert_request.go` currently handles `Thinking` and `Betas` but does not set `OutputConfig`. Effort needs to flow through `AnthropicOptions` -> `applyProviderOptions` -> `p.OutputConfig.Effort`.

## Goals / Non-Goals

**Goals:**

- Allow callers to set effort level (`low`, `medium`, `high`, `max`) via `ProviderOptions["anthropic"]`
- Pass effort as `output_config.effort` using the SDK's `BetaOutputConfigParam`
- Add the `effort-2025-11-24` beta header when effort is set
- Match the upstream Vercel AI SDK's behavior where effort is a standalone option, independent of thinking mode

**Non-Goals:**

- Structured output via `output_config.format` -- separate feature, out of scope
- Validating effort + model compatibility (the API will reject invalid combinations)
- Adding effort as a first-class field on `provider.CallOptions` -- it stays Anthropic-specific via `ProviderOptions`

## Decisions

### 1. Effort as a top-level field on `AnthropicOptions`

Add `Effort string` to `AnthropicOptions`, not nested inside `ThinkingConfig`.

**Rationale**: Upstream treats effort as independent of thinking mode. The Anthropic API sends it via `output_config`, not `thinking`. Nesting it under `ThinkingConfig` would misrepresent the API semantics and diverge from upstream's JSON shape (`{"effort":"high"}` vs `{"thinking":{"effort":"high"}}`).

**Alternative considered**: Adding effort to `ThinkingConfig` since the user feedback framed it alongside adaptive thinking. Rejected because it would break JSON compatibility with upstream and doesn't reflect the actual API structure.

### 2. Use SDK's `BetaOutputConfigParam` directly

Set `p.OutputConfig.Effort` using `BetaOutputConfigEffort(ao.Effort)` rather than building raw JSON.

**Rationale**: The Anthropic Go SDK already has typed support. Using it gives us compile-time safety and stays consistent with how we set other params (e.g., `p.Thinking`).

### 3. Build `output_config` as a shared object

Set `p.OutputConfig.Effort` without clobbering any future `p.OutputConfig.Format` usage. Since we assign individual fields rather than replacing the whole struct, adding `output_config.format` later won't conflict.

**Rationale**: Matches how upstream builds `output_config` with both `effort` and `format` as independent optional fields within the same object.

## Risks / Trade-offs

- **[Beta API]** The `effort-2025-11-24` beta header suggests this is still a beta feature. If Anthropic graduates or changes it, the beta header logic will need updating. -> Mitigation: Same pattern we already use for `interleaved-thinking-2025-05-14`; easy to update.
- **[No validation]** We don't validate that effort values are one of the four allowed strings. -> Mitigation: The Anthropic API returns a clear error for invalid values. Adding local validation would duplicate logic that belongs to the API and could break if new effort levels are added.
