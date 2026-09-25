## MODIFIED Requirements

### Requirement: Tool preparation and tool choice
The provider SHALL prepare function tools as `function` tool declarations and
provider tools by their OpenAI tool id (`openai.web_search`,
`openai.web_search_preview`, `openai.code_interpreter`, `openai.file_search`,
`openai.image_generation`, `openai.local_shell`, `openai.shell`,
`openai.apply_patch`, `openai.computer`, `openai.mcp`, `openai.tool_search`, `openai.custom`) into
the corresponding Responses tool objects. It SHALL resolve `toolChoice` of
`auto`/`none`/`required` as pass-through strings and `tool` as the typed/object
choice, with `allowedTools` overriding `toolChoice` as an `allowed_tools`
choice. Unknown tools SHALL emit an `unsupported` warning rather than erroring.
When a supported web-search tool is present, the provider SHALL automatically
add `web_search_call.action.sources` to `include` unless either a provider-owned
model capability is false or the per-call `includeWebSearchSources` option is
explicitly false. This capability SHALL default to enabled for ordinary OpenAI
Responses models. An explicit per-call true SHALL NOT override a disabled model
capability. The provider SHALL retain all caller-specified `include` values even
when automatic web sources are disabled; unrelated automatic includes SHALL
remain independent. Without a web-search tool it SHALL NOT automatically add
web sources.

#### Scenario: Function tool declaration
- **WHEN** a function tool is provided
- **THEN** the request `tools` contains a `function` declaration with name, description, and parameters schema

#### Scenario: Web search tool auto-includes sources
- **WHEN** an `openai.web_search` or `openai.web_search_preview` provider tool is provided to a default OpenAI model without a per-call opt-out
- **THEN** the request `tools` contains the web-search tool and `include` contains `web_search_call.action.sources`

#### Scenario: Per-call opt-out and explicit true
- **WHEN** a web-search tool is supplied on an enabled OpenAI model and `includeWebSearchSources` is explicitly false
- **THEN** the request retains the web tool but does not automatically include `web_search_call.action.sources`
- **AND** when the per-call value is true instead, automatic inclusion remains enabled

#### Scenario: Model capability dominates per-call true
- **WHEN** a web-search tool is supplied on a model whose source-include capability is disabled and `includeWebSearchSources` is true or unset
- **THEN** the request retains the web tool but does not automatically include `web_search_call.action.sources`

#### Scenario: Explicit include survives automatic opt-out
- **WHEN** a caller explicitly lists `web_search_call.action.sources` in `include` while the per-call option or model capability disables automatic inclusion
- **THEN** the serialized request still contains the explicitly requested source include exactly once
- **AND** independent code-interpreter, logprobs and encrypted-reasoning includes remain governed by their own options

#### Scenario: No web-search tool
- **WHEN** no web-search tool is supplied, whether `includeWebSearchSources` is true or unset
- **THEN** the request does not automatically add `web_search_call.action.sources`

#### Scenario: allowedTools overrides tool choice
- **WHEN** the `allowedTools` option lists tool names and `toolChoice` is also set
- **THEN** the request `tool_choice` is an `allowed_tools` choice listing those tools

#### Scenario: Client-executed computer declaration and choice
- **WHEN** an `openai.computer` provider tool is provided and selected by name
- **THEN** the request includes `{type: "computer"}` in `tools` and `tool_choice`
