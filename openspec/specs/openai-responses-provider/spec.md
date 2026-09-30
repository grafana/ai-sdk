# openai-responses-provider Specification

## Purpose
Define the OpenAI Responses API provider module, including request conversion,
tool preparation, response and stream mapping, provider options, error handling,
and conformance expectations needed to stay aligned with Vercel's upstream AI
SDK behavior.
## Requirements
### Requirement: Provider construction and identity
The system SHALL provide a `providers/openai` Go module exposing both
`NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel`
and
`NewResponsesWithClient(client openai.Client, modelID string, opts ...Option) provider.LanguageModel`.
Each constructor SHALL return a value implementing `provider.LanguageModel`
backed by the OpenAI Responses API via `github.com/openai/openai-go`. The model
SHALL report `SpecificationVersion() == "v4"` and `ModelID()` equal to the
constructor `modelID`. The API-key constructor SHALL create a standard OpenAI
client configured with the supplied key and report `Provider() == "openai"` by
default. The preconfigured-client constructor SHALL preserve provider-owned
client configuration. Both constructors SHALL accept functional options,
including `WithRequestOptions(...)` for model request options and
`WithProviderName(...)` for provider integrations to override the identity
reported by `Provider()`. For custom identities, the provider-options and
metadata namespace SHALL be resolved once (`"azure"` when the identity contains
`"azure"`, otherwise `"openai"`) and SHALL remain stable across calls. Azure
models SHALL fall back to `"openai"` call options when no Azure options are
present. The existing constructor with the default `"openai"` identity SHALL
retain its per-call OpenAI-first, Azure-fallback option resolution. An empty
provider-name override SHALL preserve that default identity and behavior.
Construction SHALL NOT panic or perform network calls.

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
The provider SHALL convert user messages to `{ role: "user", content: [...] }`
input items mapping text parts to `input_text`, image parts to `input_image`
(via `image_url`, `file_id`, or data URI; honoring `imageDetail`), and file
parts to `input_file` (via `file_id`, `file_url`, or `filename` + `file_data`).
Reconstructed assistant text SHALL use string content in an easy-input message,
retaining phase but omitting stale item IDs. Stored assistant text SHALL use an
`item_reference` when store is true and an item ID is present. Unsupported
file media types SHALL emit a warning or error matching upstream behavior.

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
The provider SHALL convert assistant tool-call parts to `function_call` items
(or built-in call items such as `local_shell_call`, `shell_call`,
`apply_patch_call`, `tool_search_call`, `custom_tool_call` when the corresponding
tool is present) and tool-role result parts to `function_call_output` (or the
matching built-in output item). Tool-call arguments SHALL serialize `undefined`
input to `"{}"`. When `store` is true and an item id is present, eligible
provider-defined and provider-executed calls SHALL emit an `item_reference`
instead of re-sending the call, except where `previousResponseId` semantics
dictate skipping. Ordinary client-executed function calls SHALL remain inline,
even with a stored item id, so a following `function_call_output` can pair by
`call_id`.

On subsequent turns, assistant provider-executed tool calls and results SHALL
retain the Responses item taxonomy and call/result identity established by the
prior response. Stored hosted items SHALL use their item ids as
`item_reference` entries. Non-stored hosted calls and results SHALL be omitted
or reconstructed according to the upstream tool-specific behavior. Client
provider tools SHALL preserve their native call/output item types and pair them
with the same `call_id`. Tool names SHALL be resolved through the configured
provider-tool name mapping before taxonomy dispatch. Execution-denied assistant
results SHALL be omitted when no corresponding OpenAI item exists.

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
The OpenAI Responses provider SHALL convert ordinary function tool results to `function_call_output` with the original `call_id` and existing result `caller`. Scalar text, error-text, JSON, error-JSON, and execution-denied outputs SHALL retain their current string and output-schema JSON-encoding rules. A scalar output-level cache breakpoint SHALL take precedence over the tool-result part breakpoint, both resolved under the active OpenAI/Azure provider-options namespace; either SHALL wrap the string as an `input_text` array element carrying `prompt_cache_breakpoint`. Without a selected breakpoint, scalar output SHALL remain a string. Content-array output SHALL instead preserve content order as typed `input_text`, `input_image`, and `input_file` elements, with cache breakpoints read from each content element's provider options rather than result/output-level scalar options.

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
The provider SHALL retain `custom_tool_call_output` and the original `call_id` for configured custom provider tools. Scalar text, error-text, JSON, error-JSON, and execution-denied outputs SHALL remain strings unless a cache breakpoint is selected from output-level options before tool-result part options in the active namespace; when selected, the output SHALL be an `input_text` array. Custom multipart text, inline file/image data, and file/image URLs SHALL retain typed content, image detail and per-content cache options. For custom multipart uploaded file references, the provider SHALL warn `unsupported custom tool content part type: file with data type: reference` and omit the reference content; it SHALL NOT claim or emit `file_id` support for custom outputs under the registered upstream baseline.

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
Grouped internal parallel function-tool results SHALL retain their original wrapper `call_id`, child index order, and existing continuation behavior. Each child SHALL use ordinary function-result conversion before its output is serialized: scalar strings remain strings, while multipart typed arrays become JSON-serialized strings. The wrapper SHALL join child strings with newlines. A selected scalar child output/result-part breakpoint SHALL instead produce ordered `input_text` wrapper elements (newline-prefixed after the first), carrying only the corresponding scalar child breakpoint. Multipart child-level scalar breakpoints SHALL NOT apply; multipart content element breakpoints SHALL survive inside the serialized JSON string. Unsupported multipart items SHALL emit warnings without reordering other child output. Existing hosted-tool and invalid/incomplete parallel-group dispatch SHALL remain unchanged.

#### Scenario: Ordered multipart and scalar children
- **WHEN** a grouped parallel wrapper receives child results out of index order, including generic multipart output with an uploaded reference and a scalar child
- **THEN** the wrapper emits one `function_call_output` with the wrapper `call_id` and newline-joined child outputs in original child index order
- **AND** the multipart child output is JSON-serialized typed content with its active-namespace `file_id` and per-content cache hints rather than discarded or emitted as separate native blocks

#### Scenario: Scalar child breakpoints preserve their positions
- **WHEN** one or more grouped parallel children have selected scalar output/result-part cache breakpoints
- **THEN** the wrapper output is an ordered array of `input_text` child strings with later children newline-prefixed and each selected breakpoint on its corresponding element
- **AND** multipart child result/output-level cache options do not cause wrapper-level breakpoints

### Requirement: Reasoning conversion
The provider SHALL convert assistant reasoning parts to `reasoning` input items
carrying `encrypted_content` and `summary` entries, or to an `item_reference`
when `store` is true and a reasoning item id is present. When
`conversation`/`previousResponseId` is active and a reasoning id is present, the
reasoning item SHALL be skipped. Reasoning parts lacking an item id SHALL fall
back to `encrypted_content`, and when `store` is false, reasoning items lacking
encrypted content SHALL be filtered out with a warning.

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
The OpenAI Responses provider SHALL recognize anchored `gpt-6` and later model IDs as supporting configuration updates and async tool calling, with supported request-level reasoning efforts `low`, `medium`, `high`, `xhigh`, `max`. It SHALL resolve the request effort from the existing provider-option/core-setting precedence and omit an effort not in that list with an `unsupported` warning for `reasoningEffort` listing the supported values. Earlier, non-GPT, and unrecognized models SHALL retain their existing request-effort behavior. An explicitly forced reasoning mode SHALL NOT by itself turn on GPT-6 capabilities. GPT-6 and later reasoning models SHALL omit requested logprobs and the `message.output_text.logprobs` include with an `unsupported` `logprobs` warning. They SHALL also omit legacy `promptCacheRetention` with an `unsupported` warning directing callers to `promptCacheOptions`. For the Mantle-prefixed provider variant, existing endpoint-specific overrides SHALL remain intact; enabling the new flags for that endpoint requires separate endpoint-owner verification rather than inference from its OpenAI-like model ID.

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
The OpenAI provider SHALL support `reasoningEffortUpdate` restricted to `low`, `medium`, `high`, `xhigh`, `max` and optional `compactionTrigger`. For GPT-6 and later in standard reasoning mode without configured automatic context management or automatic truncation, a configured update SHALL prepend `{ "type": "configuration_update", "reasoning": { "effort": <update> } }` to the *current request* input before all converted prompt items. On unsupported models it SHALL omit the update with an `unsupported` `reasoningEffortUpdate` warning saying only GPT-6 and later support it; on conflicting `reasoningMode: pro`, present `contextManagement` (including an explicit empty list), or `truncation: auto`, it SHALL omit the update with an `unsupported` warning explaining the standard mode/no automatic compaction or truncation condition. If `compactionTrigger` is true it SHALL append `{ "type": "compaction_trigger" }` after all converted prompt items, regardless of model; absent/false SHALL NOT append it. Neither control SHALL mutate caller-owned messages, change request-level effort, become persisted conversation content, nor alter default requests.

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
The OpenAI provider SHALL accept a tri-state `async` setting on function-tool and namespaced function-tool options, and on `openai.custom` tool arguments. It SHALL include `async: true` only when the model supports async calling; when unsupported it SHALL omit true and emit an `unsupported` warning naming the tool. It SHALL preserve explicit `async: false` on any model and omit the property when absent. Generate and stream SHALL carry present function/custom-tool call async values (including false) under the resolved OpenAI/Azure `ProviderMetadata` namespace; stream `output_item.done` SHALL take precedence, falling back to an explicitly present value from `output_item.added`. On reconstructed, non-stored continuation, the assistant function/custom-tool call SHALL retain present async, and stored item-reference behavior SHALL remain unchanged for provider-defined custom calls; ordinary function calls SHALL remain inline for `call_id`/output pairing.

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
The OpenAI provider SHALL reject an `execution-denied` tool-result continuation for a programmatic function call before sending any request, whether the result itself carries `caller.type: program` or a preceding assistant call with the same `toolCallId` carries that caller. Direct-call denied results SHALL retain their existing conversion; synthetic denied provider-executed results in assistant content SHALL remain omitted. Generate and stream orchestration SHALL preserve existing approval/denial semantics and caller metadata sufficiently for this conversion; this requirement does not introduce a new async executor.

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
The provider SHALL detect model capabilities via prefix matching mirroring
upstream `getOpenAILanguageModelCapabilities` (`isReasoningModel`,
`systemMessageMode`, `supportsFlexProcessing`, `supportsPriorityProcessing`,
`supportsNonReasoningParameters`). For reasoning models it SHALL strip
`temperature` and `topP` (unless reasoning effort is `none` and the model
supports non-reasoning parameters), emitting `unsupported` warnings. For
non-reasoning models it SHALL emit warnings when `reasoningEffort` or
`reasoningSummary` are set. It SHALL strip `serviceTier` `flex`/`priority` when
unsupported, with a warning.

#### Scenario: Temperature stripped on reasoning model
- **WHEN** the model is a reasoning model and `temperature` is set
- **THEN** the request omits `temperature` and emits an `unsupported` warning

#### Scenario: Flex tier unsupported
- **WHEN** `serviceTier == flex` on a model that does not support flex processing
- **THEN** the request omits `service_tier` and emits an `unsupported` warning

### Requirement: Provider options
The provider SHALL parse typed provider options under the `openai` key
(`provider.ResolveOption[OpenAIResponsesOptions]`) and apply them to the request,
including `previousResponseId`, `conversation`, `instructions`, `reasoningEffort`,
`reasoningSummary`, `truncation`, `store`, `metadata`, `include`, `maxToolCalls`,
`parallelToolCalls`, `serviceTier`, `textVerbosity`, `user`, `logprobs`,
`strictJsonSchema`, `systemMessageMode`, `forceReasoning`, `allowedTools`,
`promptCacheKey`, `promptCacheRetention`, `safetyIdentifier`,
`passThroughUnsupportedFiles`, and `contextManagement`. Setting both
`conversation` and `previousResponseId` SHALL emit an `unsupported` warning.

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

### Requirement: Non-streaming response conversion
`DoGenerate` SHALL convert every Responses output item to provider content:
`message` text (with annotations -> `source` parts), `reasoning` summaries,
`function_call` / `custom_tool_call` tool-calls, and provider-executed built-in
calls (`web_search_call`, `file_search_call`, `code_interpreter_call`,
`image_generation_call`, `local_shell_call`, `shell_call` + `shell_call_output`,
`apply_patch_call`, `tool_search_call` + `tool_search_output`, `computer_call`,
`mcp_call`, `mcp_approval_request`, `compaction`). Shell and local-shell call
inputs SHALL preserve optional execution constraints while translating API
snake_case fields to the provider content model's camelCase fields so later
turns can reconstruct equivalent request items. The conversion SHALL map usage
and finish reason, set provider metadata (`responseId`, logprobs,
`serviceTier`), and carry warnings. When logprobs were requested and an output-text
content part returns a non-null logprobs array, including an empty array,
`ProviderMetadata["openai"].logprobs` SHALL contain one outer entry for that
content part in response order. Each entry SHALL preserve token order and contain
`token`, `logprob`, and `top_logprobs` alternatives with `token` and `logprob`,
without provider-only byte arrays. Null or missing arrays SHALL NOT add outer
entries, and unrequested logprobs SHALL NOT add a `logprobs` metadata field.

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
- **THEN** the result contains a text content part and a `source` content part of type `url`

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
`DoStream` SHALL drive a stateful event consumer over the Responses SSE event
stream and emit `provider.StreamPart`s with ordering matching upstream:
`response.created` -> stream start + response metadata; message
`output_item.added`/`output_text.delta`/`output_item.done` -> text start/delta/end
(carrying phase and annotations); `function_call_arguments.delta`/`output_item.done`
-> tool-input start/delta/end + tool-call; reasoning summary events ->
reasoning start/delta/end; provider-executed tools -> their lifecycle parts with
`ProviderExecuted`; `response.completed`/`response.incomplete`/`response.failed`
-> finish with usage and finish reason; `error` -> error part. Unknown events
SHALL NOT error. When logprobs were requested, non-null
`response.output_text.delta.logprobs` arrays, including empty arrays, SHALL be
accumulated in event order and included as `ProviderMetadata["openai"].logprobs`
on the final `PartFinish`, using the same normalized token and top-alternative
shape as non-streaming responses. Null or missing arrays SHALL NOT add outer
entries, and unrequested stream logprobs SHALL NOT add a `logprobs` metadata
field.

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
Malformed JSON SSE data SHALL emit a nonretryable stream error without discarding
subsequent decodable events. Transport/setup failures SHALL retain their existing
preflight retry/error contract. The provider SHALL emit at most one finish, after
flushing pending input. A malformed-frame error SHALL survive later completed or
incomplete responses while retaining their usage and metadata. SDK transport and
authentication SHALL remain in control, and each acquired framing decoder SHALL
be constructed and closed exactly once.

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
Before building an OpenAI Responses request, the provider SHALL normalize each supplied JSON structured-response schema and each function-tool input and optional output schema, including function tools inside namespaces. It SHALL remove `propertyNames` whose schema has `type: "string"` and emit exactly one `compatibility` warning per affected schema with feature `JSON Schema propertyNames` and details `OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints.` It SHALL remove `propertyNames: null` without a compatibility warning and SHALL reject boolean or non-string, non-null `propertyNames` with an error before sending any HTTP request. It SHALL NOT mutate caller-owned schema data. Normalization and rejection SHALL apply whether or not strict output is requested and for both `DoGenerate` and `DoStream`.

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

#### Scenario: Generate and stream agree with strict disabled
- **WHEN** equivalent response and function-tool schemas are submitted through `DoGenerate` and `DoStream`, including with `strictJsonSchema` false
- **THEN** their requests contain the same normalized schema payloads and compatible warnings, with the requested `strict` setting unchanged
- **AND** neither call changes the caller-owned schema data

#### Scenario: Unaffected schemas remain unchanged
- **WHEN** the response or function schema contains no `propertyNames` keyword, including schemas with boolean subschemas and `additionalProperties: false`
- **THEN** the request preserves the schema without a normalization compatibility warning
