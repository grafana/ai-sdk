## Why

The strict ProviderWire V4 handler currently rejects every streaming request, so `@ai-sdk/gateway@4.0.52` cannot stream text through the production Go runtime. Phase 4 must add a bounded SSE commitment and lifecycle layer before service authentication, reusable clients, or later stream-part families can build on the protocol.

## What Changes

- Extend the strict ProviderWire V4 handler to dispatch `ai-language-model-streaming: true` requests through `DoStream` after the existing bounded body, standard JSON, schema, typed mapping, and catalog-resolution path.
- Add explicit race-safe stream setup ownership and commitment behavior: cancellation or total expiry owns unclaimed outcomes, every returned non-nil channel has exactly one cleanup owner, setup failures and nil streams remain bounded non-2xx JSON, and a successfully claimed non-nil stream commits `text/event-stream`.
- Normalize an optional provider `stream-start` into exactly one public start event using a streaming-specific value-safe warning mapper while unary output remains minimal; align Anthropic warning timing with the pinned upstream provider by moving warnings from `finish` to start after the existing initial-event preflight.
- Add private stream DTOs and a text-only state machine for canonical response metadata, text start/delta/end, non-terminal provider errors, and terminal finish; bound cumulative processing and retained ID state with a positive stream-part limit, and suppress later provider parts after authoritative finish while asynchronous drain completes.
- Add bounded complete-event SSE encoding and full writes through response-controller flushing; emit clean EOF immediately after authoritative finish, never emit `[DONE]`, and synthesize at most one safe terminal adapter error before finish.
- Add pure, controllable cancellation/timeout precedence, request cancellation, provider-work cancellation, writer-failure termination, and deadline-authoritative bounded asynchronous channel drain behavior.
- Add raw SSE, ordering, privacy, boundary, timeout, cancellation, drain, and pinned Gateway-client integration tests, including multiple ordered provider errors followed by later content and finish.

## Capabilities

### New Capabilities
- `providerwire-v4-streaming-runtime`: Strict streaming text setup, SSE commitment, normalized start/warnings, metadata and text lifecycle validation, safe stream errors, bounded framing, cancellation, timeout, drain, and clean-EOF evidence.

### Modified Capabilities
- `providerwire-v4-unary-runtime`: Extend the constructed production handler and golden replay matrix to dispatch valid streaming envelopes and add the stream-part limit while preserving phase-3 minimal unary output; add streaming-specific fixed-prose warning values without changing unary behavior.
- `providerwire-v4-http-contract`: Replace the deferred-streaming boundary with production Go streaming replay, raw SSE authority, and pinned-client execution evidence.

## Impact

- Primary code: `gateway/providerwire/v4`, a test-only stream-event schema, handler dispatch, stream invocation/state/framing code, fixed precommit and terminal errors, stream-owned bounded framing helpers, and Anthropic stream-start warning emission.
- Tests: ProviderWire V4 Go runtime tests, `test/providerwire-v4` golden replay and pinned-client integration, cross-language integration coverage for SSE behavior, and affected Anthropic provider/conformance expectations regenerated from unchanged provenance-valid inputs.
- Configuration/API: `v4.Limits` gains stream-part cardinality, complete SSE frame, stream idle, and bounded post-cancellation drain limits; no registered package baseline upgrade.
- Protocol scope: streaming text, response metadata, warnings, usage, finish, and safe provider error parts only; reasoning, tools, files, sources, custom content, raw output, authentication, discovery, the Go V4 client, and service deployment remain deferred.
