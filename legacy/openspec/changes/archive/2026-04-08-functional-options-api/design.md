## Context

`StreamText` and `GenerateText` currently accept `StreamTextParams`, a 25+ field flat struct. The struct conflates required and optional parameters, forces pointer indirection for optional scalars, and allows `GenerateText` callers to set streaming-only fields that are silently ignored. The provider boundary (`CallOptions`) is a separate struct and is not affected by this change.

Internally, `GenerateText` is a thin wrapper around `StreamText` (calls it and drains the stream). The `output` package's `GenerateObject`/`StreamObject` functions inject `Output` into the params and delegate. The `run` method in `streamtext.go` converts params to `CallOptions` with mostly 1:1 field copying for scalar generation params.

## Goals / Non-Goals

**Goals:**
- Separate required parameters (model) from optional configuration
- Eliminate pointer indirection for optional scalars
- Make streaming-only options (`OnChunk`, `IncludeRawChunks`) unavailable on `GenerateText` at compile time
- Keep the provider boundary (`CallOptions`) unchanged
- Maintain all existing behavior -- this is a pure API refactoring

**Non-Goals:**
- Changing the `output` package's `Output` interface or structured output behavior
- Modifying `provider.CallOptions` or the provider interface
- Changing result types, stream part types, or SSE wire format
- Adding new configuration capabilities
- Addressing `UIMessageStreamOptions` (tracked in #93)
- Addressing typed provider options (tracked in #96)

## Decisions

### 1. Interface-based option types with shared base config

`StreamOption` and `GenerateOption` are interfaces with unexported marker methods (sealed). Three unexported concrete types implement them:

- **`sharedOption`**: implements both `StreamOption` and `GenerateOption`. Used by all options that apply to both APIs (~22 options: messages, model params, tools, shared callbacks, etc.).
- **`streamOnlyOption`**: implements only `StreamOption`. Used by streaming-exclusive options (`OnChunk`, `WithIncludeRawChunks`).
- **`generateOnlyOption`**: implements only `GenerateOption`. Reserved for future generate-exclusive options.

Internally, `streamConfig` and `generateConfig` both embed a `baseConfig` struct holding all shared fields. Shared options set fields on `baseConfig`; stream-only options set fields on `streamConfig` directly.

**Why interfaces over function types**: A `func(*streamConfig)` type cannot simultaneously satisfy `GenerateOption`. Interfaces allow `sharedOption` to implement both via two methods, giving compile-time exclusion of streaming-only options from `GenerateText` while keeping a single `WithTemperature(0.7)` call usable in both contexts.

**Alternative considered**: Two entirely separate sets of option functions (e.g., `StreamWithTemperature` and `GenerateWithTemperature`). Rejected -- doubles the API surface for no user benefit and creates naming clutter.

### 2. Model as positional argument

`Model` moves from the options struct to a required positional parameter in both `StreamText` and `GenerateText`. It is always required and has no meaningful default, so embedding it in options would only obscure this invariant.

```go
func StreamText(ctx context.Context, model provider.LanguageModel, opts ...StreamOption) *StreamTextResult
func GenerateText(ctx context.Context, model provider.LanguageModel, opts ...GenerateOption) (*GenerateTextResult, error)
```

### 3. Output package signature change

`GenerateObject` and `StreamObject` accept the corresponding option types, with `Output` as a required positional argument (since these functions exist specifically to inject structured output):

```go
func GenerateObject[T any](ctx context.Context, model provider.LanguageModel, out aisdk.Output, opts ...aisdk.GenerateOption) (*ObjectResult[T], error)
func StreamObject[T any](ctx context.Context, model provider.LanguageModel, out aisdk.Output, opts ...aisdk.StreamOption) *StreamObjectResult[T]
```

Internally, they build a config, inject `Output`, and delegate to `StreamText`/`GenerateText` by converting to the internal config representation.

### 4. Internal config to CallOptions translation

The `run` method currently reads fields directly from `StreamTextParams`. After this change, it reads from `streamConfig` instead. The translation to `CallOptions` is mechanically identical -- field-by-field copy. No intermediate adapter or abstraction layer is needed.

The `GenerateText` function builds a `generateConfig`, converts it to a `streamConfig` (trivial -- `generateConfig` is a subset), and delegates to the internal streaming path.

### 5. File organization

All option types, interfaces, concrete option structs, and `With*`/`On*` constructor functions live in a single new file: `options.go`. The internal config structs (`baseConfig`, `streamConfig`, `generateConfig`) live in the same file since they are tightly coupled to the option types. This keeps the options API self-contained and avoids scattering across multiple files.

`StreamTextParams` is removed from `text.go` after migration.

### 6. ProviderOptions field type change

The current `ProviderOptions` field is `map[string]json.RawMessage`. The issue proposes `WithProviderOptions(opts ...provider.ProviderOption)` using the typed interface from #96. However, since #96 is a separate change, this refactoring preserves the current `map[string]json.RawMessage` type:

```go
func WithProviderOptions(opts map[string]json.RawMessage) StreamOption // also GenerateOption
```

When #96 lands, this function's signature changes to accept the typed interface.

## Risks / Trade-offs

**[Large migration surface]** ~49 callsites construct `StreamTextParams` across production code, tests, and documentation. This is a mechanical but labor-intensive migration. -> Mitigation: Tasks break migration into file-by-file steps. The compiler catches every missed site since the old type is removed.

**[Two interfaces for shared options adds indirection]** Shared options route through `sharedOption` struct implementing two interfaces, adding a layer vs. direct function types. -> Mitigation: This is internal complexity, invisible to users. The user-facing API (`WithTemperature(0.7)`) is identical regardless of the underlying mechanism.

**[output package coupling]** `GenerateObject`/`StreamObject` need access to the internal config to inject `Output` and delegate. -> Mitigation: Export a minimal helper or use an unexported `WithOutput` option. The `Output` positional arg handles the user-facing side cleanly.
