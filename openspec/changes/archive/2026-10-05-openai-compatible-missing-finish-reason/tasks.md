## 1. Reproduce

- [x] 1.1 At `0e8f35e`, confirm that a stream cut before `finish_reason`, a `[DONE]` without one, and an HTTP 200 HTML body each end with finish `other`, no error part, and `GenerateText` returning `err == nil`.
- [x] 1.2 Read the registered `@ai-sdk/openai-compatible@3.0.57` `flush` and its finish-reason assignments.

## 2. Test and fix

- [x] 2.1 Add `TestDoStream_EndsWithoutFinishReason` with the three truncated shapes, a normal completion control, and two streams whose earlier error already decided the finish reason (a nameless tool call, a dropped connection), each of which must carry exactly one error part. The test stays inside the provider module so its `go.mod` remains tidy.
- [x] 2.2 Run it before the fix: the three truncated cases fail.
- [x] 2.3 Add `finishReasonSet`, set it in `handleChoice` and both error helpers, and emit the error in `flush` when it is unset.

## 3. Validate

- [x] 3.1 `go test -race ./...` in `providers/openai-compatible`: ok.
- [x] 3.2 `mise run test-conformance`: PASS, no fixture changes. `mise run test-ai-gateway-source` and `mise run test-integration`: pass.
- [x] 3.3 `mise run fmt-check`, `mise run vet`, `mise run lint`: clean.
