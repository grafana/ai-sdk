## Context

The Go SDK's `anthropic/convert_request.go` hardcodes `MaxTokens: 4096` for every model. The upstream TypeScript SDK (`packages/anthropic/src/anthropic-messages-language-model.ts`) uses `getModelCapabilities(modelId)` to return per-model metadata and applies it for defaults, thinking budget adjustment, and clamping. The Go SDK needs equivalent logic to match upstream behavior and unblock the assistant team.

Current state:
- `models.go` only has `vertexModelMap` for Vertex model ID resolution
- `convert_request.go` uses a flat `MaxTokens: 4096` default
- Thinking budget is applied via `applyProviderOptions` but doesn't interact with `max_tokens`
- No model-specific structured output or adaptive thinking detection

## Goals / Non-Goals

**Goals:**
- Add an unexported `getModelCapabilities` function matching the upstream's model ID matching logic and returned fields (`maxOutputTokens`, `supportsAdaptiveThinking`, `isKnownModel`)
- Use model `maxOutputTokens` as the default when `CallOptions.MaxOutputTokens` is nil
- Adjust `max_tokens` by adding thinking budget when thinking is enabled
- Default to 1024 thinking budget when thinking is enabled but no budget is provided (upstream parity)
- Clamp `max_tokens` to the model's maximum for known models, emitting a warning when the user-provided value would exceed it
- Track `supportsAdaptiveThinking` flag per model

**Non-Goals:**
- Auto-injecting beta headers based on model capabilities (upstream uses clamping, not beta injection)
- Using `supportsAdaptiveThinking` to drive reasoning config resolution (future work, separate change)
- Modifying the `provider.LanguageModel` interface or `provider.CallOptions` types
- Tracking `supportsStructuredOutput` (deferred per YAGNI — add when structured output is implemented)

## Decisions

### 1. Model matching uses `strings.Contains` on substrings, ordered most-specific first

**Rationale**: Direct port of the upstream approach. The upstream uses `modelId.includes("claude-sonnet-4-6")` etc., checking in specificity order (4-6 before 4-5 before 4-). This handles date-pinned, versioned, and bare model IDs without requiring exact map lookups.

**Alternative considered**: Exact map lookup with model ID normalization. Rejected because it wouldn't handle date-suffixed variants (e.g., `claude-sonnet-4-6-20260115`) without extra stripping logic, and would diverge from the upstream's approach.

### 2. Both capabilities struct and function are unexported

**Rationale**: The `modelCapabilities` struct and `getModelCapabilities` function are internal implementation details. Tests are white-box (same package) and can access unexported symbols. Exporting would add to the public API surface without current need.

### 3. Thinking budget adjustment happens in `buildParams`, after `applyProviderOptions`

**Rationale**: `applyProviderOptions` sets `p.Thinking` based on provider options. After that, `buildParams` can inspect `p.Thinking` to determine if thinking is enabled and extract the budget. The max_tokens adjustment and clamping happen as a post-processing step, matching the upstream's flow where `baseArgs.max_tokens` is adjusted after all options are resolved.

The flow becomes:
1. Set `p.MaxTokens` to model default (from capabilities) or user-provided value
2. `applyProviderOptions` sets thinking config
3. If thinking is enabled but budget is 0, default to 1024 and emit a compatibility warning
4. If thinking is enabled, add budget to `p.MaxTokens`
5. If known model and `p.MaxTokens > maxOutputTokens`, clamp and warn

### 4. Warning uses existing `provider.Warning` struct with `Details` field and type constants

**Rationale**: The `provider.Warning` type already exists with `Type`, `Feature`, and `Details` fields. The clamping warning uses `Type: provider.WarnUnsupported`, `Feature: "maxOutputTokens"` to match upstream semantics and align with other warnings in the codebase. The default budget warning uses `Type: provider.WarnCompatibility`, `Feature: "extended thinking"` to match the upstream exactly.

### 5. `buildParams` receives the original (pre-resolved) model ID for capabilities lookup

**Rationale**: Currently `buildParams` receives the resolved model ID (e.g., `claude-sonnet-4@20250514` for Vertex). The `getModelCapabilities` function uses substring matching which works with both canonical and resolved IDs. However, to match upstream behavior precisely, we should pass the original model ID. Since `DoStream`/`DoGenerate` call `buildParams(m.resolveModel(m.modelID), params)`, and `strings.Contains` already handles both forms, no change is needed to the call site.

## Risks / Trade-offs

- **Model list staleness**: When new Claude models are released, the capabilities function must be updated. For unknown models, it falls back to 4096/no-structured-output, matching upstream behavior.  
  Mitigation: Unknown models get safe defaults; updates are a single function change.

- **Behavior change for existing users**: Users relying on the implicit 4096 default will now get model-appropriate defaults (up to 128k). This could increase API costs for users who didn't realize they were getting truncated output.  
  Mitigation: This matches the upstream behavior and is the expected default. Users can still explicitly set `MaxOutputTokens` to control this.

- **Thinking budget double-counting**: If a user sets `MaxOutputTokens` to include their thinking budget, the automatic addition could push `max_tokens` over the model limit.  
  Mitigation: The clamping step catches this and emits a warning, matching upstream behavior exactly.
