## Why

OpenAI-compatible request transforms can rewrite the body or return an error, but cannot report a compatibility adjustment through the call's warnings. Registered upstream `@ai-sdk/openai-compatible` 3.0.71 supplies the current warnings to its transform callback. This closes grafana/ai-sdk#451.

## What Changes

- Add a warning-aware request transform option, preserving the existing body/error callback.
- Pass each call's existing warnings to the callback and propagate its returned warning slice through generation and stream-start.
- Keep transformed HTTP bodies and request metadata identical, and reject transform errors or nil bodies before transport.
- Document option precedence, nil disabling, and concurrent callback responsibilities.

## Capabilities

### New Capabilities

- `openai-compatible-request-transform`: Request rewriting with per-call warning reporting and backward-compatible configuration.

### Modified Capabilities

None.

## Impact

Changes are confined to the OpenAI-compatible provider, its tests and guide. No new runtime dependencies, provider/core contracts, new stream-part types or wire formats change.

## Non-goals

No backend-specific field policy, asynchronous callbacks, transform chaining, upstream baseline upgrade, or fabricated/live provider recordings.
