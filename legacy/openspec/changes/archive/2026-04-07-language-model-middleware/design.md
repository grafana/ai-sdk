## Context

The codebase has a single model-wrapping pattern: `fallback.Model`, which decorates `provider.LanguageModel` to retry across multiple models. There is no general-purpose interception mechanism. The upstream Vercel AI SDK provides `LanguageModelV3Middleware` -- a type with optional hooks for transforming params and wrapping generate/stream calls -- composed via `wrapLanguageModel`. Our design must achieve behavioral parity while using Go idioms.

Key constraints:
- Must return `provider.LanguageModel` so it's transparent to `StreamText`/`GenerateText` and all other consumers
- Must depend only on `provider/` -- not on root `aisdk` or any provider implementation
- Must support middleware composition (multiple middlewares stacked)
- Must preserve the cross-mode access pattern from upstream (a `WrapGenerate` hook can call `DoStream`, and vice versa)

## Goals / Non-Goals

**Goals:**
- Feature parity with upstream's middleware hooks: `TransformParams`, `WrapGenerate`, `WrapStream`, plus metadata overrides
- Go-idiomatic API: context propagation, `(value, error)` returns, channels for streams
- Composable: multiple middlewares stack via `WrapLanguageModel`
- Provide a stream transformation helper for middleware authors
- Ship three built-in middlewares: `ExtractReasoning`, `SimulateStreaming`, `DefaultSettings`

**Non-Goals:**
- Middleware for non-model concerns (HTTP middleware, SSE middleware, etc.)
- Changing the `provider.LanguageModel` interface
- Replacing or refactoring `fallback.Model`
- Request/response lifecycle hooks at the orchestration layer (`StreamText` callbacks are separate)

## Decisions

### 1. Struct with function fields, not an interface

**Decision:** `Middleware` is a struct with optional function fields.

**Rationale:** Matches the upstream pattern (plain object with optional properties) and the existing codebase convention (`Tool.Execute` is a function field). An interface would force implementers to stub all hooks even when they only care about one. A struct lets you set only what matters:

```go
logging := middleware.Middleware{
    WrapGenerate: func(ctx context.Context, opts middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
        log.Printf("calling %s", opts.Model.ModelID())
        return opts.DoGenerate(ctx)
    },
}
```

**Alternatives considered:**
- **Interface with marker method** (sealed, like `TextStreamPart`): Middleware is user-extensible, not a closed set. Sealed interface is wrong here.
- **Interface without marker**: Would require a default/no-op base implementation for ergonomics, adding complexity for no benefit.

### 2. Package location: `middleware/`

**Decision:** New `middleware/` package at the root of the module.

**Rationale:** Follows the `fallback/` precedent. The middleware package depends only on `provider/` and has no dependency on the root `aisdk` package. This keeps the dependency graph clean:

```
provider/        (leaf -- defines LanguageModel, CallOptions, results)
   ^
middleware/      (wraps LanguageModel, depends only on provider/)
   ^
fallback/        (also wraps LanguageModel, depends only on provider/)
   ^
aisdk (root)     (orchestration -- receives any LanguageModel)
```

**Alternatives considered:**
- **Root `aisdk` package**: Already large; middleware is a self-contained concern that doesn't need access to orchestration types.
- **`provider/` package**: Provider is a leaf package defining contracts. Adding wrapping logic would expand its responsibility beyond type definitions.

### 3. Context propagation: closures accept `ctx`

**Decision:** The `DoGenerate` and `DoStream` closures passed to wrap hooks accept a `context.Context` parameter.

```go
type WrapGenerateParams struct {
    DoGenerate func(ctx context.Context) (*provider.GenerateResult, error)
    DoStream   func(ctx context.Context) (*provider.StreamResult, error)
    Params     provider.CallOptions
    Model      provider.LanguageModel
}
```

**Rationale:** Middleware may need to modify the context before passing it to the inner model -- adding timeouts, injecting values, or replacing it entirely. This is standard Go practice. The upstream doesn't have this concern because JavaScript doesn't have explicit context threading.

**Alternatives considered:**
- **Closures capture ctx (no parameter)**: Simpler signatures but prevents middleware from modifying context for the inner call. A timeout middleware or tracing middleware would be impossible.

### 4. Middleware composition order: first = outermost

**Decision:** When multiple middlewares are passed to `WrapLanguageModel`, the first middleware in the list is the outermost wrapper (processes input first, sees output last). Application is right-to-left internally -- the last middleware wraps the model first, then the second-to-last wraps that, etc.

This matches upstream behavior: `WrapLanguageModel(model, [A, B, C])` produces `A(B(C(model)))`.

```
Call flow:  A.TransformParams -> B.TransformParams -> C.TransformParams -> model.DoStream
Return:     A.WrapStream     <- B.WrapStream      <- C.WrapStream      <- result
```

### 5. Stream transformation helper

**Decision:** Provide a `TransformStream` utility in the `middleware` package that handles the goroutine + channel boilerplate for transforming `provider.StreamPart` channels.

```go
func TransformStream(
    ctx context.Context,
    result *provider.StreamResult,
    transform func(part provider.StreamPart, emit func(provider.StreamPart)),
    flush func(emit func(provider.StreamPart)),
) *provider.StreamResult
```

The `ctx` parameter enables context-aware cancellation (consistent with design decision #3). The `flush` callback (nil-safe) allows transforms to emit buffered data when the input stream closes -- needed by `ExtractReasoning` to handle partial tag buffers at end-of-stream.

**Rationale:** Stream transformation is the most common and most error-prone operation in middleware. The upstream uses `TransformStream` (Web Streams API). In Go, the equivalent requires a goroutine, a new channel, and proper close/drain handling. Providing this utility prevents bugs and keeps middleware implementations focused on logic. The `emit` callback (rather than returning `[]StreamPart`) allows a transform to emit zero, one, or many parts per input, and to buffer across calls.

**Alternatives considered:**
- **Return `[]StreamPart`**: Simpler but can't handle stateful buffering (needed by `ExtractReasoning` which buffers partial tags across chunks).
- **No helper, let users write goroutines**: Error-prone; channel close, context cancellation, and panic recovery are easy to get wrong.

### 6. Built-in middleware constructors are functions returning `Middleware`

**Decision:** Each built-in middleware is a constructor function returning `Middleware`, not a standalone type:

```go
func ExtractReasoning(opts ExtractReasoningOptions) Middleware
func SimulateStreaming() Middleware
func DefaultSettings(settings DefaultSettingsOptions) Middleware
```

**Rationale:** Matches upstream pattern (factory functions), keeps the public type surface small, and allows closures to capture configuration naturally.

## Risks / Trade-offs

**[Goroutine leak in stream transformation]** If a consumer abandons a transformed stream without draining it, the transform goroutine may block on channel send.
→ Mitigation: `TransformStream` should use a select on context cancellation and the middleware-wrapped model should propagate context to the goroutine. Document that streams must be drained or context cancelled.

**[Performance overhead of middleware chain]** Each middleware layer adds a function call and, for streams, a goroutine + channel hop.
→ Acceptable: the overhead is negligible compared to LLM API latency. The upstream has the same layering cost.

**[TransformParams runs once per call, not per stream part]** Unlike `WrapStream` which sees the full result, `TransformParams` only modifies the input. A middleware that needs to both transform params and transform the stream must implement both hooks.
→ This matches upstream. The separation is intentional -- param transformation is a distinct concern from result transformation.

**[Cross-mode closures may surprise users]** `WrapGenerate` receiving a `DoStream` closure (and vice versa) is powerful but unusual. A naive middleware might accidentally call the wrong mode.
→ Mitigation: Clear documentation. The cross-mode access is opt-in -- if you don't call `DoStream` from `WrapGenerate`, it's unused.
