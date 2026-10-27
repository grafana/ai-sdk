## 1. Shared helper

- [x] 1.1 Add `internal/streamrecover` with `ErrPanic`, `Error`, `Part` and `Recover`, and unit tests.

## 2. Providers

- [x] 2.1 openai-compatible: recover in the stream worker; test a panicking body before and after the first event.
- [x] 2.2 anthropic: recover in the transport reader and the stream adapter worker; test both and setup failure.
- [x] 2.3 openai: recover in the transport reader and the stream adapter worker, keep the recovered error non-retryable and pass the sentinel through preflight; test all three.

## 3. Gateway

- [x] 3.1 Treat a contained-panic part as an adapter failure in the streaming runtime.
- [x] 3.2 Add a handler-level test with a panicking openai-compatible stream and a concurrent normal request.

## 4. Middleware

- [x] 4.1 logger: contain per-part observation and terminal logging; classify the sentinel as `internal_error`.
- [x] 4.2 prometheus: contain stream finalization; classify the sentinel as `other`.
- [x] 4.3 agentobservability: contain stream finalization.

## 5. Validation

- [x] 5.1 Confirm each change with a test that fails or crashes without it.
- [x] 5.2 Run the affected modules' tests with the race detector.
