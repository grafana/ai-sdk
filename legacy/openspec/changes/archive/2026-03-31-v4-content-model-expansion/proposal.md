## Why

PR 2 of 3 for the V4 provider upgrade (#32, depends on #66). After PR 1 reshaped core types (Usage, FinishReason, ResponseMetadata), this PR expands the content model -- what types of content can exist and where they can appear. The upstream V4 spec adds new content part types (custom, reasoning-file, tool-approval-response), removes ImageContentPart (merged into FileContentPart), expands message content constraints (ToolResultPart in assistant content, ToolApprovalResponsePart in tool messages), adds a top-level `Reasoning` field to CallOptions, and renames/extends warning types.

## What Changes

- **BREAKING**: Remove `ImageContentPart` -- images are now represented as `FileContentPart` with an image media type. Remove from `UserContentPart` sealed interface, update Anthropic provider and orchestration layer conversions.
- **BREAKING**: `ToolMessage.Content` expands from `[]ToolResultContentPart` to `[]ToolMessageContentPart` (new sealed interface) to accept both `ToolResultContentPart` and `ToolApprovalResponseContentPart`.
- **BREAKING**: Warning type `"unsupported-setting"` renamed to `"unsupported"`. New `"compatibility"` variant added.
- Add `CustomContentPart` type (prompt-side) and `PartCustom` stream part constant. Provider-specific content with `Kind` field (format `"provider.type"`). Implements `AssistantContentPart`.
- Add `ReasoningFileContentPart` type (prompt-side) and `PartReasoningFile` stream part constant. Reasoning-generated files with `MediaType` and `Data`. Implements `AssistantContentPart`.
- Add `ToolApprovalResponseContentPart` for tool messages (user's approval/denial of provider-executed tool calls).
- Add `Reasoning *string` to `CallOptions` -- model reasoning effort level (`provider-default`, `none`, `minimal`, `low`, `medium`, `high`, `xhigh`).
- Item 6 (ToolResultPart in assistant content) is already done at type level -- `ToolResultContentPart` already implements `AssistantContentPart`. No additional type changes needed.

## Capabilities

### New Capabilities
- `provider-v4-content-model`: Requirements for V4 content part types, sealed interface membership, message content constraints, and content-related stream parts. Covers CustomContentPart, ReasoningFileContentPart, ToolApprovalResponseContentPart, ImageContentPart removal, ToolMessage expansion, CallOptions.Reasoning, and Warning type changes.

### Modified Capabilities
- `effort-level`: The existing spec covers Anthropic's provider-option-based effort. The new `CallOptions.Reasoning` field is the V4 standard-level reasoning control that providers map to their native effort/thinking mechanisms. The spec needs a delta noting that `CallOptions.Reasoning` is now the primary interface, with the Anthropic provider mapping it to its native effort config.

## Impact

- `provider/content.go` -- New content part types, sealed interface changes, ImageContentPart removal
- `provider/message.go` -- ToolMessage content type expansion, new ToolMessageContentPart sealed interface
- `provider/stream_part.go` -- New stream part constants (PartCustom, PartReasoningFile)
- `provider/types.go` -- Warning type rename + new variant
- `provider/language_model.go` -- CallOptions.Reasoning field, GenerateContentPart expansion
- `anthropic/convert_request.go` -- ImageContentPart removal, CustomContentPart/ReasoningFileContentPart handling, ToolApprovalResponseContentPart handling, CallOptions.Reasoning mapping
- `anthropic/convert_stream.go` -- New stream part emission for custom/reasoning-file content
- Orchestration layer (`streamtext.go`) -- ImageContentPart conversion removal, new content part handling
- Tests across both modules
