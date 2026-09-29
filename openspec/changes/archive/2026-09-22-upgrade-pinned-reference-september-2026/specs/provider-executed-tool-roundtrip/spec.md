## ADDED Requirements

### Requirement: Anthropic caller metadata survives server-tool round trips
Anthropic server tool calls and web search/fetch results SHALL preserve supplied
caller metadata in generate and streaming output. Continuation SHALL project that
metadata back into native caller fields, including direct and programmatic callers.
Web-search error-json results SHALL become native web_search_tool_result_error
blocks, retaining errorCode or using unavailable when no code can be extracted.

#### Scenario: Server call and successful result retain caller
- **WHEN** a server-tool call or web search/fetch result carries caller metadata
- **THEN** normalized output carries anthropic.caller with type and optional toolId
- **AND** continuation restores native caller type and tool_id

#### Scenario: Error result retains caller
- **WHEN** a web search/fetch error result is normalized and replayed
- **THEN** the provider-specific error block and caller both survive rather than dropping the result
