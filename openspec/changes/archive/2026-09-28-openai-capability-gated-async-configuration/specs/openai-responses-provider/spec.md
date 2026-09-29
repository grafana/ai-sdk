## ADDED Requirements

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

## MODIFIED Requirements

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
