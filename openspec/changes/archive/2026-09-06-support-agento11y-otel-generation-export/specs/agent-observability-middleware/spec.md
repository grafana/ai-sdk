## RENAMED Requirements

- FROM: `OTel span shape`
- TO: `Generation spans follow the resolved client protocol`

## MODIFIED Requirements

### Requirement: MapGenerateResult produces agento11y.Generation

`MapGenerateResult(params, result, ctxInfo)` SHALL produce an `agento11y.Generation` whose:
- `Input.Messages` is derived from `params.Prompt`, with `provider.Message{Role: RoleSystem}` entries folded into `Generation.SystemPrompt` (single concatenated string) rather than appearing as an agento11y.Message. Empty reasoning parts SHALL be omitted.
- `Input.Tools` is derived from `params.Tools`. Function tools map directly; provider-defined tools (e.g. Anthropic `web_search`, `code_execution`) MAY map with their type preserved so Agent Observability can annotate them.
- `Input.MaxTokens`, `Temperature`, `TopP`, `ToolChoice` are derived from the corresponding `provider.CallOptions` fields.
- Anthropic thinking-budget metadata (`agento11y.gen_ai.request.thinking.budget_tokens`) is derived from `params.ProviderOptions["anthropic"]` via `json.RawMessage` decoding, not by importing `providers/anthropic`.
- `Output` contains an assistant `agento11y.Message` for supported model content and additional tool-role messages for tool-result entries. Empty reasoning parts SHALL be omitted.
- `Usage` maps from `result.Usage` (input tokens, output tokens, cache hits where applicable). When `result.Usage.InputTokens.Total` is present, `Usage.InputSemantics` SHALL be `agento11y.TokenInputSemanticsInclusive`. When no input total is present, input semantics SHALL remain unspecified.
- `StopReason` is produced by `finishReasonToAgento11yStop(result.FinishReason)` and SHALL match the string values the legacy `internal/llm/claude/` path emitted (e.g. `"end_turn"`, `"max_tokens"`, `"tool_use"`, `"stop_sequence"`).
- `Metadata` starts with caller metadata, then applies reserved request and usage derivations. Derived Anthropic thinking-budget and positive server-tool request counts SHALL override conflicting caller values, matching the pinned agento11y Anthropic helper.
- Provider tool calls and results SHALL retain recoverable Anthropic discriminators, including MCP metadata and configured provider-tool aliases for web search, web fetch, code execution, and tool search. Irrecoverable provider subtypes MAY use the generic discriminator.
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

#### Scenario: Anthropic server-tool usage is recorded

- **WHEN** `result.Usage.Raw` contains positive `server_tool_use.web_search_requests` or `web_fetch_requests`
- **THEN** generation metadata SHALL contain the corresponding Agent Observability usage keys and their sum as `total_requests`

#### Scenario: Reported input usage is inclusive

- **WHEN** `result.Usage.InputTokens.Total` is present
- **THEN** `Generation.Usage.InputSemantics` SHALL equal `agento11y.TokenInputSemanticsInclusive`

#### Scenario: Unreported input usage has no semantics

- **WHEN** `result.Usage.InputTokens.Total` is absent
- **THEN** `Generation.Usage.InputSemantics` SHALL remain unspecified

#### Scenario: Byte-equal output to agento11y anthropic helper

- **GIVEN** a recorded canonical request that has both an `anthropic.MessageNewParams` form and an equivalent `provider.CallOptions` form
- **WHEN** the Anthropic form is passed through `agento11y/go-providers/anthropic.FromRequestResponse`
- **AND** the ai-sdk form is passed through `MapGenerateResult`
- **THEN** both resulting `agento11y.Generation` payloads SHALL produce byte-equal JSON modulo the fields `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

### Requirement: RecordingMiddleware wraps generate and stream

`RecordingMiddleware(opts)` SHALL return a `middleware.Middleware` whose `WrapGenerate` and `WrapStream` hooks:
1. Resolve a client via `opts.ClientResolver`. If `nil`, pass through to the inner model unchanged.
2. Build a `agento11y.GenerationStart` via `BuildGenerationStart(ctx, model.Provider(), model.ModelID(), opts.ContextProvider(ctx))`.
3. Call `client.StartGeneration` (for `WrapGenerate`) or `client.StartStreamingGeneration` (for `WrapStream`).
4. Invoke the inner model.
5. On success:
   - For generate: map the result with the original `GenerationStart` so requested-model fallback is available, then call `recorder.SetResult`.
   - For stream: tee the result stream channel, feed each part to a `StreamRecorder`, and at end-of-stream call `recorder.SetResult(streamRecorder.Generation())`.
6. On an error returned before a stream opens: call `recorder.SetCallError(err)`. When a stream emits `PartError`, call `recorder.SetCallError(err)` and also call `recorder.SetResult` with the partial generation, including aggregated usage observed before or on the error part.

`RecordingMiddleware` SHALL NOT modify `params` and SHALL NOT modify the result.

For streams, the recording goroutine SHALL select on the context supplied to `DoStream`. If that context is cancelled before upstream closure is observed, the middleware SHALL stop forwarding and reading upstream, record the context error, finalize the partial generation, and close the downstream channel. On normal closure and cancellation, recorder finalization SHALL finish before the downstream channel closes. Cancellation SHALL take precedence over an earlier `PartError`. Stopping consumption of the downstream channel alone SHALL NOT cancel the stream. The middleware SHALL NOT start a detached upstream drain; provider stream producers are responsible for honoring the call context.

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
- **AND** `recorder.SetResult` and `recorder.End` SHALL finish after upstream closure and before the downstream channel closes

#### Scenario: Stream cancellation records an error and cleans up

- **WHEN** the context supplied to `DoStream` is cancelled before the middleware observes upstream closure
- **THEN** the middleware SHALL stop forwarding and reading upstream
- **AND** the generation SHALL record the context cancellation as its call error
- **AND** the middleware SHALL finalize the partial generation and close the downstream channel
- **AND** the middleware SHALL NOT start a detached upstream drain

### Requirement: Hook transforms are authoritative and lossless

When `EvaluateHook` returns a non-nil `TransformedInput`, `HooksMiddleware` SHALL treat it as an authoritative replacement rather than a partial patch:

1. A non-empty `SystemPrompt` SHALL become one system message. An empty `SystemPrompt` SHALL carry no original system message forward.
2. Every transformed message SHALL be rebuilt in returned order. Unknown roles, unsupported part kinds, empty payload parts, and malformed tool payloads SHALL fail with `ErrHookTransformFailed`.
3. Omitted assistant parts SHALL remain omitted. The middleware SHALL NOT restore an entire original assistant message based only on visible text.
4. An unchanged reasoning part MAY reuse the exact original part to preserve its provider signature. Matching SHALL use an unambiguous unused reasoning part with identical reasoning text; changed or ambiguous signed reasoning SHALL fail closed.
5. Unchanged provider-executed tool calls and provider-specific tool results SHALL retain their provider fields only after an exact ID, name, and payload match. A provider-specific part that cannot be matched exactly SHALL fail closed.
6. Because hook evaluation intentionally excludes media, message-level provider options, text-part provider options, empty reasoning metadata, and other unsupported content, a transform of a prompt containing undisclosed content SHALL fail closed rather than silently dropping or restoring it.
7. Returned tools SHALL be matched exactly to disclosed original tool definitions. Exact retained tools MAY be preserved or reordered and omitted tools SHALL be removed; new or modified tools that cannot be reconstructed losslessly SHALL fail closed. Removing tools SHALL also fail closed when it leaves a required or specifically named `ToolChoice` unsatisfied.
8. A system-only replacement is valid.

Agento11y v0.18 converts unsupported string and numeric response roles to `user` before `HooksMiddleware` can validate them. Until agento11y preserves unsupported roles, each transformed user message SHALL match one unused original user message exactly. A changed or new transformed user message SHALL fail with `ErrHookTransformFailed`. System-prompt, assistant-message, tool-message, and tool-list transformations MAY proceed when their transformed user messages satisfy this rule.

Agento11y v0.18 decodes an HTTP response containing an empty `transformed_input` object as no transform. In that case, `HooksMiddleware` SHALL invoke the model with the original prompt and tools.

#### Scenario: Empty server transform is no transform

- **GIVEN** an original prompt and tools
- **WHEN** the Agent Observability server returns `{"action":"allow","transformed_input":{}}`
- **THEN** the inner model SHALL be invoked with the original prompt and tools unchanged

#### Scenario: Unsupported response role fails closed

- **GIVEN** an original user message
- **WHEN** agento11y v0.18 converts a transformed `system`, unknown string, or unknown numeric role to a changed user message
- **THEN** `HooksMiddleware` SHALL return `ErrHookTransformFailed`
- **AND** the model SHALL NOT be invoked

#### Scenario: Unchanged user message permits other transforms

- **GIVEN** a transformed user message that exactly matches an unused original user message
- **WHEN** the hook changes only the system prompt, assistant messages, tool messages, or retained tools
- **THEN** `HooksMiddleware` MAY apply the other transformation

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

### Requirement: Generation spans follow the resolved client protocol

`RecordingMiddleware` SHALL use `StartGeneration` or `StartStreamingGeneration` on the resolved agento11y client. It SHALL pass the returned context to the inner model so the provider call runs under the client-owned generation span. Other context values SHALL remain available to the provider.

For gRPC and HTTP generation export, `RecordingMiddleware` SHALL finalize the client-owned recorder with the mapped generation. The resolved agento11y client owns validation, queueing, and transport. Recorder completion SHALL NOT acknowledge that the Agent Observability API accepted the generation. Agento11y SHALL also emit one CLIENT metadata span. The span name SHALL be `generateText <model>` for unary calls or `streamText <model>` for streaming calls when the model is known. Its `gen_ai.operation.name` SHALL be `generateText` or `streamText`.

For OTel generation export, agento11y SHALL emit no separate generation payload. It SHALL emit one CLIENT generation span named `chat <model>` with `gen_ai.operation.name="chat"`. The span SHALL carry `agento11y.record="true"`, `agento11y.generation.id`, mapped generation attributes, and content allowed by the resolved capture mode. Streaming spans SHALL carry `gen_ai.request.stream=true`.

The middleware SHALL NOT create a second generation span. It SHALL NOT flush the application's OTel provider after a model call.

Error states on the generation path SHALL reach the trace through `recorder.SetCallError(err)`. Agento11y SHALL put `error.type` and `error.category` on its generation span. The middleware SHALL NOT add generation error attributes itself.

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

- **WHEN** the context supplied to `DoStream` is cancelled before upstream closure is observed
- **THEN** the middleware SHALL stop forwarding provider parts
- **AND** it SHALL finalize the partial generation with the context error
- **AND** the generation span and downstream channel SHALL close without a detached upstream drain

#### Scenario: OTel content follows the capture mode

- **WHEN** the resolved capture mode is `metadata_only`
- **THEN** the generation span SHALL omit system instructions, input messages, output messages, tool definitions, and media

- **WHEN** the resolved capture mode is `full_with_metadata_spans`
- **THEN** OTel generation export SHALL apply `full` capture because the generation span is the only generation copy

#### Scenario: Routed OTel generation keeps both identities

- **WHEN** response metadata changes the generation provider and model
- **THEN** the completed `chat <model>` span SHALL use the response provider and model
- **AND** `agento11y.generation.metadata` SHALL retain the wrapped provider and model as `ai_sdk.transport.provider` and `ai_sdk.transport.model`

#### Scenario: Built-in provider names use OTel registry values

- **WHEN** the wrapped or routed provider is `amazon-bedrock`
- **THEN** the mapped generation SHALL use provider `bedrock`
- **AND** the generation or hooks span SHALL use `gen_ai.provider.name="aws.bedrock"`

- **WHEN** the wrapped or routed provider is `anthropic.vertex`
- **THEN** the mapped generation SHALL use provider `vertex`
- **AND** the generation or hooks span SHALL use `gen_ai.provider.name="gcp.vertex_ai"`

#### Scenario: Disabled experimental support does not fall back

- **WHEN** a client selects OTel generation export without enabling agento11y experimental features
- **THEN** the client SHALL export no marked `chat <model>` generation span
- **AND** it SHALL NOT send the generation through gRPC or HTTP

## ADDED Requirements

### Requirement: OTel generation lifecycle remains application-owned

The middleware SHALL NOT construct an OTel exporter, tracer provider, or agento11y client. The application SHALL own these resources and select the sampling policy, export destination, and content capture mode. The application SHALL construct the client, tracer provider, and exporter outside the middleware request path. It MAY set `TracerProvider` on the client to select an explicit generation provider. The application SHALL set `Flusher` to the flush target when `Client.Flush` or `Client.Shutdown` must invoke and wait for `Flusher.ForceFlush`. A successful force-flush SHALL NOT be treated as acknowledgement of remote ingestion.

If no explicit `TracerProvider` or `MeterProvider` is configured, agento11y SHALL use the corresponding global OTel provider for generation export. `Config.Tracer` and `Config.Meter` SHALL NOT select the OTel generation pipeline.

The application SHALL call `Client.Shutdown` before shutting down its tracer provider. Documentation SHALL recommend a dedicated `AlwaysSample` provider when generation storage must not follow request-trace sampling. If the application uses a dedicated generation provider, documentation SHALL explain that `Config.Tracer` can keep tool and embedding spans on the application's normal trace pipeline.

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
- **AND** sampling or span attribute limits MAY drop the generation or remove content

### Requirement: HooksMiddleware owns the preflight span

The only span that this package SHALL open directly is the optional hooks preflight span. The agento11y client neither produces nor reads its `aisdk.hooks.*` attributes.

Span name: `aisdk.hooks.preflight`.

The span also has the `gen_ai.provider.name` and `gen_ai.request.model` semantic-convention attributes.

Span attribute keys:
- `aisdk.hooks.result` (string: `"allow"`, `"deny"`, `"transform"`).
- `aisdk.hooks.action` (string).
- `aisdk.hooks.rule_id` (string, present only on deny).

Every attribute that the middleware sets on this span, other than `gen_ai.*` semantic-convention attributes, SHALL use the `aisdk.hooks.` prefix. The middleware SHALL NOT emit attributes under the agento11y client's `agento11y.*` namespace or under any former product-named namespace.

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
