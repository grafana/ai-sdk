## Context

Provider options flow through the entire pipeline: user code constructs them, orchestration passes them through `CallOptions`, and provider implementations consume them. Currently, all `ProviderOptions` fields are `map[string]json.RawMessage` -- a direct port of TypeScript's `Record<string, JSONObject>`. In TypeScript, this is natural since the language's native data structures are JSON-shaped. In Go, it forces a pointless JSON round-trip: users marshal typed structs to `json.RawMessage`, then providers unmarshal them back to the same typed structs, all within a single process.

There are ~20 structs across `provider`, `aisdk`, and `anthropic` packages carrying `ProviderOptions` fields. The anthropic provider has 5+ distinct extraction patterns that all follow the same marshal-unmarshal ceremony.

The upstream TypeScript SDK uses `Record<string, JSONObject>` because that's idiomatic for the language. This change intentionally diverges from the upstream wire type at the Go API layer while preserving wire-format compatibility -- the SSE `ProviderMetadata` remains `map[string]json.RawMessage` (unchanged), and the `RawProviderOption` wrapper handles the bridge between JSON wire data and typed Go values.

## Goals / Non-Goals

**Goals:**
- Eliminate unnecessary JSON marshal/unmarshal round-trips for provider options within the same process
- Provide type-safe provider option construction and consumption APIs
- Maintain SSE wire-format compatibility (ProviderMetadata response side is unchanged)
- Handle the metadata-to-options round-trip path where genuine JSON data arrives from previous SSE responses
- Keep the migration mechanical -- existing behavior is preserved, only the plumbing changes

**Non-Goals:**
- Changing `ProviderMetadata` (response side) -- it stays as `map[string]json.RawMessage` since it represents actual JSON from the wire
- Deep-merge semantics for provider options -- shallow key-level replacement remains correct with typed values
- Adding validation or schema enforcement to provider options
- Changing the SSE wire format or `UIMessageChunk` serialization

## Decisions

### D1: Sealed interface with `ProviderKey()` marker method

The `ProviderOption` interface has a single method `ProviderKey() string` that returns the provider name key (e.g., `"anthropic"`). This serves dual purpose: it identifies which map key the option belongs to, and it acts as a semantic marker that a type is intended for provider option use.

**Alternative considered**: An empty marker interface (`interface{}` / `any`) with separate key specification. Rejected because it loses the self-describing property -- `BuildProviderOptions` couldn't determine the map key from the value alone, requiring callers to specify both key and value.

**Alternative considered**: Using `any` for the map value type (`map[string]any`). Rejected because it provides no type safety and requires type assertions everywhere without any compile-time guarantees.

### D2: `RawProviderOption` wrapper for round-tripped JSON

When `ConvertToModelMessages` converts UI-layer `ProviderMetadata` (genuine JSON from SSE responses) back into provider-layer `ProviderOptions`, the data genuinely came from JSON. A `RawProviderOption` struct wraps the key and raw JSON bytes, implementing `ProviderOption`.

This keeps the common path (fresh typed options) zero-allocation while handling the uncommon path (round-tripped metadata) with a thin wrapper. Providers never need to know which path the data took -- `ResolveOption` handles both transparently.

### D3: Generic `ResolveOption[T]` for provider consumption

A generic helper normalizes consumption across both fresh (typed) and round-tripped (JSON) options:
- Fresh path: direct type assertion, zero-cost
- Round-trip path: JSON unmarshal from `RawProviderOption.Raw`
- Unknown type: error with descriptive message

This eliminates the need for each extraction site to handle both cases manually. The three-return pattern `(T, bool, error)` distinguishes "not present" from "present but wrong type."

**Alternative considered**: Requiring providers to handle both cases with manual type switches. Rejected because it duplicates boilerplate across every extraction site and is error-prone.

### D4: `BuildProviderOptions` variadic constructor

A helper function takes variadic `ProviderOption` values and builds the `map[string]ProviderOption` map using each value's `ProviderKey()`. This replaces the current pattern of marshaling structs and manually building the map.

### D5: `CacheControl()` convenience helper in anthropic package

Cache control is the most common per-part provider option. A `CacheControl(cacheType string) ProviderOption` helper returns a lightweight typed value implementing `ProviderOption` with key `"anthropic"`, eliminating the need to construct full `AnthropicOptions` for the common cache-control-only case.

This requires a dedicated type (`AnthropicCacheControl`) separate from `AnthropicOptions` since both can't have the same `ProviderKey()` return value and serve different purposes. For tools that need both cache control and tool-specific options, `AnthropicToolOptions` carries a `CacheControl` field. The `extractCacheControl` function handles three typed paths: `AnthropicCacheControl` (per-part), `AnthropicToolOptions` (per-tool with cache control), and `RawProviderOption` (round-tripped JSON).

### D6: Interface lives in `provider` package

The `ProviderOption` interface, `RawProviderOption`, `BuildProviderOptions`, and `ResolveOption` all live in the `provider` package. This is the natural home since `provider` is the leaf package defining the provider contract, and the interface is part of that contract. Both `aisdk` (orchestration) and `anthropic` (implementation) already depend on `provider`.

## Risks / Trade-offs

- **[Breaking API change]** All external code constructing `ProviderOptions` must update. Mitigation: the migration is mechanical (replace marshal+map with typed values), and can be documented with before/after examples. No existing behavior changes.

- **[Two option types for anthropic cache control]** `AnthropicCacheControl` and `AnthropicOptions` both use key `"anthropic"` but are different types. `BuildProviderOptions` will use the last value for a key, so they can't coexist in the same map. Mitigation: `AnthropicCacheControl` is for the common per-part case, `AnthropicToolOptions` carries a `CacheControl` field for tools that need both cache control and tool options. They're mutually exclusive per map slot by design.

- **[ResolveOption error path]** When a `RawProviderOption` contains malformed JSON, `ResolveOption` returns an error. Providers must handle this, whereas the current code silently ignores unmarshal errors in some paths. Mitigation: this is arguably better behavior -- surfacing errors rather than silently dropping options. Review each extraction site for error handling expectations.

- **[Conformance test adaptation]** Conformance tests currently load `providerOptions` from YAML as `map[string]any` and convert to `json.RawMessage`. They'll need to construct `RawProviderOption` wrappers instead, since the test data comes from YAML (a wire format). This is a natural fit for `RawProviderOption`.
