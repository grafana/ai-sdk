## Why

Consumers need a public, curated way to discover which Anthropic model IDs this package knows about on the direct Anthropic API and on the Vertex AI partner channel. Today they must duplicate unexported package data or accept arbitrary IDs and discover Vertex incompatibility only when a request or fallback path executes.

## What Changes

- Add public helpers that return curated model ID lists for the direct Anthropic API and Vertex Anthropic surfaces.
- Add a public helper that returns model IDs available on both surfaces for fallback and model-catalog registration use cases.
- Export the existing Vertex model ID resolver so consumers can map direct Anthropic IDs to Vertex canonical IDs without duplicating package internals.
- Keep all returned lists advisory, deterministic, sorted, and safe for callers to mutate.

## Capabilities

### New Capabilities
- `anthropic-model-ids`: Public Anthropic model ID enumeration and Vertex ID resolution helpers.

### Modified Capabilities

## Impact

- Affects the `anthropic` module public API, primarily `anthropic/models.go` and its tests.
- Adds godoc-visible exported symbols for model catalog and fallback consumers.
- Does not add new dependencies or change request wire format, provider interfaces, or existing model invocation behavior.
