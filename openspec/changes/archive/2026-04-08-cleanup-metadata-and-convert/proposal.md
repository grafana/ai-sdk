## Why

The root `aisdk` package duplicates `RequestMetadata` (byte-for-byte identical to `provider.RequestMetadata`) and `ResponseMetadata` (duplicates provider fields instead of embedding). This causes manual field-by-field copying in `streamtext.go`, naming confusion across packages, and carries a dead `Messages` field. Separately, `ConvertToModelMessages` accepts an unused `context.Context` parameter, forcing all callers to pass a context for no reason. Cleaning these up reduces maintenance surface and clarifies the type hierarchy.

## What Changes

- **Remove `aisdk.RequestMetadata`**: Eliminate the duplicate type. Use `provider.RequestMetadata` directly in `StepResult` and all internal references.
- **Embed `provider.ResponseMetadata` in `aisdk.ResponseMetadata`**: Replace duplicated fields (`ID`, `ModelID`, `Timestamp`) with struct embedding. This eliminates field-by-field copying in `streamtext.go`.
- **Remove dead `Messages` field**: `aisdk.ResponseMetadata.Messages` is never written or read in production code. Remove it.
- **Remove unused `context.Context` from `ConvertToModelMessages`**: Drop the unused parameter from the signature and update all callers.

## Capabilities

### New Capabilities

None. This is a pure refactoring with no new capabilities.

### Modified Capabilities

None. No spec-level behavior or requirements change -- only internal type structure and function signatures.

## Impact

- **Public API (aisdk package)**: `RequestMetadata` type removed (consumers must use `provider.RequestMetadata`). `ResponseMetadata` gains embedded `provider.ResponseMetadata` instead of flat fields (field access unchanged, but type assertions and struct literals change). `ConvertToModelMessages` signature changes (one fewer parameter).
- **Internal code**: `streamtext.go` field-by-field copy replaced with embedding assignment. `generatetext.go` updated for new `ResponseMetadata` shape. Test helpers in `convert_test.go` updated.
- **No behavioral changes**: Wire format, SSE output, and provider interaction remain identical.
