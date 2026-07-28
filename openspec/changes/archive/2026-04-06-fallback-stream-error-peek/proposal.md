## Why

`fallback.Model.DoStream` only checks the synchronous `error` return from candidates. Providers like Anthropic return `(result, nil)` immediately and defer HTTP errors (404, auth failures) to the stream channel as `PartError`. This means fallback never triggers for streaming errors -- the broken stream is returned directly to the caller.

## What Changes

- `fallback.DoStream` will peek at the first chunk from each candidate's stream before returning
- If the first chunk is a `PartError`, it is treated as a synchronous error and the next candidate is tried
- If the first chunk is valid data, the stream is wrapped to replay the peeked chunk followed by the rest
- The existing `decider` function is applied to stream errors, maintaining consistent fallback policy

## Capabilities

### New Capabilities
- `fallback-stream-error`: Fallback detection of leading stream errors via first-chunk peek in `fallback.DoStream`

### Modified Capabilities

## Impact

- `fallback/fallback.go`: `DoStream` method changes from pass-through to peek-and-decide
- No provider changes required (Anthropic, or any future provider)
- No wire format or public API changes
- Minor latency addition: `DoStream` blocks until the first stream chunk arrives (negligible for error cases, sub-millisecond for healthy streams)
