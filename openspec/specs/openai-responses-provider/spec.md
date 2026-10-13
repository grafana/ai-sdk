# openai-responses-provider Specification

## Purpose
Define the OpenAI Responses API provider module, including request conversion,
tool preparation, response and stream mapping, provider options, error handling,
and conformance expectations needed to stay aligned with Vercel's upstream AI
SDK behavior.

## Requirements

### Requirement: Provider construction and identity

The `providers/openai` Go module SHALL expose `NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel` and `NewResponsesWithClient(client openai.Client, modelID string, opts ...Option) provider.LanguageModel`, backed by Responses via `github.com/openai/openai-go`. Both SHALL implement provider.LanguageModel, report v4 and the supplied modelID, accept functional options, and neither panic nor call the network during construction.

#### Scenario: Construct a Responses model
- **WHEN** `NewResponses("test-key", "gpt-4o")` is called
- **THEN** it returns a non-nil `provider.LanguageModel`
- **AND** `Provider()` is `"openai"`, `ModelID()` is `"gpt-4o"`, and `SpecificationVersion()` is `"v4"`

#### Scenario: Base URL override for testing
- **WHEN** `NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL)))` is constructed
- **THEN** subsequent `DoGenerate`/`DoStream` calls target the overridden base URL

#### Scenario: Provider integration supplies a configured client
- **WHEN** a provider integration passes a configured official SDK client to `NewResponsesWithClient`
- **THEN** subsequent model calls retain the client's endpoint, authentication, headers, retries, and transport

#### Scenario: Provider integration overrides identity
- **WHEN** a provider integration constructs a model with `WithProviderName("example.responses")`
- **THEN** `Provider()` returns `"example.responses"`
- **AND** generated provider metadata continues to use the stable `"openai"` namespace
- **AND** OpenAI-specific call options continue to resolve from the `"openai"` namespace

#### Scenario: Default constructor retains Azure-only options
- **WHEN** an existing `NewResponses` model receives only `ProviderOptions["azure"]`
- **THEN** generate and stream requests continue to apply those options

#### Scenario: Empty provider identity override
- **WHEN** a model is constructed with `WithProviderName("")`
- **THEN** `Provider()` remains `"openai"`

#### Scenario: Custom provider identity continues a stored response
- **WHEN** a model with a custom provider identity emits an assistant item ID and that content is included in a later prompt
- **THEN** request conversion emits an `item_reference` for the stored item instead of resending the assistant content

#### Scenario: Azure continuation omits top-level options
- **WHEN** a model configured with an Azure identity emits Azure-namespaced assistant metadata and a later continuation includes that metadata without top-level Azure options
- **THEN** request conversion still reads the Azure item ID and emits an `item_reference`

### Requirement: System message conversion
The provider SHALL convert system messages according to the resolved
`systemMessageMode`: `system` emits a `system` role input item, `developer`
emits a `developer` role input item, and `remove` drops the system message and
emits an `unsupported` warning. The mode SHALL default to `developer` for
reasoning models and `system` otherwise, and SHALL be overridable via the
`systemMessageMode` provider option.

#### Scenario: System mode emits system role
- **WHEN** the model resolves `systemMessageMode == system` and the prompt contains a system message
- **THEN** the request `input` contains an item with role `system` and the system text

#### Scenario: Developer mode for reasoning model
- **WHEN** the model id is a reasoning model and no explicit `systemMessageMode` is set, with a system message present
- **THEN** the request `input` contains an item with role `developer`

#### Scenario: Remove mode drops system message
- **WHEN** `systemMessageMode == remove` and a system message is present
- **THEN** the system message is not sent in `input`
- **AND** an `unsupported` warning for `system messages are removed for this model` is emitted

### Requirement: User and assistant message conversion

User input SHALL use role user and content arrays: text→input_text; image→input_image via image_url/file_id/data URI honoring imageDetail; file→input_file via file_id/file_url or filename+file_data. Unsupported file media SHALL warn/error as upstream. Reconstructed assistant text SHALL use easy-input string content, retain phase, omit stale IDs. Stored assistant text SHALL use item_reference when store=true with an ID.

#### Scenario: User text and image
- **WHEN** a user message contains a text part and an image URL part
- **THEN** the input item content contains an `input_text` part and an `input_image` part with `image_url`

#### Scenario: PDF file part
- **WHEN** a user message contains an `application/pdf` file part with inline data
- **THEN** the input item content contains an `input_file` part carrying the file data and a derived filename

#### Scenario: Provider-reference image uses file ID
- **WHEN** an image file part carries a provider reference containing the active OpenAI or Azure provider key
- **THEN** the input item content SHALL contain an `input_image` part whose `file_id` is the referenced provider-specific identifier

#### Scenario: Provider-reference document uses file ID
- **WHEN** a non-image file part carries a provider reference containing the active OpenAI or Azure provider key
- **THEN** the input item content SHALL contain an `input_file` part whose `file_id` is the referenced provider-specific identifier

#### Scenario: Provider reference lacks the active provider
- **WHEN** a file part carries an empty provider reference or one without the active OpenAI or Azure provider key
- **THEN** request conversion SHALL return an error rather than emitting an empty file input or falling back to another data source

#### Scenario: Stored assistant text becomes an item reference
- **WHEN** store is true and an assistant text part carries an item ID
- **THEN** the request emits an item_reference item for that ID rather than inline text

#### Scenario: Reconstructed assistant text retains phase
- **WHEN** store is false or an assistant text part has no stored item ID
- **THEN** the request emits string content, including an explicitly empty string
- **AND** phase is preserved when present without emitting an incomplete output message

### Requirement: Tool call and tool result conversion

Assistant calls SHALL become function_call or corresponding built-in call items (local_shell_call, shell_call, apply_patch_call, tool_search_call, custom_tool_call when configured); tool-role results SHALL become function_call_output or matching built-in outputs. Undefined tool input SHALL serialize to "{}". Names SHALL resolve through provider-tool mapping before taxonomy dispatch.

#### Scenario: Function call round-trip
- **WHEN** an assistant message contains a tool-call with name `getWeather` and arguments and a following tool result is present
- **THEN** the request contains a `function_call` item and a `function_call_output` item sharing the same `call_id`

#### Scenario: Empty tool input serializes to empty object
- **WHEN** a tool-call part has no input
- **THEN** the serialized `arguments` is `"{}"`, not `"null"`

#### Scenario: Stored provider-executed continuation uses item references
- **WHEN** an assistant provider-executed tool call and result are continued with `store` enabled and OpenAI item ids are available
- **THEN** the request emits the corresponding `item_reference` entries rather than flattening the history into function items
- **AND** distinct call and output item ids, such as hosted tool-search ids, remain distinct

#### Scenario: Stored ordinary function call remains inline
- **WHEN** a stored ordinary client-executed function call has an OpenAI item id and a following function result has its `call_id`
- **THEN** the request emits the inline `function_call` and `function_call_output` with their matching `call_id`, not an `item_reference` for the call item id

#### Scenario: Client provider tool continuation preserves native items
- **WHEN** prior assistant/tool history contains a client-executed local-shell, shell, apply-patch, tool-search, or custom-tool call and result
- **THEN** the request emits the matching native Responses call and output item types
- **AND** the call and output share the original `call_id`

#### Scenario: Non-stored hosted tool history follows tool-specific behavior
- **WHEN** assistant provider-executed tool history is continued with `store` disabled
- **THEN** hosted items are omitted with the upstream warning unless that tool taxonomy supports stateless reconstruction
- **AND** execution-denied synthetic results are omitted

### Requirement: Function result output conversion

Ordinary results SHALL use function_call_output with original call_id and existing result caller. Scalar text/error-text/JSON/error-JSON/execution-denied SHALL retain string and output-schema JSON encoding rules. Scalar output SHALL remain string unless a selected active OpenAI/Azure breakpoint wraps it in input_text with prompt_cache_breakpoint.

#### Scenario: Scalar results retain cache precedence and schema encoding
- **WHEN** a function output is text, JSON, error-text, error-JSON, or execution-denied and both its output and tool-result part carry distinct active-namespace cache breakpoints
- **THEN** its `function_call_output.output` is an `input_text` array carrying the output-level breakpoint and the correctly encoded scalar text
- **AND** without an output-level breakpoint the result-part breakpoint applies, and without either breakpoint output remains a string
- **AND** a function tool with an output schema quotes text/error-text/denial as JSON string literals, while JSON/error-JSON remains JSON-serialized and denial without a reason uses the default reason

#### Scenario: Generic multipart text and file content
- **WHEN** a generic function result contains mixed text, image data/URL, and non-image file data/URL content elements with per-element cache options
- **THEN** output contains ordered `input_text` text, `input_image` data URI or `image_url` (including optional image detail), and `input_file` `filename` plus base64 `file_data` or `file_url` entries
- **AND** inline non-image data without a filename uses `data`
- **AND** each surviving element carries only its own active-namespace cache breakpoint, even when the result/output also has a scalar breakpoint

#### Scenario: Generic uploaded references resolve by active namespace
- **WHEN** a generic function result contains image and non-image file references with an entry for the active OpenAI or Azure provider namespace
- **THEN** the corresponding typed `input_image` and `input_file` elements carry that entry as `file_id` and preserve image detail and element cache breakpoint
- **AND** a reference missing the active provider entry fails conversion instead of falling back to another namespace or silently dropping the file

#### Scenario: Unsupported generic content is dropped with warning
- **WHEN** a generic multipart function result contains an unsupported content type or file data variant
- **THEN** each unsupported element is omitted with an `other` warning naming `unsupported tool content part type: <type>` or `unsupported tool content part type: file with data type: <type>` respectively
- **AND** supported elements remain in their original order under the same `call_id`

### Requirement: Custom tool result output conversion

Configured custom results SHALL retain custom_tool_call_output and original call_id. Scalar text/error-text/JSON/error-JSON/execution-denied SHALL stay strings unless a selected active-namespace breakpoint (output before result part) makes an input_text array. Multipart text, inline image/file data and image/file URLs SHALL retain typed content, image detail and per-content cache options.

#### Scenario: Custom scalar breakpoint and output identity
- **WHEN** a custom tool result has a scalar output with output-level or result-part active-namespace cache breakpoint
- **THEN** `custom_tool_call_output` retains its `call_id` and wraps the scalar string in one `input_text` with output-level precedence
- **AND** without either breakpoint scalar output remains a string, without function output-schema quoting

#### Scenario: Custom multipart reference warns and drops
- **WHEN** custom multipart output contains a supported text/image/file data or URL element alongside an uploaded reference
- **THEN** supported elements remain typed and ordered with their individual cache breakpoints
- **AND** the uploaded reference is absent and an `other` warning has message `unsupported custom tool content part type: file with data type: reference`
- **AND** no custom reference is represented as `file_id`

### Requirement: Parallel wrapper function result serialization

Grouped parallel function results SHALL retain wrapper call_id, child index order and continuation behavior. Each child SHALL undergo ordinary result conversion: scalar strings stay strings, multipart typed arrays become JSON strings; wrapper SHALL join strings with newlines. Unsupported multipart items SHALL warn without reordering. Hosted-tool and invalid/incomplete group dispatch SHALL remain unchanged.

#### Scenario: Ordered multipart and scalar children
- **WHEN** a grouped parallel wrapper receives child results out of index order, including generic multipart output with an uploaded reference and a scalar child
- **THEN** the wrapper emits one `function_call_output` with the wrapper `call_id` and newline-joined child outputs in original child index order
- **AND** the multipart child output is JSON-serialized typed content with its active-namespace `file_id` and per-content cache hints rather than discarded or emitted as separate native blocks

#### Scenario: Scalar child breakpoints preserve their positions
- **WHEN** one or more grouped parallel children have selected scalar output/result-part cache breakpoints
- **THEN** the wrapper output is an ordered array of `input_text` child strings with later children newline-prefixed and each selected breakpoint on its corresponding element
- **AND** multipart child result/output-level cache options do not cause wrapper-level breakpoints

### Requirement: Reasoning conversion

Assistant reasoning SHALL become reasoning items with encrypted_content and summary, or item_reference when store=true with an ID. Active conversation/previousResponseId SHALL skip reasoning with IDs. Missing IDs SHALL fall back to encrypted_content; store=false reasoning without encrypted content SHALL be filtered with a warning.

#### Scenario: Stored reasoning becomes item reference
- **WHEN** `store` is true and a reasoning part carries an item id
- **THEN** the request emits a single `item_reference` for that reasoning id

#### Scenario: Non-stored reasoning without encrypted content is dropped
- **WHEN** `store` is false and a reasoning part has no encrypted content
- **THEN** the reasoning item is omitted from the request and an `unsupported` warning is emitted

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

### Requirement: GPT-6 Responses capability and reasoning-effort validation

Anchored gpt-6 and later SHALL support configuration updates and async calling. Request effort SHALL resolve via existing provider-option/core precedence; only low/medium/high/xhigh/max SHALL be sent, others omitted with unsupported reasoningEffort warning listing supported values. Earlier/non-GPT/unknown models SHALL retain effort behavior; forced reasoning alone SHALL NOT enable GPT-6 capabilities.

#### Scenario: Supported GPT-6 effort from provider option
- **WHEN** a `gpt-6-astra` generate or stream request sets `reasoningEffort` to `max`
- **THEN** the request uses reasoning effort `max` without a capability warning

#### Scenario: Unsupported GPT-6 effort from provider option or core fallback
- **WHEN** a GPT-6 or later generate or stream request resolves effort `none` or `minimal` from either the OpenAI provider option or the existing core reasoning setting
- **THEN** the request omits reasoning effort and emits an `unsupported` `reasoningEffort` warning naming `low, medium, high, xhigh, max`
- **AND** it does not mutate the supplied options

#### Scenario: GPT-6 omits unsupported logprobs
- **WHEN** a GPT-6 or later reasoning request asks for top logprobs or includes `message.output_text.logprobs`
- **THEN** both settings are omitted and an `unsupported` `logprobs` warning is emitted

#### Scenario: GPT-6 omits legacy prompt-cache retention
- **WHEN** a GPT-6 or later request specifies `promptCacheRetention`
- **THEN** the field is omitted with an `unsupported` warning directing the caller to `promptCacheOptions`
- **AND** pre-GPT-6 and Mantle-prefixed models retain their existing retention behavior

#### Scenario: Conservative model identity
- **WHEN** a GPT-5.6, `gpt-6chat`, custom, or other nonmatching model ID receives an existing valid reasoning effort
- **THEN** this new GPT-6 effort-list check does not change that model's previous effort behavior

### Requirement: OpenAI reasoning configuration update and explicit compaction trigger

The provider SHALL support reasoningEffortUpdate restricted to low/medium/high/xhigh/max and optional compactionTrigger. Eligible GPT-6+ standard-mode updates without automatic context management/truncation SHALL prepend {"type":"configuration_update","reasoning":{"effort":<update>}} before all converted prompt items in the current request. Neither control SHALL mutate caller messages, change request effort, persist as conversation content or alter defaults.

#### Scenario: Independent update with previous response
- **WHEN** a `gpt-6-astra` request has `reasoningEffort: low`, `reasoningEffortUpdate: high`, `previousResponseId`, and a user prompt
- **THEN** input begins with the configuration update and then the converted user prompt, the top-level reasoning effort remains `low`, and `previous_response_id` is retained without warning

#### Scenario: Unsupported configuration update
- **WHEN** a GPT-5.6 request specifies `reasoningEffortUpdate: high`
- **THEN** no configuration update is sent and an `unsupported` `reasoningEffortUpdate` warning is returned

#### Scenario: Incompatible update combinations
- **WHEN** a GPT-6 request specifies `reasoningEffortUpdate` together with `reasoningMode: pro`, a present automatic `contextManagement` value, or `truncation: auto`
- **THEN** no configuration update is sent and an `unsupported` `reasoningEffortUpdate` warning explains the conflict

#### Scenario: Trigger follows stateless history and current prompt
- **WHEN** a generate or stream request contains compaction history, a user turn, and `compactionTrigger: true`, optionally with `reasoningEffortUpdate`
- **THEN** the trigger is the final Responses input item after all converted prompt items; any eligible update is first
- **AND** the input history and reused call options remain unchanged

#### Scenario: Disabled trigger retains existing request
- **WHEN** `compactionTrigger` is absent or false
- **THEN** the request contains no compaction-trigger item, including on pre-GPT-6 models

### Requirement: Capability-gated async tools and round-trip metadata

Function-tool and namespaced-function options and openai.custom args SHALL accept tri-state async. True SHALL be included only with async capability, else omitted with unsupported warning naming the tool. False SHALL be retained on any model; absence SHALL omit it. Generate/stream calls SHALL carry present async (including false) in resolved OpenAI/Azure ProviderMetadata.

#### Scenario: Supported and unsupported function tools
- **WHEN** GPT-6 and GPT-5.6 each receive a function tool with `async: true`
- **THEN** only GPT-6 sends `async: true` and GPT-5.6 omits it with a tool-named `unsupported` warning

#### Scenario: Namespaced and custom tools
- **WHEN** namespaced function and `openai.custom` tools provide `async: true` on GPT-6 and on GPT-5.6
- **THEN** GPT-6 sends each async field on the appropriate nested or custom tool and GPT-5.6 omits each unsupported true with a tool-named warning

#### Scenario: False is distinct from absent
- **WHEN** a tool explicitly provides `async: false` on a non-GPT-6 model
- **THEN** `async: false` is sent without a capability warning; a tool without the setting omits it

#### Scenario: Invalid custom async setting fails locally
- **WHEN** an `openai.custom` tool provides a present `async` value that is not a JSON boolean
- **THEN** request preparation fails before sending an HTTP request

#### Scenario: Generate and stream metadata survives continuation
- **WHEN** non-streaming output or streaming added/done events describe a function or custom-tool call with an explicitly present `async` value and the call is continued with `store: false`
- **THEN** the emitted call and reconstructed next-request call retain the same namespaced async value and any existing item ID, namespace, and caller metadata
- **AND** an absent value stays absent; streamed done metadata wins over added metadata, or the added value is retained if done omits it

#### Scenario: Stored custom call remains a reference, function call remains inline
- **WHEN** stored async custom and ordinary function calls each have OpenAI item IDs and are continued
- **THEN** the custom call uses the existing stored-reference behavior without an extra reconstructed call
- **AND** the ordinary function call remains inline so its `function_call_output` can be matched by `call_id`

### Requirement: Programmatic denial remains a local unsupported continuation

Before HTTP, the provider SHALL reject execution-denied programmatic function results identified by result caller.type:program or a preceding assistant call with matching toolCallId and that caller. Direct denied results SHALL keep conversion; synthetic denied provider-executed assistant results SHALL stay omitted. Generate/stream orchestration SHALL preserve approval/denial semantics and sufficient caller metadata; no new async executor is introduced.

#### Scenario: Programmatic denial identified from assistant call
- **WHEN** a generate or stream continuation includes a programmatic assistant function call and a following `execution-denied` tool result with matching call ID but no result caller metadata
- **THEN** request preparation fails with an unsupported-functionality error and sends no HTTP request

#### Scenario: Programmatic denial identified from result
- **WHEN** the denied result carries OpenAI `caller.type: program` metadata
- **THEN** request preparation fails before an HTTP request even if the corresponding assistant call is not in the prompt

#### Scenario: Direct denied result remains supported
- **WHEN** a direct function-tool call has a denied result
- **THEN** continuation retains the existing `function_call_output` behavior and no programmatic-denial error occurs

#### Scenario: Provider-executed synthetic denial stays omitted
- **WHEN** a denied provider-executed result appears in assistant message history
- **THEN** no synthetic result item is sent as OpenAI conversation history

### Requirement: Model capability gating

Capabilities SHALL use upstream getOpenAILanguageModelCapabilities prefix matching: isReasoningModel, systemMessageMode, supportsFlexProcessing, supportsPriorityProcessing, supportsNonReasoningParameters. Reasoning models SHALL strip temperature/topP with unsupported warnings unless effort=none and non-reasoning parameters are supported. Non-reasoning models SHALL warn on reasoningEffort/reasoningSummary. Unsupported serviceTier flex/priority SHALL be stripped with warning.

#### Scenario: Temperature stripped on reasoning model
- **WHEN** the model is a reasoning model and `temperature` is set
- **THEN** the request omits `temperature` and emits an `unsupported` warning

#### Scenario: Flex tier unsupported
- **WHEN** `serviceTier == flex` on a model that does not support flex processing
- **THEN** the request omits `service_tier` and emits an `unsupported` warning

### Requirement: Provider options

The provider SHALL parse typed openai options with provider.ResolveOption[OpenAIResponsesOptions] and apply configured request controls. conversation plus previousResponseId SHALL emit an unsupported warning.

#### Scenario: previousResponseId continuation
- **WHEN** the `openai` provider option `previousResponseId` is set
- **THEN** the request includes `previous_response_id` with that value

#### Scenario: Conversation and previousResponseId conflict
- **WHEN** both `conversation` and `previousResponseId` are set
- **THEN** an `unsupported` warning is emitted

#### Scenario: Logprobs auto-include
- **WHEN** the `logprobs` option is set to a positive number
- **THEN** the request `include` contains `message.output_text.logprobs` and `top_logprobs` is set

### Requirement: Tool preparation and tool choice

The provider SHALL prepare function declarations with normalized input/optional output schemas and supported provider IDs as Responses objects. Unknown declarations SHALL warn unsupported, not error. auto/none/required choice SHALL pass through as strings; tool choice SHALL use typed/object shape; present allowedTools SHALL override as allowed_tools when declarations exist.

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

#### Scenario: Ordinary forced shell choice
- **WHEN** an `openai.shell` provider tool is declared as `shell` or configured with alias `terminal` and ordinary `toolChoice` selects its canonical name or configured alias without `allowedTools`
- **THEN** both unary and streaming requests contain exactly `{"type":"function","name":"shell"}` as `tool_choice`
- **AND** the tool declaration remains a `shell` declaration

#### Scenario: Ordinary forced local-shell choice
- **WHEN** an `openai.local_shell` provider tool is declared as `local_shell` or configured with alias `localTerminal` and ordinary `toolChoice` selects its canonical name or configured alias without `allowedTools`
- **THEN** both unary and streaming requests contain exactly `{"type":"function","name":"local_shell"}` as `tool_choice`
- **AND** the tool declaration remains a `local_shell` declaration

#### Scenario: Ordinary forced tool-search choice
- **WHEN** an `openai.tool_search` provider tool is declared as `tool_search` or configured with alias `discover` and ordinary `toolChoice` selects its canonical name or configured alias without `allowedTools`
- **THEN** both unary and streaming requests contain exactly `{"type":"function","name":"tool_search"}` as `tool_choice`
- **AND** the tool declaration remains a `tool_search` declaration

#### Scenario: Ordinary forced hosted controls
- **WHEN** ordinary `toolChoice` selects a canonical name or configured alias for any of `code_interpreter`, `file_search`, `image_generation`, `web_search_preview`, `web_search`, `mcp`, `apply_patch`, `computer`, or `programmatic_tool_calling` without `allowedTools`
- **THEN** `tool_choice` is exactly `{type: <canonical name>}` without a function or custom `name` field

#### Scenario: Ordinary custom and function controls
- **WHEN** ordinary `toolChoice` selects a custom provider tool named `freeform` without `allowedTools`
- **THEN** `tool_choice` is exactly `{"type":"custom","name":"freeform"}`
- **AND** an ordinary function selection `getWeather` remains exactly `{"type":"function","name":"getWeather"}`
- **AND** an unmapped selection remains a function choice with the selected name unchanged

#### Scenario: Allowed shell choices remain declaration-aware
- **WHEN** `allowedTools` selects a declared `shell` or `local_shell` canonical name or configured alias and ordinary `toolChoice` is also supplied
- **THEN** the overriding `allowed_tools` choice still uses `{type: "shell"}` or `{type: "local_shell"}`, not the ordinary function fallback
- **AND** selecting a declared `tool_search` alongside a selectable function still warns and drops `tool_search`, preserving the function

### Requirement: Non-streaming response conversion

DoGenerate SHALL map Responses output to provider content: message text/annotation sources, reasoning summaries, function/custom calls, and built-in calls/results. Conversion SHALL map usage/finish reason, provider metadata (responseId, logprobs, serviceTier) and warnings. Shell/local-shell inputs SHALL preserve optional execution constraints, translating snake_case to camelCase for equivalent later-turn reconstruction.

#### Scenario: Generated output logprobs metadata
- **WHEN** logprobs are requested and output-text content returns token logprobs with top alternatives
- **THEN** `ProviderMetadata["openai"].logprobs` contains the content-part arrays in response order
- **AND** each token and top alternative preserves its `token` and `logprob` fields in order

#### Scenario: Generated empty logprobs array is retained
- **WHEN** logprobs are requested and output-text content returns an empty logprobs array
- **THEN** `ProviderMetadata["openai"].logprobs` contains an empty array entry for that content part

#### Scenario: Generated logprobs metadata is omitted
- **WHEN** logprobs were not requested or every output-text logprobs field is null or missing
- **THEN** `ProviderMetadata["openai"]` does not contain a `logprobs` field

#### Scenario: Text and url citation
- **WHEN** the response contains a `message` item with text and a `url_citation` annotation
- **THEN** the result contains a text content part and a `source` content part of type `url` whose `Title` is the annotation title and whose `Text` is empty

#### Scenario: Document annotations carry display titles
- **WHEN** the response contains `file_citation`, `container_file_citation` or `file_path` annotations
- **THEN** each `source` content part is of type `document` with `Title` and `Filename` taken from the annotation filename, or from the file id for `file_path`, alongside its media type and provider metadata
- **AND** no `source` content part carries its title in the `Text` field

#### Scenario: Provider-executed web search
- **WHEN** the response contains a `web_search_call` item
- **THEN** the result contains a tool-call and a tool-result content part with `ProviderExecuted` set

#### Scenario: MCP approval request
- **WHEN** the response contains an `mcp_approval_request` item
- **THEN** the result contains a provider-executed dynamic tool-call and a `tool-approval-request` content part

#### Scenario: Shell execution options survive response conversion
- **WHEN** a shell or local-shell response call contains timeout, output-length, user, working-directory, or environment fields
- **THEN** the generated tool-call input preserves every present field in provider content naming
- **AND** converting that content on a later turn produces equivalent Responses request fields

#### Scenario: Finish reason with function call
- **WHEN** the response has no incomplete reason but contains a function call
- **THEN** the unified finish reason is `tool-calls`

#### Scenario: Client-executed computer call
- **WHEN** a `computer_call` contains `call_id`
- **THEN** the result contains a client-executed `computer` tool call keyed by `call_id`, with mapped actions, safety checks, status, and the response item id in provider metadata

#### Scenario: Legacy computer call
- **WHEN** a `computer_call` does not contain `call_id`
- **THEN** the result preserves the provider-executed `computer_use` call and status result

#### Scenario: Missing Responses output
- **WHEN** a successful non-streaming response omits or nulls `output`
- **THEN** the provider returns a non-retryable status-500 `APICallError` naming any incomplete reason

### Requirement: Computer call history conversion

The provider SHALL round-trip client-executed computer calls using stored item references when enabled, expanded `computer_call` items when storage is disabled, and `computer_call_output` items for screenshot results and acknowledged safety checks.

#### Scenario: Stored computer call
- **WHEN** a computer tool call has an OpenAI item id and storage is enabled
- **THEN** assistant history uses an `item_reference` while its result uses `computer_call_output` keyed by `call_id`

#### Scenario: Stateless computer screenshot
- **WHEN** storage is disabled and a computer result contains a screenshot URL or file id
- **THEN** the request includes the expanded actions and a `computer_call_output` preserving screenshot detail and acknowledged safety checks

### Requirement: Streaming response conversion

DoStream SHALL consume the Responses SSE stream statefully and emit provider.StreamParts with upstream ordering. Unknown events SHALL NOT error. Message lifecycle SHALL carry phase and annotations; function input SHALL finish before tool-call emission, and provider-executed tool lifecycle parts SHALL carry ProviderExecuted.

#### Scenario: Stream finish with logprobs
- **WHEN** logprobs are requested and output-text delta events return token logprobs with top alternatives
- **THEN** the final `PartFinish` contains `ProviderMetadata["openai"].logprobs`
- **AND** its outer arrays preserve delta order while each array preserves token and top-alternative order
- **AND** entries contain `token`, `logprob`, and `top_logprobs` fields without provider-only byte arrays

#### Scenario: Stream empty logprobs array is retained
- **WHEN** logprobs are requested and an output-text delta returns an empty logprobs array
- **THEN** the final `PartFinish` logprobs metadata contains an empty array entry for that delta

#### Scenario: Stream logprobs metadata is omitted
- **WHEN** logprobs were not requested or every output-text delta logprobs field is null or missing
- **THEN** the final `PartFinish` provider metadata does not contain a `logprobs` field

#### Scenario: Text streaming lifecycle
- **WHEN** the stream contains message added, two `output_text.delta` events, and message done
- **THEN** the parts are text-start, text-delta, text-delta, text-end in order

#### Scenario: Function call streaming
- **WHEN** the stream contains a function_call `output_item.added`, arguments deltas, and `output_item.done`
- **THEN** the parts are tool-input-start, tool-input-delta(s), tool-input-end, tool-call in order

#### Scenario: Computer call streaming
- **WHEN** a client-executed `computer_call` is added and completed
- **THEN** the parts are tool-input-start, a full mapped tool-input-delta, tool-input-end, and a client-executed tool-call keyed by `call_id`

#### Scenario: Web search streaming emits provider-executed call eagerly
- **WHEN** a `web_search_call` `output_item.added` event arrives
- **THEN** tool-input-start, tool-input-end, and a provider-executed tool-call are emitted before the result

#### Scenario: Unknown event is ignored
- **WHEN** an unrecognized SSE event type arrives
- **THEN** no error part is emitted and the stream continues

#### Scenario: Stream finish with usage
- **WHEN** a `response.completed` event arrives with usage
- **THEN** a finish part is emitted carrying mapped usage and the finish reason

### Requirement: Usage conversion
The provider SHALL convert Responses usage to `provider.Usage`: input tokens
total/noCache/cacheRead derived from `input_tokens` and
`input_tokens_details.cached_tokens`; output tokens total/text/reasoning derived
from `output_tokens` and `output_tokens_details.reasoning_tokens`; with the raw
usage retained.

#### Scenario: Cached and reasoning token split
- **WHEN** usage reports `input_tokens=100`, `cached_tokens=30`, `output_tokens=50`, `reasoning_tokens=20`
- **THEN** input noCache is 70, cacheRead is 30, output text is 30, output reasoning is 20

### Requirement: Error mapping
The provider SHALL translate OpenAI SDK API errors into
`provider.APICallError` carrying message, URL, request body, status code,
response headers, raw body, retryability, and parsed structured error data. In
streaming, errors SHALL be emitted as a `PartError` carrying an
`APICallError`; non-API errors during streaming SHALL also be wrapped as
`APICallError` parts. Construction and conversion SHALL never panic across the
API boundary.

#### Scenario: API error in DoGenerate
- **WHEN** the API returns a 400 error body
- **THEN** `DoGenerate` returns a `provider.APICallError` with status 400 and the parsed error data

#### Scenario: API error during streaming
- **WHEN** the API returns an error status while opening or reading a stream
- **THEN** the stream emits a `PartError` carrying an `APICallError`

### Requirement: Internal parallel function wrappers
The provider SHALL expand an undeclared function named parallel only when its
nonempty tool_uses array contains object parameters and functions-prefixed
recipients that are all declared function tools. Expansion SHALL be atomic and
preserve original wrapper identity and child index/count in provider metadata.
Streaming SHALL buffer wrapper input until it can emit child input lifecycles;
unexpandable wrappers SHALL retain their original input deltas and call identity.

#### Scenario: Valid wrapper expands
- **WHEN** a wrapper references two declared function tools
- **THEN** generate returns two tool calls and stream emits each child's start/delta/end/call sequence
- **AND** IDs are the wrapper call ID suffixed with the zero-based child index

#### Scenario: Declared or invalid wrapper stays a normal call
- **WHEN** parallel is itself a declared function or any nested recipient/parameters are invalid
- **THEN** the original function call is retained without partial child execution

#### Scenario: Wrapper input ends before its final item
- **WHEN** a suppressed parallel wrapper reaches EOF without a valid final item
- **THEN** its original start and buffered deltas are flushed in output-index order before any finish
- **AND** no completed tool call or input-end is invented

#### Scenario: Stateful scalar results are grouped
- **WHEN** every child result has matching wrapper metadata and unique indexes in a conversation or previous-response continuation
- **THEN** one wrapper output contains child outputs in index order
- **AND** conversations omit the existing wrapper call while previous-response chains reconstruct it
- **AND** incomplete or conflicting groups remain ordinary child results

### Requirement: Recoverable malformed Responses stream events

Malformed JSON SSE SHALL emit nonretryable error without discarding later decodable events. Transport/setup SHALL retain preflight retry/error behavior. At most one finish SHALL follow pending-input flush. Malformed-frame error SHALL survive completed/incomplete responses with their usage/metadata. SDK transport/auth SHALL stay in control; each acquired framing decoder SHALL be constructed/closed exactly once.

#### Scenario: Malformed events surround valid output
- **WHEN** malformed JSON occurs before and after valid tool or text events
- **THEN** errors and valid output retain their order through one HTTP request
- **AND** subsequent valid events remain visible
- **AND** the final finish reason is error, including when malformed data follows a completion event

#### Scenario: Custom SDK decoder owns framing resources
- **WHEN** a configured SDK decoder consumes a framing prefix or owns resources
- **THEN** one decoder consumes the stream and that same instance is closed on completion or cancellation

### Requirement: Apply-patch calls contribute tool finish reasons
Client-executed apply-patch calls SHALL contribute to tool-calls finish mapping in
both generate and completed stream calls.

#### Scenario: Completed patch call finishes
- **WHEN** a response contains a completed local apply-patch call
- **THEN** its unified finish reason is tool-calls rather than stop

### Requirement: OpenAI Responses schema normalization

Before DoGenerate/DoStream, the provider SHALL normalize JSON structured-response and function input/optional output schemas, including namespaced functions, regardless of strict setting, without mutating caller data. String-schema propertyNames SHALL be removed with one compatibility warning per affected schema; null SHALL be removed silently; boolean/non-string/non-null propertyNames SHALL error before HTTP.

#### Scenario: Recursive string property names are removed
- **WHEN** a schema contains string-schema `propertyNames` in nested `properties`, `patternProperties`, `additionalProperties`, `additionalItems`, `items`, `contains`, `not`, `allOf`, `anyOf`, `oneOf`, `definitions`, `$defs`, schema-valued `dependencies`, or `if`/`then`/`else`
- **THEN** the request omits each such `propertyNames` and preserves the other schema keywords and values, including boolean subschemas, boolean `additionalProperties`, and string-array `dependencies`
- **AND** exactly one `compatibility` warning is returned for that schema even if multiple nodes have `propertyNames`
- **AND** the caller's schema remains unchanged

#### Scenario: Null optional schemas are absent
- **WHEN** a JSON response format has `schema: null` or a regular or namespaced function tool has `outputSchema: null`
- **THEN** the response uses `json_object` or the function tool omits `output_schema`, respectively, without a schema warning or error on either generate or stream

#### Scenario: Both input and output function schemas need normalization
- **WHEN** a regular or namespaced function tool contains a string `propertyNames` schema in both its input and optional output schemas
- **THEN** the request contains normalized `parameters` and `output_schema` and returns one `compatibility` warning for each affected schema

#### Scenario: Null property names are removed without warning
- **WHEN** a JSON response or function-tool schema contains `propertyNames: null`, including in a nested schema
- **THEN** the request omits that `propertyNames` keyword without a normalization compatibility warning and the caller's schema remains unchanged

#### Scenario: Unsupported property name schemas fail locally
- **WHEN** a JSON response schema or regular or namespaced function input or optional output schema contains a boolean or non-string-schema, non-null `propertyNames` value, including in a nested schema
- **THEN** request preparation returns an error identifying unsupported `propertyNames` and sends no HTTP request

#### Scenario: Function tools default to non-strict requests
- **WHEN** a function tool has no explicit strict setting, including inside a namespace or with programmatic tool options
- **THEN** its Responses request contains `strict: false`; explicit true and false values remain unchanged

#### Scenario: Generate and stream agree with strict disabled
- **WHEN** equivalent response and function-tool schemas are submitted through `DoGenerate` and `DoStream`, including with `strictJsonSchema` false
- **THEN** their requests contain the same normalized schema payloads and compatible warnings, with the requested `strict` setting unchanged
- **AND** neither call changes the caller-owned schema data

#### Scenario: Unaffected schemas remain unchanged
- **WHEN** the response or function schema contains no `propertyNames` keyword, including schemas with boolean subschemas and `additionalProperties: false`
- **THEN** the request preserves the schema without a normalization compatibility warning

### Requirement: OpenAI client and request-option ownership

The API-key constructor SHALL create a standard OpenAI client with the supplied key and default Provider() "openai". The preconfigured-client constructor SHALL preserve provider-owned client configuration. Both SHALL accept WithRequestOptions(...) for model request options and WithProviderName(...) for integrations overriding Provider().

#### Scenario: OpenAI client and request-option ownership

- **WHEN** a configured client is supplied with a custom provider name and model request options
- **THEN** calls SHALL retain configured endpoint/authentication/headers/retries/transport, apply request options and report the overridden identity

### Requirement: OpenAI custom identity namespace resolution

Custom identities SHALL resolve options/metadata namespace once: azure if identity contains "azure", otherwise openai, stable across calls. Azure models SHALL fall back to openai call options if Azure options are absent. Default openai construction SHALL retain per-call OpenAI-first/Azure-fallback resolution. Empty name override SHALL retain default identity and behavior.

#### Scenario: OpenAI custom identity namespace resolution

- **WHEN** a custom Azure identity continues an item without top-level Azure options
- **THEN** Azure item metadata SHALL still resolve under azure; openai call options SHALL apply when Azure options are absent

### Requirement: OpenAI stored tool-call eligibility

With store=true and an item ID, eligible provider-defined/executed calls SHALL use item_reference rather than resend, except previousResponseId skipping. Ordinary client-executed function calls SHALL stay inline even with stored IDs so function_call_output pairs by call_id.

#### Scenario: OpenAI stored tool-call eligibility

- **WHEN** stored ordinary and hosted calls are continued with their outputs
- **THEN** ordinary function_call SHALL stay inline with its paired call_id; eligible hosted items SHALL reference their stored IDs unless previousResponseId requires skipping

### Requirement: OpenAI hosted and client provider history taxonomy

Subsequent assistant provider-executed calls/results SHALL retain Responses taxonomy and identity. Stored hosted items SHALL use their item IDs as references; nonstored hosted items SHALL be omitted or reconstructed per upstream tool-specific behavior. Client provider tools SHALL retain native call/output types with matching call_id. Execution-denied assistant results SHALL be omitted when no corresponding OpenAI item exists.

#### Scenario: OpenAI hosted and client provider history taxonomy

- **WHEN** stateless hosted history and a client shell call/output are supplied
- **THEN** hosted items SHALL follow their taxonomy-specific omission/reconstruction; shell SHALL retain native paired types and call_id, and synthetic unmatched denied assistant results SHALL be omitted

### Requirement: OpenAI scalar result breakpoint precedence

Scalar output-level cache breakpoints SHALL precede tool-result-part breakpoints in the active OpenAI/Azure namespace. Content-array output SHALL instead preserve ordered input_text/input_image/input_file elements and take cache breakpoints from each element, not result/output-level scalar options.

#### Scenario: OpenAI scalar result breakpoint precedence

- **WHEN** multipart output has element cache hints and conflicting result/output scalar hints
- **THEN** typed elements SHALL retain order and only their own active-namespace hints; scalar output SHALL choose output-level before result-part hints

### Requirement: OpenAI custom uploaded-reference boundary

Custom multipart uploaded references SHALL be omitted with warning `unsupported custom tool content part type: file with data type: reference`. The provider SHALL NOT claim or emit custom-output file_id support under the registered baseline.

#### Scenario: OpenAI custom uploaded-reference boundary

- **WHEN** a custom result mixes text and an uploaded file reference
- **THEN** text SHALL survive while the reference is dropped with the specified warning and no file_id

### Requirement: OpenAI parallel child cache-breakpoint placement

Selected scalar child output/result-part breakpoints SHALL produce ordered input_text wrapper elements, newline-prefixed after the first and carrying only the corresponding child breakpoint. Multipart child-level scalar breakpoints SHALL NOT apply; element breakpoints SHALL survive within serialized JSON.

#### Scenario: OpenAI parallel child cache-breakpoint placement

- **WHEN** a parallel group mixes a scalar child with a selected breakpoint and a multipart child with scalar and element hints
- **THEN** wrapper input_text elements SHALL preserve child order/newlines; multipart JSON SHALL retain only its per-element hints

### Requirement: OpenAI GPT-6 unsupported request fields

GPT-6+ reasoning models SHALL omit requested logprobs and message.output_text.logprobs include with unsupported logprobs warning. They SHALL omit legacy promptCacheRetention with unsupported warning directing callers to promptCacheOptions.

#### Scenario: OpenAI GPT-6 unsupported request fields

- **WHEN** GPT-6 requests logprobs, its include and legacy promptCacheRetention
- **THEN** all three SHALL be omitted with their specified warnings rather than sent

### Requirement: OpenAI Mantle capability ownership

Mantle-prefixed variants SHALL retain endpoint-specific overrides. Enabling new GPT-6 capability flags there SHALL require separate endpoint-owner verification, not inference from an OpenAI-like model ID.

#### Scenario: OpenAI Mantle capability ownership

- **WHEN** a Mantle-prefixed model has a GPT-6-like ID without endpoint-owner verification
- **THEN** existing endpoint capability overrides SHALL remain unchanged

### Requirement: OpenAI configuration-update rejection conditions

Unsupported models SHALL omit updates with unsupported reasoningEffortUpdate warning saying only GPT-6+ support it. reasoningMode:pro, present contextManagement (even []), or truncation:auto SHALL omit updates with unsupported warning explaining standard mode/no automatic compaction or truncation.

#### Scenario: OpenAI configuration-update rejection conditions

- **WHEN** GPT-6 supplies reasoningEffortUpdate with an explicitly empty contextManagement list
- **THEN** the update SHALL be omitted with the conflict warning, just as for pro mode or automatic truncation

### Requirement: OpenAI explicit compaction trigger placement

compactionTrigger:true SHALL append {"type":"compaction_trigger"} after all converted prompt items on any model; absent/false SHALL NOT append it. It SHALL NOT mutate caller messages, change request effort, persist as conversation content or alter default requests.

#### Scenario: OpenAI explicit compaction trigger placement

- **WHEN** a pre-GPT-6 request enables compactionTrigger after stateless history and a user prompt
- **THEN** the trigger SHALL be last without altering caller history or request-level effort

### Requirement: OpenAI async streaming metadata precedence

Stream output_item.done async SHALL override output_item.added; when done omits it, explicitly present added async SHALL be retained. Absent values SHALL remain absent.

#### Scenario: OpenAI async streaming metadata precedence

- **WHEN** added carries async:true and done carries async:false, or omits async
- **THEN** call metadata SHALL use false in the former case and retain true in the latter

### Requirement: OpenAI async continuation boundaries

Reconstructed nonstored assistant function/custom calls SHALL retain present async. Stored provider-defined custom calls SHALL retain item-reference behavior; ordinary functions SHALL remain inline for call_id/output pairing.

#### Scenario: OpenAI async continuation boundaries

- **WHEN** stored async custom and ordinary function calls have item IDs
- **THEN** custom SHALL reference its item without extra reconstruction, while ordinary function SHALL stay inline; nonstored reconstruction SHALL retain present async

### Requirement: OpenAI continuation and reasoning option projection

Options SHALL apply previousResponseId, conversation, instructions, reasoningEffort, reasoningSummary, truncation, store, metadata, include, maxToolCalls and parallelToolCalls.

#### Scenario: OpenAI continuation and reasoning option projection

- **WHEN** typed options set previousResponseId:"resp_1", instructions:"Answer briefly" and store:false
- **THEN** the request SHALL retain previous_response_id:"resp_1", instructions:"Answer briefly" and store:false; adding conversation SHALL emit the existing unsupported conflict warning

### Requirement: OpenAI service and output option projection

Options SHALL apply serviceTier, textVerbosity, user, logprobs, strictJsonSchema, systemMessageMode, forceReasoning, allowedTools, promptCacheKey, promptCacheRetention, safetyIdentifier, passThroughUnsupportedFiles and contextManagement.

#### Scenario: OpenAI service and output option projection

- **WHEN** a typed call configures positive logprobs, text verbosity and prompt cache settings
- **THEN** configured values SHALL apply, including automatic logprobs include and top_logprobs, subject to existing model capability gates

### Requirement: OpenAI supported provider-tool declarations

Supported IDs SHALL be openai.web_search, openai.web_search_preview, openai.code_interpreter, openai.file_search, openai.image_generation, openai.local_shell, openai.shell, openai.apply_patch, openai.computer, openai.mcp, openai.tool_search, openai.programmatic_tool_calling and openai.custom, converted to corresponding Responses tool objects.

#### Scenario: OpenAI supported provider-tool declarations

- **WHEN** openai.code_interpreter and openai.file_search are declared with configured names runCode and searchFiles
- **THEN** tools SHALL contain code_interpreter and file_search declarations with their corresponding bidirectional name mappings retained

### Requirement: OpenAI ordinary forced-choice classification

Without allowedTools, ordinary forced choices SHALL resolve configured aliases to canonical names. Hosted type-only choices SHALL be exactly code_interpreter, file_search, image_generation, web_search_preview, web_search, mcp, apply_patch, computer, programmatic_tool_calling. Named custom tools SHALL use {type:"custom",name:<custom name>}; others SHALL use {type:"function",name:<resolved name>}, including shell/local_shell/tool_search; unmapped spelling SHALL remain.

#### Scenario: OpenAI ordinary forced-choice classification

- **WHEN** ordinary choice selects a renamed shell provider tool or a hosted web-search alias
- **THEN** shell SHALL use the function shape with canonical shell name; search SHALL use only canonical hosted type

### Requirement: OpenAI ordinary choice classification boundary

Ordinary forced-choice classification SHALL NOT remove supported declarations/bidirectional mappings, alter separate allowedTools resolution or introduce name validation.

#### Scenario: OpenAI ordinary choice classification boundary

- **WHEN** ordinary forced shell choice is used instead of allowedTools
- **THEN** the shell declaration and mappings SHALL remain supported; a later allowedTools selection SHALL still use the declaration-aware shell shape

### Requirement: OpenAI automatic web-source include policy

With a web-search tool, automatic web_search_call.action.sources SHALL default on for ordinary OpenAI models, but SHALL be suppressed by model capability false or includeWebSearchSources:false. Per-call true SHALL NOT override disabled capability. No web-search tool SHALL mean no automatic source include.

#### Scenario: OpenAI automatic web-source include policy

- **WHEN** a web tool is supplied with per-call true on a source-include-disabled model
- **THEN** the tool SHALL remain while automatic web sources SHALL be omitted

### Requirement: OpenAI include ownership and independence

Caller include values SHALL remain even when automatic web sources are disabled. Unrelated automatic includes SHALL remain independent.

#### Scenario: OpenAI include ownership and independence

- **WHEN** caller explicitly includes web_search_call.action.sources with automatic sources disabled alongside code-interpreter, logprobs or encrypted-reasoning includes
- **THEN** explicit web sources SHALL survive and unrelated includes SHALL retain their own rules

### Requirement: OpenAI allowed-tools declaration resolution

With nonempty declarations and present allowedTools, each name SHALL resolve against emitted declarations, direct name before canonical alias, retaining source order/duplicates. Function/custom/MCP/supported hosted selections SHALL use their Responses allowed_tools shapes. Direct-name/alias collisions SHALL warn unsupported and select the direct name.

#### Scenario: OpenAI allowed-tools declaration resolution

- **WHEN** a function named web_search collides with a web-search canonical alias and is selected twice
- **THEN** both positions SHALL select the direct function and a shadowing warning SHALL be emitted

### Requirement: OpenAI unselectable and unknown allowed-tools names

Ambiguous aliases or unallowlistable known selections (namespace-contained/deferred functions and unsupported hosted kinds including tool_search) SHALL warn and drop. Unknown names SHALL warn but be sent as mapped functions. Preparation SHALL fail before transport only if no entries remain (including empty selection), identifying dropped names in source order.

#### Scenario: OpenAI unselectable and unknown allowed-tools names

- **WHEN** allowedTools mixes ambiguous mcp, deferred function, tool_search and an unknown name
- **THEN** the first three SHALL warn/drop in source order; the unknown SHALL warn and survive as a mapped function, avoiding empty-selection failure

### Requirement: OpenAI allowed-tools presence defaults and ownership

Without declarations, tools/choice SHALL be omitted even if allowedTools/toolChoice is supplied. Non-nil allowedTools SHALL override ordinary choice even with empty toolNames. Mode SHALL default auto; supported domain SHALL be auto|required without extra runtime mode check in preparation. Resolution SHALL NOT mutate caller tools, option slices or mappings.

#### Scenario: OpenAI allowed-tools presence defaults and ownership

- **WHEN** declarations exist but non-nil allowedTools has empty toolNames and an ordinary choice
- **THEN** allowedTools SHALL override and preparation SHALL fail locally for no allowed entries, leaving all caller-owned inputs unchanged

### Requirement: OpenAI generated built-in output taxonomy

Generated provider-executed built-ins SHALL include web_search_call, file_search_call, code_interpreter_call, image_generation_call, local_shell_call, shell_call+shell_call_output, apply_patch_call, tool_search_call+tool_search_output, computer_call, mcp_call, mcp_approval_request and compaction.

#### Scenario: OpenAI generated built-in output taxonomy

- **WHEN** the response contains a web_search_call with ID search_1
- **THEN** conversion SHALL emit the corresponding provider-executed tool call and tool result linked to search_1 rather than an ordinary client function call

### Requirement: OpenAI annotation source title field

Annotation-derived sources SHALL use canonical Title for URL citation title and document filename (or file ID without filename). Legacy Text SHALL NOT carry source titles.

#### Scenario: OpenAI annotation source title field

- **WHEN** message annotations include a URL citation and a document without a filename
- **THEN** sources SHALL place URL title and document file ID in Title, leaving Text empty

### Requirement: OpenAI generated logprobs metadata normalization

Requested non-null output-text logprobs arrays, even [], SHALL each add an outer entry to ProviderMetadata["openai"].logprobs in response order. Tokens SHALL preserve order and token/logprob/top_logprobs alternatives (token/logprob), without provider byte arrays. Null/missing SHALL add no entry; unrequested logprobs SHALL add no field.

#### Scenario: OpenAI generated logprobs metadata normalization

- **WHEN** requested logprobs contain token alternatives in one content part, [] in another and null in a third
- **THEN** metadata SHALL retain ordered normalized token and empty arrays, omit null and byte arrays, and be absent if unrequested

### Requirement: OpenAI streaming logprobs normalization

Requested non-null response.output_text.delta.logprobs arrays, including [], SHALL accumulate in event order on final PartFinish at ProviderMetadata["openai"].logprobs with unary-normalized token/top-alternative shape. Null/missing SHALL add no outer entry; unrequested stream logprobs SHALL add no field.

#### Scenario: OpenAI streaming logprobs normalization

- **WHEN** requested stream deltas have token alternatives, an empty array and a null logprobs value
- **THEN** final finish SHALL retain token and empty arrays in delta order, without byte fields or null entries

### Requirement: OpenAI property-name normalization warning contract

Each affected schema SHALL emit exactly one compatibility warning with feature `JSON Schema propertyNames` and details `OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints.`

#### Scenario: OpenAI property-name normalization warning contract

- **WHEN** one schema has multiple nested string-schema propertyNames keywords
- **THEN** all SHALL be removed but only one warning SHALL be emitted with the exact feature and details

### Requirement: OpenAI stream response envelope lifecycle

`response.created` SHALL emit stream start and response metadata. `response.completed`, `response.incomplete` and `response.failed` SHALL emit finish with usage and mapped finish reason; `error` SHALL emit an error part.

#### Scenario: OpenAI stream response envelope lifecycle

- **WHEN** a stream emits response.created, content events and then response.incomplete with usage
- **THEN** start and response metadata SHALL precede content, followed by finish with that usage and finish reason

### Requirement: OpenAI stream content event mapping

Message `output_item.added`/`output_text.delta`/`output_item.done` SHALL emit text start/delta/end with phase/annotations. `function_call_arguments.delta`/`output_item.done` SHALL emit tool-input start/delta/end then tool-call. Reasoning summary events SHALL emit reasoning start/delta/end. Provider-executed tools SHALL emit their lifecycle parts with ProviderExecuted.

#### Scenario: OpenAI stream content event mapping

- **WHEN** message added, output_text.delta and message done events arrive with phase and annotations
- **THEN** text-start, text-delta and text-end SHALL appear in order carrying phase and annotations

### Requirement: Tool-call input serialization matches upstream

Tool-call input strings that the OpenAI Responses adapter builds from structured data SHALL be compact JSON without HTML escaping of `<`, `>` and `&`, with fields in the order upstream serializes them.

#### Scenario: Code containing comparison operators
- **WHEN** a code interpreter call's code contains `<` or `>`
- **THEN** the tool-call input string SHALL contain those characters unescaped

#### Scenario: Apply-patch input field order
- **WHEN** an apply-patch call completes
- **THEN** its input SHALL be `{"callId":...,"operation":{"type":...,"path":...,"diff":...}}`, with no `diff` for a delete operation

#### Scenario: Local shell action field order
- **WHEN** a local shell call completes
- **THEN** its action SHALL serialize `type`, `command`, then any of `env`, `timeoutMs`, `user` and `workingDirectory`

### Requirement: Streaming finish reports the created response ID

The streaming finish part SHALL report the response ID from `response.created` as `responseId` provider metadata, even when a later event carries a different ID.

#### Scenario: Rotated response IDs
- **WHEN** `response.created` carries ID `resp_a` and `response.completed` carries ID `resp_b`
- **THEN** the finish part's `responseId` SHALL be `resp_a`
