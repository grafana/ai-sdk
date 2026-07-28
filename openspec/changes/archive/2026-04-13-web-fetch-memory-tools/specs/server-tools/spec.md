## MODIFIED Requirements

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

## ADDED Requirements

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
