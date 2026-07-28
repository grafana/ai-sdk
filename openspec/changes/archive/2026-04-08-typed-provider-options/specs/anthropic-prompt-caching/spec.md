## MODIFIED Requirements

### Requirement: Cache control extraction from ProviderOptions
The Anthropic provider SHALL extract `cache_control` configuration from the `"anthropic"` namespace within `ProviderOptions` on any message, content part, or tool definition. The provider SHALL use `provider.ResolveOption` to handle both typed options (direct `AnthropicOptions` or cache control option values) and round-tripped raw options (`RawProviderOption` from previous SSE responses). The provider SHALL accept both `cacheControl` (camelCase) and `cache_control` (snake_case) as key names when resolving from `RawProviderOption` JSON, with `cacheControl` taking precedence when both are present.

#### Scenario: cache_control set on a system message
- **WHEN** a `SystemMessage` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API `system` block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a user text content part
- **WHEN** a `TextContentPart` within a `UserMessage` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API text block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on an image content part
- **WHEN** a `FileContentPart` with image media type has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API image block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a file content part
- **WHEN** a `FileContentPart` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API document block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a tool call content part
- **WHEN** a `ToolCallContentPart` within an `AssistantMessage` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API tool_use block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a tool result content part
- **WHEN** a `ToolResultContentPart` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API tool_result block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a tool definition
- **WHEN** a `Tool` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"`
- **THEN** the resulting Anthropic API tool definition SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: snake_case key accepted in round-tripped raw options
- **WHEN** a content part has a `RawProviderOption` at key `"anthropic"` with JSON containing `"cache_control"` key and no `"cacheControl"` key
- **THEN** the provider SHALL use the `cache_control` value for the Anthropic API block

#### Scenario: camelCase takes precedence over snake_case in raw options
- **WHEN** a content part has a `RawProviderOption` at key `"anthropic"` with JSON containing both `"cacheControl"` and `"cache_control"` keys
- **THEN** the provider SHALL use the `cacheControl` value

#### Scenario: no cache_control in ProviderOptions
- **WHEN** a message or content part has no cache control configured in its `ProviderOptions["anthropic"]`
- **THEN** the resulting Anthropic API block SHALL NOT include a `cache_control` field

### Requirement: Last-part cascade fallback
When the last content part in a message does not have its own `cache_control` annotation, the provider SHALL fall back to the message-level `cache_control` from the parent message's `ProviderOptions`. This cascade SHALL only apply to the last part in a message, not to earlier parts.

#### Scenario: last part inherits message-level cache_control
- **WHEN** a `UserMessage` has `ProviderOptions["anthropic"]` containing cache control type `"ephemeral"` and its last `TextContentPart` has no cache control in its own `ProviderOptions`
- **THEN** the last text block in the Anthropic API message SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: non-last part does not inherit message-level cache_control
- **WHEN** a `UserMessage` has `ProviderOptions["anthropic"]` containing cache control and its first `TextContentPart` (not the last) has no cache control
- **THEN** the first text block SHALL NOT include `cache_control`

#### Scenario: part-level cache_control overrides message-level
- **WHEN** a `UserMessage` has message-level cache control with `ttl: "1h"` and the last part has its own cache control with `ttl: "5m"`
- **THEN** the last block SHALL use the part-level value (`ttl: "5m"`)

#### Scenario: cascade applies to assistant messages
- **WHEN** an `AssistantMessage` has message-level cache control and the last content part has no part-level cache control
- **THEN** the last block in the Anthropic API assistant message SHALL include the message-level cache control

#### Scenario: cascade applies to tool messages
- **WHEN** a `ToolMessage` has message-level cache control and the last `ToolResultContentPart` has no part-level cache control
- **THEN** the last tool_result block SHALL include the message-level cache control

### Requirement: Non-cacheable context handling
The Anthropic provider SHALL prevent `cache_control` from being applied to content types that Anthropic does not support caching on. When `cache_control` is set on a non-cacheable context, the annotation SHALL be dropped and a warning SHALL be emitted.

#### Scenario: cache_control on a thinking block
- **WHEN** a `ReasoningContentPart` has `ProviderOptions["anthropic"]` containing cache control
- **THEN** the resulting Anthropic API thinking block SHALL NOT include `cache_control` and a warning SHALL be emitted

#### Scenario: cache_control on a text block (cacheable)
- **WHEN** a `TextContentPart` has `ProviderOptions["anthropic"]` containing cache control
- **THEN** the resulting Anthropic API text block SHALL include `cache_control` (cacheable context)
