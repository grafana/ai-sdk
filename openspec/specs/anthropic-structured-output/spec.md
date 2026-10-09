## Purpose

Define Anthropic structured-output request conversion, transport capability gating, JSON-tool fallback, and response remapping behavior.

## Requirements

### Requirement: Native JSON schema mode

When `CallOptions.ResponseFormat` has `Type: "json"` and a non-nil `Schema`, and both the model and provider transport support native structured output, `buildParams` SHALL set `OutputConfig.Format` to a `BetaJSONOutputFormatParam` with the sanitized schema from `ResponseFormat.Schema`. No tool injection or tool choice modification SHALL occur.

#### Scenario: Native mode on a supported direct Anthropic model

- **WHEN** the direct Anthropic provider calls `buildParams` with `ResponseFormat` having `Type: "json"` and a valid JSON schema, and the model ID is `claude-sonnet-4-5`
- **THEN** the returned `BetaMessageNewParams.OutputConfig.Format` SHALL be set to a `BetaJSONOutputFormatParam` with the sanitized schema
- **AND** no synthetic tool SHALL be appended to the tools list
- **AND** `ToolChoice` SHALL remain unchanged from the caller's setting

#### Scenario: Native mode preserves existing OutputConfig.Effort

- **WHEN** `buildParams` is called with both `ResponseFormat` (json + schema) and provider options that set `effort`
- **THEN** `OutputConfig` SHALL include both `Effort` and `Format` fields

#### Scenario: Native Vertex output without forced tool choice

- **WHEN** the Vertex provider receives a JSON-schema response format on a native-capable model, including `claude-sonnet-5-5`, for generation or streaming
- **THEN** `OutputConfig.Format` SHALL contain the sanitized JSON schema
- **AND** no synthetic tool or forced tool choice SHALL be introduced
- **AND** existing user tools SHALL be preserved
- **AND** the automatic `structured-outputs-2025-11-13` beta SHALL NOT be added

### Requirement: Tool-based JSON fallback

When `CallOptions.ResponseFormat` has `Type: "json"` and a non-nil `Schema`, and either the model or provider transport does not support native structured output, `buildParams` SHALL synthesize a tool named `"json"` with `description: "Respond with a JSON object."` and the `ResponseFormat.Schema` as `inputSchema`. The tool SHALL be appended to the existing tools list (after any user-defined tools).

#### Scenario: Tool injection on an older model

- **WHEN** the direct Anthropic provider calls `buildParams` with `ResponseFormat` having `Type: "json"` and a valid JSON schema, and the model ID is `claude-3-haiku`
- **THEN** the returned `BetaMessageNewParams.Tools` SHALL include the user's tools plus a synthetic tool with name `"json"`, description `"Respond with a JSON object."`, and `InputSchema` matching the provided schema
- **AND** `OutputConfig.Format` SHALL NOT be set

#### Scenario: Tool injection with no existing user tools

- **WHEN** `buildParams` is called with `ResponseFormat` (json + schema), no user tools, and a model that does not support native structured output
- **THEN** the tools list SHALL contain exactly one tool: the synthetic `"json"` tool

### Requirement: Tool choice override in fallback mode

JSON-tool fallback SHALL override caller choice to required (OfAny) with `DisableParallelToolUse:true`. Models rejecting forced tools SHALL instead use auto (OfAuto), parallel-disabled, with one unsupported `toolChoice` warning for required, as in `@ai-sdk/anthropic` 4.0.67. Caller none SHALL send only json; other caller choices SHALL neither filter tools nor add warnings.

#### Scenario: Tool choice forced to required

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `auto`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

#### Scenario: Tool choice override when caller set none

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `none` with caller tools
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`
- **AND** the tools list SHALL contain only the `json` tool

#### Scenario: Tool choice auto on a model that rejects forced tool use
- **WHEN** the tool-based fallback is active for `claude-sonnet-5-5` on a provider transport without native structured output
- **THEN** `ToolChoice` SHALL be `OfAuto` with `DisableParallelToolUse: true`
- **AND** one unsupported warning with feature `toolChoice` SHALL be emitted

#### Scenario: Caller forced choice with the fallback on Sonnet 5.5
- **WHEN** the tool-based fallback is active for `claude-sonnet-5-5` and the caller set `ToolChoice` to `tool` for one of several tools
- **THEN** all caller tools and the `json` tool SHALL be sent
- **AND** exactly one `toolChoice` warning, for `required`, SHALL be emitted

### Requirement: Provider transport capability gating

Direct Anthropic SHALL enable native structured output and strict function tools. Vertex SHALL enable native output but disable strict tools. Each capability SHALL combine with the model structured-output capability and be effective only if both transport and model support it.

#### Scenario: Direct function tools preserve strict and add the beta

- **WHEN** the direct Anthropic provider uses a native-capable model with function tools whose `Strict` values are absent, `false`, and `true`
- **THEN** absent strict SHALL remain omitted and explicit `false` and `true` SHALL be sent unchanged
- **AND** the automatic `structured-outputs-2025-11-13` beta SHALL be added for every case

#### Scenario: Vertex ordinary function tool omits the beta

- **WHEN** the Vertex provider uses a native-capable model with a function tool whose `Strict` value is absent
- **THEN** the strict field SHALL be omitted without a warning
- **AND** the automatic `structured-outputs-2025-11-13` beta SHALL NOT be added

#### Scenario: Vertex drops explicit strict values

- **WHEN** the Vertex provider uses a native-capable model with a function tool whose `Strict` value is explicitly `false` or `true`
- **THEN** the strict field SHALL be omitted
- **AND** an unsupported warning with feature `strict` SHALL be emitted
- **AND** the automatic `structured-outputs-2025-11-13` beta SHALL NOT be added

#### Scenario: Direct native JSON with a function tool adds the beta

- **WHEN** the direct Anthropic provider uses a native-capable model with JSON schema response format and a function tool
- **THEN** native `OutputConfig.Format` SHALL be used
- **AND** the automatic `structured-outputs-2025-11-13` beta SHALL be added

#### Scenario: Provider-defined tools do not trigger the structured-output beta

- **WHEN** the direct Anthropic provider uses a native-capable model with provider-defined tools and no function tools
- **THEN** the automatic `structured-outputs-2025-11-13` beta SHALL NOT be added

#### Scenario: Vertex preserves an explicit structured-output beta

- **WHEN** the Vertex provider uses a function tool or JSON-tool fallback and the caller explicitly includes `structured-outputs-2025-11-13` in `AnthropicOptions.Betas`
- **THEN** the `structured-outputs-2025-11-13` beta SHALL be included in the request even though automatic transport gating would omit it

### Requirement: Stream remapping for JSON response tool

When the json response tool was injected (tool-based fallback), the `streamAdapter` SHALL intercept content blocks for the `"json"` tool and remap them to text content:
- `tool_use` content block start for `"json"` SHALL be suppressed (no `PartToolInputStart` emitted)
- `input_json_delta` deltas for the json tool SHALL be emitted as `PartTextDelta` instead of `PartToolInputDelta`
- `content_block_stop` for the json tool SHALL be suppressed (no `PartToolInputEnd` or `PartToolCall` emitted)

#### Scenario: Tool input streamed as text

- **WHEN** the json response tool is active and the model streams `input_json_delta` events for the `"json"` tool
- **THEN** the stream adapter SHALL emit `PartTextDelta` parts with the delta text
- **AND** SHALL NOT emit `PartToolInputStart`, `PartToolInputDelta`, `PartToolInputEnd`, or `PartToolCall` for the json tool

#### Scenario: Non-json tools unaffected

- **WHEN** the json response tool is active and the model calls a user-defined tool
- **THEN** the stream adapter SHALL emit `PartToolInputStart`, `PartToolInputDelta`, `PartToolInputEnd`, and `PartToolCall` normally for that tool

### Requirement: Finish reason remapping for JSON response tool

When the json response tool was injected and the model's stop reason is `tool_use`, the stream adapter SHALL remap the finish reason to `stop` so the orchestration layer treats it as a completed text response.

#### Scenario: Stop reason remapped from tool_use to stop

- **WHEN** the json response tool is active and the model finishes with `stop_reason: tool_use`
- **THEN** the stream adapter SHALL emit `PartFinish` with `FinishReason: "stop"`

#### Scenario: Stop reason preserved when not tool_use

- **WHEN** the json response tool is active and the model finishes with `stop_reason: end_turn`
- **THEN** the stream adapter SHALL emit `PartFinish` with the original finish reason

### Requirement: JSON response tool state passing

`buildParams` SHALL signal to the caller whether a json response tool was injected. The `streamAdapter` SHALL receive this signal at construction and use it to gate stream remapping logic.

#### Scenario: State passed when tool injected

- **WHEN** `buildParams` injects the json response tool
- **THEN** the returned state SHALL indicate `usesJsonResponseTool: true`
- **AND** the `streamAdapter` SHALL be constructed with this flag

#### Scenario: State not set when native mode used

- **WHEN** `buildParams` uses native `OutputConfig.Format`
- **THEN** the returned state SHALL indicate `usesJsonResponseTool: false`

### Requirement: Schemaless JSON mode unsupported

When `CallOptions.ResponseFormat` has `Type: "json"` but `Schema` is nil, `buildParams` SHALL emit a warning with type `unsupported` (matching `provider.WarnUnsupported`), feature `responseFormat`, and a message indicating that schemaless JSON mode is not supported by Anthropic. No structured output handling SHALL occur.

#### Scenario: Schemaless JSON emits warning

- **WHEN** `buildParams` is called with `ResponseFormat` having `Type: "json"` and nil `Schema`
- **THEN** a warning SHALL be emitted with feature `responseFormat`
- **AND** no `OutputConfig.Format` or synthetic tool SHALL be set

### Requirement: Text response format is a no-op

When `CallOptions.ResponseFormat` has `Type: "text"`, `buildParams` SHALL not modify the request. No warning SHALL be emitted.

#### Scenario: Text format ignored silently

- **WHEN** `buildParams` is called with `ResponseFormat` having `Type: "text"`
- **THEN** no warning SHALL be emitted
- **AND** no `OutputConfig.Format` or synthetic tool SHALL be set

### Requirement: Native structured-output schema sanitization

Native `applyResponseFormat` SHALL write a sanitized copy of `ResponseFormat.Schema` to `OutputConfig.Format`, MUST NOT mutate caller schema, and SHALL NOT sanitize JSON-tool fallback. The sanitizer SHALL preserve `$schema`, `$id`, `title`, `description`, `default`, `const`, `enum`, `type`, and `required`.

#### Scenario: Numeric constraints stripped and summarized

- **WHEN** `applyResponseFormat` runs on a native-capable model with a schema
  whose `properties.recurringIntervalMinutes` is `{type: "number", minimum: 1,
  maximum: 60, exclusiveMinimum: 0, exclusiveMaximum: 120}`
- **THEN** the schema written to `OutputConfig.Format` SHALL omit `minimum`,
  `maximum`, `exclusiveMinimum`, `exclusiveMaximum` on
  `recurringIntervalMinutes`
- **AND** `recurringIntervalMinutes.description` SHALL equal
  `"minimum: 1; maximum: 60; exclusive minimum: 0; exclusive maximum: 120."`
- **AND** the original schema passed in by the caller SHALL be unchanged

#### Scenario: String constraints and unsupported format moved to description

- **WHEN** the schema declares `{type: "string", description: "A URL slug",
  minLength: 1, maxLength: 20, pattern: "^[a-z0-9-]+$", format: "regex"}`
- **THEN** the sanitized node SHALL omit `minLength`, `maxLength`, `pattern`,
  and `format`
- **AND** `description` SHALL equal `"A URL slug\nmin length: 1; max length: 20;
  pattern: ^[a-z0-9-]+$; format: regex."`

#### Scenario: oneOf rewritten as anyOf

- **WHEN** the schema is `{oneOf: [{type: "string", minLength: 1}, {type:
  "number", minimum: 0}]}`
- **THEN** the sanitized schema SHALL contain `anyOf` (not `oneOf`) with each
  branch sanitized

#### Scenario: $ref short-circuits

- **WHEN** a node is `{$ref: "#/$defs/Foo", minLength: 1}`
- **THEN** the sanitized node SHALL be `{$ref: "#/$defs/Foo"}` with all
  sibling keywords dropped

#### Scenario: Object nodes get additionalProperties: false

- **WHEN** the sanitizer visits a node whose `type` is `"object"` or that has
  a non-nil `properties`
- **THEN** the sanitized node SHALL set `additionalProperties` to `false`,
  including when the input did not specify it

#### Scenario: Recursion into definitions, $defs, items, and composition

- **WHEN** the schema contains `$defs.PositiveInteger = {type: "integer",
  minimum: 1}` and a property `tags = {type: "array", minItems: 2, maxItems:
  4, uniqueItems: true, items: {anyOf: [{type: "string", minLength: 1},
  {type: "number", maximum: 10}]}}`
- **THEN** the sanitized schema SHALL recursively strip and summarize
  constraints inside `$defs`, `items`, and each `anyOf` branch using the same
  rules

#### Scenario: Tool-fallback path is not sanitized

- **WHEN** `applyResponseFormat` falls back to injecting the `"json"` tool
  because the model does not support native structured output
- **THEN** the schema set on the synthetic tool's `InputSchema` SHALL be the
  unsanitized schema (matching upstream's behavior)

#### Scenario: Supported format values preserved

- **WHEN** a node declares `{type: "string", format: "email"}`
- **THEN** the sanitized node SHALL retain `format: "email"` and SHALL NOT
  emit a `format: ...` entry in `description`

#### Scenario: Sanitizer is non-mutating

- **WHEN** `applyResponseFormat` runs sanitization on a caller-provided schema
- **THEN** the caller's schema (e.g., as later used by orchestration-layer
  result validation) SHALL be byte-identical to what it was before the call

### Requirement: JSON tool mode on models that reject forced tool use
When `structuredOutputMode` is `jsonTool` and the model rejects forced tool use while the model and provider transport both support native structured output, `buildParams` SHALL use native JSON schema mode instead, as `@ai-sdk/anthropic` 4.0.67 does. It SHALL emit an `unsupported` warning with feature `providerOptions.anthropic.structuredOutputMode` and details `structuredOutputMode 'jsonTool' is not supported by <modelID> because it rejects forced tool use. Using 'outputFormat' instead.`

#### Scenario: jsonTool mode on direct Sonnet 5.5
- **WHEN** the direct Anthropic provider calls `claude-sonnet-5-5` with a JSON schema response and `structuredOutputMode: jsonTool`
- **THEN** `OutputConfig.Format` SHALL contain the sanitized schema and no `json` tool SHALL be added
- **AND** the warning SHALL name `claude-sonnet-5-5`

#### Scenario: jsonTool mode on Vertex Sonnet 5.5
- **WHEN** the Vertex provider calls `claude-sonnet-5-5` with a JSON schema response and `structuredOutputMode: jsonTool`
- **THEN** `OutputConfig.Format` SHALL contain the sanitized schema, no `json` tool SHALL be added, and the same warning SHALL be emitted

#### Scenario: jsonTool mode without native transport support
- **WHEN** a provider transport without native structured output calls `claude-sonnet-5-5` with a JSON schema response
- **THEN** the tool-based fallback SHALL be used with an `auto` tool choice

### Requirement: Anthropic automatic structured-output beta gating

With direct beta support, effective native output and any function tool, `buildParams` SHALL add `structured-outputs-2025-11-13` unless JSON response-tool fallback is active, independently of absent/false/true Strict. Provider-defined tools alone SHALL NOT trigger it. Explicit caller betas SHALL remain unaffected.

#### Scenario: Anthropic automatic structured-output beta gating

- **WHEN** a direct native-capable request has a function tool with absent strict and no JSON fallback
- **THEN** the beta SHALL be added; switching to fallback or provider-defined-only tools SHALL suppress only automatic inclusion

### Requirement: Anthropic strict-tool presence by effective capability

With effective strict support, explicit Strict SHALL be sent unchanged. Without support, explicit true/false SHALL be omitted with unsupported warning feature `strict`; absent Strict SHALL be omitted without warning.

#### Scenario: Anthropic strict-tool presence by effective capability

- **WHEN** Vertex receives a function tool first with absent Strict and then with false or true
- **THEN** strict SHALL remain omitted; only the explicit boolean settings SHALL emit strict warnings

### Requirement: Anthropic stripped schema constraints

At every schema node, the sanitizer SHALL strip and summarize in description: `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`, `minLength`, `maxLength`, `pattern`, `minItems`, `maxItems`, `uniqueItems`, `minProperties`, `maxProperties`, and `not`.

#### Scenario: Anthropic stripped schema constraints

- **WHEN** a native property schema is `{type:"number",minimum:1,maximum:10,multipleOf:0.5}`
- **THEN** those three constraint keywords SHALL be removed and its description SHALL be `"minimum: 1; maximum: 10; multiple of: 0.5."`

### Requirement: Anthropic constraint appendix rendering

False boolean constraint values SHALL NOT be reported. Names SHALL be space-separated lowercase words (minLength→min length, exclusiveMinimum→exclusive minimum); strings SHALL be verbatim and other values JSON-encoded. Entries SHALL join with "; " and end with "."; an existing description SHALL precede the appendix with a newline.

#### Scenario: Anthropic constraint appendix rendering

- **WHEN** a node is `{type:"array",description:"Tags",minItems:2,uniqueItems:false}`
- **THEN** its sanitized description SHALL be `"Tags\nmin items: 2."`, with minItems and uniqueItems removed and no appendix entry for the false boolean

### Requirement: Anthropic recursive schema transformation

The sanitizer SHALL recurse into anyOf/oneOf/allOf, items, properties, definitions and $defs; oneOf SHALL become anyOf. A $ref node SHALL short-circuit to only {"$ref":<value>}, dropping siblings. Object nodes (type object or non-nil properties) SHALL receive additionalProperties:false regardless of input.

#### Scenario: Anthropic recursive schema transformation

- **WHEN** a native object property contains `{oneOf:[{type:"string"},{$ref:"#/$defs/Foo",title:"Drop me"}]}`
- **THEN** the property SHALL use anyOf, its reference branch SHALL contain only `$ref:"#/$defs/Foo"`, and the containing object SHALL have additionalProperties:false

### Requirement: Anthropic supported schema formats

The sanitizer SHALL retain formats `date-time`, `time`, `date`, `duration`, `email`, `hostname`, `uri`, `ipv4`, `ipv6`, and `uuid`; other formats SHALL be dropped and appended to description as `format: <value>`.

#### Scenario: Anthropic supported schema formats

- **WHEN** native string nodes use `email` and `regex` formats
- **THEN** email SHALL remain a format; regex SHALL be removed and reported in description
