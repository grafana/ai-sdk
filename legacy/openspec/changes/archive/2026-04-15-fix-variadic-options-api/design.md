## Context

Three public utility functions (`ConvertToModelMessages`, `WriteUIMessageStream`, `ReadUIMessageStream`) use variadic option structs (`opts ...T`) but only consume the first element, silently dropping extras. The main orchestration API (`StreamText`, `GenerateText`) already uses a proper functional options pattern with sealed interfaces per the `functional-options` spec. These utility functions predate that pattern and use a simpler struct-based approach.

The upstream TypeScript SDK doesn't have this problem because these functions use a single optional object parameter (JavaScript's built-in approach to optional config).

## Goals / Non-Goals

**Goals:**
- Make the "zero or one" options contract explicit at the type level for all three functions
- Keep call-site ergonomics simple (callers pass `nil` for defaults)
- Maintain wire-format compatibility (no SSE/chunk changes)

**Non-Goals:**
- Migrating these functions to the sealed-interface functional options pattern (the option structs are small and don't benefit from that complexity)
- Changing the behavior of any of the option fields themselves
- Changing internal/unexported functions

## Decisions

### Use `*T` pointer parameter instead of variadic

**Choice**: `func F(required, opts *T)` where callers pass `nil` for defaults.

**Alternatives considered**:
1. **Keep variadic + validate `len(opts) > 1`**: Defers the error to runtime. Callers still see a misleading `...` signature. Rejected because the goal is compile-time safety.
2. **Functional options pattern (`...Option`)**: Overkill for these small config structs (1-4 fields). The sealed-interface pattern from the `functional-options` spec is designed for the complex `StreamText`/`GenerateText` API surface. Rejected because it adds unnecessary complexity.
3. **Pointer parameter (`*T`)**: Makes the contract explicit. Callers pass `&T{...}` or `nil`. No ambiguity. Simple migration.

**Rationale**: The pointer approach is the standard Go idiom for "optional config struct" and matches the upstream TypeScript pattern of a single optional parameter.

### Internal call sites pass `nil` explicitly

All internal callers that currently omit the variadic argument will pass `nil` explicitly. This is a mechanical change.

## Risks / Trade-offs

- **[Breaking change]** → Callers that pass options will need to change from `F(x, opts)` to `F(x, &opts)`. Callers that pass no options change from `F(x)` to `F(x, nil)`. Mitigation: this is a straightforward migration and the compiler will catch all call sites.
- **[`nil` vs zero-value ambiguity]** → With a pointer, `nil` means "use defaults" while `&T{}` means "explicitly set to zero values". For these structs, zero values and defaults are identical, so there's no practical ambiguity. No mitigation needed.
