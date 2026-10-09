## Purpose

Support Anthropic server-executed tools (web search, web fetch, memory, tool search) in both streaming and non-streaming paths, with generic handling for unknown tool types and backward compatibility with existing function tools.

## Requirements

### Requirement: Provider-defined tool request building

Anthropic convertTools SHALL accept []provider.Tool and type-switch FunctionTool to existing OfTool and ProviderTool by ID to corresponding SDK tool unions. convertProviderTool SHALL return (BetaToolUnionParam, []string, *provider.Warning); required tool betas SHALL merge into the request beta set. Unknown provider IDs SHALL warn, not error, and be skipped.

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

At content_block_start, web_fetch_tool_result SHALL dispatch success/error inner types. Success SHALL emit PartToolResult with ToolCallID=tool_use_id and mapped web_fetch name, camelCase JSON {type:"web_fetch_result",url,retrievedAt,content:{type,title,citations,source:{type,mediaType,data}}}. Before emission, citationDocuments SHALL receive title (URL fallback for nil) and mediaType from source.media_type.

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

Unary web_fetch_tool_result SHALL match streaming semantics. Success SHALL produce GenerateContentPart Type tool-result, ToolCallID=tool_use_id, mapped web_fetch name and camelCase Output, tracking the document in citationDocuments before appending. Error SHALL produce Type tool-result, IsError:true, mapped web_fetch name and JSON {type,errorCode}.

#### Scenario: web_fetch_tool_result success in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_fetch_tool_result` block with inner type `web_fetch_result` containing `url`, `retrieved_at`, and document content with source
- **THEN** it pushes `{title: content.title ?? url, mediaType: source.media_type}` to `citationDocuments`
- **AND** produces a `GenerateContentPart` with `Type: "tool-result"`, structured JSON output with camelCased fields, and `ToolName` set to `mapping.toCustomToolName("web_fetch")`

#### Scenario: web_fetch_tool_result error in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_fetch_tool_result` block with inner type `web_fetch_tool_result_error` and `error_code: "invalid_url"`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"`, `IsError: true`, JSON output `{type: "web_fetch_tool_result_error", errorCode: "invalid_url"}`, and `ToolName` set to `mapping.toCustomToolName("web_fetch")`
- **AND** no entry is added to `citationDocuments`

### Requirement: Generic server_tool_use streaming

ANY server_tool_use name SHALL follow regular tool_use start/delta/stop with ProviderExecuted:true on all parts and mapped ToolName. Empty input_json_delta PartialJSON SHALL NOT emit PartToolInputDelta but SHALL still accumulate in blockState.

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

Stream adapter and response converter SHALL track prompt user `FileContentPart` entries in order for Anthropic document indexing, only application/pdf or text/plain with providerOptions["anthropic"]["citations"]["enabled"]=true. Each SHALL record title from filename (default "Untitled Document"), filename and mediaType.

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

### Requirement: Versioned Anthropic web-tool request projection

Anthropic SHALL accept anthropic.web_search_20260318 and anthropic.web_fetch_20260318 through the existing web-version conversion path. Every accepted web tool SHALL emit matching dated native type and fixed web_search/web_fetch name in generate and stream. Only declared camelCase args SHALL project to snake_case fields.

#### Scenario: Every supported dated web variant
- **WHEN** a request declares any of the six supported web search/fetch IDs with valid arguments
- **THEN** exactly that versioned native type and its declared fields SHALL appear in the request body, together with the specified version-specific beta if any

#### Scenario: Fetch 20260318 optional fields
- **WHEN** `anthropic.web_fetch_20260318` has `useCache: false`, `responseInclusion: "excluded"`, citations and other valid fetch fields
- **THEN** the native request SHALL include `use_cache: false`, `response_inclusion: "excluded"`, the projected citations and other fields, and SHALL NOT add a beta for that web version

#### Scenario: Search 20260318 optional fields
- **WHEN** `anthropic.web_search_20260318` has `responseInclusion: "full"`, a valid approximate user location and other search fields
- **THEN** the native request SHALL include `response_inclusion: "full"`, the projected location and other fields, and SHALL NOT add a beta for that web version

#### Scenario: Version-specific fields are not forwarded to older variants
- **WHEN** `useCache` or `responseInclusion` is present on an older web variant that does not declare that field
- **THEN** that field SHALL be omitted from the resulting request, preserving the version's own beta rule

### Requirement: Validate declared web-tool arguments before a request

Every supported web ID SHALL validate declared optional args before HTTP in generate/stream. Invalid present types, null or enum SHALL error without HTTP; omissions SHALL stay omitted. Unknown fields SHALL be stripped as upstream object-schema projection. Unsupported IDs SHALL retain warning-and-skip behavior.

#### Scenario: Fractional and large finite web-tool numbers
- **WHEN** a supported web tool supplies a finite fractional or out-of-int64 `maxUses` or `maxContentTokens` value
- **THEN** the HTTP request SHALL contain that numeric value in the corresponding wire field without truncation, including when unsupported tools precede it in the declared tools list; no override SHALL reinsert a tool removed by tool choice `none`

#### Scenario: Invalid numeric and domain fields
- **WHEN** a supported web tool supplies a non-number `maxUses` or `maxContentTokens`, `null` for a declared field, or non-string elements in a domain list
- **THEN** request preparation SHALL fail before any HTTP request

#### Scenario: Invalid nested objects or enum
- **WHEN** a declared web tool supplies `citations` without `enabled` or with a non-boolean `enabled`, `userLocation` without `type` or with a type other than `approximate` or non-string location fields, invalid `useCache`, or `responseInclusion` outside `full` and `excluded`
- **THEN** request preparation SHALL fail before any HTTP request

#### Scenario: Unrecognized arguments and unsupported provider tool IDs
- **WHEN** a supported web tool has an extra unknown argument, or a separately declared provider tool ID is unsupported
- **THEN** the unknown argument SHALL be stripped for the supported tool and the unsupported ID SHALL still be skipped with a warning; neither SHALL become a function tool

### Requirement: Web-tool requests preserve function-tool and no-tool boundaries
Adding versioned provider-defined tools and their validation SHALL NOT interpret a server tool's `Strict`, `InputExamples` or `ProviderOptions` as function settings. Function tools SHALL retain their own strict support/warnings, input-example conversion and Anthropic provider-option behavior. Existing auto/required/named/none tool-choice behavior with an empty tool list SHALL remain unchanged.

#### Scenario: Mixed function and provider-defined web tools
- **WHEN** a function tool with `Strict`, `InputExamples` and Anthropic tool provider options is declared alongside a provider-defined web tool
- **THEN** each SHALL be converted through its own path, preserving applicable function-only betas and warnings and web-version beta rules without leaking function-only fields onto the server tool

#### Scenario: Invalid function input examples and options retain existing handling
- **WHEN** malformed function-tool input examples or Anthropic provider options accompany a valid server web tool
- **THEN** their existing skip/warning behavior SHALL remain unchanged, and the server tool SHALL NOT inherit those fields

#### Scenario: Empty tool choice
- **WHEN** a request has no tools and an auto, required, named, or none tool choice
- **THEN** it SHALL preserve the existing selection/omission semantics for that choice, without fabricating a web tool

### Requirement: Anthropic function and provider tool cache ownership

FunctionTool cache control SHALL come from FunctionTool.ProviderOptions. ProviderTool has no ProviderOptions; its cache control SHALL use provider-specific conversion.

#### Scenario: Anthropic function and provider tool cache ownership

- **WHEN** a request mixes function and provider-defined tools
- **THEN** each SHALL follow its own cache-control path without attributing function ProviderOptions to provider tools

### Requirement: Anthropic web-search 20250305 declaration

anthropic.web_search_20250305 SHALL map to OfWebSearchTool20250305 with maxUses, allowedDomains, blockedDomains and userLocation args.

#### Scenario: Anthropic web-search 20250305 declaration

- **WHEN** that provider ID supplies all declared args
- **THEN** the corresponding native variant SHALL contain the projected args, including user location

### Requirement: Anthropic web-search 20260209 declaration

anthropic.web_search_20260209 SHALL emit type web_search_20260209 with maxUses, allowedDomains, blockedDomains and userLocation, selecting beta code-execution-web-tools-2026-02-09.

#### Scenario: Anthropic web-search 20260209 declaration

- **WHEN** that provider ID supplies its configuration
- **THEN** the matching native type, declared fields and specified beta SHALL be sent

### Requirement: Anthropic web-fetch 20250910 declaration

anthropic.web_fetch_20250910 SHALL emit type web_fetch_20250910 with maxUses, allowedDomains, blockedDomains, citations and maxContentTokens, selecting beta web-fetch-2025-09-10.

#### Scenario: Anthropic web-fetch 20250910 declaration

- **WHEN** that provider ID supplies its configuration
- **THEN** the matching native type, declared fields and specified beta SHALL be sent

### Requirement: Anthropic web-fetch 20260209 declaration

anthropic.web_fetch_20260209 SHALL emit type web_fetch_20260209 with maxUses, allowedDomains, blockedDomains, citations and maxContentTokens, selecting beta code-execution-web-tools-2026-02-09.

#### Scenario: Anthropic web-fetch 20260209 declaration

- **WHEN** that provider ID supplies its configuration
- **THEN** the matching native type, declared fields and specified beta SHALL be sent

### Requirement: Anthropic memory and discovery declarations

anthropic.memory_20250818 SHALL emit type memory_20250818, name memory, no args and beta context-management-2025-06-27. anthropic.tool_search_bm25_20251119 and anthropic.tool_search_regex_20251119 SHALL map to OfToolSearchToolBm25_20251119 and OfToolSearchToolRegex20251119 respectively.

#### Scenario: Anthropic memory and discovery declarations

- **WHEN** memory and both tool-search variants are declared
- **THEN** each SHALL emit its native variant and memory SHALL select the specified beta without args

### Requirement: Anthropic code-execution declaration versions

`anthropic.code_execution_20250522`, `anthropic.code_execution_20250825` and `anthropic.code_execution_20260120` SHALL emit code execution tools with code-execution-2025-05-22, code-execution-2025-08-25 and no beta respectively.

#### Scenario: Anthropic code-execution declaration versions

- **WHEN** all three code-execution provider IDs are declared
- **THEN** each SHALL use its dated conversion and only the first two SHALL contribute their specified betas

### Requirement: Anthropic computer declaration versions

`anthropic.computer_20241022`, `anthropic.computer_20250124` and `anthropic.computer_20251124` SHALL emit computer tools with args and computer-use-2024-10-22, computer-use-2025-01-24 and computer-use-2025-11-24 betas respectively.

#### Scenario: Anthropic computer declaration versions

- **WHEN** all three computer provider IDs are declared with valid args
- **THEN** their corresponding computer conversions SHALL project args and merge each required beta

### Requirement: Anthropic text-editor and bash declaration versions

`anthropic.text_editor_20241022` SHALL select computer-use-2024-10-22; `anthropic.text_editor_20250124` and `anthropic.text_editor_20250429` SHALL select computer-use-2025-01-24; `anthropic.text_editor_20250728` SHALL accept args with no beta. `anthropic.bash_20241022` and `anthropic.bash_20250124` SHALL select computer-use-2024-10-22 and computer-use-2025-01-24 respectively.

#### Scenario: Anthropic text-editor and bash declaration versions

- **WHEN** these text-editor and bash provider IDs are declared
- **THEN** their corresponding conversions SHALL produce the tools and specified beta rules, including no beta for text-editor 20250728

### Requirement: Anthropic streaming web-fetch error projection

Inner web_fetch_tool_result_error SHALL emit PartToolResult with IsError:true, mapping.toCustomToolName("web_fetch") and JSON {type:"web_fetch_tool_result_error",errorCode:<error_code>}.

#### Scenario: Anthropic streaming web-fetch error projection

- **WHEN** a streaming fetch result has error_code too_many_requests
- **THEN** the error result SHALL use the mapped name and camelCase errorCode, without adding a citation document

### Requirement: Anthropic streamed tool caller metadata

At tool_use/server_tool_use content_block_start with caller, the adapter SHALL store caller.type/tool_id in blockState. On stop, PartToolCall SHALL attach ProviderMetadata["anthropic"]={caller:{type:<callerType>}}, including toolId only if supplied.

#### Scenario: Anthropic streamed tool caller metadata

- **WHEN** server_tool_use has caller type code_execution_20250825 and tool_id toolu_123
- **THEN** the emitted call SHALL retain caller type and camelCase toolId in Anthropic metadata

### Requirement: Anthropic tool metadata UI propagation

Orchestration SHALL pass StreamToolCall.ProviderMetadata to ChunkToolInputAvailable and StreamToolResult.ProviderMetadata to ChunkToolOutputAvailable.

#### Scenario: Anthropic tool metadata UI propagation

- **WHEN** tool call and result stream parts carry caller metadata
- **THEN** their respective UI input/output-available chunks SHALL carry the same metadata

### Requirement: Anthropic versioned web argument fields

Search 20250305/20260209 SHALL project maxUses, allowedDomains, blockedDomains, userLocation; 20260318 SHALL add responseInclusion (full|excluded). Fetch 20250910/20260209 SHALL project maxUses, allowedDomains, blockedDomains, citations.enabled, maxContentTokens; 20260318 SHALL add useCache (including false) and responseInclusion (full|excluded).

#### Scenario: Anthropic versioned web argument fields

- **WHEN** 20260318 search/fetch include responseInclusion and fetch useCache:false while older variants supply those extras
- **THEN** new variants SHALL project their declared additions, while older variants SHALL omit undeclared fields

### Requirement: Anthropic versioned web beta selection

Fetch 20250910 SHALL select web-fetch-2025-09-10; search/fetch 20260209 SHALL select code-execution-web-tools-2026-02-09. Search/fetch 20260318 and search 20250305 SHALL select no version-specific beta. Other tool and explicit Anthropic betas SHALL still merge and deduplicate as before.

#### Scenario: Anthropic versioned web beta selection

- **WHEN** a 20260318 web tool is combined with a beta-requiring older web tool and duplicate explicit betas
- **THEN** only the older version SHALL contribute a web beta, merged once with other requested betas

### Requirement: Anthropic web numeric argument fidelity

Valid finite numeric maxUses/maxContentTokens, including fractions, SHALL serialize without integer truncation. SDK typed integer fields SHALL NOT narrow the registered upstream z.number() contract.

#### Scenario: Anthropic web numeric argument fidelity

- **WHEN** web tools supply fractional or out-of-int64 finite numbers after an unsupported tool
- **THEN** the corresponding wire numbers SHALL remain intact, without reinstating tools removed by choice none

### Requirement: Anthropic web nested argument schemas

Domain lists SHALL be string arrays. Present citations SHALL require boolean enabled; present userLocation SHALL require type approximate and permit optional string city/region/country/timezone. useCache SHALL be boolean; responseInclusion SHALL be the 20260318-only full|excluded enum.

#### Scenario: Anthropic web nested argument schemas

- **WHEN** a supported web tool has malformed citations, location, domain list, useCache or responseInclusion
- **THEN** request preparation SHALL reject it before HTTP rather than coerce or forward invalid declared fields
