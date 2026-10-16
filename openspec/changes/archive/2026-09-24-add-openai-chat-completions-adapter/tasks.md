## 1. Adapter contract

- [x] 1.1 Add independent adapter DTO and request validation.
- [x] 1.2 Translate storage/strict defaults and freeze backend policies.
- [x] 1.3 Add unary serialization and SSE state ownership.

## 2. Host integration

- [x] 2.1 Add Bearer adapter and OpenAI routes/errors with existing middleware.
- [x] 2.2 Compose the adapter handler using validated duration/byte limits.

## 3. Verification and handoff

- [x] 3.1 Test defaults, history, output, malformed lifecycle and resource bounds.
- [x] 3.2 Test official OpenAI SDK against real command and deterministic upstreams.
- [x] 3.3 Exercise concurrency/race/cancellation/backpressure/shutdown.
- [x] 3.4 Regress claimed-stream setup/committed error latency with a 200ms drain budget and a sub-100ms response requirement; prove stalled/hot cleanup termination.
- [x] 3.5 Verify shutdown with a committed adapter SSE connection plus adapter and ProviderWire unary requests against the real command.
- [x] 3.6 Resolve standalone errcheck/deprecated proxy API findings without suppressions and repeat lint.
- [x] 3.4 Publish exact support matrix, protocol authority, test ownership and limitations.
- [x] 3.5 Run module/boundary/parity checks and record exact results.
