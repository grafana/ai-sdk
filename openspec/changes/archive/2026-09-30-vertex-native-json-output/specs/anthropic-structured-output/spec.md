## MODIFIED Requirements

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

### Requirement: Provider transport capability gating

The direct Anthropic provider SHALL enable native structured output and strict function tools. The Vertex provider SHALL enable native structured output and keep strict function tools disabled. Each provider capability SHALL be combined with the selected model's structured-output capability, so a feature is effective only when both the provider transport and model support it.

When the transport supports direct beta features, effective native structured-output support is enabled, and the caller supplies any function tool, `buildParams` SHALL automatically add the `structured-outputs-2025-11-13` beta unless the JSON response-tool fallback is active. This automatic beta SHALL be independent of whether the function tool's `Strict` value is absent, `false`, or `true`. Provider-defined tools alone SHALL NOT trigger it. Explicit caller-supplied betas SHALL remain unaffected.

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
