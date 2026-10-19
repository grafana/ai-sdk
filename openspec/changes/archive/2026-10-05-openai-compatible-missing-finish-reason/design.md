## Context

At `0e8f35e`, `runStream` initializes `streamState.finishReason` to `other`, and `handleChoice` overwrites it only when a chunk carries `finish_reason`. `flush` forces `error` only when `errorEmitted` is set. A stream that simply stops therefore finishes as `other`.

Upstream `packages/openai-compatible/src/chat/openai-compatible-chat-language-model.ts` at `@ai-sdk/openai-compatible@3.0.57` declares `finishReason` as undefined, sets it from `choice.finish_reason` or to `error` on parse and error chunks, and in `flush` emits an `InvalidResponseDataError` and finishes with `error` when it is still undefined.

## Decisions

### 1. A flag instead of an unset finish reason

`streamState.finishReason` is a value type used directly in the finish part, so a `finishReasonSet` flag records the upstream "still undefined" state without changing how the finish part is built. The two error helpers that set `error` also set the flag, matching upstream, where an error chunk sets the finish reason and suppresses the flush error.

### 2. An APICallError with the module's message prefix

The Go module reports stream faults as `APICallError` values with an `openai:` prefix, and has no `InvalidResponseDataError` type. The new error follows that convention and carries the endpoint URL. The message mirrors upstream's text.
