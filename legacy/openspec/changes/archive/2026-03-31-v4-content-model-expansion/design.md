## Context

This is PR 2 of 3 for the V4 provider upgrade (#32). PR 1 (#66) reshaped core types (Usage, FinishReason, ResponseMetadata, ProviderMetadata rename, specVersion bump). This PR expands the content model — what types of content can exist, where they appear, and how messages carry them.

The upstream V4 spec introduces new content part types, removes ImageContentPart in favor of FileContentPart, expands tool message content, adds a standard-level reasoning effort field, and renames warning types. Our Go port needs to match these changes.

Current state:
- `ImageContentPart` exists but `streamtext.go` already converts it to `FileContentPart` before calling providers — it's a convenience input type
- `ToolMessage.Content` is `[]ToolResultContentPart` (concrete slice, not interface)
- `ToolResultContentPart` already implements `AssistantContentPart` (item 6 is done at type level)
- `PartToolApprovalRequest`/`PartToolApprovalResult` stream part constants exist but `StreamPart` has no dedicated fields for them
- No `PartCustom` or `PartReasoningFile` stream part constants exist
- Warning type uses `"unsupported-setting"` string value
- `CallOptions` has no `Reasoning` field
- `GenerateContentPart` and `StreamPart` have no `Kind` field for custom content

## Goals / Non-Goals

**Goals:**
- Add all V4 content part types to the provider package (CustomContentPart, ReasoningFileContentPart, ToolApprovalResponseContentPart)
- Expand sealed interfaces to match upstream V4 message content constraints
- Remove ImageContentPart from provider package
- Add CallOptions.Reasoning field and map it in the Anthropic provider
- Rename warning type constants and add the new "compatibility" variant
- Add PartCustom, PartReasoningFile stream part constants with supporting fields

**Non-Goals:**
- Anthropic provider handling of CustomContentPart or ReasoningFileContentPart in convert_request.go (Anthropic doesn't natively support these — they'll produce warnings if encountered)
- Orchestration-layer conversion fixes for ToolResultPart in assistant content (#31 remaining scope)
- Tool type split (FunctionTool + ProviderTool) — that's PR 3 (#68)
- Source document variant, ToolResult.preliminary, stream part id fields — PR 3

## Decisions

### 1. ToolMessage sealed interface

**Decision**: Introduce `ToolMessageContentPart` sealed interface with marker method `toolMessageContentPart()`. Change `ToolMessage.Content` from `[]ToolResultContentPart` to `[]ToolMessageContentPart`.

**Rationale**: V4 allows both `ToolResultPart` and `ToolApprovalResponsePart` in tool messages. A sealed interface is the Go-idiomatic way to model this discriminated union, matching the pattern already used for `UserContentPart` and `AssistantContentPart`.

**Alternative**: Keep `[]ToolResultContentPart` and add a separate `Approvals []ToolApprovalResponseContentPart` field. Rejected — diverges from upstream model, makes message construction awkward.

### 2. ImageContentPart removal

**Decision**: Remove `ImageContentPart` entirely from `provider/content.go`. Remove the `case provider.ImageContentPart` from Anthropic's `convertUserContent`. Update the orchestration layer to remove any ImageContentPart → FileContentPart conversion (since callers now use FileContentPart directly).

**Rationale**: Upstream V4 has no ImageContentPart — images are FileContentPart with image media types. Our `streamtext.go` already does this conversion internally, confirming it's lossless. The Anthropic provider's `convertUserContent` handles both image and document media types on FileContentPart.

**Alternative**: Keep ImageContentPart as a deprecated convenience type that delegates to FileContentPart. Rejected — we don't maintain backward compatibility (per project guidelines).

### 3. CustomContentPart.Kind as plain string

**Decision**: `Kind` is `string` type with documented convention `"provider.type"` (e.g. `"anthropic.cache-control"`). No runtime validation.

**Rationale**: Go has no template literal types. Runtime validation adds cost with little benefit — providers produce well-formed values, and the convention is self-documenting. Matches how `ToolChoice.Type` and `Warning.Type` work.

### 4. StreamPart field additions

**Decision**: Add to `StreamPart`:
- `Kind string` — for `PartCustom` (and reusable for future provider-specific parts)
- `ApprovalID string` — for `PartToolApprovalRequest` (makes the existing constant functional)
- `Approved *bool` — for `PartToolApprovalResult`
- `Reason string` — already exists as part of other fields? No — add for tool approval result

Reuse existing fields: `PartReasoningFile` reuses `FileData` + `MediaType` (same shape as `PartFile`).

**Rationale**: The flat union struct pattern adds fields incrementally. `Kind` is the only truly new concept. Tool approval fields make the pre-existing constants functional.

### 5. GenerateContentPart.Kind field

**Decision**: Add `Kind string` to `GenerateContentPart` for `type: "custom"` content in non-streaming results.

**Rationale**: The flat union struct needs this field to distinguish custom content. Same as StreamPart.Kind.

### 6. CallOptions.Reasoning as *string

**Decision**: `Reasoning *string` with documented constants (`ReasoningProviderDefault`, `ReasoningNone`, `ReasoningMinimal`, `ReasoningLow`, `ReasoningMedium`, `ReasoningHigh`, `ReasoningXHigh`). Named constants are `string` type, not a custom type alias.

**Rationale**: Matches the upstream string literal union. `*string` because it's optional (nil = not specified). Plain `string` constants (not a custom type) match the pattern used by `ToolChoice.Type` — callers can pass literal strings or use the constants.

### 6a. Anthropic reasoning mapping — dual-path model capability approach

**Decision**: The Anthropic provider maps `CallOptions.Reasoning` to thinking+effort config via two code paths, selected by model capabilities. This matches upstream exactly.

**Model capability detection**: Add `getModelCapabilities(modelID string)` to the Anthropic module, returning `maxOutputTokens int`, `supportsAdaptiveThinking bool`, and `isKnownModel bool`. Uses `strings.Contains` substring matching on model ID, matching upstream's `modelId.includes()` pattern:
- `claude-sonnet-4-6`, `claude-opus-4-6` → adaptive thinking, 128k max tokens
- `claude-sonnet-4-5`, `claude-opus-4-5`, `claude-haiku-4-5` → budget-based, 64k max tokens
- `claude-opus-4-1` → budget-based, 32k max tokens
- Other `claude-sonnet-4-`, `claude-opus-4-` → budget-based, 64k/32k max tokens
- Unknown → budget-based, 4096 max tokens

**Reasoning resolution** (port of upstream `resolveAnthropicReasoningConfig`):
- `nil` / `"provider-default"` → no-op (skip mapping)
- `"none"` → `thinking: disabled` (thinking suppressed entirely, no effort)
- Adaptive-capable models: set `thinking: adaptive` + map reasoning to effort via effort map (`minimal`→`low`, `low`→`low`, `medium`→`medium`, `high`→`high`, `xhigh`→`max`). Emit `compatibility` warning when mapped value differs from input.
- Older models: set `thinking: enabled` + compute `budgetTokens = clamp(round(maxOutputTokens * pct), 1024, maxOutputTokens)` with percentages: `minimal` 2%, `low` 10%, `medium` 30%, `high` 60%, `xhigh` 90%.

**Precedence**: If EITHER `ProviderOptions["anthropic"]["thinking"]` OR `ProviderOptions["anthropic"]["effort"]` is already set, skip the reasoning mapping entirely. Provider-specific options always win. This matches upstream exactly.

**Rationale**: The dual-path approach exists because older Anthropic models (4-5 family) don't support the adaptive thinking mode or effort-based control. They require explicit token budgets. Matching upstream avoids behavioral divergence that would confuse users migrating from the TypeScript SDK.

**Alternative**: Simple flat `reasoning → effort` table. Rejected — diverges from upstream, breaks for older models that don't support effort, and misses the thinking type configuration that must accompany effort.

**Out of scope**: `max_tokens` adjustment (`maxTokens + thinkingBudget`), temperature/topK/topP suppression when thinking is enabled, and default budget fallback for missing `budgetTokens` — these are existing infrastructure gaps unrelated to the reasoning field and can be addressed separately.

### 7. Warning type rename — clean break

**Decision**: Rename `"unsupported-setting"` to `"unsupported"` everywhere. Add `"compatibility"` as a new type value. No backward compatibility shim.

**Rationale**: The SSE wire format sends warnings in `stream-start` chunks. The downstream `@ai-sdk/react` hooks read warning types from the V4 format. Since we're bumping to V4, the new type values are expected by consumers.

### 8. Anthropic provider handling of new content types

**Decision**: New prompt-side types (`CustomContentPart`, `ReasoningFileContentPart`) that appear in `convertAssistantContent` produce a warning and are skipped. `ToolApprovalResponseContentPart` in `convertToolContent` is handled if/when Anthropic supports tool approval.

**Rationale**: Anthropic's API doesn't have native equivalents for custom content or reasoning files in prompts. Producing warnings (not errors) matches the existing pattern for unsupported features.

## Risks / Trade-offs

- **ImageContentPart removal is source-breaking** for any callers constructing `ImageContentPart` directly. Mitigation: the fix is mechanical (replace with `FileContentPart` using the same `DataContent` + `MediaType`). All internal usage is in tests and the Anthropic provider.

- **ToolMessage.Content type change** breaks callers iterating `[]ToolResultContentPart` without a type switch. Mitigation: callers must now type-switch on `ToolMessageContentPart`, which is the same pattern used for user/assistant content.

- **Warning type rename** could break downstream consumers comparing warning type strings. Mitigation: consumers should already be handling unknown warning types gracefully. The rename aligns with what V4 `@ai-sdk/react` expects.

- **CallOptions.Reasoning overlap with Anthropic provider options**: The standard `Reasoning` field and the provider-specific `ProviderOptions["anthropic"]["thinking"]`/`["effort"]` could conflict. Mitigation: if either provider option is set, the reasoning mapping is skipped entirely — provider-specific options always take precedence (matching upstream).

- **Model capability detection is hardcoded by model ID**: `getModelCapabilities` uses substring matching on model IDs, which requires updating when new model families launch. Mitigation: unknown model IDs fall back to conservative defaults (budget-based, 4096 max tokens). Same approach as upstream.
