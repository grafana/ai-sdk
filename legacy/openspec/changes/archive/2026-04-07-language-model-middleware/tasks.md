## 1. Core types and WrapLanguageModel

- [x] 1.1 Create `middleware/` package with `doc.go`, `Middleware` struct (function fields for `TransformParams`, `WrapGenerate`, `WrapStream`, `OverrideProvider`, `OverrideModelID`), and input types (`TransformParamsInput`, `WrapGenerateParams`, `WrapStreamParams`)
- [x] 1.2 Implement `WrapLanguageModel(model, ...Middleware)` that applies middlewares right-to-left, returning a `provider.LanguageModel`
- [x] 1.3 Implement the wrapped model type satisfying `provider.LanguageModel`: metadata delegation, override logic, `DoGenerate` and `DoStream` with hook chaining (TransformParams -> WrapGenerate/WrapStream -> inner model)
- [x] 1.4 Tests: single middleware passthrough, TransformParams modifies CallOptions, WrapGenerate intercepts and delegates, WrapStream intercepts and delegates, cross-mode access (WrapGenerate calls DoStream and vice versa), error propagation from each hook, nil hooks pass through, metadata override, multiple middleware composition order

## 2. Stream transformation utility

- [x] 2.1 Implement `TransformStream(ctx, result, transform)` utility: goroutine reads from input channel, calls transform with emit callback, writes to output channel, respects context cancellation
- [x] 2.2 Tests: one-to-one transform, one-to-many transform, stateful buffering, context cancellation stops goroutine, empty stream

## 3. Built-in middleware: DefaultSettings

- [x] 3.1 Implement `DefaultSettings(settings)` returning `Middleware` with `TransformParams` that merges defaults into CallOptions (caller values take precedence over defaults for pointer fields; map merge for Headers and ProviderOptions with caller keys winning)
- [x] 3.2 Tests: default applied when caller omits field, caller value takes precedence, multiple defaults with partial caller override, Headers merge, ProviderOptions merge

## 4. Built-in middleware: SimulateStreaming

- [x] 4.1 Implement `SimulateStreaming()` returning `Middleware` with `WrapStream` that calls DoGenerate and converts the result into synthetic StreamParts (stream-start, response-metadata, text-start/delta/end, reasoning-start/delta/end, other parts passthrough, finish)
- [x] 4.2 Tests: text content produces correct stream parts in order, reasoning content produces reasoning events, non-text content passed through, DoGenerate passes through unmodified, response metadata preserved

## 5. Built-in middleware: ExtractReasoning

- [x] 5.1 Implement `ExtractReasoning(opts)` returning `Middleware` with `WrapGenerate` that parses `<tag>...</tag>` from text content parts and converts to reasoning content parts
- [x] 5.2 Implement the `WrapStream` hook for `ExtractReasoning`: transform stream parts to detect tag boundaries in text deltas, buffer partial tags, emit reasoning-start/delta/end for tagged content and text-delta for non-tagged content
- [x] 5.3 Tests for generate: basic extraction, no tags present, multiple reasoning sections, startWithReasoning option
- [x] 5.4 Tests for stream: basic streaming extraction, tag split across chunks, startWithReasoning, empty reasoning block, transition between reasoning and text sections

## 6. Verification

- [x] 6.1 Ensure all tests pass (`go test ./middleware/...`), run vet and lint
- [x] 6.2 Verify composition with fallback: wrap a `fallback.Model` with middleware and confirm both layers work together
