## Context

We struggle to track backend errors today, and adding providers will worsen it.
The root cause is that our providers discard the structured error the provider
already computed, forcing consumers to string-sniff.

Upstream Vercel AI SDK (`~/src/ai`) was traced to anchor this design. It uses a
deliberate **two-layer error model**:

1. **Provider layer preserves, never classifies.** Every provider error goes
   through `createJsonErrorResponseHandler`
   (`packages/provider-utils/src/response-handler.ts`), which parses the
   provider's error body against a provider-specific schema and stores the whole
   parsed structure in `APICallError.data` (line 63). Anthropic's schema
   (`packages/anthropic/src/anthropic-error.ts`) captures `{type, message}`. The
   core `@ai-sdk/provider` errors package has **no** `RateLimitError`,
   `AuthenticationError`, or `ContextWindowExceededError` -- categorization does
   not happen at the provider layer.

2. **Gateway layer normalizes, centrally.** Categories live only in
   `@ai-sdk/gateway` (`packages/gateway/src/errors/`). `GatewayError` is a base
   class with an abstract **`type: string`** discriminator and a marker checked
   via `isInstance`. Subclasses: `GatewayAuthenticationError` (`type:
   "authentication_error"`), `GatewayInvalidRequestError`,
   `GatewayRateLimitError` (`"rate_limit_exceeded"`), `GatewayModelNotFoundError`
   (`"model_not_found"`, `+modelId`), `GatewayInternalServerError`,
   `GatewayResponseError`, `GatewayTimeoutError`. `asGatewayError` /
   `createGatewayErrorFromResponse` read the structured type out of the
   `APICallError` via `extractApiCallResponse` (which reads `error.data`, then
   falls back to parsing `responseBody`) and produce a `GatewayError` whose
   **`cause` is the `APICallError`** -- the gateway error replaces the
   APICallError as the primary error; it does not wrap-and-expose-both.

Our `providers/grafana` is the conceptual analog of the Vercel gateway. Our
`APICallError` already has a `Data json.RawMessage` field that round-trips on
the grafana wire (verified in `providers/grafana/provider_test.go`).

This design supersedes an earlier draft that put typed category errors in
`provider/` and wrapped `APICallError` via `errors.As`. That draft diverged from
upstream (no such provider-layer categories; gateway uses cause-replacement and
a `type` discriminator, not wrapping) and is rejected.

## Goals / Non-Goals

**Goals:**
- Stop discarding the structured provider error: populate `APICallError.Data`
  uniformly (layer 1). This alone fixes most backend-tracking pain.
- Provide one normalized `Type` discriminator string for cross-provider
  classification and logging/metrics (layer 2), mirroring `@ai-sdk/gateway`.
- Make the normalizer the single place that maps provider vocabularies ->
  normalized type, so adding a provider is "add a `Data` schema" + (optionally)
  "add one mapping case."
- Solve the fallback no-failover-on-context-window bug without inventing a
  public category upstream lacks.

**Non-Goals:**
- Adding `RateLimitError`/`ContextWindowExceededError`/etc. as provider-layer
  typed errors (upstream has none; rejected).
- Changing the `APICallError` shape or the wire (the `type` already rides inside
  `Data`).
- Exhaustive provider coverage; unmapped errors normalize to
  `internal_server_error` / stay plain `APICallError`, matching upstream's
  `default` branch.
- A public context-window category.

## Decisions

### Decision 1: Two-layer split, mirroring upstream

- **Layer 1 (every provider)**: populate `APICallError.Data` with the parsed
  native structured error. No classification. Mirrors
  `createJsonErrorResponseHandler` storing `data: parsedError`.
- **Layer 2 (gateway analog)**: a single normalizer maps `Data`'s `type` (plus
  status) to a normalized `Type`. Mirrors `createGatewayErrorFromResponse`.

Rationale: directly addresses "tracking gets worse with more providers" --
classification logic is single-sourced, and the raw structured error is always
available for forensic logging even when uncategorized.

### Decision 2: `GatewayError` analog with a `Type string` discriminator, cause = APICallError

Add to `provider/` (Go idioms, not a TS class translation):

```
type GatewayErrorType string
const (
    GatewayErrorAuthentication GatewayErrorType = "authentication_error"
    GatewayErrorInvalidRequest GatewayErrorType = "invalid_request_error"
    GatewayErrorRateLimit      GatewayErrorType = "rate_limit_exceeded"
    GatewayErrorModelNotFound  GatewayErrorType = "model_not_found"
    GatewayErrorInternalServer GatewayErrorType = "internal_server_error"
)

type GatewayError struct { Type GatewayErrorType; Message string; StatusCode int; ModelID string; ... }
func (e *GatewayError) Unwrap() error { return e.cause } // cause is *APICallError
```

- `Type` is the canonical discriminator (what backend logs/metrics key on), a
  typed string enum per project conventions.
- `Unwrap()` returns the originating `*APICallError`, so `errors.As(&APICallError{})`
  still reaches status/headers/body/retryability -- matching gateway's
  `cause = APICallError`.
- A single concrete `GatewayError` with a `Type` field (not one Go type per
  category) keeps the API small while preserving upstream's discriminator
  semantics. Consumers switch on `gwErr.Type`. (Open question: whether to also
  expose category sentinel helpers like `errors.Is(err, ErrRateLimit)`.)

**Why over per-category Go types**: upstream's category *classes* are a TS
convenience; the contract is the `type` string. A single struct + typed enum is
the idiomatic Go expression of the same discriminator and serializes cleanly.

### Decision 3: Normalizer reads `Data`, falls back to body

Provide `NormalizeAPICallError(*APICallError) *GatewayError` modeled on
`asGatewayError`/`extractApiCallResponse`: read the structured `type` from
`Data` (fall back to parsing `ResponseBody`), map via a switch to the normalized
`Type`, default to `internal_server_error`. Used by `providers/grafana` on the
wire error envelope. Plain providers (anthropic standalone) stay at layer 1 and
return `APICallError`; the normalizer can be applied by whoever owns the gateway
boundary.

### Decision 4: Context-window stays a localized fallback heuristic

Upstream has no context-window category anywhere. The motivating need is purely
"don't fail over to a candidate that will fail identically." Implement it as a
small predicate inside `fallback.Model`'s decider: when the error is an
`APICallError` with `StatusCode == 400` and a context-length signal (checked
against `Data`/`Message`), return no-failover. This is the single remaining
heuristic, confined to one decision site, versioned with us -- not a public
type, not leaked to consumers.

### Decision 5: No wire change

The structured `type` already round-trips inside `APICallError.Data` (grafana
provider test confirms `Data` survives encode/decode). The normalizer can run on
either side of the wire from `Data`. No new envelope field, no `provider-wire`
spec change.

## Risks / Trade-offs

- [Single `GatewayError` struct vs. upstream's class-per-category] -> Preserve
  the exact `Type` string vocabulary so behavior/wire-vocabulary matches upstream
  even though the Go shape differs; document the intentional idiom deviation.
- [Context-window heuristic still string/status-based] -> Confined to one
  decider predicate, versioned with us, covered by tests; strictly better than
  the heuristic living in every consumer.
- [`Data` schema drift as providers are added] -> Each provider owns a small
  error schema; the normalizer's mapping switch is the only shared surface. A
  missing mapping degrades to `internal_server_error` + intact `Data`, never a
  crash or silent loss.
- [Who runs the normalizer for non-gateway providers?] -> Layer 1 (`Data`) is
  always populated, so even a plain `APICallError` is fully inspectable; layer 2
  is opt-in at the gateway boundary, matching upstream where `asGatewayError`
  only runs in the gateway.

## Resolved Decisions (formerly Open Questions)

- **Category sentinels**: NOT exposed for now (YAGNI). Consumers switch on
  `GatewayError.Type`, matching upstream's `type` discriminator. Sentinel helpers
  (`errors.Is(err, provider.ErrRateLimit)`) can be added later if consumer
  ergonomics demand, without breaking the `Type` contract.
- **Anthropic stays layer-1 only**: `providers/anthropic` populates
  `APICallError.Data` and never emits a `GatewayError`. Normalization is run only
  at the gateway boundary (`providers/grafana`), matching upstream where
  `asGatewayError` runs only in the gateway.
- **Anthropic `overloaded_error` (529) maps to `rate_limit_exceeded`**: it is the
  closest normalized semantic (both mean "back off and retry"); consumers treat
  them identically. This is a documented deviation from upstream, whose gateway
  has no 529 case.
