## Why

The upstream Vercel AI SDK provides a middleware system for intercepting and modifying LLM calls in a model-agnostic way, enabling composable cross-cutting concerns (guardrails, RAG injection, caching, logging, default settings) without modifying provider implementations. Our Go port has no middleware layer -- the only existing wrapper is `fallback.Model`, which is single-purpose. Adding middleware achieves feature parity with upstream and unblocks a class of use cases that currently require ad-hoc model wrapping.

## What Changes

- New `middleware` package providing a `Middleware` struct with optional function-field hooks (`TransformParams`, `WrapGenerate`, `WrapStream`) and metadata overrides (`OverrideProvider`, `OverrideModelID`)
- New `WrapLanguageModel` function that applies one or more middlewares to a `provider.LanguageModel`, returning a new `LanguageModel` -- transparent to `StreamText`, `GenerateText`, and all other consumers
- Three built-in middlewares: `ExtractReasoning` (strips `<think>` tags, exposes as reasoning parts), `SimulateStreaming` (wraps non-streaming models to present a streaming interface), `DefaultSettings` (applies default temperature, maxOutputTokens, etc. when not explicitly set)
- Stream transformation utility for middleware authors who need to transform `provider.StreamPart` channels

## Capabilities

### New Capabilities

- `language-model-middleware`: Core middleware type, composition via `WrapLanguageModel`, and the wrapped model implementation that satisfies `provider.LanguageModel`
- `builtin-middleware-extract-reasoning`: `ExtractReasoning` middleware that parses XML-tagged reasoning sections from text output and exposes them as reasoning content parts
- `builtin-middleware-simulate-streaming`: `SimulateStreaming` middleware that wraps `DoGenerate` results into a synthetic stream for models that don't support native streaming
- `builtin-middleware-default-settings`: `DefaultSettings` middleware that applies fallback values for call options when the caller doesn't set them

### Modified Capabilities

(none)

## Impact

- New `middleware/` package depending only on `provider/` -- no changes to existing packages
- `provider.LanguageModel` interface unchanged -- middleware wraps it, doesn't extend it
- No breaking changes to any existing API surface
- Users of `fallback.Model` are unaffected; fallback and middleware serve different purposes and compose independently (a fallback model can be wrapped with middleware, or vice versa)
