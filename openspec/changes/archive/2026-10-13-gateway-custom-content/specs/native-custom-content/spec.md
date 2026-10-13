# Spec Delta

## Purpose

Define how native provider adapters convert registered custom content that they do not consume as a specific provider block, matching the registered upstream adapters.

## ADDED Requirements

### Requirement: Anthropic custom tool-result tool references
On the Anthropic `tool_result` path, the adapter SHALL convert a custom tool-result content entry whose `anthropic` options contain `type: "tool-reference"` into a `tool_reference` block carrying the options' `toolName` value unchanged as `tool_name`, omitting `tool_name` when `toolName` is absent. Other custom tool-result entries SHALL be omitted with a warning of type `other` and message `unsupported custom tool content part`.

#### Scenario: Tool reference entry
- **WHEN** a tool result's content contains a custom entry with `anthropic: {type: "tool-reference", toolName: "search"}`
- **THEN** the native tool_result content SHALL contain `{type: "tool_reference", tool_name: "search"}` at that position

#### Scenario: Tool reference without a name
- **WHEN** a tool-reference entry has no `toolName`
- **THEN** the native block SHALL contain `type: "tool_reference"` without a `tool_name` member

#### Scenario: Other custom entry
- **WHEN** a tool result's content contains a custom entry without Anthropic tool-reference options
- **THEN** the entry SHALL be omitted and one `other` warning SHALL be returned

### Requirement: Anthropic tool-result content after filtering
When non-empty tool-result content entries are all omitted during conversion, the Anthropic adapter SHALL send the tool_result with an empty content array rather than substituting an empty text block.

#### Scenario: Only an unsupported custom entry
- **WHEN** a tool result's content contains only one non-reference custom entry
- **THEN** the native tool_result SHALL have `content: []` and the result SHALL carry one `other` warning

### Requirement: Unknown assistant custom kinds are skipped
The Anthropic adapter SHALL skip assistant custom parts whose kind is not `anthropic.fallback` without emitting a warning. The OpenAI-compatible and Bedrock Converse adapters SHALL skip all assistant custom parts without a warning or error. Anthropic fallback parts SHALL keep their existing anthropic-fallback-content behavior.

#### Scenario: Foreign compaction history
- **WHEN** assistant history sent to Anthropic, OpenAI-compatible or Bedrock Converse contains an `openai.compaction` custom part
- **THEN** the native request SHALL omit it, the call SHALL proceed, and the result SHALL contain no warning for it

### Requirement: OpenAI compaction replay presence
The OpenAI Responses adapter SHALL treat a compaction `itemId` as present when it is supplied as a string, including an empty string; absent or null SHALL mean absent. It SHALL emit a compaction item for a present `itemId` even when `encryptedContent` is absent, omitting `encrypted_content`, unless conversation or store rules already handled the part.

#### Scenario: Compaction without encrypted content
- **WHEN** non-stored history without a conversation contains `openai.compaction` with `itemId: "cmp_1"` and no `encryptedContent`
- **THEN** the native input SHALL contain `{type: "compaction", id: "cmp_1"}` without `encrypted_content`

#### Scenario: Empty item ID is present
- **WHEN** history with store enabled and no conversation contains `openai.compaction` with `itemId: ""`
- **THEN** the native input SHALL contain an item reference with id `""`

### Requirement: Unary container upload output
Anthropic unary responses SHALL convert each native `container_upload` block into custom content with kind `anthropic.container_upload` and metadata `anthropic: {fileId}` in original content order. Streaming SHALL keep the registered upstream behavior and not emit it.

#### Scenario: Container upload block
- **WHEN** a non-streaming response contains a `container_upload` block with file ID `file_1` between text blocks
- **THEN** the result SHALL contain custom content `anthropic.container_upload` with `{"anthropic":{"fileId":"file_1"}}` between the text parts
