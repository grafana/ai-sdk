## Context

Schema handling currently uses `json.RawMessage` as the public-facing type across the SDK. Schema generation (`SchemaFor[T]`) and validation (`CompileSchema` + `Validate`) are disconnected APIs backed by two separate libraries (`invopop/jsonschema` for generation, `santhosh-tekuri/jsonschema` for validation). Users must manually coordinate them.

These utilities currently live in the `output` package alongside structured output implementations (Object, Array, Choice, JSON). This is a layering problem: schema handling is a general-purpose concern used by both structured output and tools, but `output` is conceptually about structured output modes. The `output` package imports `aisdk` (root) for the `Output` interface, which prevents `aisdk.Tool` from using any type defined in `output` due to Go's import cycle prohibition.

The upstream TS SDK separates these concerns: `@ai-sdk/provider` has raw `JSONSchema7` at the provider interface, `@ai-sdk/provider-utils` has the rich `Schema<T>` type (bundling definition + validation), and `@ai-sdk/ai` (core) uses `Schema<T>` at the user-facing tool definition level.

## Goals / Non-Goals

**Goals:**

- Create a new `schema` sub-package (`github.com/grafana/ai-sdk/schema`) as a leaf package
- Introduce `schema.Schema` that bundles JSON Schema bytes with pre-compiled validation
- Move all schema utilities (`SchemaFor[T]`, `CompileSchema`, `Validate`, `SchemaFromFile`) from `output` to `schema`
- Change `aisdk.Tool.InputSchema`/`OutputSchema` from `json.RawMessage` to `schema.Schema`
- Change `output.Object[T]`, `output.Array[T]` to accept `schema.Schema`
- Keep `provider.Tool.InputSchema` as `json.RawMessage` (provider is a zero-dependency leaf)

**Non-Goals:**

- No `FlexibleSchema` union or `asSchema()` normalizer -- Go doesn't have union types or Zod; a single concrete `Schema` type is sufficient
- No lazy/deferred schema evaluation -- Go schemas are cheap to compute eagerly
- No optional validation -- every `Schema` has a compiled validator, unlike upstream where `validate` is optional

## Decisions

### D1: New `schema` sub-package as a leaf

Schema utilities move to `github.com/grafana/ai-sdk/schema`. This package depends only on stdlib, `invopop/jsonschema`, and `santhosh-tekuri/jsonschema` -- it imports neither `aisdk` (root) nor `provider`. This makes it importable by every package in the module without creating cycles:

```
provider (leaf)     schema (new leaf)
    ^                  ^       ^
    |                  |       |
  aisdk (root) --------+       |
    ^                          |
    |                          |
  output (imports root + schema)
```

This mirrors the upstream's `@ai-sdk/provider-utils` role: a utility layer between the provider interface and the orchestration layer.

Alternatives considered:
- **Keep in `output`**: Prevents `aisdk.Tool` from using `Schema` (import cycle). Also misplaces general-purpose schema utilities in a structured-output package.
- **Put in `provider`**: Would add external dependencies (`invopop`, `santhosh-tekuri`) to the zero-dependency leaf package. Not acceptable.
- **Put in root `aisdk`**: Root would gain schema library dependencies and validation logic. Bloats the orchestration layer with utility concerns.

### D2: Schema as a concrete struct, not an interface

A concrete struct with always-present fields. Unlike the TS SDK where `validate` is optional (to support schema-only usage without runtime checking), every Go `Schema` carries a compiled validator since compilation cost is negligible and we always want validation.

Alternatives considered:
- **Interface with `JSON()` and `Validate()` methods**: Adds unnecessary indirection. Schema is a value concept, not a behavior abstraction.
- **Type alias for `json.RawMessage` with methods**: A named type over `json.RawMessage` loses the "carries compiled validation" property.

### D3: Eager compilation at construction time

All constructors (`SchemaFor[T]`, `SchemaFromJSON`, `SchemaFromFile`) compile the schema immediately and return an error if compilation fails. Invalid schemas fail fast at construction rather than at first validation call.

The upstream supports lazy evaluation via promises and function wrappers for async scenarios. In Go, all schema sources are synchronous and compilation is fast, so eager compilation is simpler and provides better error locality.

### D4: `SchemaFromJSON` as the escape hatch constructor

Named `SchemaFromJSON` to parallel `SchemaFor[T]` naming and match the upstream's `jsonSchema()` constructor role. `SchemaFromFile` wraps `SchemaFromJSON` after reading the file.

### D5: `aisdk.Tool` fields become `schema.Schema`

With `schema` as a leaf package, the root can import it without cycles. `aisdk.Tool.InputSchema` and `OutputSchema` change from `json.RawMessage` to `schema.Schema`. The `convert.go` provider boundary calls `.JSON()` to extract raw bytes for `provider.FunctionTool.InputSchema`.

This aligns with the upstream where user-facing tool definitions use the rich `FlexibleSchema` type, and the conversion to raw `JSONSchema7` happens at the provider boundary in `prepare-tools.ts`.

### D6: Keep `CompileSchema` and `Validate` as public APIs

`CompileSchema` and `Validate` move to the `schema` package and remain exported for users who need standalone validation without a bundled `Schema`. The `Schema.Validate()` method is the primary path for most users.

### D7: Output constructors accept `Schema` directly

`Object[T]`, `Array[T]` constructors change from `json.RawMessage` to `schema.Schema`. Since `Schema` already carries a compiled validator, these constructors no longer call `CompileSchema` themselves.

`Choice` is unchanged in its public signature since it takes `...string` options and builds its own schema internally.

### D8: `ObjectOutput`/`ArrayOutput` store `Schema` internally

Instead of storing separate `json.RawMessage` + `*CompiledSchema` fields, these structs store a single `schema.Schema` value. For `ArrayOutput`, the wrapper schema envelope is constructed as a new `Schema` via `SchemaFromJSON`.

## Risks / Trade-offs

- **Breaking change across multiple packages** -- `output`, root `aisdk`, and all consumers change. Mitigation: compiler catches all type mismatches; migration is mechanical.
- **New package to maintain** -- Adds `schema/` to the project structure. Mitigation: this is moving existing code, not creating new abstractions. The package is small and focused.
- **`output` package loses `schema.go`** -- Any code importing `output.SchemaFor` or `output.CompileSchema` must update imports to `schema`. Mitigation: search-and-replace migration.
- **Anthropic module impact** -- If the anthropic module constructs `aisdk.Tool` values (not just `provider.Tool`), those call sites need updating. Mitigation: check during implementation.
