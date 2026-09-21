## MODIFIED Requirements

### Requirement: Tool preparation and tool choice
The provider SHALL prepare function tools as `function` tool declarations and
provider tools by their OpenAI tool id (`openai.web_search`,
`openai.web_search_preview`, `openai.code_interpreter`, `openai.file_search`,
`openai.image_generation`, `openai.local_shell`, `openai.shell`,
`openai.apply_patch`, `openai.computer`, `openai.mcp`, `openai.tool_search`, `openai.custom`) into
the corresponding Responses tool objects. It SHALL resolve `toolChoice` of
`auto`/`none`/`required` as pass-through strings and `tool` as the typed/object
choice. With declared tools, a supplied `allowedTools` SHALL override
`toolChoice` using declaration-aware allowed-tool resolution; an explicitly
empty or fully dropped selection SHALL fail before transport. Provider-option validation SHALL reject an empty allowedTools list or an invalid mode even when no tools are declared. With valid options but no tools,
tool declarations and tool choice SHALL be omitted. Unknown provider tool
declarations SHALL emit an `unsupported` warning rather than erroring solely
because the declaration is unknown.

#### Scenario: Function tool declaration
- **WHEN** a function tool is provided
- **THEN** the request `tools` contains a `function` declaration with name, description, and parameters schema

#### Scenario: Web search tool auto-includes sources
- **WHEN** a `openai.web_search` provider tool is provided
- **THEN** the request `tools` contains a `web_search` tool and `include` contains `web_search_call.action.sources`

#### Scenario: allowedTools overrides tool choice
- **WHEN** the `allowedTools` option lists representable tool names and `toolChoice` is also set
- **THEN** the request `tool_choice` is an `allowed_tools` choice listing those tools
- **AND** the full prepared `tools` array remains unchanged

#### Scenario: Client-executed computer declaration and choice
- **WHEN** an `openai.computer` provider tool is provided and selected by name
- **THEN** the request includes `{type: "computer"}` in `tools` and `tool_choice`

## ADDED Requirements

### Requirement: Declaration-aware allowed-tool resolution
Allowed-tool resolution SHALL match `@ai-sdk/openai@4.0.70` for existing supported tool kinds, using prepared declaration identities rather than mapping every selection to a function name. Function and custom entries SHALL contain their type and name, MCP entries SHALL contain type `mcp` and `server_label`, and supported built-ins SHALL contain only their type. Mode SHALL default to `auto`, preserve explicit `required`, and reject other configured modes before tool preparation. Selection order and duplicates SHALL be preserved.

Direct declared names SHALL take precedence over canonical provider aliases, with an unsupported warning on collision. Canonical aliases resolving to different wire identities SHALL be dropped with an unsupported warning. Multiple aliases for the same wire identity SHALL NOT become ambiguous solely because multiple declarations supplied them. Unknown selections SHALL retain the mapped function-name fallback with an unsupported warning.

Namespaced/deferred function tools and native tool-search selections SHALL be dropped with unsupported warnings. When declared tools exist and no allowed entry remains, conversion SHALL return an error before HTTP rather than silently removing restrictions. Both generate and stream calls, including the shared Mantle adapter, SHALL use this policy.

#### Scenario: Mixed native and custom selections
- **WHEN** a call allows a function, aliased web search, custom tool, and MCP server
- **THEN** the allowed entries SHALL respectively use function/name, web_search type only, custom/name, and mcp/server_label
- **AND** each entry SHALL agree with the actual prepared declaration

#### Scenario: Canonical built-in name and ordering
- **WHEN** an aliased web-search declaration is selected by canonical `web_search`, followed by a function and the same web-search selection again
- **THEN** all three entries SHALL be retained in that order, including the duplicate built-in entry

#### Scenario: Declared name wins a collision
- **WHEN** a function named `web_search` coexists with a differently named provider web-search tool and the allow-list selects `web_search`
- **THEN** the entry SHALL select the function named `web_search`
- **AND** an unsupported warning SHALL explain the direct-name/provider-alias collision

#### Scenario: Ambiguous MCP canonical alias
- **WHEN** two MCP declarations have different server labels and the allow-list contains a valid function and canonical `mcp`
- **THEN** only the function SHALL remain allowed and the ambiguous entry SHALL produce an unsupported warning
- **AND** selecting either MCP declaration by its own name SHALL resolve its server label

#### Scenario: Unknown selection remains visible
- **WHEN** a selected name is not part of the request's tool declarations
- **THEN** conversion SHALL retain a function entry using the existing name mapping and warn that the tool is not part of the request
- **AND** the literal name `function` SHALL NOT resolve another function merely by its tool type

#### Scenario: Unsupported subset does not remove supported entries
- **WHEN** the allow-list contains a normal function alongside native tool search, a deferred function, or a namespaced function
- **THEN** the normal function SHALL remain allowed and each unrepresentable selection SHALL be dropped with its warning

#### Scenario: Empty surviving selection is rejected
- **WHEN** declared tools exist and the allow-list is explicitly empty or every selection is dropped
- **THEN** both generate and stream conversion SHALL return an error before issuing an HTTP request
- **AND** conversion SHALL NOT fall back to request-level `toolChoice` or unrestricted tool use

#### Scenario: No declarations retain the upstream early return
- **WHEN** no tools are declared and valid `allowedTools` or `toolChoice` is supplied
- **THEN** the request SHALL omit both tool declarations and tool choice without manufacturing an allowed-tool entry

#### Scenario: Option validation precedes the no-tools early return
- **WHEN** an empty allowedTools list or invalid mode is supplied, including a call without tool declarations
- **THEN** conversion SHALL reject the provider option before HTTP rather than silently ignoring it

### Requirement: OpenAI request schema normalization
The provider SHALL non-mutatingly normalize existing JSON response schemas and function input/output schemas, including namespaced function schemas, according to `@ai-sdk/openai@4.0.70`. String-schema `propertyNames` SHALL be removed recursively and produce a compatibility warning with feature `JSON Schema propertyNames`, explaining that the provider will not enforce property-name constraints. Each normalized schema SHALL produce at most one such warning regardless of the number of removed occurrences.

Boolean, missing-type, and non-string `propertyNames` schemas SHALL produce an error before HTTP. The traversal SHALL preserve boolean subschemas, ordinary data values, dependency name arrays, and all unrelated constraints. It SHALL cover properties, patternProperties, additionalProperties, additionalItems, items (single and tuple), contains, not, composition arrays, definitions/$defs, schema dependencies, and if/then/else. Caller schemas and local validation contracts SHALL remain unchanged. Generate and stream calls and shared adapter consumers SHALL receive the same normalized request semantics.

#### Scenario: String property-name constraints are removed and disclosed
- **WHEN** a response or function input/output schema contains string `propertyNames` at top level or nested schema locations
- **THEN** the serialized schema SHALL omit those keywords and retain unrelated constraints
- **AND** conversion SHALL emit the compatibility warning for that normalized schema

#### Scenario: Unsupported property-name schema is rejected
- **WHEN** `propertyNames` is boolean, lacks `type: string`, or declares a non-string type at any visited schema location
- **THEN** conversion SHALL return an error before HTTP rather than silently broadening the schema

#### Scenario: Caller schema and literal values are preserved
- **WHEN** a caller reuses a schema containing removable keywords and literal objects in `default`, `enum`, or examples
- **THEN** the original schema SHALL remain unchanged after conversion
- **AND** literal objects SHALL NOT be traversed as schemas

#### Scenario: Unaffected schemas and adapter identity
- **WHEN** a schema without `propertyNames` is sent through OpenAI or Mantle
- **THEN** its request semantics SHALL remain unchanged and no normalization warning SHALL be added
- **AND** provider identity, endpoint routing, and metadata namespace SHALL remain unchanged
