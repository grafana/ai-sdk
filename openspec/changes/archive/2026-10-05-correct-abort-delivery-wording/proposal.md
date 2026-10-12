## Why

The `Cancellation releases an unread stream` requirement, added by `2026-10-04-release-unread-stream-on-cancel` (#345), says a consumer still reading after cancellation receives `abort` and `finish`. No cancellation path emits `finish`: `abort()` emits the `abort` part and the stream then closes. The `conformance-testing` requirement "Cancellation after partial output" requires the stream to end with exactly one `abort` chunk, the `test/conformance/ui/stream-abort/*/expected.jsonl` fixtures end on `abort`, and upstream `ai@7.0.116` `abort()` enqueues `abort` and closes the controller. A dogfood probe asserting `[abort finish]` got `[abort]`. The requirement contradicts the code, the upstream baseline and another merged requirement, so the requirement is what is wrong.

## What Changes

- Say that a reading consumer receives the `abort` part that ends a canceled stream, and that `FullStream` then closes with no `finish` part.
- Correct the matching comment on `emit` in `streamtext.go`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `stream-text-lifecycle`: Correct the post-cancellation delivery wording in `Cancellation releases an unread stream`.

## Impact

Spec text and one code comment. No behavior, test or fixture change.
