## Why

Grafana's `DoGenerate` currently converts an HTTP 200 body read failure into a non-retryable protocol error, even though the registered Gateway client retains retryability for post-header transport failures. This prevents upstream retry/fallback policy from recognizing a genuine interrupted unary response (#224).

## What Changes

- Preserve retryability and the underlying cause when the successful unary model response body fails to read after headers, without issuing a second request inside the Grafana client.
- Keep malformed JSON, wrong media type, invalid strict result schema, response-size violations, and cancellation non-retryable; retain bounded, locally worded public errors.
- Add real HTTP post-header truncation coverage and exact-pinned Gateway client differential evidence, with checks that established stream effects never cause client replay.
- Keep non-2xx Gateway envelopes, discovery, and the closed public service-error taxonomy unchanged; check these paths for regressions.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: Define retryability and privacy for successful unary response transport read failure while preserving the existing strict normalization and one-request policy.

## Impact

`providers/grafana/model.go` and focused provider tests; `ai-gateway/test/providerwire-v4/go-client-differential.test.ts` and its existing Go capture harness; `openspec/specs/grafana-gateway-client/spec.md` on application. No new API, dependencies, implicit retry, service taxonomy, or upstream baseline change. The registered reference is commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, `@ai-sdk/gateway@4.0.87` and `@ai-sdk/provider-utils@5.0.45` in `test/conformance/upstream.yaml`; `test/conformance/PARITY.md` classifies Gateway runtime/client coverage as mixed. #87 and #121 remain separate.
