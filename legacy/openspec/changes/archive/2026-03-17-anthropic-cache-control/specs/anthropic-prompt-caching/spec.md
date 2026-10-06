## ADDED Requirements

### Requirement: Cache control extraction from ProviderOptions
The Anthropic provider SHALL extract `cache_control` configuration from the `"anthropic"` namespace within `ProviderOptions` on any message, content part, or tool definition. The provider SHALL accept both `cacheControl` (camelCase) and `cache_control` (snake_case) as key names, with `cacheControl` taking precedence when both are present.

#### Scenario: cache_control set on a system message
- **WHEN** a `SystemMessage` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API `system` block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a user text content part
- **WHEN** a `TextContentPart` within a `UserMessage` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API text block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on an image content part
- **WHEN** an `ImageContentPart` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API image block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a file content part
- **WHEN** a `FileContentPart` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API document block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a tool call content part
- **WHEN** a `ToolCallContentPart` within an `AssistantMessage` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API tool_use block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a tool result content part
- **WHEN** a `ToolResultContentPart` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API tool_result block SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: cache_control set on a tool definition
- **WHEN** a `Tool` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}`
- **THEN** the resulting Anthropic API tool definition SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: snake_case key name accepted
- **WHEN** a content part has `ProviderOptions["anthropic"]["cache_control"]` set to `{"type": "ephemeral"}` and no `cacheControl` key is present
- **THEN** the provider SHALL use the `cache_control` value for the Anthropic API block

#### Scenario: camelCase takes precedence over snake_case
- **WHEN** a content part has both `ProviderOptions["anthropic"]["cacheControl"]` and `ProviderOptions["anthropic"]["cache_control"]` set
- **THEN** the provider SHALL use the `cacheControl` value

#### Scenario: no cache_control in ProviderOptions
- **WHEN** a message or content part has no `cacheControl` or `cache_control` in its `ProviderOptions["anthropic"]`
- **THEN** the resulting Anthropic API block SHALL NOT include a `cache_control` field

### Requirement: TTL configuration
The Anthropic provider SHALL support TTL configuration on cache_control annotations. The `type` field MUST be `"ephemeral"`. The optional `ttl` field SHALL accept values `"5m"` (5-minute) and `"1h"` (1-hour).

#### Scenario: cache_control with 5-minute TTL
- **WHEN** `cache_control` is set to `{"type": "ephemeral", "ttl": "5m"}`
- **THEN** the resulting Anthropic API block SHALL include `cache_control: {"type": "ephemeral", "ttl": "5m"}`

#### Scenario: cache_control with 1-hour TTL
- **WHEN** `cache_control` is set to `{"type": "ephemeral", "ttl": "1h"}`
- **THEN** the resulting Anthropic API block SHALL include `cache_control: {"type": "ephemeral", "ttl": "1h"}`

#### Scenario: cache_control without TTL
- **WHEN** `cache_control` is set to `{"type": "ephemeral"}` with no `ttl` field
- **THEN** the resulting Anthropic API block SHALL include `cache_control: {"type": "ephemeral"}` without a `ttl` field

### Requirement: Breakpoint limit enforcement
The Anthropic provider SHALL enforce a maximum of 4 cache breakpoints per request across all block types (system messages, content parts, tool definitions). When more than 4 cache_control annotations are present, excess annotations SHALL be dropped and a warning SHALL be emitted.

#### Scenario: exactly 4 breakpoints
- **WHEN** a request contains exactly 4 elements with `cache_control` set
- **THEN** all 4 cache_control annotations SHALL be included in the API request and no warning SHALL be emitted

#### Scenario: more than 4 breakpoints
- **WHEN** a request contains 6 elements with `cache_control` set
- **THEN** the first 4 cache_control annotations (in processing order) SHALL be included, the remaining 2 SHALL be dropped, and a warning SHALL be emitted indicating the breakpoint limit was exceeded

#### Scenario: breakpoints counted across tools and messages
- **WHEN** 2 tool definitions and 3 content parts have `cache_control` set (5 total)
- **THEN** the first 4 encountered in processing order SHALL be included and the 5th SHALL be dropped with a warning

### Requirement: Non-cacheable context handling
The Anthropic provider SHALL prevent `cache_control` from being applied to content types that Anthropic does not support caching on. When `cache_control` is set on a non-cacheable context, the annotation SHALL be dropped and a warning SHALL be emitted.

#### Scenario: cache_control on a thinking block
- **WHEN** a `ReasoningContentPart` has `ProviderOptions["anthropic"]["cacheControl"]` set
- **THEN** the resulting Anthropic API thinking block SHALL NOT include `cache_control` and a warning SHALL be emitted

#### Scenario: cache_control on a text block (cacheable)
- **WHEN** a `TextContentPart` has `ProviderOptions["anthropic"]["cacheControl"]` set
- **THEN** the resulting Anthropic API text block SHALL include `cache_control` (cacheable context)

### Requirement: Last-part cascade fallback
When the last content part in a message does not have its own `cache_control` annotation, the provider SHALL fall back to the message-level `cache_control` from the parent message's `ProviderOptions`. This cascade SHALL only apply to the last part in a message, not to earlier parts.

#### Scenario: last part inherits message-level cache_control
- **WHEN** a `UserMessage` has `ProviderOptions["anthropic"]["cacheControl"]` set to `{"type": "ephemeral"}` and its last `TextContentPart` has no `cache_control` in its own `ProviderOptions`
- **THEN** the last text block in the Anthropic API message SHALL include `cache_control: {"type": "ephemeral"}`

#### Scenario: non-last part does not inherit message-level cache_control
- **WHEN** a `UserMessage` has `ProviderOptions["anthropic"]["cacheControl"]` set and its first `TextContentPart` (not the last) has no `cache_control`
- **THEN** the first text block SHALL NOT include `cache_control`

#### Scenario: part-level cache_control overrides message-level
- **WHEN** a `UserMessage` has message-level `cache_control` with `ttl: "1h"` and the last part has its own `cache_control` with `ttl: "5m"`
- **THEN** the last block SHALL use the part-level value (`ttl: "5m"`)

#### Scenario: cascade applies to assistant messages
- **WHEN** an `AssistantMessage` has message-level `cache_control` and the last content part has no part-level `cache_control`
- **THEN** the last block in the Anthropic API assistant message SHALL include the message-level `cache_control`

#### Scenario: cascade applies to tool messages
- **WHEN** a `ToolMessage` has message-level `cache_control` and the last `ToolResultContentPart` has no part-level `cache_control`
- **THEN** the last tool_result block SHALL include the message-level `cache_control`

### Requirement: Streaming cache usage metrics
The Anthropic provider SHALL expose cache usage metrics in streaming responses through the `InputTokenDetails` field on `Usage`. This SHALL be consistent with the non-streaming response path.

#### Scenario: streaming response with cache hits
- **WHEN** a streaming response includes `cache_read_input_tokens > 0` in usage data
- **THEN** the emitted `StreamPart` usage SHALL include `InputTokenDetails` with `CacheReadTokens` populated

#### Scenario: streaming response with cache writes
- **WHEN** a streaming response includes `cache_creation_input_tokens > 0` in usage data
- **THEN** the emitted `StreamPart` usage SHALL include `InputTokenDetails` with `CacheWriteTokens` populated

#### Scenario: streaming response with no cache activity
- **WHEN** a streaming response has `cache_read_input_tokens == 0` and `cache_creation_input_tokens == 0`
- **THEN** the emitted `StreamPart` usage SHALL NOT include `InputTokenDetails`
