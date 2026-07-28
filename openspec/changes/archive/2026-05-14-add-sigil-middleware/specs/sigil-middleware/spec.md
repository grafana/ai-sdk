## ADDED Requirements

### Requirement: Nested Go module for the Sigil middleware

`middleware/sigil/` SHALL be a separate Go module under the ai-sdk repository, declared with `module github.com/grafana/ai-sdk/middleware/sigil` and `replace github.com/grafana/ai-sdk => ../../`, mirroring the existing `providers/<name>/` nested-module convention.

The module SHALL depend on `github.com/grafana/ai-sdk` (root) and `github.com/grafana/sigil-sdk/go`. It SHALL NOT depend on `github.com/grafana/ai-sdk/providers/anthropic` or any other provider module.

The root ai-sdk module SHALL NOT import any symbol from `middleware/sigil/`.

`middleware/sigil/doc.go` SHALL document the public API surface and the convention that heavy middlewares with vendor SDK / gRPC / OTel dependencies live in nested modules under `middleware/`.

#### Scenario: Root module does not pull sigil-sdk

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk` (root)
- **THEN** `github.com/grafana/sigil-sdk/go` SHALL NOT appear in the consumer's transitive dependency graph

#### Scenario: Sigil middleware does not import providers/anthropic

- **WHEN** running `go list -deps ./middleware/sigil/...`
- **THEN** the output SHALL NOT contain `github.com/grafana/ai-sdk/providers/anthropic`

### Requirement: Public API surface

The `middleware/sigil` package SHALL export the following symbols. The names and shapes below are normative; renames during implementation require updating this spec.

Types:
- `WrapOptions` (struct): top-level options for `Wrap` / `Stack`.
- `RecordingOptions` (struct): options for `RecordingMiddleware`.
- `HooksOptions` (struct): options for `HooksMiddleware`, including `MaxLatency time.Duration` and an `Enabled func(ctx context.Context) bool` opt.
- `ContextInfo` (struct) with fields `UserID string`, `Metadata map[string]any`, `Tags map[string]string`, `AgentName string`, `AgentVersion string`.
- `ClientResolver` (type alias): `func(ctx context.Context) *sigilsdk.Client`.
- `ContextProvider` (type alias): `func(ctx context.Context) ContextInfo`.
- `StreamRecorder` (struct).
- `HookDenialError` (struct) with fields `Reason string`, `RuleID string`, `Cause error`.

Functions:
- `RecordingMiddleware(opts RecordingOptions) middleware.Middleware`.
- `HooksMiddleware(opts HooksOptions) middleware.Middleware`.
- `Stack(opts WrapOptions) []middleware.Middleware`.
- `Wrap(base provider.LanguageModel, opts WrapOptions) provider.LanguageModel`.
- `MapGenerateResult(params provider.CallOptions, result *provider.GenerateResult, ctxInfo ContextInfo) sigilsdk.Generation`.
- `BuildGenerationStart(ctx context.Context, providerName, modelID string, ctxInfo ContextInfo) sigilsdk.GenerationStart`.
- `NewStreamRecorder(start sigilsdk.GenerationStart, params provider.CallOptions) *StreamRecorder`.
- `(*StreamRecorder).Observe(part provider.StreamPart)`.
- `(*StreamRecorder).FirstChunkAt() time.Time`.
- `(*StreamRecorder).Generation() sigilsdk.Generation`.

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
- **THEN** `errors.Is(err, sigil.ErrHookDenied)` SHALL return `true`

#### Scenario: Wrap is equivalent to middleware.Wrap with Stack

- **WHEN** `sigil.Wrap(base, opts)` and `middleware.Wrap(middleware.WrapOptions{Model: base, Middleware: sigil.Stack(opts)})` are both called with the same opts
- **THEN** both SHALL produce a `provider.LanguageModel` with identical observable behavior

### Requirement: Composition order

`Stack(opts)` SHALL return the middleware slice in the order `[HooksMiddleware, RecordingMiddleware]` (Hooks outer, Recording inner) when both are enabled. When `opts.Hooks.Enabled` resolves to false at construction time, `Stack` SHALL omit the Hooks entry. Recording SHALL always be present in the slice returned by `Stack`.

#### Scenario: Hooks runs before Recording

- **WHEN** a request flows through `Wrap(base, opts)` with both middlewares enabled
- **THEN** `HooksMiddleware.WrapStream` (or `WrapGenerate`) SHALL be entered before `RecordingMiddleware.WrapStream` (or `WrapGenerate`)

#### Scenario: Hook denial short-circuits recording

- **WHEN** `EvaluateHook` returns a deny response
- **THEN** `RecordingMiddleware` SHALL NOT call `StartGeneration` for that request
- **AND** no `sigil.Generation` row SHALL be recorded

#### Scenario: Recording observes post-Hooks params

- **WHEN** `HooksMiddleware` applies a `TransformedInput` that modifies `params.Prompt`
- **THEN** `RecordingMiddleware` SHALL build its `sigil.Generation.Input` from the post-transform prompt, not the original

### Requirement: ClientResolver controls per-request activation

`RecordingMiddleware` and `HooksMiddleware` SHALL each call `opts.ClientResolver(ctx)` once per request to obtain the `*sigilsdk.Client` for that request. A `nil` return value SHALL cause the middleware to become a no-op for that request: the inner model is invoked unchanged, no Generation is started, no `EvaluateHook` is called.

If `opts.ClientResolver` is itself `nil`, the middleware SHALL behave as if every resolution returns `nil`.

#### Scenario: ClientResolver returns nil

- **WHEN** `ClientResolver(ctx)` returns `nil` for a request
- **THEN** the wrapped model SHALL produce identical output to the unwrapped base model
- **AND** no Sigil API calls SHALL be made for that request

#### Scenario: ClientResolver is nil

- **WHEN** `opts.ClientResolver` is `nil`
- **THEN** `Wrap`/`Stack` SHALL still return a valid `provider.LanguageModel` / middleware slice
- **AND** every request SHALL pass through unchanged

### Requirement: ContextProvider supplies request-scoped metadata

`RecordingMiddleware` SHALL call `opts.ContextProvider(ctx)` once per request and use the returned `ContextInfo` to populate:
- `sigilsdk.GenerationStart.UserID` (falls back to `sigil.UserIDFromContext(ctx)` when `ContextInfo.UserID` is empty).
- `sigilsdk.GenerationStart.Metadata` (merged on top of metadata derived from `ProviderOptions` and `params`).
- `sigilsdk.GenerationStart.Tags` (merged on top of any tags carried via context).
- `sigilsdk.GenerationStart.AgentName` / `AgentVersion` (override `sigil.AgentNameFromContext` / `sigil.AgentVersionFromContext` when set).

A zero `ContextInfo` SHALL be tolerated; every field SHALL fall back to the appropriate `sigil.*FromContext` helper or remain unset.

If `opts.ContextProvider` is `nil`, the middleware SHALL log a warning at most once per process and continue with all-`sigil.*FromContext`-derived defaults.

#### Scenario: Zero ContextInfo falls back to context helpers

- **WHEN** `ContextProvider(ctx)` returns a zero `ContextInfo`
- **THEN** `GenerationStart.UserID` SHALL equal `sigil.UserIDFromContext(ctx)`
- **AND** `GenerationStart.AgentName` SHALL equal `sigil.AgentNameFromContext(ctx)`

#### Scenario: Nil ContextProvider logs once

- **WHEN** `RecordingMiddleware` is invoked for the first time with `opts.ContextProvider == nil`
- **THEN** a Warn-level log message SHALL be emitted exactly once for the lifetime of the process

### Requirement: MapGenerateResult produces sigil.Generation

`MapGenerateResult(params, result, ctxInfo)` SHALL produce a `sigilsdk.Generation` whose:
- `Input.Messages` is derived from `params.Prompt`, with `provider.Message{Role: RoleSystem}` entries folded into `Generation.SystemPrompt` (single concatenated string) rather than appearing as a sigil message.
- `Input.Tools` is derived from `params.Tools`. Function tools map directly; provider-defined tools (e.g. Anthropic `web_search`, `code_execution`) MAY map with their type preserved so Sigil can annotate them.
- `Input.MaxTokens`, `Temperature`, `TopP`, `ToolChoice` are derived from the corresponding `provider.CallOptions` fields.
- Anthropic thinking-budget metadata (`gen_ai.request.thinking.budget_tokens`) is derived from `params.ProviderOptions["anthropic"]` via `json.RawMessage` decoding, not by importing `providers/anthropic`.
- `Output` is a single assistant `sigil.Message` whose parts mirror `result.Content` (text, tool-call, reasoning parts).
- `Usage` maps from `result.Usage` (input tokens, output tokens, cache hits where applicable).
- `StopReason` is produced by `finishReasonToSigilStop(result.FinishReason)` and SHALL match the string values the legacy `internal/llm/claude/` path emitted (e.g. `"end_turn"`, `"max_tokens"`, `"tool_use"`, `"stop_sequence"`).
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
- **THEN** the resulting `Generation.Metadata["gen_ai.request.thinking.budget_tokens"]` SHALL equal that integer

#### Scenario: Byte-equal output to sigil-sdk anthropic helper

- **GIVEN** a recorded canonical request that has both an `anthropic.MessageNewParams` form and an equivalent `provider.CallOptions` form
- **WHEN** the Anthropic form is passed through `sigil-sdk/go-providers/anthropic.FromRequestResponse`
- **AND** the ai-sdk form is passed through `MapGenerateResult`
- **THEN** both resulting `sigil.Generation` payloads SHALL produce byte-equal JSON modulo the fields `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

### Requirement: StreamRecorder accumulates streamed generation state

`StreamRecorder` SHALL accumulate `sigil.Generation.Output` from a sequence of `provider.StreamPart` values observed via `Observe`. It SHALL:
- Append `PartTextDelta` payloads into the active assistant text part.
- Append `PartReasoningDelta` payloads into the active assistant reasoning part. The recorder SHALL internally retain any `ProviderMetadata["anthropic"].signature` value observed on the delta (deduplicating identical values) so consumers that hold a reference to the source `provider.Message` can round-trip the signature on subsequent turns. Because `sigilsdk.Part` has no field for signatures today, the value is NOT serialized into `Generation.Output`; reasoning signatures live exclusively on the corresponding `provider.Message` in `params.Prompt` (see "Hooks transform preserves reasoning signatures" for the round-trip path).
- Append `PartToolCallDelta` payloads into the active assistant tool-call part.
- Record the first observed part's timestamp via `FirstChunkAt()`.
- Capture `FinishReason` and `Usage` from `PartFinish` / `PartFinishStep` (whichever the provider emits).

`Generation()` SHALL return a `sigil.Generation` whose `Output` is a single assistant message constructed from the accumulated state.

#### Scenario: Reasoning text accumulates across deltas

- **GIVEN** a stream that emits three `PartReasoningDelta` events with text fragments `"I "`, `"think "`, `"so"`
- **WHEN** `StreamRecorder.Observe` is called for each
- **AND** `StreamRecorder.Generation()` is called at end-of-stream
- **THEN** the resulting reasoning part in `Generation.Output` SHALL contain the concatenated reasoning text `"I think so"`

#### Scenario: Reasoning signature is preserved off-band

- **GIVEN** a stream that emits `PartReasoningDelta` events carrying `ProviderMetadata["anthropic"].signature = "sig-abc"`
- **WHEN** `StreamRecorder.Observe` is called for each
- **AND** `StreamRecorder.Generation()` is called at end-of-stream
- **THEN** the resulting reasoning part in `Generation.Output` MAY omit the signature (the current `sigilsdk.Part` schema has no signature field)
- **AND** the original signature SHALL remain available on the corresponding `provider.Message` in `params.Prompt`, where `HooksMiddleware.applyTransformedInput` matches by text content to preserve it across hook transforms

#### Scenario: Text deltas concatenate

- **GIVEN** a stream emits `PartTextDelta{Text: "Hello, "}` then `PartTextDelta{Text: "world"}`
- **WHEN** the recorder is observed for each
- **THEN** the resulting assistant message's text part SHALL equal `"Hello, world"`

#### Scenario: Tool-call deltas accumulate by tool-call ID

- **GIVEN** a stream emits multiple `PartToolCallDelta` events for the same tool-call ID with incremental JSON argument fragments
- **WHEN** the recorder is observed
- **THEN** the resulting tool-call part SHALL have the concatenated argument JSON

### Requirement: RecordingMiddleware wraps generate and stream

`RecordingMiddleware(opts)` SHALL return a `middleware.Middleware` whose `WrapGenerate` and `WrapStream` hooks:
1. Resolve a client via `opts.ClientResolver`. If `nil`, pass through to the inner model unchanged.
2. Build a `sigilsdk.GenerationStart` via `BuildGenerationStart(ctx, model.Provider(), model.ModelID(), opts.ContextProvider(ctx))`.
3. Call `client.StartGeneration` (for `WrapGenerate`) or `client.StartStreamingGeneration` (for `WrapStream`).
4. Invoke the inner model.
5. On success:
   - For generate: call `recorder.SetResult(MapGenerateResult(params, result, ctxInfo))`.
   - For stream: tee the result stream channel, feed each part to a `StreamRecorder`, and at end-of-stream call `recorder.SetResult(streamRecorder.Generation())`.
6. On error: call `recorder.SetCallError(err)`.

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
3. Build a `sigilsdk.HookEvaluateRequest` from `params` (phase = preflight).
4. Call `client.EvaluateHook(ctx, request)`. If `opts.MaxLatency > 0`, the call SHALL be bounded by `context.WithTimeout(ctx, opts.MaxLatency)`; otherwise the request context SHALL be inherited unchanged.
5. Branch on the response:
   - **Deny**: return `&HookDenialError{Reason, RuleID, Cause: nil}` to the caller. The inner model SHALL NOT be invoked.
   - **Allow**: invoke the inner model with `params` unchanged.
   - **TransformedInput**: rebuild `params.Prompt` from the transformed messages (preserving reasoning-block signatures — see "Hooks transform preserves reasoning signatures" below), then invoke the inner model with the new params.

#### Scenario: Allow path passes through

- **GIVEN** a `HooksMiddleware` whose `ClientResolver` returns a client
- **WHEN** `EvaluateHook` returns an allow decision
- **THEN** the inner model SHALL be invoked with `params` unchanged
- **AND** the response from the inner model SHALL be returned to the caller

#### Scenario: Deny returns typed error

- **GIVEN** a `HooksMiddleware` whose `ClientResolver` returns a client
- **WHEN** `EvaluateHook` returns a deny decision with reason "policy violation" and rule ID "rule-42"
- **THEN** the middleware SHALL return a non-nil error
- **AND** `errors.As(err, new(*sigil.HookDenialError))` SHALL succeed
- **AND** the unwrapped error SHALL have `Reason == "policy violation"` and `RuleID == "rule-42"`
- **AND** `errors.Is(err, sigil.ErrHookDenied)` SHALL return `true`
- **AND** the inner model's `DoGenerate`/`DoStream` SHALL NOT be invoked

#### Scenario: MaxLatency bounds EvaluateHook

- **GIVEN** a `HooksMiddleware` with `opts.MaxLatency = 100 * time.Millisecond`
- **WHEN** the upstream `EvaluateHook` server stalls for longer than 100ms
- **THEN** the hook call SHALL be cancelled via context deadline
- **AND** the original request context SHALL NOT be cancelled (only the derived hook-bounded context)

### Requirement: Hooks transform preserves reasoning signatures

When `EvaluateHook` returns a `TransformedInput`, `HooksMiddleware` SHALL rebuild `params.Prompt` from the transformed messages using the following algorithm:
1. Build a content-matching index over assistant-role messages in the **original** `params.Prompt` that contain reasoning parts.
2. For each transformed assistant message, look up a matching entry in the index by text content. If found, use the original message verbatim. If not found, rebuild the message from the transformed `sigil` parts.
3. Non-assistant messages (system, user, tool) are rebuilt from the transformed parts directly.

This preserves `ProviderOptions["anthropic"].signature` values on reasoning parts, which do not round-trip through Sigil's wire schema.

#### Scenario: Reasoning signature survives transform

- **GIVEN** an assistant message in `params.Prompt` carrying a reasoning part with `ProviderOptions["anthropic"].signature = "sig-xyz"`
- **AND** `EvaluateHook` returns a `TransformedInput` that modifies only user messages, leaving the assistant message text unchanged
- **WHEN** the transform is applied
- **THEN** the resulting `params.Prompt` SHALL contain an assistant message whose reasoning part has `ProviderOptions["anthropic"].signature == "sig-xyz"` byte-equal to the original

#### Scenario: Modified assistant text triggers rebuild from sigil parts

- **GIVEN** an assistant message in `params.Prompt` whose text is "abc"
- **AND** `EvaluateHook` returns a `TransformedInput` whose corresponding assistant message has text "def"
- **WHEN** the transform is applied
- **THEN** the resulting `params.Prompt` SHALL contain an assistant message whose text equals "def"
- **AND** the original signature (if any) SHALL NOT be carried forward (because the content did not match)

### Requirement: Generation-ID DAG context helpers

The following context helpers SHALL be exposed from `middleware/sigil`:

- `WithGenerationID(ctx, id)` / `GenerationIDFromContext(ctx)` — current generation ID for the call about to be made.
- `WithParentGenerationIDs(ctx, ids...)` / `ParentGenerationIDsFromContext(ctx)` — upstream generations whose output this call depends on. Used by Sigil to build the parent → child DAG.
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

The middleware SHALL emit exactly one OTel span of its own: the hooks preflight span. The canonical generation span (operation = `generateText` / `streamText`, with `gen_ai.*` semantic-convention attributes and `sigil.generation.id`) is owned by the sigil-sdk client via `StartGeneration` / `StartStreamingGeneration`; the middleware SHALL NOT wrap or duplicate it.

Span name: `aisdk.sigil.hooks.preflight`.

Span attribute keys SHALL match the legacy `claude.HooksMiddleware` shape for keys that drive existing dashboards:
- `sigil.hooks.result` (string: `"allow"`, `"deny"`, `"transform"`).
- `sigil.hooks.action` (string).
- `sigil.hooks.rule_id` (string, present only on deny).

Error states on the generation path SHALL reach the trace via `recorder.SetCallError(err)`, which sigil-sdk stamps onto its own generation span as `error.type` and `error.category`. The middleware SHALL NOT emit its own error attributes for generation calls.

#### Scenario: Allow decision sets sigil.hooks.result

- **WHEN** `EvaluateHook` returns an allow decision
- **THEN** the `aisdk.sigil.hooks.preflight` span SHALL have attribute `sigil.hooks.result = "allow"`

#### Scenario: Deny decision sets rule ID

- **WHEN** `EvaluateHook` returns a deny decision with rule ID "rule-42"
- **THEN** the `aisdk.sigil.hooks.preflight` span SHALL have attributes `sigil.hooks.result = "deny"` and `sigil.hooks.rule_id = "rule-42"`

### Requirement: Conformance fixtures

The module SHALL include a `testdata/` directory containing:
- `generation/`: paired (ai-sdk-typed `CallOptions` + `GenerateResult`, expected `sigil.Generation` JSON) triples sourced from `sigil-sdk/go-providers/anthropic` conformance helpers.
- `stream/`: captured chunk-stream fixtures (reused from `providers/anthropic/test/conformance/recorded/` where overlapping content allows).
- `hooks/`: paired (input prompt, hook response, expected post-transform prompt) triples.

A `make test-sigil-conformance` target SHALL run these tests in isolation. The conformance tests SHALL re-run on every PR that touches `middleware/sigil/` or bumps the `sigil-sdk` dependency in `go.mod`.

#### Scenario: Generation conformance fixture

- **GIVEN** a fixture triple in `testdata/generation/`
- **WHEN** `MapGenerateResult` is invoked with the fixture's params and result
- **THEN** the resulting `sigil.Generation` JSON SHALL byte-equal the expected JSON modulo `id`, `started_at`, `completed_at`, `trace_id`, `span_id`

#### Scenario: Stream conformance fixture

- **GIVEN** a captured chunk stream in `testdata/stream/`
- **WHEN** each chunk is fed to a `StreamRecorder` via `Observe`
- **AND** `Generation()` is called at end-of-stream
- **THEN** the resulting `sigil.Generation` JSON SHALL byte-equal the expected JSON modulo `id`, `started_at`, `completed_at`, `trace_id`, `span_id`
