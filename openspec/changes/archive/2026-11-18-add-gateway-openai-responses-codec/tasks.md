# Tasks

## 1. Codec

- [x] 1.1 Decode the request subset, input items, tools and settings into provider call options, and refuse everything else. Verify with `go test ./openai/responses` in `ai-gateway`.
- [x] 1.2 Add the per-target translator for `store`, parallel tool calls, reasoning summary, text verbosity and strict JSON schema. Verify with `go test ./openai/responses`.
- [x] 1.3 Encode the unary response and the fixed error documents from the category mapping. Verify with `go test ./openai/responses`.
- [x] 1.4 Encode provider stream parts as Responses SSE events with the terminal event rules. Verify with `go test -race ./openai/responses`.

## 2. Official clients over recordings

- [x] 2.1 Add `ai-gateway/test/openai-responses-codec`, which replays the in-scope recorded inputs to `openai-go` directly and through the Go OpenAI provider and the codec, and compares the results. Verify with `go test -race ./test/openai-responses-codec` in `ai-gateway`.
- [x] 2.2 Add the `openai` Node SDK comparison over the same recordings and a `mise` task that runs both. Verify with `mise run test-openai-responses-codec`.

## 3. Checks

- [x] 3.1 Run `mise run lint`, `openspec validate --all` and `mise run validate-parity-baseline`.
