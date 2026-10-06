## Context

This is PR 3 of 3 in the V3-to-V4 provider upgrade (#32). PR 1 (#66) reshaped core types (Usage, FinishReason, ResponseMetadata). PR 2 (#67) expanded the content model (CustomContentPart, ReasoningFileContentPart, ToolApprovalResponseContentPart, warnings). This PR handles the tool system split and remaining low-impact field additions.

The `provider.Tool` struct currently serves double duty -- it holds fields for both function tools (InputSchema, Description, Strict, etc.) and provider tools (ID, Args). V4 upstream splits these into distinct types. The remaining items (source document variant, preliminary field, content type expansion) are additive changes with no design tension.

## Goals / Non-Goals

**Goals:**
- Split `provider.Tool` into `FunctionTool` and `ProviderTool` behind a sealed interface
- Add document variant to SourceInfo
- Add Preliminary field to StreamPart for intermediate tool results
- Expand ToolResultContentValue with new V4 content sub-types
- Verify StreamPart.ID population in the Anthropic provider

**Non-Goals:**
- Changing the orchestration-layer `aisdk.Tool` struct (root package) -- it stays as-is, only `toolSetToProviderTools` conversion changes
- Adding new provider implementations or middleware
- Backward-compatible JSON wire format -- this is an internal provider interface change

## Decisions

### Decision 1: Sealed interface for Tool type split

Use the codebase's established sealed interface pattern for the tool split: an unexported marker method `tool()` on a `Tool` interface, with `FunctionTool` and `ProviderTool` as concrete structs.

`CallOptions.Tools` changes from `[]Tool` (struct) to `[]Tool` (interface). Consumers use type switches to dispatch.

**Why**: This is the Go-idiomatic pattern already used for `ContentPart`, `AssistantContentPart`, `UserContentPart`, etc. throughout this codebase. It provides compile-time safety and prevents mixing fields from different tool kinds.

**Alternative considered**: Keep a single struct with a discriminator field. Rejected because it means every consumer carries dead fields and must manually validate which fields are valid for a given type value. The sealed interface makes invalid states unrepresentable.

### Decision 2: ProviderTool drops ProviderOptions

V4 upstream removes `providerOptions` from provider tools. Only `FunctionTool` retains it. The Anthropic provider currently reads `ProviderOptions` from all tools (including provider tools) to extract cache control hints.

After the split, cache control for provider tools will be handled directly via the `Args` map or provider-specific conversion logic, not via `ProviderOptions`. This matches upstream semantics where provider tools are fully provider-defined and don't need a generic extensibility escape hatch.

### Decision 3: InputExamples wrapping

Change `FunctionTool.InputExamples` from `[]json.RawMessage` to `[]InputExample` where:

```go
type InputExample struct {
    Input json.RawMessage `json:"input"`
}
```

This matches upstream V4's `Array<{input: JSONObject}>` shape. The Anthropic provider's conversion (which currently unmarshals raw examples into `map[string]any`) will need a minor adjustment to unwrap the `Input` field.

### Decision 4: SourceInfo stays flat

Keep `SourceInfo` as a single struct differentiated by `SourceType` field, rather than splitting into a sealed interface. The two variants (url, document) share enough fields (ID, Title, ProviderMetadata) that a flat struct with `SourceType` as discriminator is simpler. This matches the current approach and avoids interface overhead for what is essentially a tagged union with overlapping fields.

### Decision 5: ToolResultContentValue -- rename FileID, add types

Rename `FileID json.RawMessage` to `ProviderReference map[string]string` to match upstream V4's `SharedV4ProviderReference` (which is `Record<string, string>`). Add new type constants for the expanded content types: `file-data`, `file-url`, `file-reference`, `image-data`, `image-url`, `image-file-reference`, `custom`.

**Why rename to ProviderReference**: The upstream changed from a file ID concept to a provider reference map where keys are provider names (e.g., "openai", "anthropic") and values are provider-specific identifiers. This is a semantic change, not just a rename.

### Decision 6: Type value consistency

The type value `"provider-defined"` must be updated to `"provider"` everywhere. The comment on `provider.Tool` already says `"provider"` but the Anthropic provider code still checks for `"provider-defined"`. The orchestration layer's `toolSetToProviderTools` already uses `"provider"`. This change completes the rename across the remaining call sites.

## Risks / Trade-offs

- **[Risk] Wide blast radius for tool type split** -- `provider.Tool` is referenced in ~62 locations across tests, orchestration, and the Anthropic provider. Mitigation: The compiler will catch all breakages since the change from struct to interface means all struct literal construction and field access will fail to compile.
- **[Risk] Anthropic cache control for provider tools** -- Removing ProviderOptions from ProviderTool means cache control hints currently set via ProviderOptions on provider tools will no longer work. Mitigation: Provider tools in the Anthropic provider already get cache control via the `convertProviderDefinedTool` function which could use `Args` or a dedicated path. This is a minor behavioral change and matches upstream intent.
- **[Trade-off] Flat SourceInfo vs sealed interface** -- Keeping SourceInfo flat means both variants can be constructed with fields that don't apply (e.g., URL on a document source). Accepted because the overhead of a sealed interface for two similar variants outweighs the type safety benefit.
