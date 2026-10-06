## Context

`provider.ResponseMetadata` (`ID`, `ModelID`, `Timestamp`) and the
`PartResponseMeta` stream event already expose the served model id, but not the
served provider. Each concrete model already knows its provider name via
`Provider()` (e.g. Anthropic-direct returns `anthropic`, Vertex returns
`anthropic.vertex`). The data exists at the leaf; it is simply not propagated
onto the response.

The motivating consumer is fallback-aware metrics: `fallback.Model.Provider()`
always returns `candidates[0]` (the primary), so when a request fails over to a
later candidate, consumers cannot tell which provider served it.

## Goals / Non-Goals

**Goals:**
- Expose the served provider on the response metadata for both generate and
  stream paths.
- Keep the change additive and wire-backward-compatible.
- Require no change to `fallback.Model`: the served candidate tags its own
  output and the wrapper forwards it.

**Non-Goals:**
- A dedicated "fallback activated" signal/metric (separate concern).
- Backfilling provider for providers other than Anthropic in this change
  (others can populate the field incrementally; empty is a valid zero value).
- Any change to the SSE `UIMessageChunk` wire format consumed by `@ai-sdk/react`.

## Decisions

**Decision: Carry the provider on the response metadata, not via the fallback wrapper.**
Tag the served candidate's own output (`GenerateResponse` and
`PartResponseMeta`) with its provider, then let every wrapper forward it
verbatim. This mirrors how `ModelID` already flows and is universal — retry,
fallback, and any future router benefit without per-wrapper code.
- Alternative considered: add an "active candidate" accessor/callback on
  `fallback.Model`. Rejected: fallback-specific, more invasive, and does not
  generalize to other wrappers.
- Alternative considered: have consumers unwrap/inspect the fallback candidates.
  Rejected: fragile, couples consumers to wrapper internals, and still cannot
  identify which candidate actually ran.

**Decision: Add `Provider` to `ResponseMetadata` (shared by `GenerateResponse`) and to `StreamPart`.**
`GenerateResponse` embeds `ResponseMetadata`, so the generate path gets the field
for free. The stream path carries response metadata fields directly on
`StreamPart` (`ResponseID`, `ModelID`), so `Provider` is added there too and
mapped into `ResponseMetadata` during `StreamText` step processing.

**Decision: Thread the provider name from the model into the converters.**
`convertResponse` and `consumeStream`/`streamAdapter` receive the model's
`providerName` so they can set it on the emitted metadata, exactly where
`ModelID` is already set.

**Decision: All new fields are optional with `omitempty`.**
Empty provider remains valid for providers that don't set it yet; JSON wire
round-trip is unaffected for existing payloads.

## Risks / Trade-offs

- [Wire/JSON compatibility] → New fields use `omitempty`; absent on existing
  payloads, so decoders and the provider-wire round-trip stay compatible. The
  provider-wire spec is updated to cover the new field.
- [Spec drift] → `provider-v4-core-types` currently pins `ResponseMetadata` to
  three fields; the delta spec updates that requirement to four and adds a
  requirement for provider population, keeping spec and code in sync.
- [Partial provider coverage] → Only Anthropic populates `Provider` in this
  change; other providers leave it empty until updated. Acceptable: empty is a
  safe zero value and consumers already tolerate missing model ids.
