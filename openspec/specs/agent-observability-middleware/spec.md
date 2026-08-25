# Agent Observability Middleware

## Purpose

Define the provider-agnostic middleware that records AI SDK model calls in
Grafana Agent Observability and evaluates preflight policy hooks while preserving
the underlying agento11y SDK wire and telemetry contracts.

## Requirements

### Requirement: Nested Go module for the Agent Observability middleware

`middleware/agentobservability/` SHALL be a separate Go module under the ai-sdk repository, declared with `module github.com/grafana/ai-sdk/middleware/agentobservability` and `replace github.com/grafana/ai-sdk => ../../`, mirroring the existing `providers/<name>/` nested-module convention.

The module SHALL depend on `github.com/grafana/ai-sdk` (root) and `github.com/grafana/agento11y/go`. It SHALL NOT depend on `github.com/grafana/ai-sdk/providers/anthropic` or any other provider module.

The root ai-sdk module SHALL NOT import any symbol from `middleware/agentobservability/`.

`middleware/agentobservability/doc.go` SHALL document the public API surface and the convention that heavy middlewares with vendor SDK / gRPC / OTel dependencies live in nested modules under `middleware/`.

#### Scenario: Root module does not pull agento11y

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk` (root)
- **THEN** `github.com/grafana/agento11y/go` SHALL NOT appear in the consumer's transitive dependency graph

#### Scenario: Agent Observability middleware does not import providers/anthropic

- **WHEN** running `cd middleware/agentobservability && go list -deps ./...`
- **THEN** the output SHALL NOT contain `github.com/grafana/ai-sdk/providers/anthropic`

### Requirement: Public API surface

The `middleware/agentobservability` package SHALL export the following symbols. The names and shapes below are normative; renames during implementation require updating this spec.

Types:
- `WrapOptions` (struct): top-level options for `Wrap` / `Stack`.
- `RecordingOptions` (struct): options for `RecordingMiddleware`.
- `HooksOptions` (struct): options for `HooksMiddleware`, including `MaxLatency time.Duration` and an `Enabled func(ctx context.Context) bool` opt.
- `ContextInfo` (struct) with fields `UserID string`, `Metadata map[string]any`, `Tags map[string]string`, `AgentName string`, `AgentVersion string`.
- `ClientResolver` (type alias): `func(ctx context.Context) *agento11y.Client`.
- `ContextProvider` (type alias): `func(ctx context.Context) ContextInfo`.
- `StreamRecorder` (struct).
- `HookDenialError` (struct) with fields `Reason string`, `RuleID string`, `Cause error`.

Functions:
- `RecordingMiddleware(opts RecordingOptions) middleware.Middleware`.
- `HooksMiddleware(opts HooksOptions) middleware.Middleware`.
- `Stack(opts WrapOptions) []middleware.Middleware`.
- `Wrap(base provider.LanguageModel, opts WrapOptions) provider.LanguageModel`.
- `MapGenerateResult(params provider.CallOptions, result *provider.GenerateResult, ctxInfo ContextInfo) agento11y.Generation`.
- `BuildGenerationStart(ctx context.Context, providerName, modelID string, ctxInfo ContextInfo) agento11y.GenerationStart`.
- `NewStreamRecorder(start agento11y.GenerationStart, params provider.CallOptions) *StreamRecorder`.
- `(*StreamRecorder).Observe(part provider.StreamPart)`.
- `(*StreamRecorder).FirstChunkAt() time.Time`.
- `(*StreamRecorder).Generation() agento11y.Generation`.

Context helpers:
- `WithGenerationID(ctx context.Context, id string) context.Context`.
- `GenerationIDFromContext(ctx context.Context) string`.
- `NewGenerationID() string`.
- `WithParentGenerationIDs(ctx context.Context, ids ...string) context.Context`.
- `ParentGenerationIDsFromContext(ctx context.Context) []string`.
- `WithLinkedGenerationID(ctx context.Context, id string) context.Context`.

Sentinel error:
- `ErrHookDenied error`.

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

`RecordingMiddleware` SHALL call `opts.ClientResolver(ctx)` once per request to
obtain the `*agento11y.Client`. `HooksMiddleware` SHALL do the same after the
request passes its `Enabled` gate; a disabled request SHALL not resolve a
client. A `nil` return value SHALL cause the middleware to become a no-op for
that request: the inner model is invoked unchanged, no Generation is started,
and no `EvaluateHook` is called.

If `opts.ClientResolver` is itself `nil`, the middleware SHALL behave as if every resolution returns `nil`.

#### Scenario: ClientResolver returns nil

- **WHEN** `ClientResolver(ctx)` returns `nil` for a request
- **THEN** the wrapped model SHALL produce identical output to the unwrapped base model
- **AND** no Agent Observability API calls SHALL be made for that request

#### Scenario: ClientResolver is nil

- **WHEN** `opts.ClientResolver` is `nil`
- **THEN** `Wrap`/`Stack` SHALL still return a valid `provider.LanguageModel` / middleware slice
- **AND** every request SHALL pass through unchanged

### Requirement: ContextProvider supplies request-scoped metadata

`RecordingMiddleware` SHALL call `opts.ContextProvider(ctx)` once per request and use the returned `ContextInfo` to populate:
- `agento11y.GenerationStart.UserID` (falls back to `agento11y.UserIDFromContext(ctx)` when `ContextInfo.UserID` is empty).
- `agento11y.GenerationStart.Metadata` (merged on top of metadata derived from `ProviderOptions` and `params`).
- `agento11y.GenerationStart.Tags` (merged on top of any tags carried via context).
- `agento11y.GenerationStart.AgentName` / `AgentVersion` (override `agento11y.AgentNameFromContext` / `agento11y.AgentVersionFromContext` when set).

A zero `ContextInfo` SHALL be tolerated; every field SHALL fall back to the appropriate `agento11y.*FromContext` helper or remain unset.

If `opts.ContextProvider` is `nil`, the middleware SHALL log a warning at most once per process and continue with all-`agento11y.*FromContext`-derived defaults.

#### Scenario: Zero ContextInfo falls back to context helpers

- **WHEN** `ContextProvider(ctx)` returns a zero `ContextInfo`
- **THEN** `GenerationStart.UserID` SHALL equal `agento11y.UserIDFromContext(ctx)`
- **AND** `GenerationStart.AgentName` SHALL equal `agento11y.AgentNameFromContext(ctx)`

#### Scenario: Nil ContextProvider logs once

- **WHEN** `RecordingMiddleware` is invoked for the first time with `opts.ContextProvider == nil`
- **THEN** a Warn-level log message SHALL be emitted exactly once for the lifetime of the process

### Requirement: MapGenerateResult produces agento11y.Generation

`MapGenerateResult(params, result, ctxInfo)` SHALL produce an `agento11y.Generation` whose:
- `Input.Messages` is derived from `params.Prompt`, with `provider.Message{Role: RoleSystem}` entries folded into `Generation.SystemPrompt` (single concatenated string) rather than appearing as an agento11y.Message. Empty reasoning parts SHALL be omitted.
- `Input.Tools` is derived from `params.Tools`. Function tools map directly; provider-defined tools (e.g. Anthropic `web_search`, `code_execution`) MAY map with their type preserved so Agent Observability can annotate them.
- `Input.MaxTokens`, `Temperature`, `TopP`, `ToolChoice` are derived from the corresponding `provider.CallOptions` fields.
- Anthropic thinking-budget metadata (`agento11y.gen_ai.request.thinking.budget_tokens`) is derived from `params.ProviderOptions["anthropic"]` via `json.RawMessage` decoding, not by importing `providers/anthropic`.
- `Output` is a single assistant `agento11y.Message` whose parts mirror supported `result.Content` entries (text, tool-call, reasoning, file, and reasoning-file parts). Empty reasoning parts SHALL be omitted.
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

### Requirement: Recording maps file parts to Agent Observability media

Recording SHALL map supported `file` and `reasoning-file` content from prompts, generated results, and provider streams to `agento11y.PartKindMedia` parts without changing the model request, provider result, or provider/UI wire types. The recorded media metadata SHALL preserve whether the source was a `file` or `reasoning-file` part.

Only image and video media SHALL be recorded. Byte and base64 payloads SHALL be converted to base64 data URLs. Valid data URLs and HTTP(S) URLs SHALL be retained verbatim; recording SHALL NOT fetch remote URLs. The mapper SHALL determine a concrete MIME type from the declared media type, data URL, filename, URL path, or sniffed inline bytes, in that order.

The mapper SHALL skip data with multiple sources, provider references, inline text file data, malformed base64 or data URLs, URL credentials, non-HTTP(S) remote schemes, unsupported or ambiguous media, and conflicting concrete declared and data-URL MIME types. Percent-escaped data-URL payloads SHALL be decoded for validation without changing the retained URL. Base64 containing CR or LF SHALL be treated as malformed.

Hook preflight evaluation SHALL exclude `file` and `reasoning-file` media so recording support does not widen the hook disclosure boundary. Metadata-only agento11y export SHALL omit media URLs.

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

`StreamRecorder` SHALL accumulate `agento11y.Generation.Output` from a sequence of `provider.StreamPart` values observed via `Observe`. It SHALL:
- Append `PartTextDelta` payloads into the active assistant text part.
- Append non-empty `PartReasoningDelta` payloads into the active assistant reasoning part. Signature-only reasoning blocks with no visible text SHALL NOT produce an Agent Observability thinking part.
- Append `PartToolCallDelta` payloads into the active assistant tool-call part.
- Map supported `PartFile` and `PartReasoningFile` events to media parts.
- Record the first observed payload-bearing part's timestamp via `FirstChunkAt()`; supported file events SHALL be payload-bearing.
- Capture `FinishReason` from `PartFinish` and observe `Usage` from every usage-bearing stream part using the shared streaming aggregation behavior.

`Generation()` SHALL return an `agento11y.Generation` whose `Output` is a single assistant message constructed from the accumulated state. Assistant text, reasoning, tool-call, and media parts SHALL retain the order in which their first provider events were observed.

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

- **GIVEN** a stream emits multiple `PartToolCallDelta` events for the same tool-call ID with incremental JSON argument fragments
- **WHEN** the recorder is observed
- **THEN** the resulting tool-call part SHALL have the concatenated argument JSON

#### Scenario: Streamed media preserves observed assistant-part order

- **GIVEN** a stream interleaves supported file, text, reasoning, tool-call, and reasoning-file events
- **WHEN** the recorder observes the stream and produces a generation
- **THEN** the assistant output parts SHALL follow the order in which each part was first observed
- **AND** the first supported file event SHALL set `FirstChunkAt()` when no earlier payload-bearing event was observed

### Requirement: Recording uses response model identity when available

Agent Observability recording SHALL use backend response metadata as the canonical generation model identity when a successful provider response supplies both provider and model ID. When response metadata omits either provider or model ID, recording SHALL preserve the model identity from `GenerationStart`.

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

`RecordingMiddleware(opts)` SHALL return a `middleware.Middleware` whose `WrapGenerate` and `WrapStream` hooks:
1. Resolve a client via `opts.ClientResolver`. If `nil`, pass through to the inner model unchanged.
2. Build a `agento11y.GenerationStart` via `BuildGenerationStart(ctx, model.Provider(), model.ModelID(), opts.ContextProvider(ctx))`.
3. Call `client.StartGeneration` (for `WrapGenerate`) or `client.StartStreamingGeneration` (for `WrapStream`).
4. Invoke the inner model.
5. On success:
   - For generate: call `recorder.SetResult(MapGenerateResult(params, result, ctxInfo))`.
   - For stream: tee the result stream channel, feed each part to a `StreamRecorder`, and at end-of-stream call `recorder.SetResult(streamRecorder.Generation())`.
6. On an error returned before a stream opens: call `recorder.SetCallError(err)`. When a stream emits `PartError`, call `recorder.SetCallError(err)` and also call `recorder.SetResult` with the partial generation, including aggregated usage observed before or on the error part.

`RecordingMiddleware` SHALL NOT modify `params` and SHALL NOT modify the result.

For streams, the recording goroutine SHALL select on `ctx.Done()` to avoid blocking on consumer disconnect, and SHALL drain the upstream channel after consumer abandonment to avoid leaking the producer goroutine.

#### Scenario: Generate path records on success

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil result and `nil` error
- **THEN** the middleware SHALL call `StartGeneration` once
- **AND** SHALL call `recorder.SetResult` once with `MapGenerateResult(params, result, ctxInfo)`
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

#### Scenario: Stream goroutine cleans up on consumer disconnect

- **WHEN** the consumer abandons the result stream by cancelling its context before the upstream completes
- **THEN** the recording goroutine SHALL NOT block indefinitely
- **AND** the upstream channel SHALL be drained so the producer can return

### Requirement: HooksMiddleware enforces preflight policy

`HooksMiddleware(opts)` SHALL return a `middleware.Middleware` whose `WrapGenerate` and `WrapStream` hooks:
1. If `opts.Enabled` is non-nil and returns `false` for the request context, pass through to the inner model unchanged.
2. Resolve a client via `opts.ClientResolver`. If `nil`, pass through unchanged.
3. Build an `agento11y.HookEvaluateRequest` from `params` (phase = preflight), excluding `file` and `reasoning-file` media.
4. Call `client.EvaluateHook(ctx, request)`. If `opts.MaxLatency > 0`, the call SHALL be bounded by `context.WithTimeout(ctx, opts.MaxLatency)`; otherwise the request context SHALL be inherited unchanged.
5. Branch on the response:
   - **Deny**: return `&HookDenialError{Reason, RuleID, Cause: nil}` to the caller. The inner model SHALL NOT be invoked.
   - **Allow**: invoke the inner model with `params` unchanged.
   - **TransformedInput**: treat the transformed input as an authoritative replacement, rebuild `params.Prompt` and the retained subset of `params.Tools`, and invoke the inner model with the new params. If any returned content cannot be reconstructed without loss or reintroducing omitted content, return `ErrHookTransformFailed` without invoking the model.

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

### Requirement: Hook transforms are authoritative and lossless

When `EvaluateHook` returns a non-nil `TransformedInput`, `HooksMiddleware` SHALL treat it as an authoritative replacement rather than a partial patch:

1. A non-empty `SystemPrompt` SHALL become one system message. An empty `SystemPrompt` SHALL carry no original system message forward.
2. Every transformed message SHALL be rebuilt in returned order. Unknown roles, unsupported part kinds, empty payload parts, and malformed tool payloads SHALL fail with `ErrHookTransformFailed`.
3. Omitted assistant parts SHALL remain omitted. The middleware SHALL NOT restore an entire original assistant message based only on visible text.
4. An unchanged reasoning part MAY reuse the exact original part to preserve its provider signature. Matching SHALL use an unambiguous unused reasoning part with identical reasoning text; changed or ambiguous signed reasoning SHALL fail closed.
5. Unchanged provider-executed tool calls and provider-specific tool results SHALL retain their provider fields only after an exact ID, name, and payload match. A provider-specific part that cannot be matched exactly SHALL fail closed.
6. Because hook evaluation intentionally excludes media, message-level provider options, text-part provider options, empty reasoning metadata, and other unsupported content, a transform of a prompt containing undisclosed content SHALL fail closed rather than silently dropping or restoring it.
7. Returned tools SHALL be matched exactly to disclosed original tool definitions. Exact retained tools MAY be preserved or reordered and omitted tools SHALL be removed; new or modified tools that cannot be reconstructed losslessly SHALL fail closed. Removing tools SHALL also fail closed when it leaves a required or specifically named `ToolChoice` unsatisfied.
8. An empty transform SHALL fail closed. A system-only replacement is valid.

#### Scenario: Empty transform fails closed

- **GIVEN** a non-empty original prompt
- **AND** `EvaluateHook` returns a non-nil but empty `TransformedInput`
- **WHEN** the transform is applied
- **THEN** `ErrHookTransformFailed` SHALL be returned
- **AND** the inner model SHALL NOT be invoked with the original prompt

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

### Requirement: Generation-ID DAG context helpers

The following context helpers SHALL be exposed from `middleware/agentobservability`:

- `WithGenerationID(ctx, id)` / `GenerationIDFromContext(ctx)` — current generation ID for the call about to be made.
- `WithParentGenerationIDs(ctx, ids...)` / `ParentGenerationIDsFromContext(ctx)` — upstream generations whose output this call depends on. Used by Agent Observability to build the parent → child DAG.
- `WithLinkedGenerationID(ctx, id)` — sibling/peer link (e.g. an evaluation generation that complements a primary generation).
- `NewGenerationID()` — generates a new opaque generation ID suitable for `WithGenerationID`.

`RecordingMiddleware` SHALL read `GenerationIDFromContext(ctx)` and use it as `GenerationStart.ID` when non-empty. It SHALL read `ParentGenerationIDsFromContext(ctx)` and pass them through as `GenerationStart.ParentGenerationIDs`.

#### Scenario: GenerationID flows into the recorder

- **GIVEN** a context with `WithGenerationID(ctx, "gen-123")` applied
- **WHEN** `RecordingMiddleware` invokes the inner model on that context
- **THEN** the resulting `GenerationStart.ID` SHALL equal `"gen-123"`

#### Scenario: ParentGenerationIDs flow into the recorder

- **GIVEN** a context with `WithParentGenerationIDs(ctx, "p1", "p2")` applied
- **WHEN** `RecordingMiddleware` invokes the inner model on that context
- **THEN** the resulting `GenerationStart.ParentGenerationIDs` SHALL contain exactly `["p1", "p2"]` in that order

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
