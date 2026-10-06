## MODIFIED Requirements

### Requirement: Generic server_tool_use streaming

The Anthropic stream adapter SHALL handle `server_tool_use` content blocks generically for ANY tool name. The handling SHALL follow the same start/delta/stop pattern as regular `tool_use` blocks, but with `ProviderExecuted` set to `true` on all emitted stream parts. The `ToolName` in emitted parts SHALL be resolved through the tool name mapping.

When processing `input_json_delta` events, the adapter SHALL skip emitting `PartToolInputDelta` if the `PartialJSON` value is empty. Empty deltas SHALL still be accumulated in blockState.

When a `content_block_start` event of type `tool_use` or `server_tool_use` includes a `caller` field, the adapter SHALL store the caller's `type` and `tool_id` in blockState. When the corresponding `content_block_stop` event is processed and a `PartToolCall` is emitted, the adapter SHALL attach `ProviderMetadata` with key `"anthropic"` containing `{"caller": {"type": <callerType>}}`. If the caller also has a `tool_id`, it SHALL be included as `"toolId"` in the caller object.

The orchestration layer SHALL pass `ProviderMetadata` from `StreamToolCall` through to the `ChunkToolInputAvailable` UI chunk, and from `StreamToolResult` through to the `ChunkToolOutputAvailable` UI chunk.

#### Scenario: server_tool_use block start

- **WHEN** a `content_block_start` event arrives with type `"server_tool_use"`
- **THEN** the adapter stores the raw wire name in block state, records `serverToolCalls[block.ID] = wireName`, and emits a `PartToolInputStart` stream part with the block's ID, `ToolName` set to `mapping.toCustomToolName(wireName)`, and `ProviderExecuted: true`

#### Scenario: server_tool_use input delta

- **WHEN** an `input_json_delta` arrives for a `server_tool_use` block
- **THEN** the adapter emits a `PartToolInputDelta` with the partial JSON and accumulates the input

#### Scenario: Empty input_json_delta skipped

- **WHEN** an `input_json_delta` arrives with an empty `partial_json` value
- **THEN** the adapter SHALL NOT emit a `PartToolInputDelta` stream part
- **AND** the empty string SHALL still be accumulated in blockState (no-op concat)

#### Scenario: server_tool_use block stop

- **WHEN** a `content_block_stop` event arrives for a `server_tool_use` block
- **THEN** the adapter emits `PartToolInputEnd` followed by a `PartToolCall` with `ProviderExecuted: true`, the tool's call ID, mapped name, and accumulated input

#### Scenario: Unknown server tool name handled generically

- **WHEN** a `server_tool_use` block arrives with a tool name not known to the SDK (e.g., `"future_tool"`)
- **THEN** the adapter handles it identically to known server tools -- emitting `PartToolInputStart`, deltas, and `PartToolCall` with `ProviderExecuted: true`, with the unmapped name passed through

#### Scenario: tool_use block with caller metadata

- **WHEN** a `content_block_start` event arrives with type `"tool_use"` and a `caller` field with `type: "direct"`
- **THEN** the adapter stores the caller type in blockState
- **AND** when the block stops, the `PartToolCall` includes `ProviderMetadata` with `{"anthropic": {"caller": {"type": "direct"}}}`

#### Scenario: tool_use block with caller including tool_id

- **WHEN** a `content_block_start` event arrives with type `"tool_use"` and a `caller` field with `type: "code_execution_20250825"` and `tool_id: "toolu_123"`
- **THEN** the adapter stores both caller type and tool ID in blockState
- **AND** when the block stops, the `PartToolCall` includes `ProviderMetadata` with `{"anthropic": {"caller": {"type": "code_execution_20250825", "toolId": "toolu_123"}}}`

#### Scenario: tool_use block without caller

- **WHEN** a `content_block_start` event arrives with type `"tool_use"` and no `caller` field (or empty caller type)
- **THEN** the `PartToolCall` emitted at block stop SHALL NOT include caller-related `ProviderMetadata`

#### Scenario: ProviderMetadata passes through to ChunkToolInputAvailable

- **WHEN** a `StreamToolCall` with non-nil `ProviderMetadata` is mapped to a UI chunk
- **THEN** the resulting `ChunkToolInputAvailable` SHALL include the same `ProviderMetadata`

#### Scenario: ProviderMetadata passes through to ChunkToolOutputAvailable

- **WHEN** a `StreamToolResult` with non-nil `ProviderMetadata` is mapped to a UI chunk
- **THEN** the resulting `ChunkToolOutputAvailable` SHALL include the same `ProviderMetadata`

#### Scenario: ChunkToolOutputAvailable serializes ProviderMetadata to wire

- **WHEN** a `ChunkToolOutputAvailable` chunk with non-nil `ProviderMetadata` is marshaled to JSON
- **THEN** the JSON output SHALL include a `"providerMetadata"` field with the serialized metadata

#### Scenario: Locally-executed tool result inherits ProviderMetadata from ToolCall

- **WHEN** a tool is executed locally (not provider-executed) and its originating `ToolCall` has non-nil `ProviderMetadata`
- **THEN** the `StreamToolResult` emitted for that tool execution SHALL carry the same `ProviderMetadata` from the `ToolCall`
