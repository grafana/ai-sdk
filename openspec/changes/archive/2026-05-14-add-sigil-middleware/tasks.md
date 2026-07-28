## 1. Module skeleton

- [x] 1.1 Create `middleware/sigil/go.mod` with `module github.com/grafana/ai-sdk/middleware/sigil`, `replace github.com/grafana/ai-sdk => ../../`, and required deps (`github.com/grafana/ai-sdk`, `github.com/grafana/sigil-sdk/go`, `github.com/stretchr/testify`).
- [x] 1.2 Add `middleware/sigil/doc.go` with a package overview that documents the public API surface and the "heavy middlewares live in nested modules" convention.
- [x] 1.3 Add a `go.sum` and run `go mod tidy` within the nested module.
- [x] 1.4 Update root `Makefile` to include `middleware/sigil` in the cross-module test loop (`make test` descends into `middleware/sigil` like it does for `providers/anthropic`/`providers/grafana`).
- [x] 1.5 Verified via `go mod why -m github.com/grafana/sigil-sdk/go` from the root: "main module does not need module github.com/grafana/sigil-sdk/go".

## 2. Public types and options

- [x] 2.1 Define `WrapOptions`, `RecordingOptions`, `HooksOptions`, `ContextInfo` in `middleware/sigil/options.go`.
- [x] 2.2 Define `ClientResolver` and `ContextProvider` type aliases.
- [x] 2.3 Define `HookDenialError` struct with `Reason`, `RuleID`, `Cause` fields, `Error()` method, `Unwrap()` returning `ErrHookDenied` in `middleware/sigil/errors.go`.
- [x] 2.4 Define `ErrHookDenied` sentinel using `errors.New("sigil: hook denied request")`.
- [x] 2.5 Unit tests: `errors.Is(&HookDenialError{}, ErrHookDenied)` returns true; `errors.As` extracts a `*HookDenialError`.

## 3. Context-key helpers

- [x] 3.1 Implement `WithGenerationID`, `GenerationIDFromContext`, `NewGenerationID` in `middleware/sigil/context.go`. ID format: opaque string (UUIDv7 or random hex; pick once and document).
- [x] 3.2 Implement `WithParentGenerationIDs`, `ParentGenerationIDsFromContext`. Multiple `With...` calls SHALL append, not replace.
- [x] 3.3 Implement `WithLinkedGenerationID`. Document semantics (sibling/peer linkage).
- [x] 3.4 Unit tests: round-trip for each helper; append-not-replace for parent IDs; zero-value returns for missing keys.

## 4. Request mapper

- [x] 4.1 Implement `messagesToSigil([]provider.Message) (system string, messages []sigilsdk.Message)` in `middleware/sigil/map_request.go`. Fold `RoleSystem` entries into a concatenated system string; convert other roles role-by-role.
- [x] 4.2 Implement `toolsToSigil([]provider.Tool) []sigilsdk.ToolDefinition`. Function tools map directly; provider-defined tools preserve their type for Sigil annotation.
- [x] 4.3 Implement `controlsFromCallOptions(provider.CallOptions) requestControls` (internal helper) capturing `MaxTokens`, `Temperature`, `TopP`, `ToolChoice`.
- [x] 4.4 Implement `map_provider_options.go`: decode `ProviderOptions["anthropic"]` JSON for `thinking.budget_tokens` → `metadata["gen_ai.request.thinking.budget_tokens"]`. Use `encoding/json` directly on `json.RawMessage`; do NOT import `providers/anthropic`.
- [x] 4.5 Unit tests: system-message folding, tool conversion, control extraction, anthropic thinking-budget extraction.

## 5. Response mapper

- [x] 5.1 Implement `contentToSigilOutput([]provider.ContentPart) sigilsdk.Message` in `middleware/sigil/map_response.go` (single assistant message with text/tool-call/reasoning parts).
- [x] 5.2 Implement `usageToSigil(provider.Usage) sigilsdk.TokenUsage`.
- [x] 5.3 Implement `finishReasonToSigilStop(provider.FinishReason) string` returning the legacy strings (`"end_turn"`, `"max_tokens"`, `"tool_use"`, `"stop_sequence"`, etc.).
- [x] 5.4 Unit tests for each mapper helper. Include table-driven cases for every `provider.FinishReason` value.

## 6. MapGenerateResult + BuildGenerationStart

- [x] 6.1 Implement `BuildGenerationStart(ctx, providerName, modelID, ctxInfo) sigilsdk.GenerationStart` in `middleware/sigil/generation.go`. Reads context keys for generation ID + parent IDs; uses `sigil.UserIDFromContext` / `sigil.AgentNameFromContext` / `sigil.AgentVersionFromContext` as fallbacks for `ContextInfo` fields.
- [x] 6.2 Implement `MapGenerateResult(params, result, ctxInfo) sigilsdk.Generation`. Composes the request mapper, response mapper, and metadata merging.
- [x] 6.3 Unit tests: round-trip a canonical request through `MapGenerateResult` and assert every `sigil.Generation` field.

## 7. StreamRecorder

- [x] 7.1 Implement `StreamRecorder` type in `middleware/sigil/map_stream.go`: unexported state for accumulated text part, accumulated reasoning part (with signature merge), accumulated tool-call parts (keyed by tool-call ID), first-chunk timestamp, finish reason, usage.
- [x] 7.2 Implement `NewStreamRecorder(start, params) *StreamRecorder`, `Observe(part)`, `FirstChunkAt() time.Time`, `Generation() sigilsdk.Generation`.
- [x] 7.3 Handle `PartTextDelta`, `PartReasoningDelta` (with `ProviderOptions["anthropic"].signature` merge), `PartToolInputDelta`/`PartToolCall`, `PartFinish` (usage + finish reason), `PartError` (record-call-error state). Note: `PartFinishStep` is not part of the actual provider StreamPartType set (only `PartFinish` exists); multi-step is tested via repeated `PartFinish` events.
- [x] 7.4 Unit tests: text-only stream, text + reasoning + signature, text + tool-call, multi-step (finish-step + start-step), error mid-stream, empty stream.

## 8. RecordingMiddleware

- [x] 8.1 Implement `RecordingMiddleware(opts) middleware.Middleware` in `middleware/sigil/recording.go`. `WrapGenerate` hook: client-resolve → start → invoke → `SetResult`/`SetCallError`.
- [x] 8.2 Implement the `WrapStream` hook: tee the result channel using a bounded buffered channel; select on `ctx.Done()` on every send; drain upstream after consumer abandonment; call `streamRecorder.Generation()` after upstream closes, then `recorder.SetResult`.
- [x] 8.3 Emit a once-per-process Warn log when `opts.ContextProvider == nil`.
- [~] 8.4 **SUPERSEDED** during follow-up review. The `aisdk.sigil.recording.generate` / `aisdk.sigil.recording.stream` spans were removed because sigil-sdk's `Client.StartGeneration` / `StartStreamingGeneration` already open the canonical generation span with full `gen_ai.*` semantic-convention attributes (plus `sigil.generation.id`, `gen_ai.conversation.id`, `user.id`, `gen_ai.agent.{name,version}`). Wrapping that span produced a redundant middleware-internal hop with one unique attribute (`sigil.recording.outcome`) whose three values are already representable on the parent span via sigil-sdk's `error.type` / `error.category`. Net effect: cleaner trace tree, zero observational regression, lower infra cost per call. The hooks preflight span (`aisdk.sigil.hooks.preflight`) is unchanged — sigil-sdk does NOT span EvaluateHook, so the middleware owns that span legitimately. See `recording.go` doc-comment and `doc.go` "OTel span and attribute contract" section.
- [x] 8.5 Unit tests: nil ClientResolver passes through; generate success → SetResult; generate error → SetCallError; stream success → SetResult with accumulated Generation; stream consumer abandons → goroutine cleanup; nil ContextProvider logs once.

## 9. HooksMiddleware

- [x] 9.1 Implement `HooksMiddleware(opts) middleware.Middleware` in `middleware/sigil/hooks.go`. `WrapGenerate` and `WrapStream` both call the same internal `evaluateHook` helper.
- [x] 9.2 Implement `buildHookEvaluateRequest(params provider.CallOptions, ctxInfo ContextInfo) sigilsdk.HookEvaluateRequest`. Phase = preflight.
- [x] 9.3 Implement `MaxLatency` enforcement via `context.WithTimeout` on a derived context (NOT on the request context — that would propagate to the inner model call).
- [x] 9.4 Implement the deny branch: return `&HookDenialError{Reason, RuleID}` without invoking the inner model.
- [x] 9.5 Implement `applyTransformedInput(originalPrompt []provider.Message, transformed sigilsdk.HookInput) []provider.Message` with the content-match-and-preserve-signatures algorithm. (Type is `HookInput`, not `HookTransformedInput`; spec wording was approximate.)
- [x] 9.6 Add the `aisdk.sigil.hooks.preflight` OTel span with attributes `sigil.hooks.result`, `sigil.hooks.action`, `sigil.hooks.rule_id`.
- [x] 9.7 Unit tests for the helpers: hook request construction, deny error shape, allow passthrough, MaxLatency cancels only the hook call.
- [x] 9.8 Unit tests for `applyTransformedInput`: reasoning signature preserved when text matches; rebuild from sigil parts when text differs; non-assistant messages rebuilt from parts directly.

## 10. Composition helpers

- [x] 10.1 Implement `Stack(opts) []middleware.Middleware` in `middleware/sigil/stack.go`. Returns `[HooksMiddleware, RecordingMiddleware]` when both enabled; omits Hooks when disabled.
- [x] 10.2 Implement `Wrap(base, opts) provider.LanguageModel` as `middleware.Wrap(middleware.WrapOptions{Model: base, Middleware: Stack(opts)})`.
- [x] 10.3 Unit tests: `Stack` ordering with both enabled, with Hooks disabled, with neither enabled (returns Recording only); `Wrap` equivalence to manual `middleware.Wrap`.

## 11. Conformance fixtures

- [x] 11.1 Add `middleware/sigil/testdata/generation/` with 5 fixture triples: plain_text, tool_call, reasoning_with_signature, max_tokens_stop, tool_use_stop. Each triple has `params.json` (ai-sdk-typed), `result.json`, `expected_generation.json`.
- [x] 11.2 Add `middleware/sigil/testdata/stream/` with 3 captured chunk-stream fixtures: text_only, text_reasoning_signature, text_tool_call.
- [x] 11.3 Add `middleware/sigil/testdata/hooks/` with 3 fixtures: allow, deny (with reason + rule ID), transform_preserves_signature.
- [x] 11.4 Implement `conformance_test.go`: walks each `testdata/*/` directory and runs the appropriate mapper against the fixture; asserts byte-equal expected output modulo recorder-set fields (`id`, `started_at`, `completed_at`, `trace_id`, `span_id`). Run with `SIGIL_REGEN=1` to refresh snapshots.
- [x] 11.5 Cross-path parity test in `parity_test.go`: builds equivalent Anthropic-SDK and ai-sdk inputs, runs both through their respective mappers, asserts byte-equal JSON of the resulting `sigil.Generation` modulo recorder-set fields.

## 12. Build and test integration

- [x] 12.1 Add a `make test-sigil-conformance` target that runs only the conformance + parity tests in the nested module.
- [x] 12.2 Ensure `make test` (root) descends into `middleware/sigil` and runs all tests via `test-sigil-middleware`.
- [~] 12.3 No GitHub workflows currently exist in the repo; the `make test` Makefile loop is the test entry point. When CI workflows are added, they should call `make test` so `test-sigil-middleware` runs automatically.
- [x] 12.4 Add a renovate rule that groups `sigil-sdk/go` and `sigil-sdk/go-providers/anthropic` bumps under "sigil-sdk bumps (require conformance review)" with `automerge: false` and a `sigil-conformance-review` label.

## 13. Documentation

- [x] 13.1 Update `README.md` (root) with a short subsection under `## Architecture` covering `aisdk/middleware`, the heavy-middleware convention, and a pointer to `middleware/sigil/doc.go`.
- [x] 13.2 Add usage examples in `middleware/sigil/doc.go`: minimal Wrap, composed Stack with other middlewares, ClientResolver returning nil for opt-out.
- [x] 13.3 Document the OTel span name + attribute key contract in `doc.go` so downstream dashboards have a stable reference.

## 14. Verification

- [x] 14.1 Ran `go vet ./...` from both root and `middleware/sigil`; no findings.
- [x] 14.2 Ran `golangci-lint run ./...` in the nested module; clean after silencing two SA1012 warnings on intentional nil-context tests.
- [x] 14.3 Ran `go test ./middleware/sigil/...` from the nested module; all tests pass.
- [x] 14.4 Ran `make test` from the root; tests across root, providers/anthropic, providers/grafana, middleware/sigil all pass.
- [x] 14.5 Verified root module does NOT pull `sigil-sdk`: `go mod why -m github.com/grafana/sigil-sdk/go` returns "main module does not need module".
- [x] 14.6 Added `api_smoke_test.go` that compile-references every spec-mandated public symbol (types, functions, context helpers, sentinel error) so drift surfaces as a compile error.
