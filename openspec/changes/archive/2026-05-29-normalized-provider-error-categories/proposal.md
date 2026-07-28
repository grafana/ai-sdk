## Why

Consumers of `provider.APICallError` cannot classify provider failures (rate
limit, authentication, model-not-found, context-window) without string-sniffing
`Message`/`ResponseBody` and guessing from `StatusCode`. The provider already
knows the category -- Anthropic returns a structured error `type`
(`rate_limit_error`, `invalid_request_error`, `overloaded_error`,
`authentication_error`), and the Grafana wire envelope carries an error type --
but our providers drop it into raw `ResponseBody` and leave the existing
`APICallError.Data` field empty. As we add more providers, every consumer
re-deriving semantics with brittle, per-provider heuristics gets strictly worse.

Upstream Vercel AI SDK solves exactly this with a **two-layer split**: providers
preserve the native structured error in `APICallError.data` (lossless, no
classification); a single gateway-layer normalizer maps the heterogeneous
vocabularies into one normalized `type` discriminator. We are currently
regressing on layer one (not populating `Data`) and missing layer two entirely.

## What Changes

- **Layer 1 (preserve)**: Each provider SHALL populate `APICallError.Data` with
  its native structured error payload (including the provider error `type`),
  matching upstream's `createJsonErrorResponseHandler` behavior. Nothing is
  classified or flattened away.
  - Anthropic: parse the `{type, message}` error envelope into `Data`.
  - Grafana: ensure the decoded wire error's `Data` is preserved end-to-end.
- **Layer 2 (normalize)**: Introduce a single gateway-style normalizer in
  `provider/` -- a `GatewayError` analog with a normalized `Type string`
  discriminator and a small typed set (`authentication_error`,
  `invalid_request_error`, `rate_limit_exceeded`, `model_not_found`,
  `internal_server_error`) mirroring `@ai-sdk/gateway`. The normalized error
  holds the originating `*APICallError` as its `cause` (replacing it as the
  primary error), exactly as upstream's `asGatewayError` does.
- `providers/grafana` (our gateway analog) is the place that produces normalized
  errors from the wire envelope's error type. Plain providers stay at layer 1.
- **Context-window / fallback**: Upstream has no context-window category. Keep a
  small, localized heuristic ONLY inside `fallback.Model`'s no-failover decision
  (status 400 + context-length signal) so a `fallback.Model` does not fail over
  on an error the next candidate would also hit. No public context-window
  category is introduced.

## Capabilities

### New Capabilities
- `gateway-error-normalization`: A gateway-style normalized error in `provider/`
  with a `Type` discriminator and the upstream category vocabulary, a
  `FromAPICallError`-style normalizer that reads the structured type out of
  `APICallError.Data`, and `errors.As`/`Unwrap` semantics that expose the
  originating `APICallError` as the cause.

### Modified Capabilities
- `api-call-error`: Add the requirement that the Anthropic provider populates
  `APICallError.Data` with the parsed structured error (layer 1). Extend the
  fallback-decider requirement so context-window failures suppress failover via
  a localized decider heuristic (no public category).
- `grafana-provider`: Extend error reconstruction so the decoded wire error's
  `Data`/error-type is preserved and mapped into the normalized gateway error
  when a category is present, falling back to a plain `APICallError`.

## Impact

- `provider/`: new `GatewayError` analog + normalizer + `Type` constants.
- `provider/api_call_error.go`: `Data` becomes the canonical home for the
  structured provider error (no shape change; it already exists and round-trips).
- `providers/anthropic/wrap_api_error.go`: parse the error envelope into `Data`.
- `providers/grafana/model.go`: preserve `Data`; produce normalized error from
  the wire error type.
- `fallback/fallback.go`: localized context-window no-failover heuristic.
- Wire: no new field required -- the structured type already round-trips inside
  `APICallError.Data` (verified in grafana provider tests).
- Downstream `grafana-assistant-app`: retires `isAISDKQuotaMessage` /
  `isAISDKContextLengthMessage`; switches on the normalized `Type` (or
  `errors.As` against the gateway error).
