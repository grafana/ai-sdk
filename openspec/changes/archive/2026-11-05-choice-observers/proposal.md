# Observe native alternatives without merging or duplicate billing

The opt-in Go choice domain can interleave identical native item IDs across
alternatives. Existing observers merge those items, close usage on individual
finishes, and can treat partial completion as successful cancellation.

Preserve each alternative independently, expose explicit output ranges in
observability metadata, honor existing logger capture policy, and wait for the
global usage and completion boundary. Buffered fallback accepts only complete
sets of requested alternatives.

This is an intentional extension of pinned `@ai-sdk/openai-compatible@3.0.82`,
which projects only the first choice. The default single-choice behavior and
ProviderWire serialization are unchanged; synthetic tests prove the Go extension,
not upstream or live-provider parity.
