## Context

The `anthropic` module already contains `vertexModelMap`, an unexported mapping from direct Anthropic model IDs to Vertex partner-channel IDs, and `resolveVertexModelID`, an unexported resolver used by `NewVertex`. There is no public direct-Anthropic model list, no public Vertex model list, and no public way to compute the overlap needed by model catalogs or fallback chains.

Upstream Vercel ai-sdk exposes separate Anthropic and Google Vertex Anthropic model ID unions. The Go port should expose equivalent discoverability through functions, while preserving Go's plain `string` model ID API and keeping arbitrary future model IDs accepted by `New` and `NewVertex`.

## Goals / Non-Goals

**Goals:**

- Provide deterministic, copied, sorted model ID slices for direct Anthropic and Vertex Anthropic surfaces.
- Provide a deterministic intersection helper for models usable with both `New` and `NewVertex`.
- Export the existing Vertex ID resolution behavior for downstream consumers that need the canonical Vertex ID.
- Keep the curated lists source-aligned with upstream ai-sdk and official Anthropic/Vertex documentation.

**Non-Goals:**

- Add typed-string enum constants for each model.
- Validate or reject model IDs passed to `New` or `NewVertex`.
- Add live model discovery through provider APIs.
- Add model capability metadata beyond the existing internal `getModelCapabilities` behavior.

## Decisions

1. Expose functions instead of exported mutable package variables.

   `ModelIDs`, `VertexModelIDs`, and `DualAvailableModelIDs` will return new slices on every call. This gives callers the discoverability they need without exposing mutable global state or the internal direct-to-Vertex mapping shape.

2. Keep surface-specific ID formats distinct.

   `ModelIDs` will return IDs in the form accepted by `New`, including dash-separated date-pinned IDs where applicable. `VertexModelIDs` will return Vertex partner-channel IDs in the form expected by `NewVertex` after resolution, including `@YYYYMMDD` pins or bare names where Vertex serves a model without a date suffix. `DualAvailableModelIDs` will return IDs in the direct `New` form so consumers can build direct and Vertex fallback pairs from the same ID.

3. Reuse resolver semantics for normalization.

   The dual-availability helper will compare direct Anthropic IDs against Vertex availability after applying the same direct-to-Vertex normalization used by `NewVertex`. This avoids a second mapping path and makes the helper's guarantee match runtime behavior.

4. Export the resolver as `ResolveVertexModelID` and keep the unexported call path simple.

   `ResolveVertexModelID` will preserve the current behavior: known direct IDs map to curated Vertex IDs, already `@`-pinned IDs are returned unchanged, and unknown unpinned IDs receive `@latest`. `NewVertex` can call the exported function directly or keep a small unexported alias only if needed to minimize churn.

5. Test the API contract rather than hard-coding every model behavior in callers.

   Tests will cover sorting, copy safety, deterministic results, subset/intersection guarantees, and resolver behavior for representative known, pinned, and unknown IDs. The curated data itself remains package-owned and can be updated as upstream model lists change.

## Risks / Trade-offs

- Curated lists can drift from upstream docs -> keep source URLs in godoc and tests close to the data so updates are obvious.
- Returning Vertex IDs in `@`-pinned form differs from direct Anthropic date-pinned dash IDs -> keep the two surfaces separate and make `DualAvailableModelIDs` return direct `New` form for fallback registration.
- The dual helper only guarantees package-recognized overlap, not live regional availability or account entitlements -> document the list as advisory and avoid runtime network discovery.
- Exporting the resolver makes current unknown-ID `@latest` behavior public -> retain that behavior because it is already observable through `NewVertex` request construction and useful for consumers.
