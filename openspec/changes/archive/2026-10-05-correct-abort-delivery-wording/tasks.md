## 1. Correct

- [x] 1.1 Replace "a reading consumer still receives `abort` and `finish`" with the `abort` part that ends a canceled stream, and make the scenario say `FullStream` closes with no `finish` part.
- [x] 1.2 Correct the comment on `emit` that mentions losing abort and finish parts.

## 2. Validate

- [x] 2.1 Confirm `TestStreamTextContextCancellation` and `test/conformance/ui/stream-abort/*/expected.jsonl` already encode `abort` with no `finish`: `go test ./ -run TestStreamTextContextCancellation` and `mise run test-conformance` pass unchanged.
- [x] 2.2 `openspec validate --strict` on this change: valid.
