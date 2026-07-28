## Context

The ai-sdk is a Go port of Vercel's AI SDK that exposes a neutral `provider.LanguageModel` interface. Downstream, Grafana wraps every Claude/Vertex call through `internal/llm/claude/` (in `grafana-assistant-app`), which integrates Sigil two ways:

1. **Recording.** `backend_common.go:streamWithSigil` / `generateWithClient` call `StartGeneration` / `StartStreamingGeneration` on a per-tenant `*sigil.Client`, feeding `sigil.Generation` payloads built from `anthropic.MessageNewParams` via the helper module `sigil-sdk/go-providers/anthropic.FromRequestResponse` / `FromStream`. A small `sigil_compat.go` patches the GA-vs-Beta Anthropic type mismatch.
2. **Hooks preflight.** `hooks_middleware.go` wraps `claude.Router`, builds `sigil.HookEvaluateRequest` from `anthropic.MessageParam`, calls `EvaluateHook`, and either denies (`failure.RequestBlocked`) or replaces `params.Messages` from a `TransformedInput` response. The transform path must preserve cryptographic signatures attached to reasoning blocks (`findMatchingOriginalWithThinking`); those signatures don't round-trip through Sigil's wire schema.

Consumers are migrating to ai-sdk's `provider.LanguageModel`. The first cutover is Lodestone (`assistant.lodestone.ai-sdk-engine`); the chat sidebar follows behind `assistant.chat.ai-sdk-engine`. On the ai-sdk path, both integrations are **uninstrumented**: no Sigil records, no policy enforcement. The legacy fallback through `claude.Router` is the only thing keeping the lights on, and it goes away as soon as the migration completes.

`sigil.Generation` is already provider-agnostic — its fields are neutral (`[]sigil.Message`, `[]sigil.ToolDefinition`, `sigil.TokenUsage`, `string` stop reason). The blocker is a translator from ai-sdk neutral types (`provider.CallOptions`, `provider.GenerateResult`, `provider.StreamPart`) to that schema. Once that mapper exists, both cross-cuts become composable `middleware.Middleware` values wrapping any `provider.LanguageModel`.

Constraints inherited from the current claude path that this design must preserve to avoid breaking dashboards and BigQuery views:

- Byte-equal `sigil.Generation` JSON output for the same logical request, modulo the `id`, `started_at`, `completed_at`, `trace_id`, `span_id` fields that come from the recorder itself.
- Same `EvaluateHook` semantics: phase = preflight, deny → typed error, allow → pass-through, transform → replace `params.Prompt` (with reasoning-signature preservation).
- Same OTel span attribute keys (`sigil.hooks.result`, `sigil.hooks.action`, `sigil.hooks.rule_id`, etc.) so existing trace filters match. Namespace shifts from `llm.claude.*` to `aisdk.sigil.*`.
- Same tenant routing: a `ClientResolver` callback drops in the consumer's existing per-tenant registry (`sigilregistry.ForContext`).

## Goals / Non-Goals

**Goals:**

- Provider-agnostic mapper from `provider.CallOptions` + `provider.GenerateResult` / `<-chan provider.StreamPart` to `sigil.Generation`. Provider-specific knobs (Anthropic thinking budget, cache control, server-tool flags) flow through opaque `provider.ProviderOptions["anthropic"]` JSON.
- Two composable `middleware.Middleware` values (`RecordingMiddleware`, `HooksMiddleware`) plus a `Stack(opts)` ordering helper and a `Wrap(base, opts)` convenience.
- Behavioral parity with the legacy Anthropic-typed claude path: same Generation wire shape, same hook decision flow, same span attribute keys.
- Clean seams for consumers: `ClientResolver` and `ContextProvider` callbacks keep the module free of Grafana-specific concepts (tenants, OSS mode, stack IDs).
- First-citizen support for Anthropic-style reasoning blocks with signature metadata — both during streaming accumulation and during hooks `TransformedInput` round-trip.
- Establish the precedent that **heavy middlewares live in nested `middleware/<name>/` Go modules**, mirroring the existing `providers/<name>/` pattern.

**Non-Goals:**

- Replacing the legacy `internal/llm/claude/` Sigil integration in this change. That code stays alive as long as non-ai-sdk consumers of `claude.Router` exist (Slack, CLI, `agentic/agent.go`).
- Building Prometheus / OTel metrics middleware. Sibling concern, separate change.
- Adopting a Grafana-side registry abstraction (`sigilregistry.Registry`) in ai-sdk. The registry stays in the consumer repo; the middleware accepts a `ClientResolver` callback.
- Building `BuildSigilMetadata` / `BuildSigilTags` (which read Grafana concepts like tenant_id, stack_id, oss_mode). They stay in the consumer and feed the `ContextProvider` callback.
- Upstream contribution to `vercel/ai`. Sigil is Grafana-specific; `middleware/sigil/` is a Grafana-fork-only addition.
- Defining new Sigil wire fields. The mapper produces `sigil.Generation` using the existing schema; gaps surface as bug reports against `sigil-sdk`.
- Consumer wiring (Lodestone's `builder.go`, `aisdkprovider/factory.go`, chat-sidebar). Out of scope here; lands downstream after this change ships and the subtree is bumped.

## Decisions

### 1. Nested Go module under `middleware/sigil/`

**Decision:** Create `middleware/sigil/` as its own Go module with `replace github.com/grafana/ai-sdk => ../../`, matching the `providers/anthropic/` and `providers/grafana/` pattern.

**Rationale:** Sigil depends on `sigil-sdk/go`, which transitively pulls the OTel SDK and gRPC. The repo's existing convention is "any package with a heavy dep lives in its own nested module so consumers who don't need it pay nothing." Today that's only providers; this change extends the convention to middlewares. The root `middleware` package keeps the existing flat-file layout for lightweight middlewares (`ExtractReasoning`, `SimulateStreaming`, `DefaultSettings`, `TransformStream`).

**Alternatives considered:**
- **Flat file in root `middleware` package** — would pull `sigil-sdk` into the root module's `go.sum`, forcing every consumer (most of whom don't run Sigil) to download OTel + gRPC. Rejected.
- **Standalone repo** — `github.com/grafana/ai-sdk-sigil` — adds release coordination overhead and obscures the dependency relationship. The nested-module pattern handles this cleanly.

### 2. Two independent middlewares + composition helpers

**Decision:** Ship `RecordingMiddleware(opts)` and `HooksMiddleware(opts)` as independent `middleware.Middleware` values, plus `Stack(opts) []middleware.Middleware` returning the canonical order and `Wrap(base, opts) provider.LanguageModel` as a one-call convenience.

**Composition order (outer → inner):**
```
Hooks      (can deny early; can mutate prompt)
Recording  (wraps the actual model call; sees post-Hooks params)
<inner model>
```

**Rationale:**
- Hooks must run first so a denial short-circuits Recording — we don't want a Generation row for a request that was never sent upstream.
- Recording must observe post-Hooks params so the recorded payload matches what was actually sent.
- Separating the two concerns lets a consumer enable Recording without Hooks (or vice versa) by opting out via `opts.Hooks.Enabled` / `opts.Recording.Enabled`.
- This is a faithful translation of today's `claude.Chain` ordering (`Hooks → Metrics → Tracing → Router`); Recording-as-middleware replaces "recording inside the backend".

**Alternatives considered:**
- **Single combined middleware** — couples two orthogonal concerns; harder to test in isolation.
- **Inverted order (Recording outer, Hooks inner)** — would record denials as completed generations with empty output. Wrong semantics.

### 3. `ClientResolver` + `ContextProvider` callbacks for consumer-injection

**Decision:** Two callback types fill the seams the consumer owns:

```go
type ClientResolver func(ctx context.Context) *sigilsdk.Client
type ContextProvider func(ctx context.Context) ContextInfo

type ContextInfo struct {
    UserID       string
    Metadata     map[string]any
    Tags         map[string]string
    AgentName    string
    AgentVersion string
}
```

`ClientResolver` returning `nil` makes Recording a no-op for that request. `ContextProvider` returning a zero value is tolerated; fields fall back to whatever `sigil.UserIDFromContext` / `sigil.AgentNameFromContext` etc. provide.

**Rationale:** The middleware needs per-tenant routing (Grafana's `sigilregistry.ForContext`) and per-request metadata (tenant_id, stack_id, oss_mode), but those are Grafana-specific concepts that don't belong in `ai-sdk`. The callback boundary keeps the module reusable.

**Alternatives considered:**
- **Direct `*sigilsdk.Client` field on opts** — fails for multi-tenant consumers who route per-context. Rejected.
- **`context.Context`-keyed client** — works but is opaque and harder to mock; an explicit callback is clearer.

### 4. Provider-specific knobs flow through `ProviderOptions[name]` as opaque JSON

**Decision:** The mapper reads Anthropic-specific options (thinking budget, cache control hints, server-tool flags) through `provider.ProviderOptions["anthropic"]`, which is a `json.RawMessage`-typed map value. It decodes the subset it needs locally (`map_provider_options.go`); it never imports `providers/anthropic`.

**Rationale:** `middleware/sigil/` is a sibling module to `providers/anthropic/`; importing it from a middleware would invert the dependency arrow (providers depend on the root, never vice versa) and would also drag the Anthropic SDK into a place that doesn't need it. Reading `ProviderOptions` as opaque JSON preserves the abstraction.

**Consequence:** When OpenAI / Google / Grok land with their own `ProviderOptions[name]` keys, the mapper handles them the same way — decode the subset Sigil cares about, attach the rest as `sigil.Generation.Metadata` entries via `map_provider_options.go`.

### 5. `StreamRecorder` stateful accumulator

**Decision:** A `StreamRecorder` type observes each `provider.StreamPart`, accumulates `sigil.Generation.Output`, tracks usage / finish reason / first-chunk timestamp, and yields the final `sigil.Generation` at end-of-stream:

```go
type StreamRecorder struct{ /* unexported state */ }
func NewStreamRecorder(req sigilsdk.GenerationStart, params provider.CallOptions) *StreamRecorder
func (r *StreamRecorder) Observe(part provider.StreamPart)
func (r *StreamRecorder) FirstChunkAt() time.Time
func (r *StreamRecorder) Generation() sigilsdk.Generation
```

The middleware tees the stream channel (one goroutine reads from the inner model, fans out to the consumer and to `Observe`).

**Reasoning-signature handling.** Anthropic's `PartReasoningEnd` is empty; the signature lives in `PartReasoningDelta.ProviderOptions["anthropic"].signature`. The recorder merges signatures across deltas into the consolidated reasoning part — this is the same trap Lodestone's `AccumulateAssistantContent` had to solve, and the fix is portable code (their `lodestone/aisdk/stream_accumulator.go` is the reference implementation).

**Alternatives considered:**
- **Buffer all parts, build Generation in one pass at end-of-stream** — simpler but doubles memory residency for large streams.
- **Inline observation in the consumer's read loop** — couples Sigil to every caller; defeats the middleware abstraction.

### 6. Mapper: byte-equal `sigil.Generation` parity with `sigil-sdk/go-providers/anthropic`

**Decision:** For inputs that are logically the same (an ai-sdk-typed equivalent of an Anthropic-SDK request), `MapGenerateResult` / `StreamRecorder.Generation()` produce **byte-equal** `sigil.Generation` JSON to what `sigil-sdk/go-providers/anthropic.FromRequestResponse` / `FromStream` produces today, modulo recorder-set fields (`id`, `started_at`, `completed_at`, `trace_id`, `span_id`).

**Rationale:** Sigil dashboards and BigQuery views filter on `stop_reason`, `metadata.gen_ai.request.*`, tags, etc. If those drift between the claude path and the ai-sdk path, half the existing observability silently regresses.

**Enforcement:** Phase 1 conformance tests pair (ai-sdk-typed input, Anthropic-SDK-typed input) fixtures and assert byte-equal output. The fixture set is the existing conformance data in `ai-sdk/providers/anthropic/test/conformance/recorded/` plus a new `middleware/sigil/testdata/` set captured from `sigil-sdk/go-providers/anthropic` conformance tests.

### 7. Hooks transform preserves reasoning signatures

**Decision:** When `EvaluateHook` returns a `TransformedInput`, `HooksMiddleware` replaces `params.Prompt` but reuses the original assistant-role messages that contain reasoning parts (matched by text content), because the cryptographic signatures don't round-trip through Sigil's wire schema.

**Algorithm:**
1. Build a content-matching index over assistant-role messages in the **original** `params.Prompt` that contain reasoning parts.
2. For each transformed assistant message, look up the matching original by text content; if found, use the original message verbatim; otherwise rebuild from `sigil` parts.

This is the provider-agnostic version of `hooks_middleware.go:findMatchingOriginalWithThinking`. Phase 3 includes a wire-byte round-trip test asserting reasoning-part `ProviderOptions["anthropic"].signature` equality before/after.

### 8. Context-key DAG plumbing lives in `middleware/sigil/`

**Decision:** Generation-ID DAG context helpers — `WithGenerationID`, `NewGenerationID`, `WithParentGenerationIDs`, `WithLinkedGenerationID`, `GenerationIDFromContext`, `ParentGenerationIDsFromContext` — live in `middleware/sigil/context.go`. Consumers import them as `sigil.WithGenerationID(ctx, id)`.

The names drop the `Sigil` prefix from their old `internal/llm/claude/sigil_generation_context.go` form because they live in the `sigil` package now (idiomatic Go: avoid stuttering).

**Rationale:** The DAG linkage is Sigil-specific by name but ai-sdk-shaped by content (it's about `context.Context` values that the mapper reads when building a `GenerationStart`). It belongs with the Sigil mapper code.

**Open consideration:** If a consumer wants DAG plumbing without the heavy middleware module, they could end up importing the nested module just for context keys. Flagged in Open Questions. If that proves painful, a follow-up change can extract context keys into a tiny `sigil/` root package; the middleware module re-exports them. Not blocking this change.

### 9. Typed error model for hook denials

**Decision:**

```go
var ErrHookDenied = errors.New("sigil: hook denied request")

type HookDenialError struct {
    Reason string
    RuleID string
    Cause  error
}

func (e *HookDenialError) Error() string { /* ... */ }
func (e *HookDenialError) Unwrap() error { return ErrHookDenied }
```

`HooksMiddleware` returns `*HookDenialError` on deny. Consumers translate to their transport-layer error (e.g. `failure.RequestBlocked`) at the API boundary. `errors.Is(err, sigil.ErrHookDenied)` works for generic deny detection.

**Rationale:** Sentinel + typed-wrapper is the standard ai-sdk pattern (see `provider/api-call-error`, `aisdk/retry_error.go`). The consumer keeps full structured context (reason, rule_id) for logging.

## Risks / Trade-offs

| Risk | Severity | Mitigation |
|---|---|---|
| Mapper drifts from `sigil-sdk/go-providers/anthropic` and produces a subtly different `sigil.Generation` on the same logical request | High | Phase 1 conformance tests assert byte-equal Generation JSON on shared fixtures. A `make test-sigil-conformance` target re-runs on every PR that touches `middleware/sigil/` or bumps `sigil-sdk` in `go.mod`. |
| Reasoning-block signatures lost in the `TransformedInput` round-trip → silent quality regression on Claude | High | Port `findMatchingOriginalWithThinking` to the provider-agnostic mapper. Phase 3 adds a wire-byte round-trip test on a captured reasoning thread asserting signature byte-equality. |
| `StreamRecorder` misses signature metadata because `PartReasoningEnd` is empty for Anthropic | High | Recorder merges `ProviderOptions["anthropic"].signature` across `PartReasoningDelta`s into the consolidated reasoning part. Mirrors the lodestone `stream_accumulator.go` fix verbatim. |
| `provider.LanguageModel` concurrency contract is undocumented; the Recording tee goroutine could overlap with the consumer reading the streamed parts | Medium | Mirror the established `aisdkprovider/logger.go` pattern: bounded buffered channel; select on `ctx.Done()` on every send; drain upstream on consumer abandonment so the producer goroutine doesn't leak. |
| `sigil-sdk` adds a new field to `Generation` that ai-sdk's mapper doesn't populate, and dashboards depending on that field stop working on the ai-sdk path | Medium | Conformance gate re-runs on every subtree bump (CI alarms on Generation drift). Documented in `middleware/sigil/doc.go` as a maintenance hazard. |
| `EvaluateHook` server stalls block the request indefinitely (no current timeout in `claude.HooksMiddleware`) | Medium | Match today's behavior by default (inherit request context). Add `HooksOpts.MaxLatency time.Duration` (zero = unset = today's behavior) so operators can opt into a budget without code changes. |
| Consumer forgets to set `ContextProvider`; Generation rows land without `tenant_id` and Sigil's tenant filtering breaks | Medium | `RecordingMiddleware` logs a Warn once-per-process on first call if `ContextProvider` is nil. The middleware does NOT refuse to record — that would mask the problem; operator visibility is enough. |
| Two consumers wrap the same base model with `sigil.Wrap` independently → two generation rows per request | Medium | Document that `sigil.Wrap` applies at the topmost model construction site, not at every call site. Same hygiene as today's `claude.Chain` ordering. |
| OTel span attribute keys drift from `llm.claude.*` shape that dashboards filter on | Medium | Span-attribute snapshot test in Phase 1. The namespace shifts to `aisdk.sigil.*` (intentional), but all attribute keys (`sigil.hooks.result`, `sigil.hooks.action`, `sigil.hooks.rule_id`, etc.) stay identical. |
| Per-tenant `*sigil.Client` resolution adds to the request path | Low | `ClientResolver` runs once per request, not per token. Today's `sigilregistry.Registry.ForContext` is an in-memory `RLock` + map lookup; negligible. |
| New nested middleware module creates a coordination cost when bumping subtrees in `grafana-assistant-app` | Low | Same coordination pattern as today's `providers/anthropic/` subtree. Documented in `middleware/sigil/doc.go`. |

## Migration Plan

This change is **additive only** — no existing code paths are modified.

1. **Land** `middleware/sigil/` in `github.com/grafana/ai-sdk` `main`. Includes module, mapper, both middlewares, context helpers, conformance fixtures, and tests.
2. Downstream `grafana-assistant-app` bumps its vendored subtree (separate PR) to a SHA that includes this change.
3. Downstream consumer wiring (`api/internal/sigilaisdk/`, Lodestone's `builder.go`, `aisdkprovider/factory.go`) lands behind existing feature flags (`assistant.lodestone.ai-sdk-engine`, `assistant.chat.ai-sdk-engine`). Out of scope for this change.

**Rollback** of this change alone: revert the new module's directory. Because nothing imports `middleware/sigil/` from the root or any other provider module, removal is local. Downstream consumers fall back to whatever they had before bumping the subtree.

## Open Questions

These are flagged so they get resolved during implementation or by the first consumer, not before the spec lands. None block this change.

1. **Should context-key helpers move to a lightweight `sigil/` root package** if a consumer wants DAG plumbing without `sigil-sdk` as a transitive dep? Not blocking — `middleware/sigil/` covers every known caller today. Revisit if pain emerges.
2. **Span attribute namespace.** Decision is `aisdk.sigil.*` (shift from `llm.claude.*` because the legacy path is being supplanted). Open: do we keep the exact attribute KEYS unchanged inside that namespace, or also shift to `aisdk.sigil.hooks.*`? Default in this design is "keep keys identical so existing dashboard filters match without OR clauses" — Phase 1 snapshot test enforces it.
3. **Conformance fixtures source.** Phase 1 sources expected `sigil.Generation` JSON from `sigil-sdk/go-providers/anthropic` conformance helpers. If those helpers don't expose the shape we need (e.g., they only test through `FromRequestResponse` end-to-end), we may need to snapshot from a recorded request the first time and treat sigil-sdk as the source of truth going forward. Validated when the first fixture lands.
