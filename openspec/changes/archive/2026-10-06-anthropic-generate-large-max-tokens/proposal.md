## Why

A dogfood run of `main` found that `DoGenerate` never sends a request when `MaxOutputTokens` is unset or above about 21333. `anthropic-sdk-go`'s `CalculateNonStreamingTimeout` refuses a non-streaming call it expects to run past ten minutes, unless a request timeout is set, and returns "streaming is required for operations that may take longer than 10 minutes". Current models' default maximums trip it. `GenerateText` streams and is unaffected, but three callers use `DoGenerate`: the AI Gateway's unary ProviderWire path when a caller leaves `maxOutputTokens` unset, `middleware.SimulateStreaming()`, and `fallback.Model`, which treats the non-`APICallError` as unknown and silently moves to the next candidate. The registered `@ai-sdk/anthropic@4.0.65` posts these requests directly, with no client-side refusal.

## What Changes

- Before calling the SDK, `DoGenerate` asks `anthropic.CalculateNonStreamingTimeout` whether it would refuse. Only then does it add a request timeout: the remaining context deadline if any, otherwise the SDK's own estimate, at least ten minutes.
- A caller's own request timeout passes the SDK check, so it is never replaced, and small requests keep the SDK's default.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `anthropic-provider-construction`: Require that non-streaming requests are not refused before sending.

## Impact

`providers/anthropic/model.go` and its tests; `NewVertex` shares `DoGenerate` and gets the same fix. No conformance input exercises this path. The request now carries an `X-Stainless-Timeout` header with the chosen timeout, as every SDK request with a timeout does.
