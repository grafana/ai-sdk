## 1. Model ID Data

- [x] 1.1 Add a curated direct Anthropic model ID list in `anthropic/models.go`, sourced from upstream ai-sdk and official Anthropic model docs.
- [x] 1.2 Keep the Vertex Anthropic curated data in one source of truth that supports both list enumeration and direct-to-Vertex resolution.
- [x] 1.3 Ensure curated lists are stored or prepared so exported accessors can return deterministic sorted output.

## 2. Public API Implementation

- [x] 2.1 Export `ModelIDs() []string` with a godoc comment documenting its advisory direct-Anthropic surface semantics.
- [x] 2.2 Export `VertexModelIDs() []string` with a godoc comment documenting its Vertex partner-channel ID format.
- [x] 2.3 Export `DualAvailableModelIDs() []string` and compute the intersection using `ResolveVertexModelID` semantics.
- [x] 2.4 Export `ResolveVertexModelID(modelID string) string` while preserving known-model, already-pinned, and unknown `@latest` behavior.
- [x] 2.5 Update `NewVertex` to use the exported resolver without changing constructor behavior.

## 3. Tests

- [x] 3.1 Update resolver tests to call `ResolveVertexModelID` and cover known, already-pinned, bare Vertex, and unknown inputs.
- [x] 3.2 Add tests that `ModelIDs`, `VertexModelIDs`, and `DualAvailableModelIDs` are non-empty, sorted, deterministic, and copy-safe.
- [x] 3.3 Add tests that dual-available IDs are a subset of `ModelIDs` and resolve to IDs present in `VertexModelIDs` without `@latest`.
- [x] 3.4 Add tests that `New` and `NewVertex` still accept IDs absent from the curated lists.

## 4. Verification

- [x] 4.1 Run `gofmt` on changed Go files.
- [x] 4.2 Run `go test ./...` from the `anthropic/` module.
