## Context

The `aisdk.Tool` struct (`tool.go`) represents user-facing tool definitions. It has fields for function tools (Description, InputSchema, Execute, etc.) but lacks the fields needed for provider-defined tools. The `toolSetToProviderTools()` function (`convert.go:261`) always constructs `provider.Tool{Type: "function", ...}`.

Meanwhile, `provider.Tool` (`provider/language_model.go:58`) already supports both types:
- `Type: "function"` with `Name`, `Description`, `InputSchema`, etc.
- `Type: "provider"` with `ID` (e.g. `"anthropic.web_search_20250305"`), `Name` (user-chosen), and `Args` (tool-specific config)

The Anthropic provider's `convertTools()` (`anthropic/convert_request.go:432`) already switches on `t.Type` and handles provider tools correctly. The orchestration layer (`streamtext.go`) already skips execution for `ProviderExecuted` tool calls and routes callbacks by tool name. The gap is purely at the user-facing type and conversion boundary.

The upstream Vercel AI SDK handles this via a union type on `Tool` with `type: 'provider'`, `id`, and `args` fields, and `prepareTools()` switches on `tool.type` to build the appropriate provider-level tool.

## Goals / Non-Goals

**Goals:**

- Allow users to declare provider-defined tools in `ToolSet` for use with `StreamText` / `GenerateText`
- Pass through `Type`, `ID`, and `Args` to `provider.Tool` in the conversion layer
- Maintain full backward compatibility -- empty `Type` defaults to `"function"`
- Support callbacks (`OnInputStart`, `OnInputDelta`, `OnInputAvailable`) on provider-defined tools

**Non-Goals:**

- Provider-specific tool factory helpers (e.g. `anthropic.WebSearch(opts)`) -- that's a separate concern for the provider packages, not the core SDK
- Validation that provider-defined tools have valid IDs or Args -- providers handle this

## Decisions

### 1. Add `Type`, `ID`, and `Args` directly to `aisdk.Tool`

**Decision**: Add three fields to the existing `Tool` struct: `Type string`, `ID string`, and `Args map[string]json.RawMessage`.

**Rationale**: This mirrors the `provider.Tool` shape and matches how the upstream TS SDK does it (union on `type` field). Go doesn't have tagged unions, so a flat struct with a type discriminator is the idiomatic approach. The same pattern is already used at the provider level.

**Type values**: `""` (empty, defaults to function), `"function"`, `"provider"`. Empty defaults to function for backward compatibility -- all existing code that creates `Tool{}` without a Type continues to work. The value `"provider"` aligns with upstream's naming convention.

**Alternative considered**: Separate `ProviderTool` type with a different `ToolSet` variant. Rejected because it adds complexity and diverges from both the upstream pattern and the existing `provider.Tool` approach.

### 2. Branch in `toolSetToProviderTools()` based on Type

**Decision**: Add a switch on `t.Type` in `toolSetToProviderTools()`. When `"provider"`, construct `provider.Tool` with `Type`, `Name` (from map key), `ID`, and `Args`. When `""` or `"function"`, use existing function tool path. Unknown types emit a warning and are skipped, matching upstream's exhaustive check.

**Rationale**: Directly mirrors the upstream `prepareTools()` switch pattern. Minimal code change -- just wrap the existing append in a switch. The explicit unknown-type handling prevents silent misconfiguration.

### 3. Provider-defined tools skip Execute, InputSchema, Description

**Decision**: For provider-defined tools, `toolSetToProviderTools()` only passes `Type`, `Name`, `ID`, and `Args` to `provider.Tool`. Fields like `Description`, `InputSchema`, `Strict` are irrelevant for provider-defined tools and are not passed through.

**Rationale**: Provider-defined tools have their schemas and behavior defined by the provider, not the user. The upstream `prepareTools()` does the same -- for `type: 'provider'`, it only sends `type`, `name`, `id`, `args`. User-side callbacks (`OnInputStart`, etc.) remain on the `aisdk.Tool` and are invoked via the tool name lookup in `handleToolCall` -- they don't need to be on `provider.Tool`.

## Risks / Trade-offs

**[Trade-off] No factory helpers**: Users must construct `Tool{Type: "provider", ID: "anthropic.web_search_20250305", Args: ...}` manually. This is more verbose than the upstream's factory functions. Accepted because: (1) factory helpers are a provider-level concern, (2) they can be added as a follow-up without changing this design.
