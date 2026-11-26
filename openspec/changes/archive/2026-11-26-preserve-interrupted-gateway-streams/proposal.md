## Why

The Gateway replaces a provider's `interrupted` finish with a retryable internal error when the provider was cut off inside a text, reasoning or tool-input block. Clients receive the partial output but lose the actual finish reason.

## What changes

- Accept an interrupted finish with open blocks without writing synthetic block ends or changing finish data.
- Keep every other finish reason and all unfinished tool previews strict. Retire interrupted tool input without releasing its ID or allowing it to become a call.
- Verify ProviderWire SSE, pinned and Go Gateway clients, and shared stream observers.

## Capabilities

### Modified capabilities

- `gateway-execution-runtime`: Define the interrupted finish transition in the shared output lifecycle.
- `providerwire-v4-streaming-runtime`: Forward an interrupted segment as emitted and preserve normal stream termination.

## Impact

The shared validator affects execution and extension observations. ProviderWire clients need no update; the schema already supports `interrupted`. Native adapter mappings are unchanged.

## Non-goals

Provider-side interruption detection, native protocol mapping changes, core/UI changes, and relaxing preliminary tool-result handling.
