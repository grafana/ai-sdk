## Context

The `Part` interface in `message.go` defines `PartType() string` as the public type discriminator for UIMessage parts. Two concrete types have `PartType()` return values that differ from their JSON wire `type`:

| Type | `PartType()` now | Wire `type` | Upstream TS `type` |
|------|-----------------|-------------|-------------------|
| `DataPart` | `"data"` | `"data-{DataName}"` | `data-${name}` |
| `ToolInvocationPart` | `"tool-invocation"` | `"tool-{toolName}"` | `tool-${name}` |

All other Part types (`TextPart`, `ReasoningPart`, `FilePart`, etc.) already have `PartType()` matching their wire type exactly.

Internally, the Go codebase uses Go type switches (not `PartType()` strings) so the mismatch hasn't caused bugs. However, `PartType()` is exported and its current values are misleading to consumers.

## Goals / Non-Goals

**Goals:**
- Make `PartType()` return the wire-compatible type string for `DataPart` and `ToolInvocationPart`, aligning with the upstream TypeScript SDK.
- Maintain full JSON wire format compatibility (no serialization changes).
- Update all test assertions to use the new return values.

**Non-Goals:**
- Changing the serialization/deserialization logic in `message_json.go` (it's already correct).
- Removing the `UIPartData` or `UIPartToolInvocation` constants (they remain useful as base prefixes for `strings.HasPrefix` checks).
- Adding new Part types or changing the Part interface signature.

## Decisions

### 1. Use value receiver with field access for PartType()

`DataPart.PartType()` becomes `"data-" + p.DataName` and `ToolInvocationPart.PartType()` becomes `"tool-" + p.ToolName`. This requires changing from a type receiver `(DataPart)` to a value receiver `(p DataPart)` to access the instance fields.

**Why not a method on pointer receiver?** Part implementations are value types throughout the codebase. Switching to pointer receivers would break the `Part` interface satisfaction for value instances.

### 2. Keep UIPartData and UIPartToolInvocation constants

The constants `UIPartData = "data"` and `UIPartToolInvocation = "tool-invocation"` remain as-is. They serve as the base prefix for prefix-matching logic (e.g., `strings.HasPrefix(env.Type, "data-")`). Their doc comments will clarify they are prefixes, not the exact `PartType()` return value.

**Alternative considered:** Remove the constants entirely. Rejected because they're used in `message_json.go` unmarshal logic and removing them would be a separate refactor.

### 3. Update tests to match new behavior

Tests in `message_json_test.go` (`TestAllPartTypes`) and `http_test.go` that assert `PartType()` values will be updated. `TestAllPartTypes` currently expects `"tool-invocation"` and `"data"` -- these change to the wire-compatible forms.

## Risks / Trade-offs

- **[Breaking change for PartType() consumers]** → Any external code comparing `part.PartType() == "data"` or `== "tool-invocation"` will break. Mitigated by the fact that Go type switches (the idiomatic pattern) are unaffected, and the codebase itself already uses type switches everywhere. This aligns with the upstream SDK behavior.
- **[Zero-value edge case]** → `DataPart{}.PartType()` returns `"data-"` (trailing hyphen) and `ToolInvocationPart{}.PartType()` returns `"tool-"`. This matches the marshal behavior (which would produce `"data-"` for an empty DataName) and is consistent -- a zero-value part has no meaningful name.
