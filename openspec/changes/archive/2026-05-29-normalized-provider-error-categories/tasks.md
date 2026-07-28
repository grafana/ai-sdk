## 1. Resolve open questions

- [x] 1.1 Decide whether to expose category sentinels (`errors.Is(err, provider.ErrRateLimit)`) in addition to `GatewayError.Type` switching; record in design.md
- [x] 1.2 Decide Anthropic `overloaded_error` (529) mapping: `rate_limit_exceeded` vs `internal_server_error`; record in design.md
- [x] 1.3 Confirm `providers/anthropic` standalone stays layer-1 only (no normalization); record in design.md

## 2. Layer 1 -- preserve structured error in APICallError.Data

- [x] 2.1 Anthropic: parse the `{type, message}` error envelope from `sdk.Error.RawJSON()` and set `APICallError.Data` in `providers/anthropic/wrap_api_error.go` (both `wrapAPIError` and `wrapAsAPICallError`)
- [x] 2.2 Anthropic: keep raw body in `ResponseBody` when the envelope cannot be parsed; leave `Data` empty
- [x] 2.3 Grafana: ensure `decodeOrSynthesizeHTTPError` preserves the decoded `Data` (confirm `wire.DecodeErrorResponse` round-trips it; add coverage if missing)
- [x] 2.4 Unit tests: Anthropic `Data` populated with the structured `type` for rate-limit/auth/invalid-request; empty `Data` + intact `ResponseBody` on unparseable body; end-to-end `DoStream` 529 emits `PartError` with structured `Data` (run from `providers/anthropic/`)

## 3. Layer 2 -- gateway error normalization in provider/

- [x] 3.1 Add `GatewayErrorType` typed string enum + constants (`authentication_error`, `invalid_request_error`, `rate_limit_exceeded`, `model_not_found`, `internal_server_error`) in `provider/` (e.g. `provider/gateway_error.go`)
- [x] 3.2 Add `GatewayError` struct (`Type`, `Message`, `StatusCode`, optional `ModelID`, unexported `cause`) with `Error()` and `Unwrap()` returning the originating `*APICallError`
- [x] 3.3 Add compile-time interface check `var _ error = (*GatewayError)(nil)`
- [x] 3.4 Add a normalizer `NormalizeAPICallError(*APICallError) *GatewayError` that reads the structured `type` from `Data` (falling back to parsing `ResponseBody`), maps via switch to `Type`, defaults to `internal_server_error`, and sets the `*APICallError` as cause -- modeled on upstream `extractApiCallResponse` + `createGatewayErrorFromResponse`
- [x] 3.5 Populate `ModelID` for `model_not_found` when the structured payload reports it
- [x] 3.6 Unit tests: each mapping; `Data`-first then `ResponseBody` fallback; unknown -> `internal_server_error`; `errors.As` reaches both `*GatewayError` and the originating `*APICallError`

## 4. Grafana gateway-analog integration

- [x] 4.1 In `providers/grafana/model.go`, run `provider.NormalizeAPICallError` on the decoded error and surface `*GatewayError` when categorized, plain `*APICallError` otherwise
- [x] 4.2 Unit tests: categorized (rate-limit/auth/model-not-found) surfaces `GatewayError`; uncategorized stays plain; `Data` preserved; `errors.As(&APICallError{})` still works

## 5. Fallback context-window heuristic

- [x] 5.1 In `fallback/fallback.go` `defaultDecider`, add: `*APICallError` with `StatusCode == 400` and a context-length signal (from `Data`/`Message`) -> return no-failover; keep existing `IsRetryable` behavior otherwise
- [x] 5.2 Confine the context-length matcher to the decider; no public category
- [x] 5.3 Unit tests: context-window 400 stops fallback; retryable triggers; non-retryable stops; unknown triggers; wire-reconstructed `APICallError` still works

## 6. Verification

- [x] 6.1 `make fmt && make vet && make test` (root + providers)
- [x] 6.2 `openspec validate normalized-provider-error-categories --strict`
- [x] 6.3 Note downstream migration: `grafana-assistant-app` switches on `GatewayError.Type` (or `errors.As(&provider.GatewayError{})`), retiring `isAISDKQuotaMessage`/`isAISDKContextLengthMessage` (out of this repo)
