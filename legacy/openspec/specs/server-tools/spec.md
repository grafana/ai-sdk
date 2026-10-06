## Purpose

Support Anthropic server-executed tools (web search, web fetch, memory, tool search) in both streaming and non-streaming paths, with generic handling for unknown tool types and backward compatibility with existing function tools.

## Requirements

### Requirement: Provider-defined tool request building

The Anthropic provider's `convertTools()` function SHALL accept `[]provider.Tool` (interface) and use a type switch to dispatch on `provider.FunctionTool` and `provider.ProviderTool`. `provider.ProviderTool` entries SHALL be converted into the corresponding Anthropic SDK tool union variants, dispatching on the tool's `ID` field. `provider.FunctionTool` entries SHALL continue to use the existing `OfTool` path.

The `convertProviderTool` function SHALL return `(BetaToolUnionParam, []string, *provider.Warning)` where the `[]string` contains beta header strings required by the tool. The caller SHALL merge these betas into the request's beta set.

The following tool IDs SHALL be supported:
- `"anthropic.web_search_20250305"` -> `OfWebSearchTool20250305` with args: `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation`
- `"anthropic.web_search_20260209"` -> web search tool with type `web_search_20260209` and args: `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation`, with beta `code-execution-web-tools-2026-02-09`
- `"anthropic.web_fetch_20250910"` -> web fetch tool with type `web_fetch_20250910` and args: `maxUses`, `allowedDomains`, `blockedDomains`, `citations`, `maxContentTokens`, with beta `web-fetch-2025-09-10`
- `"anthropic.web_fetch_20260209"` -> web fetch tool with type `web_fetch_20260209` and args: `maxUses`, `allowedDomains`, `blockedDomains`, `citations`, `maxContentTokens`, with beta `code-execution-web-tools-2026-02-09`
- `"anthropic.memory_20250818"` -> memory tool with type `memory_20250818` and name `memory`, with beta `context-management-2025-06-27` (no args)
- `"anthropic.tool_search_bm25_20251119"` -> `OfToolSearchToolBm25_20251119`
- `"anthropic.tool_search_regex_20251119"` -> `OfToolSearchToolRegex20251119`
- `"anthropic.code_execution_20250522"` -> code execution tool with beta `code-execution-2025-05-22`
- `"anthropic.code_execution_20250825"` -> code execution tool with beta `code-execution-2025-08-25`
- `"anthropic.code_execution_20260120"` -> code execution tool (no beta)
- `"anthropic.computer_20241022"` -> computer tool with args and beta `computer-use-2024-10-22`
- `"anthropic.computer_20250124"` -> computer tool with args and beta `computer-use-2025-01-24`
- `"anthropic.computer_20251124"` -> computer tool with args and beta `computer-use-2025-11-24`
- `"anthropic.text_editor_20241022"` -> text editor tool with beta `computer-use-2024-10-22`
- `"anthropic.text_editor_20250124"` -> text editor tool with beta `computer-use-2025-01-24`
- `"anthropic.text_editor_20250429"` -> text editor tool with beta `computer-use-2025-01-24`
- `"anthropic.text_editor_20250728"` -> text editor tool with args (no beta)
- `"anthropic.bash_20241022"` -> bash tool with beta `computer-use-2024-10-22`
- `"anthropic.bash_20250124"` -> bash tool with beta `computer-use-2025-01-24`

Unrecognized provider tool IDs SHALL produce a warning (not an error) and be skipped.

Cache control for `FunctionTool` entries SHALL be read from `FunctionTool.ProviderOptions`. `ProviderTool` entries do not carry `ProviderOptions`; cache control for provider tools SHALL be handled via the provider-specific conversion path.

#### Scenario: Web search tool with configuration

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_search_20250305"` and `Args` containing `maxUses`, `allowedDomains`, and `blockedDomains`
- **THEN** it produces a `BetaToolUnionParam` with `OfWebSearchTool20250305` populated, including the `MaxUses`, `AllowedDomains`, and `BlockedDomains` fields from the args

#### Scenario: Web search tool with no configuration

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_search_20250305"` and empty `Args`
- **THEN** it produces a `BetaToolUnionParam` with `OfWebSearchTool20250305` populated with default/zero values

#### Scenario: Web search v2 tool with configuration

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_search_20260209"` and `Args` containing `maxUses`, `allowedDomains`, `blockedDomains`, and `userLocation`
- **THEN** it produces a tool with type `web_search_20260209`, name `web_search`, and the args mapped to snake_case fields
- **AND** returns beta `"code-execution-web-tools-2026-02-09"`

#### Scenario: Web fetch tool with configuration (v1)

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_fetch_20250910"` and `Args` containing `maxUses`, `allowedDomains`, `blockedDomains`, `citations`, and `maxContentTokens`
- **THEN** it produces a tool with type `web_fetch_20250910`, name `web_fetch`, and the args mapped to snake_case fields
- **AND** returns beta `"web-fetch-2025-09-10"`

#### Scenario: Web fetch tool with configuration (v2)

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.web_fetch_20260209"` and `Args` containing `maxUses`, `allowedDomains`, `blockedDomains`, `citations`, and `maxContentTokens`
- **THEN** it produces a tool with type `web_fetch_20260209`, name `web_fetch`, and the args mapped to snake_case fields
- **AND** returns beta `"code-execution-web-tools-2026-02-09"`

#### Scenario: Memory tool definition

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.memory_20250818"`
- **THEN** it produces a tool with type `memory_20250818` and name `memory` (no args)
- **AND** returns beta `"context-management-2025-06-27"`

#### Scenario: Tool search BM25

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.tool_search_bm25_20251119"`
- **THEN** it produces a `BetaToolUnionParam` with `OfToolSearchToolBm25_20251119` populated

#### Scenario: Tool search regex

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with `ID: "anthropic.tool_search_regex_20251119"`
- **THEN** it produces a `BetaToolUnionParam` with `OfToolSearchToolRegex20251119` populated

#### Scenario: Unrecognized provider tool ID

- **WHEN** `convertTools()` receives a `provider.ProviderTool` with an unrecognized `ID`
- **THEN** a warning is added and the tool is skipped (not included in the output)

#### Scenario: Mixed function and provider tools

- **WHEN** `convertTools()` receives a mix of `provider.FunctionTool` and `provider.ProviderTool` entries
- **THEN** both types are converted and included in the output slice

#### Scenario: Function tool InputExamples unwrapping

- **WHEN** `convertTools()` receives a `provider.FunctionTool` with `InputExamples` containing `InputExample` values
- **THEN** the Anthropic conversion SHALL unwrap the `Input` field from each `InputExample` for the Anthropic SDK's expected format

#### Scenario: hasFunctionTools uses type switch

- **WHEN** `hasFunctionTools()` checks a `[]provider.Tool` slice
- **THEN** it SHALL use a type switch on `provider.FunctionTool` (not string comparison on Type field) to determine if function tools are present

#### Scenario: Provider tool betas merged into request

- **WHEN** `convertProviderTool` returns a non-empty beta slice for a tool
- **THEN** `convertTools` SHALL merge those betas into the beta set returned to `buildParams`
- **AND** `buildParams` SHALL include them in the `Betas` field of the API request

#### Scenario: Provider tool with no beta

- **WHEN** `convertProviderTool` returns an empty or nil beta slice (e.g., for `code_execution_20260120`)
- **THEN** no additional betas are added for that tool

### Requirement: web_fetch_tool_result streaming

The Anthropic stream adapter SHALL handle `web_fetch_tool_result` content blocks at `content_block_start`. The handler SHALL dispatch on the inner content type to handle success and error subtypes.

For success (`web_fetch_result`): The handler SHALL emit a `PartToolResult` with `ToolCallID` from the block's `tool_use_id`, `ToolName` set to `mapping.toCustomToolName("web_fetch")`, and structured JSON output containing camelCased fields: `type` (`"web_fetch_result"`), `url`, `retrievedAt`, and nested `content` with `type`, `title`, `citations`, and `source` (with `type`, `mediaType`, `data`). Before emitting, the handler SHALL push the fetched document into `citationDocuments` with `title` (falling back to `url` when title is nil) and `mediaType` from `source.media_type`.

For error (`web_fetch_tool_result_error`): The handler SHALL emit a `PartToolResult` with `IsError: true`, `ToolName` set to `mapping.toCustomToolName("web_fetch")`, and JSON output `{type: "web_fetch_tool_result_error", errorCode: <error_code>}`.

#### Scenario: Successful web fetch result with document content

- **WHEN** a `web_fetch_tool_result` content block arrives with inner type `web_fetch_result` containing `url: "https://example.com"`, `retrieved_at: "2025-01-01T00:00:00Z"`, and content with `type: "document"`, `title: "Example"`, `source: {type: "text", media_type: "text/plain", data: "..."}`
- **THEN** the adapter pushes `{title: "Example", mediaType: "text/plain"}` to `citationDocuments`
- **AND** emits a `PartToolResult` with structured JSON output containing `type: "web_fetch_result"`, `url`, `retrievedAt`, and nested content with camelCased field names
- **AND** `ToolName` is set to `mapping.toCustomToolName("web_fetch")`

#### Scenario: Successful web fetch result with nil title falls back to URL

- **WHEN** a `web_fetch_tool_result` content block arrives with `web_fetch_result` where `content.title` is nil and `url` is `"https://example.com"`
- **THEN** the adapter pushes `{title: "https://example.com", mediaType: <source.media_type>}` to `citationDocuments`

#### Scenario: Successful web fetch result with PDF source

- **WHEN** a `web_fetch_tool_result` content block arrives with `web_fetch_result` containing source `{type: "base64", media_type: "application/pdf", data: "..."}`
- **THEN** the output JSON contains `source: {type: "base64", mediaType: "application/pdf", data: "..."}`
- **AND** `citationDocuments` receives an entry with `mediaType: "application/pdf"`

#### Scenario: Web fetch error result

- **WHEN** a `web_fetch_tool_result` content block arrives with inner type `web_fetch_tool_result_error` and `error_code: "too_many_requests"`
- **THEN** the adapter emits a `PartToolResult` with `IsError: true` and JSON output `{type: "web_fetch_tool_result_error", errorCode: "too_many_requests"}`
- **AND** `ToolName` is set to `mapping.toCustomToolName("web_fetch")`
- **AND** no entry is added to `citationDocuments`

### Requirement: web_fetch_tool_result non-streaming

The Anthropic response converter SHALL handle `web_fetch_tool_result` content blocks in non-streaming responses with the same semantics as the streaming path.

For success (`web_fetch_result`): Produce a `GenerateContentPart` with `Type: "tool-result"`, `ToolCallID` from `tool_use_id`, `ToolName` set to `mapping.toCustomToolName("web_fetch")`, and structured JSON `Output` with camelCased fields. Push the document into `citationDocuments` before appending the result.

For error (`web_fetch_tool_result_error`): Produce a `GenerateContentPart` with `Type: "tool-result"`, `IsError: true`, `ToolName` set to `mapping.toCustomToolName("web_fetch")`, and JSON output `{type, errorCode}`.

#### Scenario: web_fetch_tool_result success in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_fetch_tool_result` block with inner type `web_fetch_result` containing `url`, `retrieved_at`, and document content with source
- **THEN** it pushes `{title: content.title ?? url, mediaType: source.media_type}` to `citationDocuments`
- **AND** produces a `GenerateContentPart` with `Type: "tool-result"`, structured JSON output with camelCased fields, and `ToolName` set to `mapping.toCustomToolName("web_fetch")`

#### Scenario: web_fetch_tool_result error in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_fetch_tool_result` block with inner type `web_fetch_tool_result_error` and `error_code: "invalid_url"`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"`, `IsError: true`, JSON output `{type: "web_fetch_tool_result_error", errorCode: "invalid_url"}`, and `ToolName` set to `mapping.toCustomToolName("web_fetch")`
- **AND** no entry is added to `citationDocuments`

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

### Requirement: Generic server_tool_use non-streaming

The Anthropic response converter SHALL handle `server_tool_use` content blocks in non-streaming responses generically for ANY tool name. Each `server_tool_use` block SHALL produce a `GenerateContentPart` with `Type: "tool-call"`, the block's ID, mapped name, input, and `ProviderExecuted: true`.

#### Scenario: server_tool_use in non-streaming response

- **WHEN** `convertResponse()` encounters a content block with type `"server_tool_use"`
- **THEN** it records `serverToolCalls[block.ID] = wireName` and produces a `GenerateContentPart` with `Type: "tool-call"`, the block's `ID` as `ToolCallID`, `ToolName` set to `mapping.toCustomToolName(wireName)`, the block's `Input` serialized as JSON, and `ProviderExecuted: true`

### Requirement: web_search_tool_result streaming

The Anthropic stream adapter SHALL handle `web_search_tool_result` content blocks by emitting a `PartToolResult` with the result data, followed by a `PartSource` for each search result URL. The `ToolName` in emitted parts SHALL be resolved through the tool name mapping rather than hardcoded.

#### Scenario: Successful web search result with URLs

- **WHEN** a `web_search_tool_result` content block arrives with an array of search results
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID` linking to the originating `server_tool_use`, `ToolName` set to `mapping.toCustomToolName("web_search")`, and the result array serialized as JSON in `Result`
- **AND** for each result in the array, the adapter emits a `PartSource` with `SourceType: "url"`, the result's `URL`, `Title`, and `pageAge` in provider metadata

#### Scenario: Web search error result

- **WHEN** a `web_search_tool_result` content block arrives with an error (not an array)
- **THEN** the adapter emits a `PartToolResult` with `ToolName` set to `mapping.toCustomToolName("web_search")` and the error information serialized in `Result`
- **AND** no `PartSource` events are emitted

### Requirement: web_search_tool_result non-streaming

The Anthropic response converter SHALL handle `web_search_tool_result` content blocks in non-streaming responses, producing `GenerateContentPart` entries for both the tool result and individual source citations. The `ToolName` in emitted parts SHALL be resolved through the tool name mapping rather than hardcoded.

#### Scenario: web_search_tool_result in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_search_tool_result` content block with an array of results
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` containing the serialized results, with `ToolName` set to `mapping.toCustomToolName("web_search")`
- **AND** it produces additional `GenerateContentPart` entries with `Type: "source"` for each URL in the results

### Requirement: tool_search_tool_result streaming

The Anthropic stream adapter SHALL handle `tool_search_tool_result` content blocks by emitting a `PartToolResult` with the result data serialized as JSON. The `ToolName` in emitted parts SHALL be resolved by looking up the originating `server_tool_use` block's wire name from the `serverToolCalls` tracking map, then passing it through the tool name mapping. If the tracking map has no entry, the handler SHALL fall back to checking which tool_search variant has a mapping entry.

#### Scenario: Successful tool search result

- **WHEN** a `tool_search_tool_result` content block arrives with tool references and `serverToolCalls` contains the originating wire name
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID` linking to the originating `server_tool_use`, `ToolName` resolved via `mapping.toCustomToolName(serverToolCalls[toolUseID])`, and the result data serialized as JSON in `Result`

#### Scenario: Tool search error result

- **WHEN** a `tool_search_tool_result` content block arrives with an error and `serverToolCalls` contains the originating wire name
- **THEN** the adapter emits a `PartToolResult` with `ToolName` resolved via `mapping.toCustomToolName(serverToolCalls[toolUseID])` and the error information serialized in `Result`

### Requirement: tool_search_tool_result non-streaming

The Anthropic response converter SHALL handle `tool_search_tool_result` content blocks in non-streaming responses. The `ToolName` in emitted parts SHALL be resolved by looking up the originating wire name from the `serverToolCalls` tracking map, then passing it through the tool name mapping. If the tracking map has no entry, the handler SHALL fall back to checking which tool_search variant has a mapping entry.

#### Scenario: tool_search_tool_result in non-streaming response

- **WHEN** `convertResponse()` encounters a `tool_search_tool_result` content block and `serverToolCalls` contains the originating wire name
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` containing the serialized result data, with `ToolName` resolved via `mapping.toCustomToolName(serverToolCalls[toolUseID])`

### Requirement: Backward compatibility

Adding server tool support SHALL NOT change the behavior of existing function tool handling. The `convertTools()` function SHALL continue to produce `OfTool` for `provider.FunctionTool` entries.

#### Scenario: Existing function tools unchanged

- **WHEN** `convertTools()` receives only `provider.FunctionTool` entries
- **THEN** the output is identical to the current behavior (all `OfTool` variants)

### Requirement: citations_delta streaming

The Anthropic stream adapter SHALL handle `citations_delta` events in `BetaRawContentBlockDeltaEvent` by converting each citation to a `PartSource` stream part with appropriate `SourceInfo`. The conversion SHALL dispatch on the citation's concrete type via `.AsAny()` and handle `web_search_result_location`, `page_location`, and `char_location` variants. Unknown citation types SHALL be silently skipped.

#### Scenario: web_search_result_location citation delta

- **WHEN** a `citations_delta` event arrives with a `BetaCitationsWebSearchResultLocation` citation containing `URL`, `Title`, `CitedText`, and `EncryptedIndex`
- **THEN** the adapter emits a `PartSource` with `SourceInfo{SourceType: "url", URL: citation.URL, Title: citation.Title}` and `ProviderMetadata{"anthropic": {"citedText": citation.CitedText, "encryptedIndex": citation.EncryptedIndex}}`

#### Scenario: page_location citation delta with tracked document

- **WHEN** a `citations_delta` event arrives with a `BetaCitationPageLocation` citation referencing `DocumentIndex: 0` and the adapter's `citationDocuments` has a document at index 0 with `Title: "Report"`, `MediaType: "application/pdf"`, `Filename: "report.pdf"`
- **THEN** the adapter emits a `PartSource` with `SourceInfo{SourceType: "document", MediaType: "application/pdf", Title: "Report", Filename: "report.pdf"}` and `ProviderMetadata{"anthropic": {"citedText": citation.CitedText, "startPageNumber": citation.StartPageNumber, "endPageNumber": citation.EndPageNumber}}`

#### Scenario: page_location citation delta uses document title from citation when available

- **WHEN** a `citations_delta` event arrives with a `BetaCitationPageLocation` citation that has a non-empty `DocumentTitle` field
- **THEN** the adapter uses `citation.DocumentTitle` as the source `Title`, overriding the tracked document's title

#### Scenario: char_location citation delta with tracked document

- **WHEN** a `citations_delta` event arrives with a `BetaCitationCharLocation` citation referencing a valid `DocumentIndex` in the adapter's `citationDocuments`
- **THEN** the adapter emits a `PartSource` with `SourceInfo{SourceType: "document"}` using the tracked document's `MediaType` and `Filename`, and `ProviderMetadata{"anthropic": {"citedText": citation.CitedText, "startCharIndex": citation.StartCharIndex, "endCharIndex": citation.EndCharIndex}}`

#### Scenario: document-based citation with out-of-range index

- **WHEN** a `citations_delta` event arrives with a `page_location` or `char_location` citation whose `DocumentIndex` exceeds the length of `citationDocuments`
- **THEN** the adapter silently skips the citation without emitting a `PartSource`

#### Scenario: unknown citation type silently skipped

- **WHEN** a `citations_delta` event arrives with a citation type not matching `web_search_result_location`, `page_location`, or `char_location` (e.g., `content_block_location` or `search_result_location`)
- **THEN** no `PartSource` is emitted and no error is produced

### Requirement: Citation document tracking

The Anthropic stream adapter and response converter SHALL track citation documents extracted from the prompt's user messages. The extraction SHALL collect file content parts with `MediaType` of `application/pdf` or `text/plain` that have `providerOptions["anthropic"]["citations"]["enabled"]` set to `true`. Each tracked document SHALL record `title` (from filename, defaulting to `"Untitled Document"`), `filename`, and `mediaType`. The documents SHALL be tracked in prompt order to match Anthropic's document indexing.

#### Scenario: PDF file part with citations enabled

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "application/pdf"`, `Filename: "report.pdf"`, and provider metadata `{"anthropic": {"citations": {"enabled": true}}}`
- **THEN** the citation documents list includes `{title: "report.pdf", filename: "report.pdf", mediaType: "application/pdf"}`

#### Scenario: Text file part with citations enabled

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "text/plain"`, `Filename: "notes.txt"`, and provider metadata `{"anthropic": {"citations": {"enabled": true}}}`
- **THEN** the citation documents list includes `{title: "notes.txt", filename: "notes.txt", mediaType: "text/plain"}`

#### Scenario: File part without citations enabled is excluded

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "application/pdf"` but no `citations.enabled` in provider metadata
- **THEN** the file is NOT included in the citation documents list

#### Scenario: File part with non-citation media type is excluded

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "image/png"` and `citations.enabled: true`
- **THEN** the file is NOT included in the citation documents list

#### Scenario: File part without filename defaults title

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "application/pdf"`, no `Filename`, and `citations.enabled: true`
- **THEN** the citation documents list includes an entry with `title: "Untitled Document"` and empty `filename`

### Requirement: Text block citation handling in non-streaming responses

The Anthropic response converter SHALL process the `Citations` array on text content blocks in non-streaming responses. For each citation, it SHALL append a `GenerateContentPart` with `Type: "source"` using the same citation-to-source conversion logic as the streaming path. Unknown citation types SHALL be silently skipped.

#### Scenario: Non-streaming text block with web search citations

- **WHEN** `convertResponse()` encounters a text content block with a `Citations` array containing `web_search_result_location` entries
- **THEN** it appends a text `GenerateContentPart` followed by source `GenerateContentPart` entries with `Type: "source"`, `SourceType: "url"`, `URL`, and `Title` from each citation

#### Scenario: Non-streaming text block with document citations

- **WHEN** `convertResponse()` encounters a text content block with `page_location` or `char_location` citations and `citationDocuments` is populated
- **THEN** it appends source `GenerateContentPart` entries with `Type: "source"`, `SourceType: "document"`, resolved `MediaType`, `Title`, and `Filename` from the tracked documents

#### Scenario: Non-streaming text block with no citations

- **WHEN** `convertResponse()` encounters a text content block with an empty `Citations` array
- **THEN** only the text `GenerateContentPart` is appended, no source entries

### Requirement: Citation source ID generation

Each `PartSource` (streaming) and source `GenerateContentPart` (non-streaming) emitted from citation conversion SHALL include a unique `ID` generated by the model's ID generator. This ID SHALL be set on `SourceInfo.ID` (streaming) or `GenerateContentPart.SourceID` (non-streaming).

#### Scenario: Each citation source gets a unique ID

- **WHEN** two `citations_delta` events arrive in sequence
- **THEN** each emitted `PartSource` has a distinct `SourceInfo.ID` value
