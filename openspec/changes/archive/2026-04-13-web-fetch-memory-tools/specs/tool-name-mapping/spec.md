## MODIFIED Requirements

### Requirement: Static provider tool names table

The Anthropic provider SHALL maintain a static table mapping provider-defined tool IDs to their Anthropic API wire names. The table SHALL include entries for all currently supported provider-defined tools.

#### Scenario: Web search tool mapping

- **WHEN** a tool with `ID: "anthropic.web_search_20250305"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_search"`

#### Scenario: Web search v2 tool mapping

- **WHEN** a tool with `ID: "anthropic.web_search_20260209"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_search"`

#### Scenario: Web fetch v1 tool mapping

- **WHEN** a tool with `ID: "anthropic.web_fetch_20250910"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_fetch"`

#### Scenario: Web fetch v2 tool mapping

- **WHEN** a tool with `ID: "anthropic.web_fetch_20260209"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_fetch"`

#### Scenario: Memory tool mapping

- **WHEN** a tool with `ID: "anthropic.memory_20250818"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"memory"`

#### Scenario: Tool search BM25 mapping

- **WHEN** a tool with `ID: "anthropic.tool_search_bm25_20251119"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"tool_search_tool_bm25"`

#### Scenario: Tool search regex mapping

- **WHEN** a tool with `ID: "anthropic.tool_search_regex_20251119"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"tool_search_tool_regex"`

#### Scenario: Code execution tool mappings

- **WHEN** a tool with `ID: "anthropic.code_execution_20250522"`, `"anthropic.code_execution_20250825"`, or `"anthropic.code_execution_20260120"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"code_execution"`

#### Scenario: Computer tool mappings

- **WHEN** a tool with `ID: "anthropic.computer_20241022"`, `"anthropic.computer_20250124"`, or `"anthropic.computer_20251124"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"computer"`

#### Scenario: Text editor 20241022 and 20250124 mappings

- **WHEN** a tool with `ID: "anthropic.text_editor_20241022"` or `"anthropic.text_editor_20250124"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"str_replace_editor"`

#### Scenario: Text editor 20250429 and 20250728 mappings

- **WHEN** a tool with `ID: "anthropic.text_editor_20250429"` or `"anthropic.text_editor_20250728"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"str_replace_based_edit_tool"`

#### Scenario: Bash tool mappings

- **WHEN** a tool with `ID: "anthropic.bash_20241022"` or `"anthropic.bash_20250124"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"bash"`

#### Scenario: Unsupported tool ID not in table

- **WHEN** a provider-defined tool has an ID not present in the static table
- **THEN** no mapping entry is created for that tool (it is skipped)
