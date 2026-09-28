## 1. Reproduce the generated-output gap

- [ ] 1.1 Add table-driven `GenerateText` and `ToolLoopAgent.Generate` focused tests using synthetic provider streams: for each object, array, choice, and JSON mode, cross `stop`, `length`, `content-filter`, `error`, `other`, and `tool-calls` with valid nonempty (non-null for JSON), invalid nonempty, and empty final text. Assert parsed value or `OutputError`/missing output, `output.Value[T]` via `output.ObjectResult[T]` or a test-local `OutputAccessor` adapter (not directly on `GenerateTextResult`), retained `Text`/`FinishReason`, and no top-level parse error. Include schema-invalid nonempty object/array/choice and malformed JSON input. Keep raw JSON `null` outside this valid-output matrix: it parses to nil without error but the existing typed accessor reports missing output.
- [ ] 1.2 Update `TestGenerateText_OutputWithLengthFinishReason` to expect its existing valid JSON to parse, and confirm focused tests fail on the current stop-only generated behavior before changing implementation. Include a final-step-after-tool continuation case so intermediate tool output is not mistaken for final output.

## 2. Align terminal generated parsing

- [ ] 2.1 Implement generated-only completion selection in `options.go`, `agent.go`, and `streamtext.go`: on the final step parse if finish is `stop` or finish is not `tool-calls` and final text is nonempty; leave streaming completion rules unchanged.
- [ ] 2.2 Run the focused generated test matrix and confirm stop-empty produces a parse error, non-stop-empty and all `tool-calls` finishes skip parsing, and non-stop-invalid fills `OutputError` without discarding raw text, finish, or result.

## 3. Guard existing streaming and parity boundaries

- [ ] 3.1 Run existing `StreamText` and agent streaming length-finish tests and partial/array-element delivery tests; verify the recorded `test/conformance/openai/recorded/structured-json-output-length/input.chunks.txt` remains byte-identical and no synthetic input is placed in provider `recorded/` or `upstream/`.
- [ ] 3.2 Run `go test ./...` and `mise run parity-check`; inspect whether any committed expectation changes are justified. No UI wire changes are anticipated, so do not regenerate frontend snapshots without evidence. Update the coverage map only if the stable evidence boundary changes.
