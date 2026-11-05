## RENAMED Requirements

- FROM: `### Requirement: OTel span shape`
- TO: `### Requirement: HooksMiddleware owns the preflight span`
- FROM: `### Requirement: Normalized hook transforms are authoritative and reconstructable`
- TO: `### Requirement: Inactive helpers reconstruct normalized hook transforms`
- FROM: `### Requirement: Hook reasoning and provider-part preservation`
- TO: `### Requirement: Inactive hook reasoning and provider-part preservation`
- FROM: `### Requirement: Hook undisclosed content and tool reconstruction`
- TO: `### Requirement: Inactive hook undisclosed content and tool reconstruction`

## MODIFIED Requirements

### Requirement: Composition order

`Stack(opts)` SHALL return the middleware slice in the order
`[HooksMiddleware, RecordingMiddleware]` (Hooks outer, Recording inner) when a
top-level or Hooks-specific `ClientResolver` is configured. Without a resolver,
`Stack` SHALL omit Hooks because no request can evaluate them. `Hooks.Enabled`
SHALL gate evaluation per request and SHALL NOT be evaluated by `Stack`.
Recording SHALL always be present in the slice returned by `Stack`.

#### Scenario: Hooks runs before Recording

- **WHEN** a request flows through `Wrap(base, opts)` with both middlewares enabled
- **THEN** `HooksMiddleware.WrapStream` (or `WrapGenerate`) SHALL be entered before `RecordingMiddleware.WrapStream` (or `WrapGenerate`)

#### Scenario: Hook denial short-circuits recording

- **WHEN** `EvaluateHook` returns a deny response
- **THEN** `RecordingMiddleware` SHALL NOT call `StartGeneration` for that request
- **AND** no `agento11y.Generation` row SHALL be recorded

#### Scenario: Rejected transform short-circuits recording

- **WHEN** `EvaluateHook` returns a non-deny response with non-nil decoded `TransformedInput`
- **THEN** `HooksMiddleware` SHALL return `ErrHookTransformFailed`
- **AND** `RecordingMiddleware` SHALL NOT start a generation
- **AND** the provider SHALL NOT be invoked

### Requirement: MapGenerateResult produces agento11y.Generation

`MapGenerateResult(params, result, ctxInfo)` SHALL produce `agento11y.Generation` with `Input.Messages` derived from `params.Prompt`. System messages SHALL fold into one concatenated `SystemPrompt`, not input messages. Empty input/output reasoning SHALL be omitted. Output SHALL contain an assistant message for supported model content and additional tool-role messages for tool-result entries.

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

#### Scenario: Anthropic server-tool usage is recorded

- **WHEN** `result.Usage.Raw` contains positive `server_tool_use.web_search_requests` or `web_fetch_requests`
- **THEN** generation metadata SHALL contain the corresponding Agent Observability usage keys and their sum as `total_requests`

#### Scenario: Reported input usage is inclusive

- **WHEN** `result.Usage.InputTokens.Total` is present and does not contradict the reported input-token buckets
- **THEN** `Generation.Usage.InputSemantics` SHALL equal `agento11y.TokenInputSemanticsInclusive`

#### Scenario: Unreported input usage has no semantics

- **WHEN** `result.Usage.InputTokens.Total` is absent
- **THEN** `Generation.Usage.InputSemantics` SHALL remain unspecified

#### Scenario: Byte-equal output to agento11y anthropic helper

- **GIVEN** a recorded canonical request that has both an `anthropic.MessageNewParams` form and an equivalent `provider.CallOptions` form
- **WHEN** the Anthropic form is passed through `agento11y/go-providers/anthropic.FromRequestResponse`
- **AND** the ai-sdk form is passed through `MapGenerateResult`
- **THEN** both resulting `agento11y.Generation` payloads SHALL produce byte-equal JSON modulo the fields `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

#### Scenario: Call settings and tools populate generation input
- **WHEN** `MapGenerateResult` maps a generate result
- **THEN** `Input.Tools` SHALL be derived from `params.Tools`; function tools SHALL map directly, and provider-defined tools (e.g. Anthropic `web_search`, `code_execution`) MAY map with their type preserved for Agent Observability annotation
- **AND** `Input.MaxTokens`, `Temperature`, `TopP`, `ToolChoice` SHALL derive from the corresponding call-option fields
- **AND** Anthropic thinking-budget metadata (`agento11y.gen_ai.request.thinking.budget_tokens`) SHALL derive from `params.ProviderOptions["anthropic"]` through `json.RawMessage` decoding without importing `providers/anthropic`

#### Scenario: Finish reasons retain legacy stop strings
- **WHEN** a generate result supplies a finish reason
- **THEN** `StopReason` SHALL be produced by `finishReasonToAgento11yStop(result.FinishReason)` and match the legacy `internal/llm/claude/` strings (e.g. `"end_turn"`, `"max_tokens"`, `"tool_use"`, `"stop_sequence"`)

#### Scenario: Caller metadata and tags are merged
- **WHEN** `MapGenerateResult` maps a generate result
- **THEN** `Metadata` SHALL start with caller metadata, then apply reserved request and usage derivations
- **AND** derived Anthropic thinking-budget and positive server-tool request counts SHALL override conflicts, matching the pinned agento11y Anthropic helper
- **AND** `Tags` SHALL merge `ctxInfo.Tags` and context tags

#### Scenario: Provider tools preserve recoverable discriminators
- **WHEN** provider tool calls and results are mapped
- **THEN** they SHALL retain recoverable Anthropic discriminators, including MCP metadata and configured aliases for web search, web fetch, code execution, and tool search
- **AND** irrecoverable provider subtypes MAY use the generic discriminator

### Requirement: Recording maps file parts to Agent Observability media

Recording SHALL map supported prompt, generated, and streamed `file` / `reasoning-file` content to `agento11y.PartKindMedia`, preserving the source kind without changing requests, results, or provider/UI wire types. Only image/video media SHALL be recorded. Hook preflight SHALL exclude file and reasoning-file media; metadata-only agento11y export SHALL omit media URLs.

#### Scenario: Prompt and generated file parts become media

- **GIVEN** a prompt or generated result containing an image/video `file` or `reasoning-file` part with supported data
- **WHEN** the generation is mapped for recording
- **THEN** the resulting Agent Observability message SHALL contain a media part with the inferred concrete MIME type
- **AND** byte or base64 data SHALL be represented as a base64 data URL
- **AND** the media metadata SHALL identify the source as `file` or `reasoning_file`

#### Scenario: Unsafe or unsupported file data is skipped

- **GIVEN** a file part containing a reference, inline text data, malformed base64, conflicting MIME types, URL credentials, a non-HTTP(S) remote URL, or unsupported/ambiguous media
- **WHEN** the generation is mapped for recording
- **THEN** no media part SHALL be added for that file part

#### Scenario: Hook preflight excludes media

- **GIVEN** a prompt containing text and file media
- **WHEN** `HooksMiddleware` builds its preflight `HookEvaluateRequest`
- **THEN** the request SHALL contain the supported non-media prompt content
- **AND** SHALL NOT contain the file media or its URL/data payload

#### Scenario: Percent-escaped base64 is validated without rewriting the URL

- **GIVEN** a valid data URL whose base64 payload contains percent-escaped base64 characters
- **WHEN** the file part is mapped for recording
- **THEN** the decoded payload SHALL pass strict base64 validation
- **AND** the original data URL SHALL be retained unchanged in the media part

### Requirement: Recording uses response model identity when available

Agent Observability recording SHALL use backend response metadata as the canonical generation model identity when a successful provider response supplies both provider and model ID. When response metadata omits either provider or model ID, recording SHALL preserve the model identity from `GenerationStart`. Generate and stream `ResponseModel` SHALL default to the requested model and SHALL be overridden by a non-empty response model.

#### Scenario: Provider errors leave the response model empty

- **WHEN** `DoGenerate` or `DoStream` returns an error instead of a result or stream
- **THEN** the recorded generation's `ResponseModel` SHALL be empty

#### Scenario: Generate response overrides transport model identity

- **GIVEN** a wrapped model whose `Provider()` returns `grafana` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful `DoGenerate` result whose `Response.Provider` is `anthropic` and `Response.ModelID` is `claude-sonnet-4-5-20250929`
- **THEN** the resulting Agent Observability generation's `Model.Provider` SHALL equal `anthropic`
- **AND** the resulting Agent Observability generation's `Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

#### Scenario: Generate response without provider keeps seed identity

- **GIVEN** a wrapped model whose `Provider()` returns `grafana` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful `DoGenerate` result whose `Response.ModelID` is populated but `Response.Provider` is empty
- **THEN** the resulting Agent Observability generation's `Model.Provider` SHALL equal `grafana`
- **AND** the resulting Agent Observability generation's `Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

#### Scenario: Stream response metadata overrides transport model identity

- **GIVEN** a stream recorder seeded with `GenerationStart.Model.Provider` equal to `grafana` and `GenerationStart.Model.Name` equal to `claude-sonnet-4-5-20250929`
- **WHEN** `StreamRecorder.Observe` receives `provider.StreamPart{Type: PartResponseMeta, Provider: "anthropic", ModelID: "claude-sonnet-4-5-20250929"}` before stream completion
- **THEN** `StreamRecorder.Generation().Model.Provider` SHALL equal `anthropic`
- **AND** `StreamRecorder.Generation().Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

#### Scenario: Stream response metadata without provider keeps seed identity

- **GIVEN** a stream recorder seeded with `GenerationStart.Model.Provider` equal to `grafana` and `GenerationStart.Model.Name` equal to `claude-sonnet-4-5-20250929`
- **WHEN** `StreamRecorder.Observe` receives `provider.StreamPart{Type: PartResponseMeta, ModelID: "claude-sonnet-4-5-20250929"}` without a provider
- **THEN** `StreamRecorder.Generation().Model.Provider` SHALL equal `grafana`
- **AND** `StreamRecorder.Generation().Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

### Requirement: Recording preserves transport identity metadata

When response metadata changes the canonical generation model identity from the wrapped model identity, Agent Observability recording SHALL add generic transport identity metadata. The metadata SHALL include `ai_sdk.transport.provider` with the wrapped model provider and `ai_sdk.transport.model` with the wrapped model ID. Recording SHALL NOT add this metadata when the final canonical model identity matches the wrapped model identity or when response metadata is incomplete.

#### Scenario: Generate records transport metadata when response identity differs

- **GIVEN** a wrapped model whose `Provider()` returns `grafana` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful `DoGenerate` result whose `Response.Provider` is `anthropic` and `Response.ModelID` is `claude-sonnet-4-5-20250929`
- **THEN** the resulting Agent Observability generation metadata SHALL contain `ai_sdk.transport.provider` equal to `grafana`
- **AND** the resulting Agent Observability generation metadata SHALL contain `ai_sdk.transport.model` equal to `claude-sonnet-4-5-20250929`

#### Scenario: Stream records transport metadata when response identity differs

- **GIVEN** a stream recorder seeded with `GenerationStart.Model.Provider` equal to `grafana` and `GenerationStart.Model.Name` equal to `claude-sonnet-4-5-20250929`
- **WHEN** `StreamRecorder.Observe` receives `provider.StreamPart{Type: PartResponseMeta, Provider: "anthropic", ModelID: "claude-sonnet-4-5-20250929"}` before stream completion
- **THEN** `StreamRecorder.Generation().Metadata` SHALL contain `ai_sdk.transport.provider` equal to `grafana`
- **AND** `StreamRecorder.Generation().Metadata` SHALL contain `ai_sdk.transport.model` equal to `claude-sonnet-4-5-20250929`

#### Scenario: Normalized providers keep the AI SDK transport name

- **GIVEN** a wrapped model whose `Provider()` returns `amazon-bedrock`
- **WHEN** `RecordingMiddleware` records a response whose identity differs from the wrapped identity after provider-name normalization
- **THEN** `ai_sdk.transport.provider` SHALL equal `amazon-bedrock`
- **AND** a response from `amazon-bedrock` with the wrapped model ID SHALL NOT add transport metadata
- **AND** a `StreamRecorder` created with `NewStreamRecorder` SHALL use the `GenerationStart` provider

#### Scenario: Direct provider does not record transport metadata

- **GIVEN** a wrapped model whose `Provider()` returns `anthropic` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful response whose response provider and model match the wrapped model identity
- **THEN** the resulting Agent Observability generation metadata SHALL NOT contain `ai_sdk.transport.provider`
- **AND** the resulting Agent Observability generation metadata SHALL NOT contain `ai_sdk.transport.model`

### Requirement: RecordingMiddleware wraps generate and stream

`RecordingMiddleware(opts)` SHALL return `middleware.Middleware` with generate and stream hooks. A nil resolved client SHALL pass through unchanged. Active calls SHALL build `GenerationStart` as `BuildGenerationStart(ctx, model.Provider(), model.ModelID(), opts.ContextProvider(ctx))` does, applying `ContextSource`, call `client.StartGeneration` or `client.StartStreamingGeneration` respectively, then invoke the inner model. Params and results SHALL NOT be modified.

For streams, the recording goroutine SHALL select on the context supplied to `DoStream`. On context end, it SHALL stop forwarding and observing upstream parts. It SHALL select the generation and call error under the multi-segment finalization rules below. Before any observed `PartFinish`, the context error SHALL take precedence over an earlier `PartError`. After a finish, context end SHALL NOT become the call error.

After normal upstream closure or context end, the middleware SHALL call `recorder.SetResult` and then `recorder.End`. Both calls SHALL finish before the downstream channel closes. A `PartFinish` SHALL NOT end recording. A consumer that stops reading without cancelling SHALL NOT trigger cancellation. By default, the middleware SHALL NOT start a detached upstream drain. A configured `StreamDrainTimeout` MAY enable a finite drain without delaying finalization. Provider stream producers SHALL honor the call context.

#### Scenario: Generate path records on success

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil result and `nil` error
- **THEN** the middleware SHALL call `StartGeneration` once
- **AND** SHALL call `recorder.SetResult` once with the generation mapped from `params`, `result`, `ctxInfo`, and the original `GenerationStart`
- **AND** SHALL NOT call `recorder.SetCallError`

#### Scenario: Generate path records on error

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil error
- **THEN** the middleware SHALL call `recorder.SetResult` with the generation mapped from `params` without a result, after the generation filter
- **AND** the middleware SHALL call `recorder.SetCallError(err)` once
- **AND** the same error SHALL be returned to the caller

#### Scenario: Stream error records partial generation usage

- **GIVEN** a stream reports usage and then emits `PartError`
- **AND** upstream closes before the request context ends
- **WHEN** the recording goroutine finalizes
- **THEN** it SHALL call `recorder.SetCallError` with the stream error
- **AND** it SHALL call `recorder.SetResult` with the partial generation and aggregated usage

#### Scenario: Stream path records at end of stream

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoStream` returns a result stream that closes normally after N parts
- **THEN** the middleware SHALL call `StartStreamingGeneration` once
- **AND** the consumer SHALL receive exactly the same N parts in the same order
- **AND** after upstream closure, the middleware SHALL call `recorder.SetResult` and then `recorder.End`
- **AND** both calls SHALL finish before the downstream channel closes

#### Scenario: Stream cancellation records an error and cleans up

- **WHEN** the context supplied to `DoStream` is cancelled before observed upstream closure and before any observed `PartFinish`
- **THEN** the middleware SHALL stop forwarding and observing upstream parts
- **AND** it SHALL call `recorder.SetCallError` with the context error
- **AND** it SHALL call `recorder.SetResult` with the partial generation and then call `recorder.End`
- **AND** it SHALL close the downstream channel after both calls finish
- **AND** by default it SHALL NOT start a detached upstream drain

#### Scenario: Successful calls finalize the corresponding recorder
- **WHEN** an active generate or stream call succeeds
- **THEN** generate SHALL use `client.StartGeneration`, map the result with the original `GenerationStart` for requested-model fallback, and call `recorder.SetResult`
- **AND** stream SHALL use `client.StartStreamingGeneration`, tee each result part into `StreamRecorder`, and call `recorder.SetResult` once at finalization with the stream generation selected by the finalization rules

#### Scenario: Opening errors and usage on error parts are recorded
- **WHEN** a call returns an error before a stream opens or a stream emits `PartError`
- **THEN** an opening error SHALL call `recorder.SetResult` with the filtered generation mapped from `params` and then `recorder.SetCallError(err)`
- **AND** recording SHALL observe usage reported before or on a `PartError` without ending the stream at that error part
- **AND** stream finalization SHALL select the call error and generation, including aggregated usage, under the multi-segment finalization rules

### Requirement: HooksMiddleware enforces preflight policy

`HooksMiddleware(opts)` SHALL wrap generate and stream preflight policy. A false non-nil `Enabled` gate or nil resolved client SHALL pass through unchanged. It SHALL build a preflight `agento11y.HookEvaluateRequest` from params excluding file/reasoning-file media, then call `client.EvaluateHook`. Positive `MaxLatency` SHALL use `context.WithTimeout(ctx, opts.MaxLatency)`; otherwise inherit the request context unchanged.

The middleware SHALL apply decoded decisions in this order:
1. Deny SHALL return `&HookDenialError{Reason, RuleID, Cause: nil}` without invoking the inner model, even if a transform is present.
2. Any non-nil decoded `TransformedInput` SHALL return `ErrHookTransformFailed` without invoking the inner model, regardless of `FailOpen`.
3. Allow without a transform SHALL invoke the inner model with params unchanged.

#### Scenario: Allow path passes through

- **GIVEN** a `HooksMiddleware` whose `ClientResolver` returns a client
- **WHEN** `EvaluateHook` returns an allow decision with nil `TransformedInput`
- **THEN** the inner model SHALL be invoked with `params` unchanged
- **AND** the response from the inner model SHALL be returned to the caller

#### Scenario: Deny returns typed error

- **GIVEN** a `HooksMiddleware` whose `ClientResolver` returns a client
- **WHEN** `EvaluateHook` returns a deny decision with reason "policy violation" and rule ID "rule-42"
- **THEN** the middleware SHALL return a non-nil error
- **AND** `errors.As(err, new(*agentobservability.HookDenialError))` SHALL succeed
- **AND** the unwrapped error SHALL have `Reason == "policy violation"` and `RuleID == "rule-42"`
- **AND** `errors.Is(err, agentobservability.ErrHookDenied)` SHALL return `true`
- **AND** the inner model's `DoGenerate`/`DoStream` SHALL NOT be invoked

#### Scenario: MaxLatency bounds EvaluateHook

- **GIVEN** a `HooksMiddleware` with `opts.MaxLatency = 100 * time.Millisecond`
- **WHEN** the upstream `EvaluateHook` server stalls for longer than 100ms
- **THEN** the hook call SHALL be cancelled via context deadline
- **AND** the original request context SHALL NOT be cancelled (only the derived hook-bounded context)

#### Scenario: Decisions apply after client wire normalization
- **WHEN** the pinned client returns a decoded and normalized hook response
- **THEN** middleware SHALL apply policy to that response, not recover discarded wire information
- **AND** the client SHALL own wire field names, protobuf/base64 decoding, role/part normalization, and conversation/trace correlation
- **AND** deny SHALL take precedence over a decoded transform
- **AND** allow with nil `TransformedInput` SHALL invoke the inner model with params unchanged

### Requirement: Inactive helpers reconstruct normalized hook transforms

This requirement and its scenarios describe inactive reconstruction helpers, not accepted middleware transforms. They SHALL NOT authorize a provider call while transforms are disabled.

Helpers SHALL use the pinned client's normalized hook semantics and SHALL NOT promise lossless validation of original wire responses. A non-nil normalized `TransformedInput` SHALL be an authoritative replacement, not a patch. An empty wire transform normalized to nil SHALL mean no transform. Invalid reconstruction SHALL fail closed with `ErrHookTransformFailed`; a non-nil transform without a usable prompt SHALL fail closed, while a system-only replacement is valid.

#### Scenario: Empty wire transform is a no-op

- **GIVEN** a non-empty original prompt
- **AND** the hook service returns an allow response with `transformed_input: {}`
- **WHEN** the pinned client normalizes that input to nil
- **THEN** the model SHALL receive the original call options unchanged

#### Scenario: Normalization precedes validation

- **GIVEN** an allow response with an unknown message role, a supported text part, and an unsupported payload-less part
- **WHEN** the pinned client normalizes the role to user and drops the unsupported part
- **THEN** the decoded message SHALL contain only the normalized user text
- **AND** reconstruction helpers SHALL NOT recover information discarded by the decoder
- **AND** active middleware SHALL still reject the non-nil decoded transform before calling the provider

#### Scenario: Normalized message without usable content fails closed

- **GIVEN** the client returns a non-nil transform containing a message whose parts were all dropped
- **WHEN** the inactive helper reconstructs the normalized prompt
- **THEN** it SHALL return `ErrHookTransformFailed`
- **AND** active middleware SHALL NOT invoke the inner model

#### Scenario: Provider discriminator discarded during decoding

- **GIVEN** an original provider-executed tool call
- **AND** the normalized transform retains its ID, name, and payload but lacks its required provider discriminator
- **WHEN** the inactive helper reconstructs the normalized prompt
- **THEN** it SHALL fail closed rather than silently convert the call to a client-executed tool

#### Scenario: Removed assistant parts stay removed

- **GIVEN** an original assistant message containing signed reasoning, a tool call, and visible text
- **AND** the transformed assistant message retains the same reasoning and text but omits the tool call
- **WHEN** the inactive helper reconstructs the replacement
- **THEN** only the unchanged reasoning part and transformed text SHALL be present
- **AND** the omitted tool call SHALL NOT be restored
- **AND** the unchanged reasoning part SHALL retain its original signature

#### Scenario: Multimodal transform fails closed

- **GIVEN** an original prompt containing text and undisclosed media
- **AND** the hook returns a transformed text message
- **WHEN** the inactive helper reconstructs the replacement
- **THEN** it SHALL return `ErrHookTransformFailed`
- **AND** it SHALL NOT produce a prompt with the media silently removed

#### Scenario: Tool removal is applied

- **GIVEN** the original request exposes a tool
- **AND** `TransformedInput.Tools` omits that tool
- **WHEN** the inactive helper reconstructs the replacement
- **THEN** the reconstructed call options SHALL omit that tool

#### Scenario: Hook replaces system prompt

- **GIVEN** `params.Prompt` contains system messages "be helpful" and "be concise"
- **AND** `EvaluateHook` returns a `TransformedInput` with `SystemPrompt: "internal-only assistant"`
- **WHEN** the inactive helper reconstructs the replacement
- **THEN** the resulting prompt SHALL begin with a single system message whose text equals "internal-only assistant"
- **AND** the original system messages SHALL NOT appear in the prompt

#### Scenario: Client normalization may discard wire distinctions
- **WHEN** the pinned client normalizes a wire transform before middleware validation
- **THEN** unknown roles may become user roles, unsupported or empty parts may be dropped or normalized to text, JSON payloads may be recovered from base64 or strings, and part metadata may be discarded
- **AND** reconstruction helpers SHALL validate only the remaining normalized content

#### Scenario: Non-empty transformed system prompt becomes one system message
- **WHEN** a non-nil normalized transform has a non-empty `SystemPrompt`
- **THEN** that `SystemPrompt` SHALL become one system message

#### Scenario: Empty transformed system prompt removes original systems
- **WHEN** a non-nil normalized transform has an empty `SystemPrompt`
- **THEN** no original system message SHALL be carried forward

#### Scenario: Invalid normalized prompt content is rejected
- **WHEN** normalized transformed messages still contain invalid roles, part kinds, empty payloads, or malformed tool payloads after client normalization
- **THEN** reconstruction SHALL fail with `ErrHookTransformFailed`

#### Scenario: Valid transformed messages retain order and omissions
- **WHEN** normalized transformed messages are valid for reconstruction
- **THEN** every message SHALL be rebuilt in returned order
- **AND** omitted assistant parts SHALL remain omitted without restoring an entire original assistant message based only on visible text

#### Scenario: System-only replacement is usable
- **WHEN** a non-nil normalized transform contains only a usable system prompt
- **THEN** the replacement SHALL be valid for the inactive helper
- **AND** active middleware SHALL still reject it before calling the provider

### Requirement: HooksMiddleware owns the preflight span

The optional hooks preflight span is the only span that this package SHALL open directly. `HooksMiddleware` SHALL name it `aisdk.hooks.preflight` and set its `gen_ai.provider.name` and `gen_ai.request.model` semantic-convention attributes. The agento11y client SHALL neither produce nor read the preflight span's `aisdk.hooks.*` attributes.

Span attribute keys:
- `aisdk.hooks.result` (string: `"allow"`, `"deny"`, `"transform"`).
- `aisdk.hooks.action` (string).
- `aisdk.hooks.rule_id` (string, present only on deny).

Every attribute that the middleware sets on this span, other than `gen_ai.*` semantic-convention attributes, SHALL use the `aisdk.hooks.` prefix. The middleware SHALL NOT emit attributes under the agento11y client's `agento11y.*` namespace or under any former product-named namespace.

#### Scenario: Allow decision sets aisdk.hooks.result

- **WHEN** `EvaluateHook` returns an allow decision with nil `TransformedInput`
- **THEN** the `aisdk.hooks.preflight` span SHALL have attribute `aisdk.hooks.result = "allow"`

#### Scenario: Deny decision sets rule ID

- **WHEN** `EvaluateHook` returns a deny decision with rule ID "rule-42"
- **THEN** the `aisdk.hooks.preflight` span SHALL have attributes `aisdk.hooks.result = "deny"` and `aisdk.hooks.rule_id = "rule-42"`

#### Scenario: Transform decision records the action (inactive)

This scenario is inactive while transforms are disabled. It describes future successful reconstruction, not an accepted provider call with pinned agento11y v0.19.0.

- **GIVEN** transforms are re-enabled after the decoder fix
- **WHEN** `EvaluateHook` returns a transform decision carrying an action
- **THEN** the `aisdk.hooks.preflight` span SHALL have attribute `aisdk.hooks.result = "transform"`
- **AND** it SHALL carry `aisdk.hooks.action` with that decision's action

#### Scenario: Span shape is covered by tests

- **WHEN** the module's test suite runs
- **THEN** tests SHALL assert the preflight span name, allow/deny results, action, and deny rule ID through an OpenTelemetry span recorder
- **AND** tests SHALL assert that rejected transforms record an error without invoking the provider
- **AND** the inactive `"transform"` result SHALL NOT imply an accepted provider call

#### Scenario: Hook span carries owned decision and semantic attributes
- **WHEN** ai-sdk opens `aisdk.hooks.preflight`
- **THEN** it SHALL carry `gen_ai.provider.name` and `gen_ai.request.model`, also set by agento11y on its generation span
- **AND** decision keys SHALL be `aisdk.hooks.result` (string: `"allow"`, `"deny"`, `"transform"`), `aisdk.hooks.action` (string), and `aisdk.hooks.rule_id` (string, present only on deny)
- **AND** `aisdk.hooks.*` keys SHALL be ai-sdk-owned; agento11y neither produces nor reads them

#### Scenario: Generation errors belong to the client span
- **WHEN** generation calls encounter an error
- **THEN** it SHALL reach the trace through `recorder.SetCallError(err)`, which agento11y stamps as `error.type` and `error.category` on its own span
- **AND** middleware SHALL NOT emit its own generation-call error attributes

### Requirement: Conformance fixtures

The module SHALL include `testdata/generation/`, `stream/`, and `hooks/` conformance fixtures. Hook fixtures SHALL describe inactive reconstruction helpers and SHALL NOT imply that middleware accepts transforms. `mise run test-agent-observability-conformance` SHALL run them in isolation. Tests SHALL re-run on every PR touching `middleware/agentobservability/` or bumping `agento11y` in `go.mod`. Regeneration SHALL use only `AGENTO11Y_REGEN`; every skip/assertion message naming that variable SHALL use `AGENTO11Y_REGEN`.

Middleware tests SHALL cover rejection of every non-nil decoded transform before provider invocation for both generate and stream calls, regardless of `FailOpen`. They SHALL also cover allow without a transform, deny precedence, and empty HTTP `transformed_input:{}` normalization to nil.

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

#### Scenario: Generation and stream fixture provenance is retained
- **WHEN** the module's `testdata/` fixture corpus is inspected
- **THEN** `generation/` SHALL contain paired (ai-sdk-typed `CallOptions` + `GenerateResult`, expected `agento11y.Generation` JSON) triples sourced from `agento11y/go-providers/anthropic` conformance helpers
- **AND** `stream/` SHALL contain captured chunk streams reused from `providers/anthropic/test/conformance/recorded/` where overlapping content allows

#### Scenario: Hook fixtures exercise the real decoder boundary
- **WHEN** hook conformance fixtures and focused HTTP-boundary tests run
- **THEN** `hooks/` SHALL contain paired (input prompt, hook wire response, expected reconstructed prompt) triples
- **AND** the conformance test SHALL replay each transform triple through the inactive helpers and compare the expected reconstructed prompt
- **AND** focused HTTP-boundary tests SHALL use the real client decoder and middleware to assert transform rejection, allow without a transform, and deny precedence
- **AND** decoder and helper tests SHALL retain coverage of wire tool schemas, base64 payloads, normalization, correlation, and media exclusion

### Requirement: Inclusive Agent Observability token usage

`Usage` SHALL map normalized `result.Usage`. A reported input total SHALL set `InputSemantics` to `TokenInputSemanticsInclusive` unless reported input-token buckets contradict that total. Known buckets SHALL NOT exceed the total; a complete no-cache/cache-read/cache-write breakdown SHALL sum to the total. If the total is absent or inconsistent, input semantics SHALL remain unspecified.

Input totals already include cache reads/writes; cache buckets SHALL NOT be added again. Total tokens SHALL equal normalized input plus output. This SHALL apply to generate mapping, observed streamed usage, generation export, and client-owned token telemetry. Streams with no observed usage MAY leave usage and its semantics marker unset.

#### Scenario: Cache tokens are not double counted
- **WHEN** normalized usage reports a consistent input total, cache-read, cache-write, and output tokens
- **THEN** Agent Observability usage SHALL retain inclusive input semantics and total tokens SHALL equal input plus output without adding cache buckets again
- **AND** generate mapping, streamed usage, export, and client-owned token telemetry SHALL use the same contract

#### Scenario: Stream has no usage
- **WHEN** no usage is observed in a stream
- **THEN** its usage value and semantics marker MAY remain unset

### Requirement: Recorded media data and MIME resolution

Byte/base64 media SHALL become base64 data URLs. Valid data and HTTP(S) URLs SHALL remain unchanged; recording SHALL NOT fetch remote URLs. Concrete MIME resolution SHALL use declared media type, data URL, filename, URL path, then sniffed inline bytes, in that order. Percent-escaped data-URL payloads SHALL be decoded for validation without rewriting the URL; base64 with CR/LF SHALL be malformed.

#### Scenario: Infer image MIME and retain a remote URL
- **WHEN** a supported media file has no concrete declared type but its HTTP(S) URL path identifies its MIME type
- **THEN** the mapper SHALL resolve MIME in the specified precedence order and retain the URL unchanged without fetching it

#### Scenario: Reject line breaks in base64
- **WHEN** inline base64 contains CR or LF
- **THEN** the mapper SHALL treat it as malformed and skip the media

### Requirement: Inactive hook reasoning and provider-part preservation

This requirement and its scenarios apply only to inactive reconstruction helpers. They SHALL NOT authorize a provider call while transforms are disabled.

An unchanged reasoning part MAY reuse the exact original to preserve its provider signature only via an unambiguous unused part with identical text; changed or ambiguous signed reasoning SHALL fail closed. Unchanged provider-executed calls and provider-specific results SHALL retain provider fields only after exact ID, name, payload, and discriminator matching. Unmatchable provider-specific parts, including discarded required discriminators, SHALL fail closed.

#### Scenario: Signed reasoning cannot be ambiguously restored
- **WHEN** transformed signed reasoning is changed or has ambiguous identical-text matches
- **THEN** reconstruction SHALL fail closed rather than reuse an original signature

#### Scenario: Provider-specific parts require exact matches
- **WHEN** transformed provider-executed tool calls or provider-specific results differ in ID, name, payload, or required provider discriminator
- **THEN** reconstruction SHALL fail closed instead of retaining provider fields without an exact match

### Requirement: Inactive hook undisclosed content and tool reconstruction

This requirement and its scenarios apply only to inactive reconstruction helpers. They SHALL NOT authorize a provider call while transforms are disabled.

Transforms of prompts with undisclosed content SHALL fail closed rather than drop or restore it. Hook evaluation excludes media, message/text-part provider options, empty reasoning metadata, and other unsupported content. Returned tools SHALL match disclosed originals exactly; retained tools MAY be preserved/reordered and omitted tools SHALL be removed. New/modified tools not losslessly reconstructable or removal leaving required/named `ToolChoice` unsatisfied SHALL fail closed.

#### Scenario: Undisclosed metadata prevents lossy transformation
- **WHEN** the original prompt contains undisclosed message-level provider options, text-part provider options, empty reasoning metadata, media, or other unsupported content
- **AND** the hook supplies a transform
- **THEN** reconstruction SHALL fail closed rather than silently drop or restore that content

#### Scenario: Tool definitions and choice remain reconstructable
- **WHEN** returned tools are reconstructed
- **THEN** exact disclosed originals MAY be retained or reordered, and omitted tools SHALL be removed
- **AND** new or modified tools not reconstructable losslessly SHALL fail closed
- **AND** removal that leaves required or specifically named `ToolChoice` unsatisfied SHALL fail closed

## ADDED Requirements

### Requirement: Hook transforms are disabled until decoding is lossless

With pinned agento11y v0.19.0, `HooksMiddleware` SHALL reject every non-nil decoded `TransformedInput` before calling the provider. Deny SHALL take precedence and return `ErrHookDenied`; otherwise, the middleware SHALL return `ErrHookTransformFailed`. This SHALL apply to generate and stream calls regardless of `FailOpen`. Valid assistant, system-prompt, tool-message, tool-list, and user changes SHALL also be rejected. An unchanged replacement SHALL NOT bypass this restriction.

The dependency converts unsupported response roles to `user` and drops malformed parts before middleware validation. It exposes no raw response API. Validation of decoded input therefore cannot prove that the server response was valid or lossless. Transform support SHALL remain disabled until the decoder preserves invalid data for validation or rejects it.

Agento11y v0.19.0 normalizes an empty HTTP `transformed_input:{}` to nil. An allow response with nil `TransformedInput` SHALL pass the original request through unchanged. Deny behavior SHALL remain unchanged.

#### Scenario: Every decoded transform fails closed

- **GIVEN** either a generate or stream call, with `FailOpen` enabled or disabled
- **WHEN** `EvaluateHook` returns a non-deny response with non-nil decoded `TransformedInput`
- **THEN** `errors.Is(err, ErrHookTransformFailed)` SHALL be true
- **AND** the provider SHALL NOT be invoked
- **AND** the original prompt and tools SHALL remain unchanged

#### Scenario: Valid transforms are also rejected

- **GIVEN** a valid replacement that changes assistant messages, the system prompt, tool messages, the tool list, or user messages
- **WHEN** `EvaluateHook` returns an allow response carrying that replacement as a non-nil decoded `TransformedInput`
- **THEN** `HooksMiddleware` SHALL return `ErrHookTransformFailed` without invoking the provider
- **AND** unchanged user messages SHALL NOT permit other transformations

#### Scenario: Unchanged replacement is still a transform

- **WHEN** an allow response contains a non-nil decoded `TransformedInput` that exactly matches the disclosed original input
- **THEN** `HooksMiddleware` SHALL return `ErrHookTransformFailed` without invoking the provider

#### Scenario: Decoder drops malformed parts

- **GIVEN** an HTTP transform containing valid content and malformed parts
- **AND** agento11y v0.19.0 drops the malformed parts, leaving a non-nil decoded `TransformedInput`
- **WHEN** `HooksMiddleware` receives the decoded allow response
- **THEN** it SHALL return `ErrHookTransformFailed` even if the remaining content passes reconstruction validation
- **AND** the provider SHALL NOT be invoked

#### Scenario: Empty server transform is no transform

- **GIVEN** an original prompt and tools
- **WHEN** the Agent Observability server returns `{"action":"allow","transformed_input":{}}`
- **THEN** the dependency SHALL normalize `TransformedInput` to nil
- **AND** the inner model SHALL receive the original prompt and tools unchanged

### Requirement: Generation spans follow the resolved client protocol

`RecordingMiddleware` SHALL use `StartGeneration` or `StartStreamingGeneration` on the resolved agento11y client. It SHALL pass the returned context to the inner model so the provider call runs under the client-owned generation span. Other context values SHALL remain available to the provider.

For gRPC and HTTP generation export, `RecordingMiddleware` SHALL finalize the client-owned recorder with the mapped generation. The resolved agento11y client owns validation, queueing, and transport. Recorder completion SHALL NOT acknowledge that the Agent Observability API accepted the generation. Agento11y SHALL also emit one CLIENT metadata span. The span name SHALL be `generateText <model>` for unary calls or `streamText <model>` for streaming calls when the model is known. Its `gen_ai.operation.name` SHALL be `generateText` or `streamText`.

For OTel generation export, agento11y SHALL emit no separate generation payload. It SHALL emit one CLIENT generation span named `chat <model>` with `gen_ai.operation.name="chat"`. The span SHALL carry `agento11y.record="true"`, `agento11y.generation.id`, mapped generation attributes, and content allowed by the resolved capture mode. Streaming spans SHALL carry `gen_ai.request.stream=true`.

The middleware SHALL NOT create a second generation span. It SHALL NOT flush the application's OTel provider after a model call.

Error states on the generation path SHALL reach the trace through `recorder.SetCallError(err)`. Agento11y SHALL put `error.type` and `error.category` on its generation span. The middleware SHALL NOT add generation error attributes itself.

The OTel generation span is compared with the `chat <model>` span of `@ai-sdk/otel@1.0.153`, the release that depends on the registered `ai@7.0.153` and `@ai-sdk/provider@4.0.34`. `@ai-sdk/otel` is not registered in `test/conformance/upstream.yaml`, so no conformance test enforces this comparison. Both spans are CLIENT spans with `gen_ai.operation.name="chat"` and GenAI request, response, usage, and content attributes. These differences are intentional:

- Upstream names the span after the requested model, keeps the call-start provider and request model, and records the backend model as `gen_ai.response.model`. With response-preferred identity, Agent Observability uses the complete backend identity for the span name, provider, and request model, and keeps the wrapped identity in `ai_sdk.transport.*` metadata.
- Upstream reports the unified finish reason, such as `stop`. Agent Observability reports the provider's raw finish reason when present and otherwise maps unified reasons to `end_turn`, `max_tokens`, `tool_use`, `error`, or `other`.
- Upstream maps `anthropic.vertex` to `anthropic`. Agent Observability reports `gcp.vertex_ai`.
- Upstream sets `gen_ai.client.operation.duration`, `gen_ai.client.operation.time_to_first_chunk`, and `gen_ai.client.operation.time_per_output_chunk` on the span. Agent Observability sets `gen_ai.response.time_to_first_chunk` on streaming spans and records operation timing as metrics.
- The Go mapper does not export `gen_ai.request.frequency_penalty`, `gen_ai.request.presence_penalty`, `gen_ai.request.top_k`, or `gen_ai.request.stop_sequences`.
- Agent Observability adds generation IDs, parent generation IDs, tags, metadata, token semantics, and error categories.
- Upstream nests the span under `invoke_agent` and step spans, and it has a telemetry registration API and per-call input and output recording controls. The Go integration has none of these.

#### Scenario: gRPC and HTTP keep a metadata span

- **WHEN** the resolved client uses gRPC or HTTP generation export
- **THEN** `RecordingMiddleware` SHALL finalize the client-owned recorder with the mapped generation
- **AND** the provider call SHALL run under a `generateText <model>` or `streamText <model>` metadata span
- **AND** the middleware SHALL NOT add another generation span
- **AND** recorder completion SHALL NOT acknowledge remote receipt

#### Scenario: Unary OTel export hands the active span to the provider

- **GIVEN** a caller context with an active parent span and another context value
- **AND** the client uses full content capture
- **WHEN** `RecordingMiddleware` resolves an OTel-configured agento11y client and invokes a unary provider call
- **THEN** the provider SHALL receive the child `chat <model>` generation span as its active span
- **AND** the provider SHALL receive the other context value unchanged
- **AND** the completed generation span SHALL contain the mapped input, output, usage, provider identity, and model identity

#### Scenario: Streaming OTel span ends after normal provider closure

- **WHEN** the wrapped provider returns a stream and its channel later closes normally
- **THEN** `RecordingMiddleware` SHALL return the consumer stream while the generation span remains open
- **AND** each provider part SHALL reach the consumer unchanged
- **AND** the generation span SHALL end after the provider channel closes

#### Scenario: Streaming OTel span ends on cancellation

- **WHEN** the context supplied to `DoStream` ends before upstream closure is observed
- **THEN** the middleware SHALL stop forwarding provider parts
- **AND** it SHALL use the context error only if no `PartFinish` was observed
- **AND** after a finish it SHALL retain the latest provider error, if any, rather than the context error
- **AND** it SHALL call `recorder.SetResult` with the generation selected by the multi-segment finalization rules
- **AND** it SHALL call `recorder.End` before closing the downstream channel
- **AND** by default the middleware SHALL NOT start a detached upstream drain
- **AND** a configured `StreamDrainTimeout` MAY enable the existing finite drain

#### Scenario: OTel content follows the capture mode

- **WHEN** the resolved capture mode is `metadata_only`
- **THEN** the generation span SHALL omit system instructions, input messages, output messages, tool definitions, and media

- **WHEN** the resolved capture mode is `full_with_metadata_spans`
- **THEN** OTel generation export SHALL apply `full` capture because the generation span is the only generation copy

#### Scenario: Routed OTel generation keeps both identities

- **GIVEN** the default response-preferred identity mode
- **WHEN** response metadata supplies both a provider and model ID
- **AND** that provider/model pair differs from the wrapped model identity
- **THEN** the completed `chat <model>` span SHALL use the response provider and model
- **AND** `agento11y.generation.metadata` SHALL retain the wrapped provider and model as `ai_sdk.transport.provider` and `ai_sdk.transport.model`

#### Scenario: Built-in provider names use OTel registry values

- **WHEN** the wrapped or routed provider is `amazon-bedrock`
- **THEN** the mapped generation and the hook request SHALL use provider `bedrock` with every export protocol
- **AND** the OTel generation span and the hooks span SHALL use `gen_ai.provider.name="aws.bedrock"`
- **AND** the gRPC or HTTP metadata span SHALL use the stored provider `bedrock`

- **WHEN** the wrapped or routed provider is `anthropic.vertex`
- **THEN** the mapped generation and the hook request SHALL use provider `vertex` with every export protocol
- **AND** the OTel generation span and the hooks span SHALL use `gen_ai.provider.name="gcp.vertex_ai"`
- **AND** the gRPC or HTTP metadata span SHALL use the stored provider `vertex`

- **WHEN** the stored provider has an agento11y OTel registry spelling, such as `mistral`
- **THEN** the hooks span SHALL use the same `gen_ai.provider.name` value as the OTel generation span

#### Scenario: Disabled experimental support does not fall back

- **WHEN** a client selects OTel generation export without enabling agento11y experimental features
- **THEN** the client SHALL export no marked `chat <model>` generation span
- **AND** it SHALL NOT send the generation through gRPC or HTTP

#### Scenario: OTel recording respects consumer filters

- **GIVEN** a configured `GenerationFilter`
- **WHEN** a unary or streaming call finishes, including an immediate provider error
- **THEN** the filter SHALL receive the mapped generation once
- **AND** `GenerationStart` SHALL include no request content, controls, or provider metadata
- **AND** fields removed by the filter SHALL NOT reappear, except context fields that `GenerationStart` includes: user ID, agent name and version, conversation ID, parent generation IDs, tags, and metadata
- **AND** a filter panic SHALL preserve the existing minimal-generation fallback
- **AND** requested identity and provided-only context SHALL retain their existing behavior

### Requirement: OTel generation lifecycle remains application-owned

The middleware SHALL use an application-owned agento11y client and SHALL NOT construct the client, an OTel tracer provider, or an OTel exporter. The application SHALL construct these resources outside the middleware request path and select the sampling policy, export destination, and content capture mode. It MAY set the client's `TracerProvider` to select the OTel tracer provider for generation spans. To make `Client.Flush` and `Client.Shutdown` invoke and wait for `Flusher.ForceFlush`, the application SHALL set `Flusher` to the required flush target. A successful force-flush SHALL NOT acknowledge remote ingestion.

If `TracerProvider` is not configured explicitly, agento11y SHALL use the global OTel tracer provider for generation export. If `MeterProvider` is not configured explicitly, agento11y SHALL use the global OTel meter provider. `Config.Tracer` and `Config.Meter` SHALL NOT select the OTel generation pipeline.

The application SHALL call `Client.Shutdown` before shutting down its tracer provider. Documentation SHALL recommend a dedicated `AlwaysSample` OTel tracer provider when request-trace sampling must not suppress generation storage. For applications that use this dedicated tracer provider for generation spans, documentation SHALL explain that `Config.Tracer` can keep tool and embedding spans on the application's normal trace pipeline.

#### Scenario: Application supplies a flusher

- **WHEN** the application sets the same tracer provider as `TracerProvider` and `Flusher`
- **THEN** generation recording SHALL NOT flush that provider per request
- **AND** `Client.Shutdown` SHALL invoke and wait for `Flusher.ForceFlush` before the application shuts down the provider
- **AND** a successful force-flush SHALL NOT acknowledge remote ingestion

#### Scenario: OTel destination does not ingest generations

- **WHEN** `agento11y.record="true"` spans are sent only to a traces destination that does not ingest Agent Observability generations
- **THEN** the traces destination MAY store the span
- **AND** no Agent Observability generation record SHALL be created

#### Scenario: Delivery has no per-generation acknowledgement

- **WHEN** an OTel generation span ends
- **THEN** the middleware SHALL NOT report direct delivery acknowledgement
- **AND** sampling MAY drop the generation
- **AND** span attribute limits MAY remove generation content
