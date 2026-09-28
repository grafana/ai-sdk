## MODIFIED Requirements

### Requirement: Tool preparation and tool choice
The provider SHALL prepare function tools as `function` tool declarations with normalized input and optional output schemas and
provider tools by their OpenAI tool id (`openai.web_search`,
`openai.web_search_preview`, `openai.code_interpreter`, `openai.file_search`,
`openai.image_generation`, `openai.local_shell`, `openai.shell`,
`openai.apply_patch`, `openai.computer`, `openai.mcp`, `openai.tool_search`,
`openai.programmatic_tool_calling`, `openai.custom`) into
the corresponding Responses tool objects. It SHALL resolve `toolChoice` of
`auto`/`none`/`required` as pass-through strings and `tool` as the typed/object
choice, with `allowedTools` overriding `toolChoice` as an `allowed_tools`
choice when declarations exist. Unknown declarations SHALL emit an `unsupported` warning rather than erroring.
When a supported web-search tool is present, the provider SHALL automatically
add `web_search_call.action.sources` to `include` unless either a provider-owned
model capability is false or the per-call `includeWebSearchSources` option is
explicitly false. This capability SHALL default to enabled for ordinary OpenAI
Responses models. An explicit per-call true SHALL NOT override a disabled model
capability. The provider SHALL retain all caller-specified `include` values even
when automatic web sources are disabled; unrelated automatic includes SHALL
remain independent. Without a web-search tool it SHALL NOT automatically add
web sources.

For nonempty declarations and a present `allowedTools` option, the provider SHALL resolve each selected name against the emitted declarations, preferring a direct declaration name to a canonical provider-name alias, preserving source order and duplicate selections. It SHALL select function, custom, MCP, and supported hosted tools in their Responses `allowed_tools` request shapes. It SHALL emit an `unsupported` warning for a direct-name/alias collision and select the direct name; it SHALL warn and drop ambiguous aliases or known selections that cannot be allow-listed (namespace-contained/deferred functions and unsupported hosted kinds including `tool_search`). An unknown name SHALL warn but be sent as a mapped function entry. It SHALL fail before transport only when no allowed entries remain (including an empty selection list), identifying dropped names in their original order. If no tools are declared, it SHALL omit tools and tool choice even when `allowedTools` or `toolChoice` is supplied. A non-nil `allowedTools` option SHALL override ordinary `toolChoice` even if `toolNames` is empty; its mode SHALL default to `auto` if omitted, and the supported option domain is `auto` or `required` without an additional runtime mode check in tool preparation. Resolution SHALL NOT mutate caller-provided tools, option slices, or mappings.

#### Scenario: Function tool declaration
- **WHEN** a function tool is provided
- **THEN** the request `tools` contains a `function` declaration with name, description, and normalized parameters schema

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
- **WHEN** tools are declared, `allowedTools` selects a function and `toolChoice` is also set
- **THEN** the request `tool_choice` is an `allowed_tools` choice containing `{type: "function", name: <selected name>}` and the supplied `auto` or `required` mode, defaulting to `auto` when absent

#### Scenario: Mixed kinds preserve order and duplicates
- **WHEN** the declarations include a function, web search, custom, and MCP server and `allowedTools` selects their names in mixed order with a duplicate
- **THEN** `tool_choice.tools` preserves every selected position using respectively `{type: "function", name: ...}`, `{type: "web_search"}`, `{type: "custom", name: ...}`, and `{type: "mcp", server_label: ...}` with no deduplication
- **AND** each other supported hosted declaration (`file_search`, `web_search_preview`, `image_generation`, `code_interpreter`, `computer`, `apply_patch`, `shell`, `local_shell`, `programmatic_tool_calling`) is represented by its type-only entry when selected

#### Scenario: Canonical alias and direct-name priority
- **WHEN** a declared provider tool named `search` has canonical name `web_search` and `allowedTools` selects `web_search`
- **THEN** its hosted `{type: "web_search"}` entry is selected
- **AND** if a direct function named `web_search` is also declared, that function is selected instead and an `unsupported` shadowing warning is emitted

#### Scenario: Equivalent canonical aliases are not ambiguous
- **WHEN** two `openai.web_search` declarations named `first_search` and `second_search` both have canonical alias `web_search`, and `allowedTools` selects `web_search`
- **THEN** the selection yields one `{type: "web_search"}` entry without an ambiguity warning

#### Scenario: Ambiguous alias and directly named MCP servers
- **WHEN** two MCP declarations named `alpha` and `beta` share canonical alias `mcp`, and the selection includes `mcp` and `beta`
- **THEN** `mcp` is warned and dropped as ambiguous, while `beta` is emitted as `{type: "mcp", server_label: <beta server label>}`

#### Scenario: Unsupported declared selections
- **WHEN** the selection contains a deferred or namespace-contained function or a `tool_search` declaration alongside a selectable function
- **THEN** each unsupported selection is warned and dropped, and the selectable function remains
- **AND** when only unsupported/ambiguous selections or no names are supplied, request preparation fails before HTTP with the dropped names in source order

#### Scenario: Unknown name remains a function entry
- **WHEN** tools are declared and an undeclared name is selected, whether alongside other names or alone
- **THEN** an `unsupported` warning identifies that name and the request still includes its mapped `{type: "function", name: ...}` entry

#### Scenario: No declared tools
- **WHEN** the call supplies no tool declarations and `allowedTools` or ordinary `toolChoice` is supplied
- **THEN** neither `tools` nor `tool_choice` is sent

#### Scenario: Caller inputs are unchanged
- **WHEN** the same tool declarations and allowed-tools option are reused across unary and streaming request construction
- **THEN** both generated requests have the same resolved choice and the caller's names, tools, and provider options remain unchanged

#### Scenario: Client-executed computer declaration and choice
- **WHEN** an `openai.computer` provider tool is provided and selected by name
- **THEN** the request includes `{type: "computer"}` in `tools` and `tool_choice`
