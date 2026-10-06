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

### Requirement: Tool-based JSON fallback

When `CallOptions.ResponseFormat` has `Type: "json"` and a non-nil `Schema`, and either the model or provider transport does not support native structured output, `buildParams` SHALL synthesize a tool named `"json"` with `description: "Respond with a JSON object."` and the `ResponseFormat.Schema` as `inputSchema`. The tool SHALL be appended to the existing tools list (after any user-defined tools).

#### Scenario: Tool injection on an older model

- **WHEN** the direct Anthropic provider calls `buildParams` with `ResponseFormat` having `Type: "json"` and a valid JSON schema, and the model ID is `claude-3-haiku`
- **THEN** the returned `BetaMessageNewParams.Tools` SHALL include the user's tools plus a synthetic tool with name `"json"`, description `"Respond with a JSON object."`, and `InputSchema` matching the provided schema
- **AND** `OutputConfig.Format` SHALL NOT be set

#### Scenario: Vertex tool injection on a native-capable model

- **WHEN** the Vertex provider calls `buildParams` with `ResponseFormat` having `Type: "json"`, a valid JSON schema, a function tool, and the model ID is `claude-sonnet-4-6`
- **THEN** the returned `BetaMessageNewParams.Tools` SHALL include the user's function tool plus the synthetic `"json"` tool
- **AND** `OutputConfig.Format` SHALL NOT be set
- **AND** the automatic `structured-outputs-2025-11-13` beta SHALL NOT be added

#### Scenario: Tool injection with no existing user tools

- **WHEN** `buildParams` is called with `ResponseFormat` (json + schema), no user tools, and a model that does not support native structured output
- **THEN** the tools list SHALL contain exactly one tool: the synthetic `"json"` tool

### Requirement: Tool choice override in fallback mode

When the tool-based fallback is active, `buildParams` SHALL override `ToolChoice` to `required` (OfAny) with `DisableParallelToolUse` set to `true`, regardless of the caller's original `ToolChoice` setting.

#### Scenario: Tool choice forced to required

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `auto`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

#### Scenario: Tool choice override when caller set none

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `none`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

### Requirement: Provider transport capability gating

The direct Anthropic provider SHALL enable native structured output and strict function tools. The Vertex provider SHALL disable both capabilities. Each provider capability SHALL be combined with the selected model's structured-output capability, so a feature is effective only when both the provider transport and model support it.

When effective native structured-output support is enabled and the caller supplies any function tool, `buildParams` SHALL automatically add the `structured-outputs-2025-11-13` beta unless the JSON response-tool fallback is active. This automatic beta SHALL be independent of whether the function tool's `Strict` value is absent, `false`, or `true`. Provider-defined tools alone SHALL NOT trigger it. Explicit caller-supplied betas SHALL remain unaffected.

When effective strict-tool support is enabled, an explicit `Strict` value SHALL be sent unchanged. When it is disabled, explicit `true` and `false` values SHALL both be omitted and SHALL produce an unsupported warning for feature `strict`; an absent value SHALL be omitted without a warning.

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

On the native structured-output path, `applyResponseFormat` SHALL write a sanitized copy of `ResponseFormat.Schema` to `OutputConfig.Format`, MUST NOT mutate the caller's original schema, and SHALL NOT apply sanitization on the tool-based JSON fallback path.

The sanitizer SHALL strip the following JSON Schema validation keywords from
every schema node and append them as a human-readable summary to the node's
`description`:

- `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`
- `minLength`, `maxLength`, `pattern`
- `minItems`, `maxItems`, `uniqueItems`
- `minProperties`, `maxProperties`
- `not`

Boolean constraint values equal to `false` SHALL NOT be reported in the
appendix. Constraint names SHALL be rendered as space-separated lowercase
words (e.g., `minLength` -> `min length`, `exclusiveMinimum` -> `exclusive
minimum`). String constraint values SHALL be rendered verbatim; all other
values SHALL be rendered as their JSON encoding. Appendix entries SHALL be
joined with `"; "` and terminated with `"."`. When a node already has a
`description`, the appendix SHALL be appended after a newline.

The sanitizer SHALL preserve `$schema`, `$id`, `title`, `description`,
`default`, `const`, `enum`, `type`, and `required` keywords.

The sanitizer SHALL recurse into composition (`anyOf`, `oneOf`, `allOf`),
`items`, `properties`, `definitions`, and `$defs`. `oneOf` SHALL be rewritten
as `anyOf` on the output. A node containing `$ref` SHALL short-circuit and
emit only `{ "$ref": <value> }`, dropping all sibling keywords. Object nodes
(those with `type: "object"` or a non-nil `properties`) SHALL have
`additionalProperties: false` set on the output regardless of the input.

The sanitizer SHALL retain `format` values from the supported set
(`date-time`, `time`, `date`, `duration`, `email`, `hostname`, `uri`, `ipv4`,
`ipv6`, `uuid`) and SHALL drop other `format` values, appending them to
`description` as `format: <value>`.

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
