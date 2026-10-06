## MODIFIED Requirements

### Requirement: MapGenerateResult produces agento11y.Generation

`MapGenerateResult(params, result, ctxInfo)` SHALL produce an `agento11y.Generation` whose:
- `Input.Messages` is derived from `params.Prompt`, with `provider.Message{Role: RoleSystem}` entries folded into `Generation.SystemPrompt` (single concatenated string) rather than appearing as an agento11y.Message.
- `Input.Tools` is derived from `params.Tools`. Function tools map directly; provider-defined tools (e.g. Anthropic `web_search`, `code_execution`) MAY map with their type preserved so Agent Observability can annotate them.
- `Input.MaxTokens`, `Temperature`, `TopP`, `ToolChoice` are derived from the corresponding `provider.CallOptions` fields.
- Anthropic thinking-budget metadata (`agento11y.gen_ai.request.thinking.budget_tokens`) is derived from `params.ProviderOptions["anthropic"]` via `json.RawMessage` decoding, not by importing `providers/anthropic`.
- `Output` is a single assistant `agento11y.Message` whose parts mirror supported `result.Content` entries (text, tool-call, reasoning, file, and reasoning-file parts).
- `Usage` maps from `result.Usage` (input tokens, output tokens, cache hits where applicable).
- `StopReason` is produced by `finishReasonToAgento11yStop(result.FinishReason)` and SHALL match the string values the legacy `internal/llm/claude/` path emitted (e.g. `"end_turn"`, `"max_tokens"`, `"tool_use"`, `"stop_sequence"`).
- `Metadata` is a merge of `ctxInfo.Metadata` and `map_provider_options.go` derivations.
- `Tags` is a merge of `ctxInfo.Tags` and any tags from context.

#### Scenario: System message folds into SystemPrompt

- **WHEN** `params.Prompt` contains one or more `provider.Message{Role: RoleSystem}` entries
- **THEN** the resulting `Generation.SystemPrompt` SHALL contain the concatenated system text
- **AND** the resulting `Generation.Input.Messages` SHALL NOT contain any system-role entries

#### Scenario: FinishReason mapping matches legacy strings

- **WHEN** `result.FinishReason` is `provider.FinishReasonLength`
- **THEN** `Generation.StopReason` SHALL equal `"max_tokens"`

- **WHEN** `result.FinishReason` is `provider.FinishReasonToolCalls`
- **THEN** `Generation.StopReason` SHALL equal `"tool_use"`

#### Scenario: Anthropic thinking budget metadata is read through ProviderOptions

- **WHEN** `params.ProviderOptions["anthropic"]` carries a JSON object containing a `thinking` field with a positive `budget_tokens` value
- **THEN** the resulting `Generation.Metadata["agento11y.gen_ai.request.thinking.budget_tokens"]` SHALL equal that integer

#### Scenario: Byte-equal output to agento11y anthropic helper

- **GIVEN** a recorded canonical request that has both an `anthropic.MessageNewParams` form and an equivalent `provider.CallOptions` form
- **WHEN** the Anthropic form is passed through `agento11y/go-providers/anthropic.FromRequestResponse`
- **AND** the ai-sdk form is passed through `MapGenerateResult`
- **THEN** both resulting `agento11y.Generation` payloads SHALL produce byte-equal JSON modulo the fields `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

### Requirement: Hooks transform preserves reasoning signatures and system context

When `EvaluateHook` returns a `TransformedInput`, `HooksMiddleware` SHALL rebuild `params.Prompt` from the transformed messages using the following algorithm:

1. **System messages**: if `transformed.SystemPrompt` is non-empty, the resulting prompt SHALL begin with a single `provider.Message{Role: RoleSystem}` carrying that text and SHALL NOT carry any original system messages forward. If `transformed.SystemPrompt` is empty (the zero value), the resulting prompt SHALL preserve every original system-role message from `params.Prompt` in their original order. The empty case is treated as "the hook did not touch the system context" because `messagesToAgento11y` produces an empty `SystemPrompt` both when the original prompt has no system messages AND when the hook didn't return one — the two cases are indistinguishable on the underlying SDK wire.
2. **Reasoning preservation**: build a content-matching index over assistant-role messages in the **original** `params.Prompt` that contain reasoning parts.
3. For each transformed non-system message:
   - If role is assistant AND the same concatenated text appears in the index AND has not yet been consumed, use the original message verbatim (preserves `ProviderOptions["anthropic"].signature`).
   - Otherwise, rebuild the message from the transformed `agento11y` parts.
4. Non-assistant messages (user, tool) are rebuilt from the transformed parts directly.

The resulting prompt is the concatenation of (system messages from step 1) + (rebuilt non-system messages from steps 2-4) in that order.

This preserves `ProviderOptions["anthropic"].signature` values on reasoning parts, which do not round-trip through Agent Observability's wire schema, **and** preserves the original system context across hook transforms that only touch user / assistant messages.

#### Scenario: Reasoning signature survives transform

- **GIVEN** an assistant message in `params.Prompt` carrying a reasoning part with `ProviderOptions["anthropic"].signature = "sig-xyz"`
- **AND** `EvaluateHook` returns a `TransformedInput` that modifies only user messages, leaving the assistant message text unchanged
- **WHEN** the transform is applied
- **THEN** the resulting `params.Prompt` SHALL contain an assistant message whose reasoning part has `ProviderOptions["anthropic"].signature == "sig-xyz"` byte-equal to the original

#### Scenario: Modified assistant text triggers rebuild from agento11y parts

- **GIVEN** an assistant message in `params.Prompt` whose text is "abc"
- **AND** `EvaluateHook` returns a `TransformedInput` whose corresponding assistant message has text "def"
- **WHEN** the transform is applied
- **THEN** the resulting `params.Prompt` SHALL contain an assistant message whose text equals "def"
- **AND** the original signature (if any) SHALL NOT be carried forward (because the content did not match)

#### Scenario: System messages survive a user-only transform

- **GIVEN** `params.Prompt` contains a `provider.Message{Role: RoleSystem}` with text "you are helpful" followed by a user message
- **AND** `EvaluateHook` returns a `TransformedInput` with `SystemPrompt: ""` and a transformed user message
- **WHEN** the transform is applied
- **THEN** the resulting `params.Prompt` SHALL begin with the original system message verbatim
- **AND** the inner model SHALL receive a non-empty system context

#### Scenario: Hook overrides system prompt

- **GIVEN** `params.Prompt` contains system messages "be helpful" and "be concise"
- **AND** `EvaluateHook` returns a `TransformedInput` with `SystemPrompt: "internal-only assistant"`
- **WHEN** the transform is applied
- **THEN** the resulting `params.Prompt` SHALL begin with a single system message whose text equals "internal-only assistant"
- **AND** the original system messages SHALL NOT appear in the prompt

### Requirement: OTel span shape

The middleware SHALL emit exactly one OTel span of its own: the hooks preflight span. The canonical generation span (operation = `generateText` / `streamText`, with `gen_ai.*` semantic-convention attributes and `agento11y.generation.id`) is owned by the agento11y client via `StartGeneration` / `StartStreamingGeneration`; the middleware SHALL NOT wrap or duplicate it.

Span name: `aisdk.hooks.preflight`. The span is opened by ai-sdk and its
`aisdk.hooks.*` attribute keys are ai-sdk's own: the agento11y SDK neither
produces nor reads them. The span also carries the `gen_ai.provider.name` and
`gen_ai.request.model` semantic-convention attributes, which the agento11y SDK
sets on its own generation span too.

Span attribute keys:
- `aisdk.hooks.result` (string: `"allow"`, `"deny"`, `"transform"`).
- `aisdk.hooks.action` (string).
- `aisdk.hooks.rule_id` (string, present only on deny).

Every attribute the middleware sets on this span, other than `gen_ai.*` semantic-convention attributes, SHALL use the `aisdk.hooks.` prefix. The middleware SHALL NOT emit attributes under the agento11y client's `agento11y.*` namespace or under any former product-named namespace.

Error states on the generation path SHALL reach the trace via `recorder.SetCallError(err)`, which agento11y stamps onto its own generation span as `error.type` and `error.category`. The middleware SHALL NOT emit its own error attributes for generation calls.

#### Scenario: Allow decision sets aisdk.hooks.result

- **WHEN** `EvaluateHook` returns an allow decision
- **THEN** the `aisdk.hooks.preflight` span SHALL have attribute `aisdk.hooks.result = "allow"`

#### Scenario: Deny decision sets rule ID

- **WHEN** `EvaluateHook` returns a deny decision with rule ID "rule-42"
- **THEN** the `aisdk.hooks.preflight` span SHALL have attributes `aisdk.hooks.result = "deny"` and `aisdk.hooks.rule_id = "rule-42"`

#### Scenario: Transform decision records the action

- **WHEN** `EvaluateHook` returns a transform decision carrying an action
- **THEN** the `aisdk.hooks.preflight` span SHALL have attribute `aisdk.hooks.result = "transform"`
- **AND** it SHALL carry `aisdk.hooks.action` with that decision's action

#### Scenario: Span shape is covered by tests

- **WHEN** the module's test suite runs
- **THEN** at least one test SHALL assert the span name and all three decision attribute keys through an OpenTelemetry span recorder

### Requirement: Conformance fixtures

The module SHALL include a `testdata/` directory containing:
- `generation/`: paired (ai-sdk-typed `CallOptions` + `GenerateResult`, expected `agento11y.Generation` JSON) triples sourced from `agento11y/go-providers/anthropic` conformance helpers.
- `stream/`: captured chunk-stream fixtures (reused from `providers/anthropic/test/conformance/recorded/` where overlapping content allows).
- `hooks/`: paired (input prompt, hook response, expected post-transform prompt) triples.

A `mise run test-agent-observability-conformance` task SHALL run these tests in isolation. The conformance tests SHALL re-run on every PR that touches `middleware/agentobservability/` or bumps the `agento11y` dependency in `go.mod`.

Fixture regeneration SHALL be controlled by the `AGENTO11Y_REGEN` environment variable and SHALL NOT consult any other name. Every skip message and assertion failure message that names the variable SHALL name `AGENTO11Y_REGEN`.

#### Scenario: Generation conformance fixture

- **GIVEN** a fixture triple in `testdata/generation/`
- **WHEN** `MapGenerateResult` is invoked with the fixture's params and result
- **THEN** the resulting `agento11y.Generation` JSON SHALL byte-equal the expected JSON modulo `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

#### Scenario: Stream conformance fixture

- **GIVEN** a captured chunk stream in `testdata/stream/`
- **WHEN** each chunk is fed to a `StreamRecorder` via `Observe`
- **AND** `Generation()` is called at end-of-stream
- **THEN** the resulting `agento11y.Generation` JSON SHALL byte-equal the expected JSON modulo `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

#### Scenario: Regeneration under AGENTO11Y_REGEN reproduces the recorded snapshots

- **WHEN** the suite runs with `AGENTO11Y_REGEN=1` on an unchanged tree
- **THEN** it SHALL pass
- **AND** every `expected_generation.json` and `expected_prompt.json` snapshot under `testdata/` SHALL report no changes under version control

#### Scenario: Default run does not regenerate fixtures

- **WHEN** the suite runs without `AGENTO11Y_REGEN`
- **THEN** the fixture-writing tests SHALL skip with a message naming `AGENTO11Y_REGEN`
