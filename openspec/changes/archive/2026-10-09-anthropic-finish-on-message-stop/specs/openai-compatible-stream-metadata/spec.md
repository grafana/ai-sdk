## ADDED Requirements

### Requirement: Tool input parts are identified by id

OpenAI-compatible streaming tool calls SHALL emit `tool-input-start`, `tool-input-delta` and `tool-input-end` parts identified by `id`. Only `tool-input-start` SHALL carry the tool name. None of them SHALL carry a separate tool call ID field.

#### Scenario: Streamed tool call
- **WHEN** a streamed tool call produces input parts
- **THEN** each part SHALL carry the call `id`, only the start part SHALL carry `toolName`, and the final `tool-call` part SHALL carry `toolCallId` and `toolName`
