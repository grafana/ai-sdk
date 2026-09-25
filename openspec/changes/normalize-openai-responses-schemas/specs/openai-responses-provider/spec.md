## ADDED Requirements

### Requirement: OpenAI Responses schema normalization
Before building an OpenAI Responses request, the provider SHALL normalize each supplied JSON structured-response schema and each function-tool input and optional output schema, including function tools inside namespaces. It SHALL remove `propertyNames` whose schema has `type: "string"` and emit exactly one `compatibility` warning per affected schema with feature `JSON Schema propertyNames` and details `OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints.` It SHALL remove `propertyNames: null` without a compatibility warning and SHALL reject boolean or non-string, non-null `propertyNames` with an error before sending any HTTP request. It SHALL NOT mutate caller-owned schema data. Normalization and rejection SHALL apply whether or not strict output is requested and for both `DoGenerate` and `DoStream`.

#### Scenario: Recursive string property names are removed
- **WHEN** a schema contains string-schema `propertyNames` in nested `properties`, `patternProperties`, `additionalProperties`, `additionalItems`, `items`, `contains`, `not`, `allOf`, `anyOf`, `oneOf`, `definitions`, `$defs`, schema-valued `dependencies`, or `if`/`then`/`else`
- **THEN** the request omits each such `propertyNames` and preserves the other schema keywords and values, including boolean subschemas, boolean `additionalProperties`, and string-array `dependencies`
- **AND** exactly one `compatibility` warning is returned for that schema even if multiple nodes have `propertyNames`
- **AND** the caller's schema remains unchanged

#### Scenario: Both input and output function schemas need normalization
- **WHEN** a regular or namespaced function tool contains a string `propertyNames` schema in both its input and optional output schemas
- **THEN** the request contains normalized `parameters` and `output_schema` and returns one `compatibility` warning for each affected schema

#### Scenario: Null property names are removed without warning
- **WHEN** a JSON response or function-tool schema contains `propertyNames: null`, including in a nested schema
- **THEN** the request omits that `propertyNames` keyword without a normalization compatibility warning and the caller's schema remains unchanged

#### Scenario: Unsupported property name schemas fail locally
- **WHEN** a JSON response schema or regular or namespaced function input or optional output schema contains a boolean or non-string-schema, non-null `propertyNames` value, including in a nested schema
- **THEN** request preparation returns an error identifying unsupported `propertyNames` and sends no HTTP request

#### Scenario: Generate and stream agree with strict disabled
- **WHEN** equivalent response and function-tool schemas are submitted through `DoGenerate` and `DoStream`, including with `strictJsonSchema` false
- **THEN** their requests contain the same normalized schema payloads and compatible warnings, with the requested `strict` setting unchanged
- **AND** neither call changes the caller-owned schema data

#### Scenario: Unaffected schemas remain unchanged
- **WHEN** the response or function schema contains no `propertyNames` keyword, including schemas with boolean subschemas and `additionalProperties: false`
- **THEN** the request preserves the schema without a normalization compatibility warning

## MODIFIED Requirements

### Requirement: Request parameters and structured output
The provider SHALL map `temperature`, `topP`, and `maxOutputTokens` to the
Responses request, and emit `unsupported` warnings (without erroring) for
`topK`, `seed`, `presencePenalty`, `frequencyPenalty`, and `stopSequences`. For
`responseFormat` of type `json` it SHALL set `text.format` to a `json_schema`
format (honoring `strictJsonSchema`, name, description, normalized schema) or `json_object`
when no schema is provided.

#### Scenario: Unsupported sampling parameter warning
- **WHEN** `CallOptions` sets `seed`
- **THEN** the request omits a seed and an `unsupported` warning for `seed` is emitted

#### Scenario: JSON schema structured output
- **WHEN** `responseFormat` is `json` with a schema and name
- **THEN** the request `text.format` is a `json_schema` format carrying the normalized schema, name, and `strict` flag

### Requirement: Tool preparation and tool choice
The provider SHALL prepare function tools as `function` tool declarations with normalized input and optional output schemas and
provider tools by their OpenAI tool id (`openai.web_search`,
`openai.web_search_preview`, `openai.code_interpreter`, `openai.file_search`,
`openai.image_generation`, `openai.local_shell`, `openai.shell`,
`openai.apply_patch`, `openai.computer`, `openai.mcp`, `openai.tool_search`, `openai.custom`) into
the corresponding Responses tool objects. It SHALL resolve `toolChoice` of
`auto`/`none`/`required` as pass-through strings and `tool` as the typed/object
choice, with `allowedTools` overriding `toolChoice` as an `allowed_tools`
choice. Unknown tools SHALL emit an `unsupported` warning rather than erroring.

#### Scenario: Function tool declaration
- **WHEN** a function tool is provided
- **THEN** the request `tools` contains a `function` declaration with name, description, and normalized parameters schema

#### Scenario: Web search tool auto-includes sources
- **WHEN** a `openai.web_search` provider tool is provided
- **THEN** the request `tools` contains a `web_search` tool and `include` contains `web_search_call.action.sources`

#### Scenario: allowedTools overrides tool choice
- **WHEN** the `allowedTools` option lists tool names and `toolChoice` is also set
- **THEN** the request `tool_choice` is an `allowed_tools` choice listing those tools

#### Scenario: Client-executed computer declaration and choice
- **WHEN** an `openai.computer` provider tool is provided and selected by name
- **THEN** the request includes `{type: "computer"}` in `tools` and `tool_choice`
