# Agent Observability Middleware

## Purpose

Define the provider-agnostic middleware that records AI SDK model calls in
Grafana Agent Observability and evaluates preflight policy hooks while preserving
the underlying agento11y SDK wire and telemetry contracts.
## Requirements
### Requirement: Nested Go module for the Agent Observability middleware

`middleware/agentobservability/` SHALL be a separate Go module: `module github.com/grafana/ai-sdk/middleware/agentobservability`, `replace github.com/grafana/ai-sdk => ../../`, mirroring `providers/<name>/`. It SHALL depend on root ai-sdk and `github.com/grafana/agento11y/go`, not `providers/anthropic` or any other provider module. Root ai-sdk SHALL NOT import any symbol from it.

#### Scenario: Root module does not pull agento11y

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk` (root)
- **THEN** `github.com/grafana/agento11y/go` SHALL NOT appear in the consumer's transitive dependency graph

#### Scenario: Agent Observability middleware does not import providers/anthropic

- **WHEN** running `cd middleware/agentobservability && go list -deps ./...`
- **THEN** the output SHALL NOT contain `github.com/grafana/ai-sdk/providers/anthropic`

#### Scenario: Nested middleware convention is documented
- **WHEN** a consumer reads `middleware/agentobservability/doc.go`
- **THEN** it SHALL document the public API surface and the convention that heavy middlewares with vendor SDK / gRPC / OTel dependencies live in nested modules under `middleware/`

### Requirement: Public API surface

`middleware/agentobservability` SHALL export recording, hooks, wrapping, mapping, stream recording, and context helper APIs. The specified names and shapes SHALL be normative; implementation renames require updating this spec. It SHALL export sentinel errors `ErrHookDenied error` and `ErrHookTransformFailed error`.

#### Scenario: HookDenialError unwraps to sentinel

- **WHEN** `HooksMiddleware` returns a `*HookDenialError`
- **THEN** `errors.Is(err, agentobservability.ErrHookDenied)` SHALL return `true`

#### Scenario: Wrap is equivalent to middleware.Wrap with Stack

- **WHEN** `agentobservability.Wrap(base, opts)` and `middleware.Wrap(middleware.WrapOptions{Model: base, Middleware: agentobservability.Stack(opts)})` are both called with the same opts
- **THEN** both SHALL produce a `provider.LanguageModel` with identical observable behavior

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

#### Scenario: Recording observes post-Hooks params

- **WHEN** `HooksMiddleware` applies a `TransformedInput` that modifies `params.Prompt`
- **THEN** `RecordingMiddleware` SHALL build its `agento11y.Generation.Input` from the post-transform prompt, not the original

### Requirement: ClientResolver controls per-request activation

Recording SHALL call `opts.ClientResolver(ctx)` once per request. Hooks SHALL resolve once after its `Enabled` gate; disabled requests SHALL NOT resolve. A nil client SHALL make that middleware a no-op: invoke the inner model unchanged, start no Generation, and call no `EvaluateHook`. A nil resolver SHALL behave as if every resolution returns nil.

#### Scenario: ClientResolver returns nil

- **WHEN** `ClientResolver(ctx)` returns `nil` for a request
- **THEN** the wrapped model SHALL produce identical output to the unwrapped base model
- **AND** no Agent Observability API calls SHALL be made for that request

#### Scenario: ClientResolver is nil

- **WHEN** `opts.ClientResolver` is `nil`
- **THEN** `Wrap`/`Stack` SHALL still return a valid `provider.LanguageModel` / middleware slice
- **AND** every request SHALL pass through unchanged

### Requirement: ContextProvider supplies request-scoped metadata

Recording SHALL call `opts.ContextProvider(ctx)` once per request to populate `GenerationStart`. Zero `ContextInfo` SHALL fall back to appropriate `agento11y.*FromContext` helpers or remain unset. A nil provider SHALL log a warning at most once per process and continue with all-`agento11y.*FromContext` defaults.

#### Scenario: Zero ContextInfo falls back to context helpers

- **WHEN** `ContextProvider(ctx)` returns a zero `ContextInfo`
- **THEN** `GenerationStart.UserID` SHALL equal `agento11y.UserIDFromContext(ctx)`
- **AND** `GenerationStart.AgentName` SHALL equal `agento11y.AgentNameFromContext(ctx)`

#### Scenario: Nil ContextProvider logs once

- **WHEN** `RecordingMiddleware` is invoked for the first time with `opts.ContextProvider == nil`
- **THEN** a Warn-level log message SHALL be emitted exactly once for the lifetime of the process

#### Scenario: ContextInfo overrides selected ambient fields
- **WHEN** `ContextProvider` returns request-scoped `ContextInfo`
- **THEN** `GenerationStart.UserID` SHALL use `ContextInfo.UserID`, falling back to `agento11y.UserIDFromContext(ctx)` when empty
- **AND** `GenerationStart.Metadata` SHALL use caller metadata, with reserved derivations from `ProviderOptions`, `params`, and usage overriding conflicts in the final generation
- **AND** `GenerationStart.Tags` SHALL merge `ContextInfo.Tags` on top of context tags
- **AND** set `AgentName` / `AgentVersion` SHALL override `agento11y.AgentNameFromContext` / `agento11y.AgentVersionFromContext`

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
- **AND** the original data URL SHALL be retained verbatim in the media part

### Requirement: StreamRecorder accumulates streamed generation state

`StreamRecorder.Observe` SHALL accumulate `Generation.Output` from provider stream parts. `Generation()` SHALL contain an assistant message for accumulated model parts and tool-role messages for completed results. Assistant text, reasoning, tool-call, and media parts SHALL retain first-observed provider-event order. Finish reason SHALL come from `PartFinish`; every usage-bearing part SHALL use shared streaming aggregation.

#### Scenario: Stream usage preserves strongest values

- **GIVEN** usage is split across multiple stream parts
- **AND** a later finish part omits or reports lower provisional normalized counters
- **WHEN** the recorder produces a generation
- **THEN** its usage SHALL use the independently aggregated strongest normalized counters supported by the Agent Observability usage schema

#### Scenario: Reasoning text accumulates across deltas

- **GIVEN** a stream that emits three `PartReasoningDelta` events with text fragments `"I "`, `"think "`, `"so"`
- **WHEN** `StreamRecorder.Observe` is called for each
- **AND** `StreamRecorder.Generation()` is called at end-of-stream
- **THEN** the resulting reasoning part in `Generation.Output` SHALL contain the concatenated reasoning text `"I think so"`

#### Scenario: Signature-only reasoning is omitted

- **GIVEN** a stream emits a reasoning block containing an Anthropic signature but no reasoning text
- **WHEN** `StreamRecorder.Generation()` is called
- **THEN** `Generation.Output` SHALL NOT contain an empty thinking part

#### Scenario: Text deltas concatenate

- **GIVEN** a stream emits `PartTextDelta{Text: "Hello, "}` then `PartTextDelta{Text: "world"}`
- **WHEN** the recorder is observed for each
- **THEN** the resulting assistant message's text part SHALL equal `"Hello, world"`

#### Scenario: Tool-call deltas accumulate by tool-call ID

- **GIVEN** a stream emits multiple `PartToolInputDelta` events for the same tool-call ID with incremental JSON argument fragments
- **WHEN** the recorder is observed
- **THEN** the resulting tool-call part SHALL have the concatenated argument JSON

#### Scenario: Streamed media preserves observed assistant-part order

- **GIVEN** a stream interleaves supported file, text, reasoning, tool-call, and reasoning-file events
- **WHEN** the recorder observes the stream and produces a generation
- **THEN** the assistant output parts SHALL follow the order in which each part was first observed
- **AND** the first supported file event SHALL set `FirstChunkAt()` when no earlier payload-bearing event was observed

#### Scenario: Deltas and supported files accumulate into assistant parts
- **WHEN** `Observe` receives text, reasoning, tool-input, and supported file events
- **THEN** `PartTextDelta` SHALL append to the active assistant text part, non-empty `PartReasoningDelta` to reasoning, and `PartToolInputDelta` to the active tool-call part
- **AND** signature-only reasoning without visible text SHALL NOT produce a thinking part
- **AND** supported `PartFile` and `PartReasoningFile` SHALL map to media parts

#### Scenario: Preliminary tool results coalesce without collapsing completed results
- **WHEN** a stream emits tool results
- **THEN** preliminary updates SHALL coalesce by exact tool-call ID and tool name
- **AND** only completed tool results SHALL enter final output
- **AND** distinct completed results SHALL remain distinct even when an ID is reused

### Requirement: Recording uses response model identity when available

Agent Observability recording SHALL use backend response metadata as the canonical generation model identity when a successful provider response supplies both provider and model ID. When response metadata omits either provider or model ID, recording SHALL preserve the model identity from `GenerationStart`. Generate and stream `ResponseModel` SHALL default to the requested model and SHALL be overridden by a non-empty response model.

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

#### Scenario: Direct provider does not record transport metadata

- **GIVEN** a wrapped model whose `Provider()` returns `anthropic` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful response whose response provider and model match the wrapped model identity
- **THEN** the resulting Agent Observability generation metadata SHALL NOT contain `ai_sdk.transport.provider`
- **AND** the resulting Agent Observability generation metadata SHALL NOT contain `ai_sdk.transport.model`

### Requirement: RecordingMiddleware wraps generate and stream

`RecordingMiddleware(opts)` SHALL return `middleware.Middleware` with generate and stream hooks. A nil resolved client SHALL pass through unchanged. Active calls SHALL build `GenerationStart` using `BuildGenerationStart(ctx, model.Provider(), model.ModelID(), opts.ContextProvider(ctx))`, call `client.StartGeneration` or `client.StartStreamingGeneration` respectively, then invoke the inner model. Params and results SHALL NOT be modified.

#### Scenario: Generate path records on success

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil result and `nil` error
- **THEN** the middleware SHALL call `StartGeneration` once
- **AND** SHALL call `recorder.SetResult` once with the generation mapped from `params`, `result`, `ctxInfo`, and the original `GenerationStart`
- **AND** SHALL NOT call `recorder.SetCallError`

#### Scenario: Generate path records on error

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil error
- **THEN** the middleware SHALL call `recorder.SetCallError(err)` once
- **AND** the same error SHALL be returned to the caller

#### Scenario: Stream error records partial generation usage

- **GIVEN** a stream reports usage and then emits `PartError`
- **WHEN** the recording goroutine finalizes
- **THEN** it SHALL call `recorder.SetCallError` with the stream error
- **AND** it SHALL call `recorder.SetResult` with the partial generation and aggregated usage

#### Scenario: Stream path records at end of stream

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoStream` returns a result stream that closes normally after N parts
- **THEN** the middleware SHALL call `StartStreamingGeneration` once
- **AND** the consumer SHALL receive exactly the same N parts in the same order
- **AND** `recorder.SetResult` SHALL be called once after the upstream channel closes

#### Scenario: Stream cancellation records an error and cleans up

- **WHEN** the consumer cancels its context before the upstream completes
- **THEN** the recording goroutine SHALL NOT block indefinitely
- **AND** the generation SHALL record the context cancellation as its call error
- **AND** the middleware SHALL NOT start a detached goroutine that waits indefinitely for the upstream channel to close

#### Scenario: Successful calls finalize the corresponding recorder
- **WHEN** an active generate or stream call succeeds
- **THEN** generate SHALL use `client.StartGeneration`, map the result with the original `GenerationStart` for requested-model fallback, and call `recorder.SetResult`
- **AND** stream SHALL use `client.StartStreamingGeneration`, tee each result part into `StreamRecorder`, and call `recorder.SetResult(streamRecorder.Generation())` at end-of-stream

#### Scenario: Opening errors and usage on error parts are recorded
- **WHEN** a call returns an error before a stream opens or a stream emits `PartError`
- **THEN** an opening error SHALL call `recorder.SetCallError(err)`
- **AND** a `PartError` SHALL call `recorder.SetCallError(err)` and `recorder.SetResult` with partial generation and aggregated usage observed before or on the error part

### Requirement: HooksMiddleware enforces preflight policy

`HooksMiddleware(opts)` SHALL wrap generate and stream preflight policy. A false non-nil `Enabled` gate or nil resolved client SHALL pass through unchanged. It SHALL build a preflight `agento11y.HookEvaluateRequest` from params excluding file/reasoning-file media, then call `client.EvaluateHook`. Positive `MaxLatency` SHALL use `context.WithTimeout(ctx, opts.MaxLatency)`; otherwise inherit the request context unchanged.

#### Scenario: Allow path passes through

- **GIVEN** a `HooksMiddleware` whose `ClientResolver` returns a client
- **WHEN** `EvaluateHook` returns an allow decision
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
- **THEN** middleware SHALL validate the normalized response, not recover discarded wire information
- **AND** the client SHALL own wire field names, protobuf/base64 decoding, role/part normalization, and conversation/trace correlation
- **AND** deny SHALL return `&HookDenialError{Reason, RuleID, Cause: nil}` without invoking the inner model, taking precedence over a decoded transform
- **AND** allow SHALL invoke the inner model with params unchanged

#### Scenario: Transformed input replaces prompt and retained tools
- **WHEN** the normalized hook response contains `TransformedInput` and is not denied
- **THEN** it SHALL authoritatively replace `params.Prompt` and rebuild the retained subset of `params.Tools` before invoking the model
- **AND** content that cannot be reconstructed without loss or reintroducing omitted content SHALL return `ErrHookTransformFailed` without invoking the model

### Requirement: Normalized hook transforms are authoritative and reconstructable

Middleware SHALL adopt the pinned client's normalized hook semantics and SHALL NOT promise lossless validation of original wire responses. A non-nil normalized `TransformedInput` SHALL be an authoritative replacement, not a patch. An empty wire transform normalized to nil SHALL mean no transform. Invalid reconstruction SHALL fail closed with `ErrHookTransformFailed`; a non-nil transform without a usable prompt SHALL fail closed, while a system-only replacement is valid.

#### Scenario: Empty wire transform is a no-op

- **GIVEN** a non-empty original prompt
- **AND** the hook service returns an allow response with `transformed_input: {}`
- **WHEN** the pinned client normalizes that input to nil
- **THEN** the model SHALL receive the original call options unchanged

#### Scenario: Normalization precedes validation

- **GIVEN** an allow response with an unknown message role, a supported text part, and an unsupported payload-less part
- **WHEN** the pinned client normalizes the role to user and drops the unsupported part
- **THEN** the model SHALL receive the normalized user text
- **AND** the middleware SHALL NOT reject information no longer present in the decoded response

#### Scenario: Normalized message without usable content fails closed

- **GIVEN** the client returns a non-nil transform containing a message whose parts were all dropped
- **WHEN** the middleware reconstructs the normalized prompt
- **THEN** `ErrHookTransformFailed` SHALL be returned
- **AND** the inner model SHALL NOT be invoked

#### Scenario: Provider discriminator discarded during decoding

- **GIVEN** an original provider-executed tool call
- **AND** the normalized transform retains its ID, name, and payload but lacks its required provider discriminator
- **WHEN** the middleware reconstructs the normalized prompt
- **THEN** it SHALL fail closed rather than silently convert the call to a client-executed tool

#### Scenario: Removed assistant parts stay removed

- **GIVEN** an original assistant message containing signed reasoning, a tool call, and visible text
- **AND** the transformed assistant message retains the same reasoning and text but omits the tool call
- **WHEN** the transform is applied
- **THEN** only the unchanged reasoning part and transformed text SHALL be present
- **AND** the omitted tool call SHALL NOT be restored
- **AND** the unchanged reasoning part SHALL retain its original signature

#### Scenario: Multimodal transform fails closed

- **GIVEN** an original prompt containing text and undisclosed media
- **AND** the hook returns a transformed text message
- **WHEN** the transform is applied
- **THEN** `ErrHookTransformFailed` SHALL be returned
- **AND** the model SHALL NOT receive a prompt with the media silently removed

#### Scenario: Tool removal is applied

- **GIVEN** the original request exposes a tool
- **AND** `TransformedInput.Tools` omits that tool
- **WHEN** the transform is applied
- **THEN** the model SHALL receive transformed call options without that tool

#### Scenario: Hook replaces system prompt

- **GIVEN** `params.Prompt` contains system messages "be helpful" and "be concise"
- **AND** `EvaluateHook` returns a `TransformedInput` with `SystemPrompt: "internal-only assistant"`
- **WHEN** the transform is applied
- **THEN** the resulting prompt SHALL begin with a single system message whose text equals "internal-only assistant"
- **AND** the original system messages SHALL NOT appear in the prompt

#### Scenario: Client normalization may discard wire distinctions
- **WHEN** the pinned client normalizes a wire transform before middleware validation
- **THEN** unknown roles may become user roles, unsupported or empty parts may be dropped or normalized to text, JSON payloads may be recovered from base64 or strings, and part metadata may be discarded
- **AND** middleware SHALL validate only the remaining normalized content

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
- **THEN** the replacement SHALL be valid

### Requirement: Generation-ID DAG context helpers

The package SHALL expose context helpers for current generation, parent-generation DAG dependencies, sibling/peer links, and opaque new IDs. Recording SHALL use non-empty `GenerationIDFromContext(ctx)` as `GenerationStart.ID` and pass `ParentGenerationIDsFromContext(ctx)` through as `GenerationStart.ParentGenerationIDs`.

#### Scenario: GenerationID flows into the recorder

- **GIVEN** a context with `WithGenerationID(ctx, "gen-123")` applied
- **WHEN** `RecordingMiddleware` invokes the inner model on that context
- **THEN** the resulting `GenerationStart.ID` SHALL equal `"gen-123"`

#### Scenario: ParentGenerationIDs flow into the recorder

- **GIVEN** a context with `WithParentGenerationIDs(ctx, "p1", "p2")` applied
- **WHEN** `RecordingMiddleware` invokes the inner model on that context
- **THEN** the resulting `GenerationStart.ParentGenerationIDs` SHALL contain exactly `["p1", "p2"]` in that order

#### Scenario: Set and retrieve generation relationships
- **WHEN** a caller builds generation relationships in context
- **THEN** the package SHALL expose `WithGenerationID(ctx context.Context, id string) context.Context` and `GenerationIDFromContext(ctx context.Context) string` for the current call's generation ID
- **AND** it SHALL expose `WithParentGenerationIDs(ctx context.Context, ids ...string) context.Context` and `ParentGenerationIDsFromContext(ctx context.Context) []string` for upstream generations whose outputs the call depends on, forming the parent → child DAG
- **AND** it SHALL expose `WithLinkedGenerationID(ctx context.Context, id string) context.Context` for sibling/peer links (e.g. an evaluation complementing a primary generation)
- **AND** `NewGenerationID() string` SHALL generate an opaque ID suitable for `WithGenerationID`

### Requirement: OTel span shape

Middleware SHALL emit exactly one span of its own: `aisdk.hooks.preflight`. The canonical `generateText` / `streamText` generation span with `gen_ai.*` and `agento11y.generation.id` SHALL remain owned by the client via `StartGeneration` / `StartStreamingGeneration`; middleware SHALL NOT wrap or duplicate it. Its own attributes SHALL use `aisdk.hooks.` except `gen_ai.*`, never `agento11y.*` or former product namespaces.

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

The module SHALL include `testdata/generation/`, `stream/`, and `hooks/` conformance fixtures. `mise run test-agent-observability-conformance` SHALL run them in isolation. Tests SHALL re-run on every PR touching `middleware/agentobservability/` or bumping `agento11y` in `go.mod`. Regeneration SHALL use only `AGENTO11Y_REGEN`; every skip/assertion message naming that variable SHALL use `AGENTO11Y_REGEN`.

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
- **THEN** `hooks/` SHALL contain paired (input prompt, hook wire response, expected post-transform prompt) triples replayed through the real client HTTP decoder and middleware
- **AND** focused HTTP-boundary tests SHALL additionally cover wire tool schemas, base64 payloads, normalization, deny precedence, correlation, and media exclusion

### Requirement: Selectable requested model identity
Agent Observability recording options SHALL provide a named identity-source setting. Its zero value SHALL preserve the current response-preferred behavior. Requested identity mode SHALL keep `GenerationStart` wrapped-model identity as the final generation model, SHALL ignore unary and streaming response identity for recording, and SHALL omit response model, provider response ID, and transport identity metadata. It SHALL observe and return all provider results and stream parts unchanged.

#### Scenario: Requested identity suppresses backend identity
- **WHEN** requested identity mode starts with provider `grafana` and model `grafana/assistant`, and a result or stream metadata names provider `anthropic`, a backend model ID, and a response ID
- **THEN** the finalized generation SHALL identify only provider `grafana` and model `grafana/assistant`
- **AND** response model, response ID, and transport identity metadata SHALL be absent
- **AND** the original result or stream part SHALL retain its response metadata

#### Scenario: Zero value preserves existing behavior
- **WHEN** identity source is not configured and complete response identity differs from wrapped model identity
- **THEN** generation identity and transport metadata SHALL retain the existing response-preferred behavior

### Requirement: Configurable bounded recording stream drain
Agent Observability recording options SHALL provide an optional positive stream-drain duration. When configured, cancellation cleanup SHALL drain the immediate upstream only until channel close or the absolute deadline, including for a continuously ready channel. Zero SHALL preserve the reconciled direct-consumer behavior of not draining after cancellation.

#### Scenario: Configured drain expires
- **WHEN** downstream cancellation occurs and the immediate upstream never closes or remains continuously ready
- **THEN** the Agent Observability-owned drain goroutine SHALL exit no later than the configured absolute deadline
- **AND** generation finalization and downstream channel closure SHALL each occur exactly once without waiting for the drain

### Requirement: Consumer-owned final generation policy

Recording options SHALL offer an optional generation filter invoked once per mapped unary or streaming generation, including a successful nil stream, with the mapped generation and observed provider finish reason. Nil SHALL preserve mapping. The filter SHALL NOT mutate provider call options, results, or stream parts. A panic SHALL be recovered, produce only a minimal generation seeded with requested model identity, and leave the model call result unchanged.

#### Scenario: Gateway filter drops mapper-only private metadata
- **WHEN** a consumer filter receives a generation containing provider-option metadata, raw usage metadata, response identity, tags, or a provider-native finish reason
- **THEN** the filter MAY retain only its approved normalized observation fields and the unified finish value
- **AND** the original model result or stream parts SHALL remain unchanged

#### Scenario: Filter panic remains fail-open
- **WHEN** a configured generation filter panics
- **THEN** recording SHALL finalize a minimal requested-identity generation or report a local recording error
- **AND** the model call result, error, or stream SHALL remain unchanged

### Requirement: Provided-only recording context
Agent Observability recording options SHALL provide a context-source mode whose zero value preserves existing ambient fallbacks. Provided-only mode SHALL retain cancellation, deadlines, and the active OpenTelemetry parent while preventing the Agent Observability client and mapper from reading arbitrary request values or Agent Observability conversation, user, agent, tag, experiment, generation, and parent-generation context helpers. The original context SHALL still reach the wrapped model.

#### Scenario: Ambient observation values are isolated
- **WHEN** provided-only mode receives a context containing Agent Observability values and an unrelated provider value
- **THEN** the exported generation and span SHALL omit the ambient Agent Observability values
- **AND** the provider SHALL still receive its unrelated value, cancellation, deadline, and the generated recording span

### Requirement: Fail-open local recording diagnostics
Agent Observability recording options SHALL provide an optional record-error handler invoked after finalization when local validation or enqueueing fails and an optional completion handler invoked once after every started recorder ends. Handler panics SHALL be recovered, and neither recorder failure nor handler failure SHALL alter the model call result or stream. Streaming recorders SHALL end before downstream EOF while completion/error callbacks SHALL NOT delay that EOF.

#### Scenario: Queue failure is reported without failing traffic
- **WHEN** a recorder cannot enqueue a finalized generation
- **THEN** the handler SHALL receive the local error once
- **AND** the original model response SHALL remain unchanged even if the handler panics

#### Scenario: Process owner tracks an abandoned stream
- **WHEN** a stream consumer stops reading and request cancellation ends its recorder asynchronously
- **THEN** the completion handler SHALL run exactly once after the generation is finalized
- **AND** a blocking completion or error handler SHALL NOT delay downstream EOF

### Requirement: Agent Observability option and context types

The package SHALL export concrete wrapping, recording, hooks, and context types.

#### Scenario: Configure wrapping and hook evaluation
- **WHEN** a caller configures `Wrap`, `Stack`, `RecordingMiddleware`, or `HooksMiddleware`
- **THEN** exported types SHALL include `WrapOptions` (struct, top-level options for `Wrap` / `Stack`), `RecordingOptions` (struct, options for `RecordingMiddleware`), and `HooksOptions` (struct, options for `HooksMiddleware`, including `MaxLatency time.Duration` and an `Enabled func(ctx context.Context) bool` opt)

#### Scenario: Supply request-scoped observability context
- **WHEN** a caller supplies context metadata and client resolution
- **THEN** `ContextInfo` SHALL be a struct with fields `UserID string`, `Metadata map[string]any`, `Tags map[string]string`, `AgentName string`, `AgentVersion string`
- **AND** `ClientResolver` SHALL be a type alias for `func(ctx context.Context) *agento11y.Client`
- **AND** `ContextProvider` SHALL be a type alias for `func(ctx context.Context) ContextInfo`

#### Scenario: Inspect a hook denial
- **WHEN** a caller inspects `HookDenialError`
- **THEN** the package SHALL expose it as a struct with fields `Reason string`, `RuleID string`, `Cause error`

### Requirement: Agent Observability constructor and mapper signatures

The package SHALL export direct constructors, wrapping helpers, and generation mapping functions.

#### Scenario: Construct and wrap middleware
- **WHEN** a caller constructs Agent Observability middleware or wraps a model
- **THEN** the package SHALL export `RecordingMiddleware(opts RecordingOptions) middleware.Middleware`, `HooksMiddleware(opts HooksOptions) middleware.Middleware`, `Stack(opts WrapOptions) []middleware.Middleware`, and `Wrap(base provider.LanguageModel, opts WrapOptions) provider.LanguageModel`

#### Scenario: Map a generate result or start a generation
- **WHEN** a caller maps a provider result or builds a generation start
- **THEN** the package SHALL export `MapGenerateResult(params provider.CallOptions, result *provider.GenerateResult, ctxInfo ContextInfo) agento11y.Generation`
- **AND** it SHALL export `BuildGenerationStart(ctx context.Context, providerName, modelID string, ctxInfo ContextInfo) agento11y.GenerationStart`

### Requirement: Agent Observability stream recorder API

The package SHALL export the `StreamRecorder` struct and its constructor and observation methods.

#### Scenario: Observe a stream through the recorder API
- **WHEN** a caller constructs and queries a `StreamRecorder`
- **THEN** the package SHALL export `NewStreamRecorder(start agento11y.GenerationStart, params provider.CallOptions) *StreamRecorder`, `(*StreamRecorder).Observe(part provider.StreamPart)`, `(*StreamRecorder).FirstChunkAt() time.Time`, and `(*StreamRecorder).Generation() agento11y.Generation`

### Requirement: Inclusive Agent Observability token usage

`Usage` SHALL map normalized `result.Usage` and set `InputSemantics` to `TokenInputSemanticsInclusive`. Input totals already include cache reads/writes; cache buckets SHALL NOT be added again. Total tokens SHALL equal normalized input plus output. This SHALL apply to generate mapping, observed streamed usage, generation export, and client-owned token telemetry. Streams with no observed usage MAY leave usage and its semantics marker unset.

#### Scenario: Cache tokens are not double counted
- **WHEN** normalized usage reports input, cache-read, cache-write, and output tokens
- **THEN** Agent Observability usage SHALL retain inclusive input semantics and total tokens SHALL equal input plus output without adding cache buckets again
- **AND** generate mapping, streamed usage, export, and client-owned token telemetry SHALL use the same contract

#### Scenario: Stream has no usage
- **WHEN** no usage is observed in a stream
- **THEN** its usage value and semantics marker MAY remain unset

### Requirement: Recorded media data and MIME resolution

Byte/base64 media SHALL become base64 data URLs. Valid data and HTTP(S) URLs SHALL remain verbatim; recording SHALL NOT fetch remote URLs. Concrete MIME resolution SHALL use declared media type, data URL, filename, URL path, then sniffed inline bytes, in that order. Percent-escaped data-URL payloads SHALL be decoded for validation without rewriting the URL; base64 with CR/LF SHALL be malformed.

#### Scenario: Infer image MIME and retain a remote URL
- **WHEN** a supported media file has no concrete declared type but its HTTP(S) URL path identifies its MIME type
- **THEN** the mapper SHALL resolve MIME in the specified precedence order and retain the URL verbatim without fetching it

#### Scenario: Reject line breaks in base64
- **WHEN** inline base64 contains CR or LF
- **THEN** the mapper SHALL treat it as malformed and skip the media

### Requirement: Unsafe recorded media exclusion

The mapper SHALL skip multiple data sources, provider references, inline text file data, malformed base64/data URLs, URL credentials, non-HTTP(S) remote schemes, unsupported/ambiguous media, and conflicting concrete declared/data-URL MIME types.

#### Scenario: Ambiguous or unsafe data cannot become media
- **WHEN** file data has multiple sources, a provider reference, inline text, malformed encoding, URL credentials, a non-HTTP(S) remote scheme, unsupported/ambiguous media, or conflicting concrete declared and data-URL MIME types
- **THEN** the mapper SHALL add no media part for that data

### Requirement: Agent Observability first semantic output timestamp

`FirstChunkAt()` SHALL record the first semantic model output: non-empty text, reasoning, and tool-input deltas, tool calls, and supported file events. Finish, error, tool-result, metadata, and empty-delta events SHALL NOT establish time to first token.

#### Scenario: Non-output events do not establish first token time
- **WHEN** finish, error, tool-result, metadata, or empty-delta events precede the first non-empty text, reasoning, or tool-input delta, tool call, or supported file event
- **THEN** `FirstChunkAt()` SHALL be established by that first payload-bearing event, not by the preceding events

### Requirement: Recording cancellation precedence and producer ownership

The recording goroutine SHALL select on `ctx.Done()` to avoid blocking on consumer disconnect. Cancellation before observed upstream completion SHALL be the call error, taking precedence over an earlier `PartError`. The middleware SHALL NOT start an unbounded detached drain if the provider ignores cancellation; provider producers SHALL be responsible for honoring the call context.

#### Scenario: Cancellation supersedes an earlier stream error
- **WHEN** a stream emits `PartError` and the context cancels before observed upstream completion
- **THEN** recording SHALL use cancellation as the call error
- **AND** the recording goroutine SHALL avoid blocked downstream sends without starting an unbounded detached drain

### Requirement: Hook reasoning and provider-part preservation

An unchanged reasoning part MAY reuse the exact original to preserve its provider signature only via an unambiguous unused part with identical text; changed or ambiguous signed reasoning SHALL fail closed. Unchanged provider-executed calls and provider-specific results SHALL retain provider fields only after exact ID, name, payload, and discriminator matching. Unmatchable provider-specific parts, including discarded required discriminators, SHALL fail closed.

#### Scenario: Signed reasoning cannot be ambiguously restored
- **WHEN** transformed signed reasoning is changed or has ambiguous identical-text matches
- **THEN** reconstruction SHALL fail closed rather than reuse an original signature

#### Scenario: Provider-specific parts require exact matches
- **WHEN** transformed provider-executed tool calls or provider-specific results differ in ID, name, payload, or required provider discriminator
- **THEN** reconstruction SHALL fail closed instead of retaining provider fields without an exact match

### Requirement: Hook undisclosed content and tool reconstruction

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
