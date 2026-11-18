# Proposal

## Why

The Prometheus middleware classified a stream as `canceled` whenever its context was canceled by the time the stream finalized, before looking at an error part it had already observed or a recorded denial (#822). A consumer that stops reading after the first in-band error cancels the context, so a provider error or a policy denial was counted as a cancellation. The structured logger ranks the same stream denied, then error, then canceled, so logs and metrics disagreed for one call.

## What Changes

- Finalize a stream with its observed error or denial outcome before considering cancellation.
- Ignore an error part the tee receives after the context is canceled, so a stream canceled first stays `canceled` as the existing scenario requires. The tee's select can pick a buffered part after cancellation, and without this a provider error raised by the cancellation itself would count as `error` about half the time.
- State the precedence in the stream requirement's scenarios and the package documentation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `prometheus-middleware`: an observed error part or denial outranks a later cancellation.

## Impact

`middleware/prometheus` only. A stream that carried an error part and was then canceled now records `status="error"` or `status="denied"` instead of `canceled`. No API change.
