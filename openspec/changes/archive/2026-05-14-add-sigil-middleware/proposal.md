## Why

Sigil (Grafana's LLM observability + policy product) is wired into Anthropic-typed code paths in the consumer repo (`grafana/grafana-assistant-app`'s `internal/llm/claude/`): recording lives inside the backend, and hook preflight wraps `claude.Router`. As consumers migrate to ai-sdk's neutral `provider.LanguageModel`, that instrumentation is left behind — the ai-sdk path runs uninstrumented, with no `StartGeneration` records and no `EvaluateHook` policy enforcement. The blocking dependency is a translator from ai-sdk neutral types (`provider.CallOptions`, `provider.GenerateResult`, `provider.StreamPart`) to the provider-agnostic `sigil.Generation` schema. Once that mapper exists, Sigil instrumentation becomes a `middleware.Middleware` that wraps **any** `provider.LanguageModel` — Anthropic, Vertex, OpenAI, Google, anything ai-sdk supports today or tomorrow.

## What Changes

- New nested Go module `middleware/sigil/` (with `replace github.com/grafana/ai-sdk => ../../`), establishing the convention that **heavy middlewares with vendor SDK / gRPC / OTel dependencies live in nested modules** under `middleware/`, mirroring the established `providers/<name>/` pattern. This is the first nested middleware module in the repo.
- A provider-agnostic mapper from ai-sdk neutral types to `sigil.Generation`:
  - `MapGenerateResult(params, result, ctxInfo) sigilsdk.Generation` for non-streaming.
  - `StreamRecorder` stateful accumulator for streaming (handles text, reasoning + signature metadata, tool calls, usage, finish reasons).
  - Reads provider-specific knobs (thinking budget, cache control, server-tool flags) through opaque `provider.ProviderOptions["anthropic"]` JSON; the middleware never imports `providers/anthropic`.
- Two composable middlewares plus convenience helpers:
  - `RecordingMiddleware(opts)` — wraps `WrapGenerate` and `WrapStream`; calls `StartGeneration` / `StartStreamingGeneration`, tees the stream, and emits `SetResult` (or `SetCallError` on failure).
  - `HooksMiddleware(opts)` — calls `EvaluateHook` preflight; on deny returns a typed `*HookDenialError` (unwrappable to sentinel `ErrHookDenied`); on `TransformedInput` replaces `params.Prompt` while preserving Anthropic reasoning-block signatures.
  - `sigil.Stack(opts) []middleware.Middleware` returning the canonical [Hooks, Recording] order.
  - `sigil.Wrap(base, opts)` convenience equal to `middleware.Wrap{Model: base, Middleware: Stack(opts)}`.
- Provider-agnostic context helpers in `middleware/sigil/context.go`: `WithGenerationID`, `NewGenerationID`, `WithParentGenerationIDs`, `WithLinkedGenerationID`, `GenerationIDFromContext`, `ParentGenerationIDsFromContext`. These wire the parent → child generation DAG that Sigil uses to track multi-agent investigations.
- Consumer-injection seams that keep the module Grafana-agnostic:
  - `ClientResolver func(ctx context.Context) *sigilsdk.Client` — resolves per-tenant Sigil clients; returning `nil` makes recording a no-op for that request.
  - `ContextProvider func(ctx context.Context) ContextInfo` — supplies `UserID`, `Metadata`, `Tags`, `AgentName`, `AgentVersion`. The middleware has no knowledge of Grafana tenants, OSS mode, or stack IDs.

## Capabilities

### New Capabilities

- `sigil-middleware`: Provider-agnostic Sigil instrumentation for ai-sdk language models — neutral-type-to-`sigil.Generation` mapper, `RecordingMiddleware`, `HooksMiddleware`, composition helpers, context-key DAG plumbing, and the public consumer-injection contracts (`ClientResolver`, `ContextProvider`).

### Modified Capabilities

(none — `language-model-middleware` is unchanged; this is a new sibling under `middleware/`)

## Impact

- **New nested Go module** `middleware/sigil/` brings in `github.com/grafana/sigil-sdk/go` (OTel SDK + gRPC). Consumers who don't import the new module pay zero dependency cost — same pattern as `providers/anthropic` / `providers/grafana` today.
- **Root module unaffected**: `middleware/sigil/` depends on the ai-sdk root (`provider`, `middleware`) via `replace ../../`, but the root never imports it. The existing flat-file middlewares in `package middleware` continue working unchanged.
- **No changes to `provider.LanguageModel`** — sigil wraps it; doesn't extend it.
- **New convention**: heavy middlewares follow the nested-module pattern. This precedent is captured in `middleware/sigil/doc.go` (and via this OpenSpec spec). Future contributors who want to add metrics / tracing middlewares with `prometheus` / `otel-grpc` deps should follow the same shape.
- **No changes to existing public API surface** of the root module or any provider.
- **Upstream-first land strategy**: change lands in `github.com/grafana/ai-sdk` first; downstream `grafana-assistant-app` bumps its vendored subtree to consume it. Consumer wiring (Lodestone's `builder.go`, `aisdkprovider/factory.go`, chat-sidebar) is **out of scope** for this change and tracked downstream.
- **Out of scope** (called out explicitly so they don't block this change):
  - Replacing the legacy `internal/llm/claude/` Sigil integration (it stays until every non-ai-sdk consumer of `claude.Router` is gone).
  - Metrics / OTel-span middlewares — sibling concern, lands in `middleware/observability/` (or similar) under its own change.
  - The Grafana-side registry abstraction (`sigilregistry.Registry`) and `BuildSigilMetadata` / `BuildSigilTags` helpers — they stay in the consumer repo and plug in via `ClientResolver` / `ContextProvider`.
  - Upstream contribution to `vercel/ai` — Sigil is Grafana-specific; `middleware/sigil/` is a Grafana-fork-only addition.
