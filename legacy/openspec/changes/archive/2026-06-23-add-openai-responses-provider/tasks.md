## 1. Module scaffold and transport spike

- [x] 1.1 Create `providers/openai/go.mod` (`module github.com/grafana/ai-sdk/providers/openai`, `go 1.26`, `replace github.com/grafana/ai-sdk => ../../`) and add `github.com/openai/openai-go`, root, and `testify` requires; run `go mod tidy`
- [x] 1.2 Pin a known-good `github.com/openai/openai-go` version that exposes the full `responses` surface (params, `Response`, streaming events) and record it in the module
- [x] 1.3 Transport spike: build a minimal `model` that issues one Responses request via the SDK against an `httptest` server, capture the outbound body, and confirm field names/shape match upstream's `expected-requests.jsonl` for a simple text request; document any SDK escape-hatch (raw/extra-field) needed
- [x] 1.4 Add the new module to `Makefile` (build/fmt/vet/lint/test targets) and `.github/workflows/ci.yml` (build + test-short matrix)
- [x] 1.5 Create `providers/openai/doc.go` package godoc overview

## 2. Model skeleton, options, and capabilities

- [x] 2.1 Implement `model.go`: model struct (client, modelID, providerName `"openai"`, `generateID`, request options), `NewResponses(apiKey, modelID, opts...)`, interface one-liners (`SpecificationVersion`/`Provider`/`ModelID`/`SupportedURLs`), and `var _ provider.LanguageModel = (*model)(nil)`
- [x] 2.2 Implement `options.go`: `type Option func(*model)` with `WithRequestOptions(...)` and an injectable `WithGenerateID(...)` for deterministic tests
- [x] 2.3 Implement typed `OpenAIResponsesOptions` struct with `ProviderKey() == "openai"` and JSON tags for every documented option (previousResponseId, conversation, instructions, reasoningEffort, reasoningSummary, truncation, store, metadata, include, maxToolCalls, parallelToolCalls, serviceTier, textVerbosity, user, logprobs, strictJsonSchema, systemMessageMode, forceReasoning, allowedTools, promptCacheKey, promptCacheRetention, safetyIdentifier, passThroughUnsupportedFiles, contextManagement) plus per-tool and per-part option structs (itemId, reasoningEncryptedContent, approvalRequestId, imageDetail, namespace, phase)
- [x] 2.4 Implement `models.go`: `modelCapabilities` struct + `getModelCapabilities(modelID)` prefix matcher mirroring upstream (isReasoningModel, systemMessageMode, supportsFlexProcessing, supportsPriorityProcessing, supportsNonReasoningParameters); plus exported `ModelIDs()`
- [x] 2.5 Unit tests: `options_test.go` (ProviderKey, marshal round-trip, compile-time `provider.ProviderOption` check), `models_test.go` (capability matrix by model prefix)

## 3. Request conversion (input items + params)

- [x] 3.1 Implement `convert_request.go` `buildParams(modelID, opts, stream)` skeleton resolving capabilities, provider options, `store`, `hasConversation`/`hasPreviousResponseId`, and `systemMessageMode`
- [x] 3.2 System message conversion (`system`/`developer`/`remove` with warning)
- [x] 3.3 User message conversion: text -> input_text; images -> input_image (image_url/file_id/data URI + imageDetail); files -> input_file (file_id/file_url/filename+file_data); unsupported media handling + warnings
- [x] 3.4 Assistant content conversion: output_text vs `item_reference` (store+id), reasoning items + item_reference + non-store filtering, compaction custom part
- [x] 3.5 Tool-call conversion: function_call + serializeToolCallArguments + store/conversation/previousResponseId skip rules done; stored built-in tool calls round-trip via item_reference (all tool types). NOTE: non-stored built-in call items (shell/apply_patch/tool_search/local_shell) fall back to function_call — rare multi-turn edge documented in design.md
- [x] 3.6 Tool-result conversion: function_call_output + tool-approval-response -> mcp_approval_response + full tool output value mapping done; stored built-in results round-trip via item_reference. NOTE: non-stored built-in output items fall back to function_call_output — documented in design.md
- [x] 3.7 Scalar params (temperature/topP/maxOutputTokens) + unsupported-param warnings (topK/seed/presencePenalty/frequencyPenalty/stopSequences)
- [x] 3.8 Structured output: `text.format` json_schema/json_object honoring strictJsonSchema/name/description/schema
- [x] 3.9 Apply provider options to request body (previousResponseId, conversation, instructions, metadata, store, truncation, serviceTier, textVerbosity, user, maxToolCalls, parallelToolCalls, promptCacheKey/Retention, safetyIdentifier, contextManagement) and conversation+previousResponseId conflict warning
- [x] 3.10 Capability gating: strip temperature/topP on reasoning models (unless effort none + supportsNonReasoningParameters), reasoningEffort/reasoningSummary warnings on non-reasoning models, serviceTier flex/priority stripping with warnings; reasoning effort/summary block assembly
- [x] 3.11 `include` auto-population (logprobs -> message.output_text.logprobs + top_logprobs; web_search -> web_search_call.action.sources; code_interpreter -> outputs; non-store reasoning -> reasoning.encrypted_content)
- [x] 3.12 Unit tests: `convert_request_test.go` covering every scenario in the `openai-responses-provider` spec (system modes, user image/pdf, stored references, function round-trip, empty args, reasoning paths, unsupported params, json schema, provider options, capability gating, include population)

## 4. Tool preparation and tool choice

- [x] 4.1 Implement `prepare_tools.go`: function tools -> function declarations; provider tools per id (web_search, web_search_preview, code_interpreter, file_search, image_generation, local_shell, shell, apply_patch, mcp, tool_search, custom) including arg mapping (shell environment, mcp require_approval default `never`, file_search filters, image_generation params)
- [x] 4.2 Tool-name mapping integration (provider tool ids <-> custom names) consistent with root `tool-name-mapping`
- [x] 4.3 Tool choice resolution: auto/none/required pass-through, `tool` typed/object/custom/function, `allowedTools` override -> allowed_tools choice; unknown tool -> unsupported warning
- [x] 4.4 Unit tests: `prepare_tools_test.go` covering function declaration, each provider tool, tool_choice variants, allowedTools override, unknown-tool warning

## 5. Non-streaming response conversion (DoGenerate)

- [x] 5.1 Implement `DoGenerate` in `model.go`: build params, SDK call, error wrap, convert, append warnings, set response metadata
- [x] 5.2 Implement `convert_response.go` output-item switch: message text + phase + provider metadata, reasoning summaries, function_call/custom_tool_call
- [x] 5.3 Provider-executed items: web_search_call, file_search_call, code_interpreter_call, image_generation_call, computer_call, local_shell_call, shell_call(+output), apply_patch_call, tool_search_call(+output), mcp_call, mcp_list_tools (skip), mcp_approval_request (+ approval-request part), compaction
- [x] 5.4 Implement `sources.go`: annotations (url_citation/file_citation/container_file_citation/file_path) -> `source` content parts with provider metadata
- [x] 5.5 Implement `convert_usage.go` (`convertUsage`) and `finish_reason.go` (`mapFinishReason` with hasFunctionCall semantics)
- [x] 5.6 Provider metadata assembly (responseId, logprobs, serviceTier)
- [x] 5.7 Unit tests: `convert_response_test.go`, `convert_usage_test.go`, `finish_reason_test.go` covering spec scenarios (url citation, provider-executed web search, mcp approval, finish reason with function call, usage token split)

## 6. Streaming response conversion (DoStream)

- [x] 6.1 Implement `DoStream` in `model.go`: build params (stream=true), open SDK stream, buffered channel + goroutine + `consumeStream` with graceful error emission as `PartError`
- [x] 6.2 Implement `convert_stream.go` `streamAdapter` state (ongoingToolCalls by output_index, ongoingAnnotations, activeReasoning by item id, activeMessagePhase, hasFunctionCall, responseId, usage, finishReason, serviceTier, logprobs, approvalRequestId maps)
- [x] 6.3 `response.created` -> stream-start + response metadata; `response.completed`/`incomplete`/`failed` -> finish with usage + finish reason; `error` -> error part; unknown events -> no-op (no error)
- [x] 6.4 Message lifecycle: output_item.added (text-start), output_text.delta (text-delta), annotation.added accumulation, output_item.done (text-end with phase + annotations)
- [x] 6.5 Function/custom tool streaming: output_item.added (tool-input-start), function_call_arguments.delta / custom_tool_call_input.delta (tool-input-delta), output_item.done (tool-input-end + tool-call with namespace metadata)
- [x] 6.6 Reasoning streaming: reasoning output_item.added (reasoning-start `itemId:0`), reasoning_summary_part.added, reasoning_summary_text.delta (reasoning-delta), reasoning_summary_part.done / output_item.done (reasoning-end)
- [x] 6.7 Provider-executed tool streaming (eager call emission per upstream): web_search_call, computer_call, code_interpreter_call (seed input-delta + code deltas), file_search_call, image_generation_call, tool_search_call, apply_patch_call (escaped diff deltas), shell_call(+output), mcp_call/mcp_approval_request on output_item.done with id aliasing + approval-request part
- [x] 6.8 Unit tests: `convert_stream_test.go` with `unmarshalEvent`/`collectParts` drivers covering spec scenarios (text lifecycle, function call, web-search eager call, unknown event ignored, finish with usage) plus reasoning, code-interpreter, apply-patch, mcp approval

## 7. Error mapping

- [x] 7.1 Implement `wrap_api_error.go`: `wrapAPIError(err, requestBody)` via `errors.As` on the openai-go error type -> `provider.NewAPICallError` (status, headers, body, retryability, structured `Data`); forced-wrap variant for stream parts; non-API DoGenerate errors pass through
- [x] 7.2 Unit tests: `wrap_api_error_test.go` with `httptest` server returning a 400 -> `DoGenerate` returns `APICallError`; streaming error -> drained `PartError` carrying `APICallError`; `require.NotPanics` on construction

## 8. Conformance integration (runner + tooling)

- [x] 8.1 Add `"openai"` request-header allowlist to `test/conformance/runner.go` (redact Authorization, normalize OpenAI-* headers, exclude volatile)
- [x] 8.2 Add OpenAI request-body normalization in `runner.go` (preserve `input` array order; object-order-insensitive via existing comparison) and unit-test it in `request_snapshot_test.go`
- [x] 8.3 Create `test/conformance/openai/conformance_test.go` (`//go:build conformance`) wiring `DiscoverTestCases(".")` to a factory building `openai.NewResponses` with base-URL request option + deterministic `WithGenerateID`
- [x] 8.4 Add `@ai-sdk/openai` to `test/conformance/tools/package.json`; add OpenAI branch to `createModel()` in `generate.mts` and `record.mts`; add `PROVIDER_BASE_URLS` entry + `OPENAI_API_KEY` to `checkAPIKeys` in `record.mts`; add any builders in `common.mts`
- [x] 8.5 Update conformance `README.md` "Adding a New Provider" notes and the conformance CI job to include OpenAI cases

## 9. Conformance fixtures

- [x] 9.1 Create `test/conformance/openai/upstream/` cases by copying Vercel `packages/openai/src/responses/__fixtures__`/`__snapshots__` inputs into `*.chunks.txt`; seed `upstream/INDEX.yaml`
- [x] 9.2 Add `config.yaml` per case and run `make generate-conformance` to produce `expected.jsonl` + `expected-requests.jsonl` (no API keys); commit goldens
- [x] 9.3 Ensure coverage: simple text, function tool calling, reasoning with summaries, structured (json schema) output, provider-executed web search with sources, conversation continuation via previous_response_id
- [x] 9.4 Add at least one `recorded/` case (recorded from the real API where feasible) demonstrating the recorded path; document the recording command — DONE: nine live-recorded cases committed under `openai/recorded/` (simple-text, tool-call, parallel-tool-calls, multi-tool-chain, reasoning-text, reasoning-tool-use, web-search, code-interpreter, structured-json-output), all passing Go conformance byte-identically. Recorded via `make record-conformance SCENARIO=openai/recorded/<name>`; command + ZDR caveats documented in test/conformance/README.md. Recording surfaced and fixed four real parity bugs (function_call id, function_call_output JSON ordering, empty reasoning summary, web_search action + streaming annotations) — see design.md.
- [x] 9.5 Run `make test-conformance` and confirm all OpenAI cases pass (byte-identical chunks + request snapshots)

## 10. Docs and final verification

- [x] 10.1 Add `docs/providers/openai.md` (setup, supported features, provider options, built-in tools, limitations) and add a router entry in root `README.md` + `docs/README.md` index
- [x] 10.2 Run `make fmt vet lint` across the new module and `make test` (root + providers) and `make test-conformance`; fix all failures
- [x] 10.3 Review the Go implementation against upstream for behavioral drift (request body fields, event ordering, finish reason, usage, warnings) and document any intentional deviations in `design.md` Open Questions
- [x] 10.4 Update `AGENTS.md` Project Structure to list `providers/openai`
