## 1. Public ToResponseMessages helper

- [x] 1.1 Create `to_response_messages.go` exporting `func ToResponseMessages(parts []provider.ContentPart) []provider.Message`. (Final shipped signature drops the `tools ToolSet` parameter — the Go port runs `Tool.ToModelOutput` eagerly during execution, so by the time content reaches this helper the per-tool conversion is already done. See doc-comment for the upstream-divergence rationale.)
- [x] 1.2 Implement the assistant-content pass: iterate `parts`, dispatch on `ContentPartType`, append to a working `[]provider.ContentPart` (skipping sources, empty text, and non-provider-executed `tool-result` / `tool-approval-response`). Cover `text`, `reasoning`, `reasoning-file`, `file`, `custom`, `tool-call`, and provider-executed inline `tool-result` arms.
- [x] 1.3 In the `tool-call` arm, sanitize a non-object `Input` (using a small `isJSONObject` helper, copy/extract from `streamtext.go`) by replacing it with `{}` to mirror upstream's invalid-call collapse.
- [x] 1.4 In the `tool-call` arm, look up any matching provider-executed `tool-result` by `ToolCallID` and append it inline immediately after the call, copying `Output` and `ProviderOptions`.
- [x] 1.5 Implement the tool-content pass: iterate `parts` again, collect non-provider-executed `tool-result` parts, then handle `tool-approval-response` parts including the `Approved == false` synthetic `execution-denied` `tool-result`.
- [x] 1.6 Append a single assistant `provider.Message` if any assistant content was produced, then a single tool `provider.Message` if any tool content was produced. Return the resulting `[]provider.Message`.
- [x] 1.7 Add doc comments on `ToResponseMessages` summarizing upstream parity (link to `packages/ai/src/generate-text/to-response-messages.ts`) and the per-variant behavior table.
- [x] 1.8 (Obsoleted by final signature: `tools` parameter dropped — N/A.)

## 2. Refactor appendToolResults to delegate

- [x] 2.1 In `streamtext.go`, replace the body of `appendToolResults` with: build a `[]provider.ContentPart` for the step (reasoning blocks first, then text if non-empty, then for each tool call followed immediately by its provider-executed result, then any remaining non-provider-executed results), call `ToResponseMessages(parts)`, and append the returned messages onto the input `msgs`.
- [x] 2.2 (Obsoleted by final signature: `tools` parameter dropped from both `ToResponseMessages` and `appendToolResults`. Final signatures: `func ToResponseMessages(parts []provider.ContentPart) []provider.Message` and `func appendToolResults(msgs []provider.Message, step StepResult) []provider.Message`.)
- [x] 2.3 (Obsoleted: call site at `streamtext.go` is `msgs = appendToolResults(currentMsgs, step)` — no second arg.)
- [x] 2.4 Remove dead helpers (`isJSONObject` if it ends up unused elsewhere) — `isJSONObject` remains in `streamtext.go` because `buildResponseContent` uses it; the public helper uses the same function in-package.
- [x] 2.5 Confirm by reading `streamtext.go` that `appendToolResults` no longer references `step.Text` directly except via the parts builder, and no longer constructs `provider.Message` directly.

## 3. Build the per-step content slice and surface Messages on Response

- [x] 3.1 In `text.go`, re-add `Messages []provider.Message` field on `aisdk.ResponseMetadata` with `json:"-"` tag and a doc comment that it carries the `ToResponseMessages` output (in-process only).
- [x] 3.2 In `streamtext.go`, factor a small private helper `buildResponseContent(step StepResult) []provider.ContentPart` that reuses the same ordering as `appendToolResults`'s parts builder (reasoning, text, tool-calls, tool-results). Both `appendToolResults` and the per-step Response.Messages population SHALL use this helper.
- [x] 3.3 In `processStep` (after the `if completed { ... }` block where `step.Content = buildContent(step)` is set), populate `step.Response.Messages = ToResponseMessages(buildResponseContent(step))`.
- [x] 3.4 In `streamTextWithConfig.run`, after pushing the step into `r.steps`, also assign `r.lastResponse = step.Response` so `result.Response().Messages` reflects the last step (this is already done; verify the assignment carries the new field through).
- [x] 3.5 Confirm `result.Response().Messages` matches `result.Steps()[len-1].Response.Messages` in a multi-step run (covered by `TestStreamTextResponseMessages`).

## 4. Unit tests for ToResponseMessages

- [x] 4.1 Create `to_response_messages_test.go` in `package aisdk`, importing `github.com/grafana/ai-sdk/provider` and `github.com/stretchr/testify/{assert,require}`.
- [x] 4.2 Add a table-driven test `TestToResponseMessages` covering: empty input, text only, reasoning only with provider signature (signature is preserved), text + tool-call, tool-call + tool-result -> assistant + tool messages, multipart tool result via `ToolResultOutput.Type == ToolOutputJSON`, file part appended, reasoning-file appended, redacted reasoning + thinking + final text in correct order, empty text dropped, mixed images + reasoning + tool-calls preserve order.
- [x] 4.3 Add provider-executed cases: provider-executed tool-call + tool-result are inlined in assistant message; mixed provider-executed and non-provider-executed routes correctly; provider-executed-only step produces no tool message.
- [x] 4.4 Add tool-approval-response cases: Approved=true with non-provider-executed tool-result still routes the result to the tool message; Approved=false adds a synthetic execution-denied tool-result.
- [x] 4.5 Add invalid tool-call sanitization case: `tool-call` with non-object `Input` (e.g. raw string) becomes `{}` in the assistant message.
- [x] 4.6 Add ProviderOptions preservation cases: text, reasoning, tool-call, tool-result each preserve `ProviderOptions` end-to-end.
- [x] 4.7 (Obsoleted by final signature: `tools` parameter dropped — N/A.)

## 5. Regression tests around appendToolResults

- [x] 5.1 (Obsoleted by final signature: `appendToolResults` no longer takes a `tools ToolSet` argument. Call sites in `streamtext_test.go::TestAppendToolResults_ProviderExecutedRouting` use the two-arg form `appendToolResults(nil, step)`.)
- [x] 5.2 Add a new subtest `reasoning preserved across tool-result rounds`: build a `StepResult` with `step.Reasoning = []ReasoningOutput{{Text: "thinking", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"sig_xyz"}`)}}}`, one `ToolCall`, one `ToolResult`. Call `appendToolResults`, assert the assistant message's first content part is `Type == ContentPartTypeReasoning`, `Text == "thinking"`, and `ProviderOptions["anthropic"]` resolves to a `RawProviderOption` carrying `{"signature":"sig_xyz"}`.
- [x] 5.3 Add a subtest `text omitted when empty`: build a `StepResult` with `step.Text == ""` and one `ToolCall`. Assert the assistant message contains only the `tool-call` part — no empty `text` part.
- [x] 5.4 Add a subtest `result.Response().Messages populated`: drive a small two-step `StreamText` run via the existing test mock (or extend an existing test) and assert `result.Response().Messages` is non-empty and equal to `result.Steps()[len-1].Response.Messages`.

## 6. Documentation

- [x] 6.1 Update `doc.go` (or the package overview comment) if it mentions message-building helpers, adding a one-line pointer to `ToResponseMessages`.
- [x] 6.2 `README.md` does not include a multi-step example that needs updating; the `ToResponseMessages` doc comment plus the new field doc on `ResponseMetadata` provide the canonical reference.

## 7. Verification

- [x] 7.1 Run `go build ./...` from the repo root and confirm a clean build.
- [x] 7.2 Run `go vet ./...` and confirm no warnings on touched files.
- [x] 7.3 Run `make test` and confirm root + anthropic + grafana test suites pass.
- [x] 7.4 Run `make lint` (golangci-lint) and confirm clean lint (0 issues across all three modules).
- [x] 7.5 Run the conformance suite (`make test-conformance`); all `TestConformance/recorded/*` and `TestConformance/upstream/*` cases pass — fixture parity preserved.
- [x] 7.6 Confirm wire format is unchanged: `make test-integration` (UI message stream tests) passes without any update to fixtures.
