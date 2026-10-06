## Why

A dogfood run of `main` at `0e8f35e` found that the OpenAI-compatible provider reports a truncated stream as a normal completion. `streamState` starts with finish reason `other`, so a stream that closes before any chunk carries `finish_reason`, a stream that sends `[DONE]` without one, and an HTTP 200 HTML body from a misbehaving proxy all end with finish `other` and no error part. `GenerateText` then returns the partial text, or no text, with `err == nil`. The registered `@ai-sdk/openai-compatible@3.0.57` `flush` sets finish reason `error` and enqueues `InvalidResponseDataError("Response stream ended without a finish reason.")` in that case. #349 reports the same class of bug for Bedrock.

## What Changes

- Track whether the stream reported a finish reason, counting the error paths that already set `error`.
- At flush, when none was reported, emit an error part with `openai: response stream ended without a finish reason` and finish with `error`.
- Add a regression test covering the three truncated shapes and a normal completion.

## Capabilities

### New Capabilities

- `openai-compatible-stream-termination`: How the OpenAI-compatible provider ends a stream that never reported a finish reason.

### Modified Capabilities

None.

## Impact

`providers/openai-compatible/stream.go` and its tests. Every committed OpenAI-compatible conformance input carries a `finish_reason`, so no fixture or expectation changes. The AI Gateway image builds with `go.gateway.work`, which uses this module from the same commit, so a Gateway built after this change reports truncated `openai-compatible` upstream streams as errors too.
