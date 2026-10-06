## 1. Reproduce

- [x] 1.1 At `0e8f35e`, confirm `DoGenerate` with `MaxOutputTokens` unset or 32000 returns "streaming is required for operations that may take longer than 10 minutes" without contacting the server.
- [x] 1.2 Confirm the SDK guard is identical in v1.78.0, the version pinned on `main`.

## 2. Test and fix

- [x] 2.1 Add `TestDoGenerate_LargeMaxTokensReachTheAPI` covering the model default, 32000 tokens, a context deadline, a caller timeout, a refusal caused by a model's non-streaming token limit (`claude-opus-4-0` at 10000 tokens, which needs the ten-minute floor) and small `max_tokens`, asserting the request arrives and its `X-Stainless-Timeout` value. Removing the floor makes the token-limit case fail.
- [x] 2.2 Run it before the fix: the default, 32000 and deadline cases fail.
- [x] 2.3 Add `nonStreamingTimeout` and apply it in `DoGenerate`.

## 3. Validate

- [x] 3.1 `go test -race ./...` in `providers/anthropic`: ok.
- [x] 3.2 `mise run test-conformance`: PASS.
- [x] 3.3 `mise run fmt-check`, `mise run vet`, `mise run lint`: clean.
