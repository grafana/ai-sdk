## Context

The codebase already uses the typed string enum pattern in two places: `StreamPartType` in `provider/stream_part.go` and `ChunkType` in `chunk.go`. Both follow the same pattern: a named `string` type with a const block of typed constants. This pattern provides compile-time safety without affecting JSON wire compatibility.

Nine other discriminator groups and one set of reasoning effort constants still use bare `string`, accepting any arbitrary string value at compile time. The issue is systematic -- it comes from the original port treating TypeScript string literal unions as plain Go strings. Three additional discriminator groups (`aisdk.Tool.Type`, `ThinkingConfig.Type`, UIMessage part types) were identified during implementation and addressed in the same change.

## Goals / Non-Goals

**Goals:**
- Add typed string enum types for all thirteen identified discriminator groups
- Replace all inline string literals at usage sites with named constants
- Maintain JSON wire compatibility (typed strings marshal identically)
- Follow the established `StreamPartType`/`ChunkType` pattern consistently

**Non-Goals:**
- Custom `MarshalJSON`/`UnmarshalJSON` for the new types -- not needed since Go typed strings serialize as plain strings
- Validation or enforcement at deserialization boundaries -- Go's JSON decoder will accept any string into a typed string, same as today. Runtime validation is a separate concern
- Changing the `ToolInvocationPart.State` or `DynamicToolUIPart.State` fields to use a sealed interface pattern -- the typed string approach is sufficient and consistent with other discriminator fields

## Decisions

### 1. Type definitions co-located with their structs

Each new typed string type is defined in the same file as the struct that uses it. This keeps related types together and avoids a monolithic "enums" file.

- `ToolChoiceType`, `WarningType`, `ReasoningEffort`, `ToolResultContentType` in `provider/types.go`
- `ResponseFormatType`, `ToolType`, `GenerateContentType` in `provider/language_model.go`
- `SourceType` in `provider/stream_part.go` -- defined once in `provider` and used from root via the qualified name
- `StepType` in `text.go`
- `ToolInvocationState`, `UIPartType` in `message.go`
- `UserToolType` in `tool.go`
- `ThinkingType` in `anthropic/options.go`

**Alternative considered**: A single `provider/enums.go` file. Rejected because it creates coupling between unrelated types and diverges from the existing pattern where `StreamPartType` lives alongside `StreamPart`.

### 2. SourceType defined in provider package only

`SourceType` is used in three structs: `Source` (root), `SourceInfo` (provider), and `GenerateContentPart` (provider). Defining it in `provider` avoids duplication. The root `Source` struct references it as `provider.SourceType`.

**Alternative considered**: Define a root-level `SourceType` with re-export. Rejected because the root package already imports `provider` and using the qualified name is idiomatic.

### 3. Reasoning field type changes from `*string` to `*ReasoningEffort`

`CallOptions.Reasoning` changes from `*string` to `*ReasoningEffort`. This is the only pointer-typed discriminator field. Callers currently must create a temporary variable: `r := provider.ReasoningHigh; opts.Reasoning = &r`. With the new type, the pattern stays the same but the variable type is `ReasoningEffort` instead of `string`, which is marginally better for readability and catches errors.

A `ReasoningEffortPtr` helper function would eliminate the awkward temp variable, but that is out of scope -- it's a convenience, not a type safety issue.

### 4. ToolType shared across FunctionTool and ProviderTool

Both `FunctionTool.Type` and `ProviderTool.Type` use the same `ToolType` enum. The values are `ToolTypeFunction` and `ToolTypeProvider`. This matches the upstream pattern where both are variants of the same discriminator.

### 5. GenerateContentType has many values

`GenerateContentPart.Type` takes at least seven values: "text", "reasoning", "tool-call", "tool-result", "source", "file", "reasoning-file". These are defined as `GenerateContentType` constants. The `Kind` field on `GenerateContentPart` (used for reasoning variants) remains a bare `string` -- it is not a finite discriminator set and is only used for provider-specific sub-classification.

### 6. Warning constant names preserved

The existing `WarnUnsupported`, `WarnCompatibility`, `WarnOther` constant names stay the same -- they just become typed as `WarningType` instead of untyped `string`. This minimizes churn at the ~50 usage sites.

## Risks / Trade-offs

**[Anthropic module churn]** The anthropic module has the most usage sites (~70+ for `GenerateContentPart`, ~50 for `Warning`, ~7 for `ToolChoice`). The migration is mechanical but creates a large diff. Mitigation: Batch by file, run tests after each file.

**[Breaking change for external callers]** Any code using raw string literals like `ToolChoice{Type: "auto"}` will fail to compile. Mitigation: The migration is search-and-replace mechanical, and the issue explicitly acknowledges this.

**[No runtime enforcement on deserialization]** JSON decoding will still accept any string into these typed fields. A misspelled value like `"auot"` would compile as a constant but be silently accepted from JSON. This is the same behavior as `StreamPartType` and `ChunkType` today -- consistent, not ideal, but runtime validation is a separate effort.
