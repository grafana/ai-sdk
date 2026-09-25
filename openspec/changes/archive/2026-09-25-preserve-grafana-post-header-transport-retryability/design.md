## Context

`providers/grafana/model.go` currently sends a single request, calls `readJSON` for successful unary responses, then wraps **all** read failures with `protocolError` (non-retryable). `readJSON` in `discovery.go` has distinct media, bounded `io.ReadAll`, byte-limit, and JSON-validation branches, but its callers receive one undifferentiated `error`. Streaming `consumeStream` already distinguishes transport read errors from stream limits and does not replay. Non-2xx responses use a separate closed service-error mapping. The Gateway client is a Grafana extension; `test/conformance/PARITY.md` marks its coverage mixed.

The exact registered reference is `test/conformance/upstream.yaml` commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`, `@ai-sdk/gateway@4.0.87`, `@ai-sdk/provider-utils@5.0.45` (npm package sources installed from `test/pnpm-lock.yaml`, not upstream main). Gateway `gateway-language-model.ts` `doGenerate` catches `postJsonToApi` failures and calls `asGatewayError`; `as-gateway-error.ts` preserves an `APICallError`'s retryable flag when its status is absent or `<400`. Provider-utils `response-handler.ts` reads the response before `safeParseJSON`; `post-to-api.ts` wraps successful-response handler failure as `APICallError` with status. `handle-fetch-error.ts` marks recognized network causes retryable, including `UND_ERR_SOCKET`/`ECONNRESET`. `createJsonResponseHandler` separately labels malformed JSON `Invalid JSON response`. The pinned npm package omits its test sources; compare the executable pinned client in the differential test rather than claiming imported upstream tests were run. Its `z.any()` success schema is an intentional departure from Grafana's strict family mapper, not a reason to relax it.

## Goals / Non-Goals

**Goals:** Distinguish HTTP 200 unary body read I/O failure from protocol/validation/size errors; preserve retryable `*provider.APICallError` with status and inspectable cause, local bounded message, no result, and one HTTP request. Verify comparable retryability against the pinned Gateway client on a real aborted HTTP response. Preserve cancellation identity and existing limits.

**Non-Goals:** No automatic request replay, stream resumption or retry after delivered stream parts; no widening non-2xx or `/config` transport classification; no change to service categories, response parsing strictness, pins, or public API. #87 and #121 have separate ownership.

## Decisions

1. **Classify only at the successful `DoGenerate` boundary.** Give the unary caller a way to distinguish the `io.ReadAll` failure branch from `readJSON`'s media, byte-limit, and invalid-JSON branches without changing the behavior of discovery or non-2xx `readGatewayError` callers. A private typed/sentinel read-failure discriminator (with underlying error retained through wrapping) or a unary-specific read helper is acceptable; prefer the smallest solution supported by code and tests. On a genuine read I/O error and live context, return `provider.NewAPICallError` with a fixed `grafana:` message, HTTP status, cause, and explicit retryability only if the partial body remains within the unary byte limit. If the bounded read returns both an over-limit partial body and a read error, classify it as a non-retryable unary byte-limit protocol failure, not a retryable transport failure; this requires checking the limit before classifying the read error (currently `readJSON` checks the error first). Keep `protocolError` for all other failures and for `decodeGenerate`. Check context cancellation before both byte-limit and retry classification; retain `errors.Is(err, context.Canceled/DeadlineExceeded)` and no retryable result. An alternative, marking all `readJSON` errors retryable, would misclassify malformed JSON and resource limits and change unrelated callers. Using `io.ErrUnexpectedEOF` alone as a classifier would miss other genuine socket read errors; branch provenance is stronger than string/type matching.

2. **Use an actual HTTP server to prove post-header failure.** In a Go `httptest` handler write HTTP 200 and `Content-Type: application/json`, declare a `Content-Length` greater than the partial body, flush headers/body, then terminate the connection to force a real body read error (assert the fixture produces it). Ensure the bytes sent are below the configured unary limit. Check `errors.As` to API error, status 200, retryable flag, `errors.Is` for the underlying unexpected EOF (or the observed transport error), bounded local error prose without body/secret leakage, body cleanup, and exactly one model request. Compare the same protocol failure in `ai-gateway/test/providerwire-v4/go-client-differential.test.ts` using the pinned `createGateway` and Go capture helper. Assert retryability and request counts, not identical error messages or permissive response parsing. Existing malformed/limit tests and additional wrong-media/schema and cancellation checks cover non-transport paths. A synthetic `RoundTripper` error before headers does not establish the intended behavior.

3. **Do not retry inside the client.** Error classification is for caller-owned orchestration only; established SSE output remains a one-request invocation. An alternative, replaying on HTTP 200, risks duplicate model effects and violates the existing no-implicit-retry contract.

## Risks / Trade-offs

- [HTTP truncation accidentally surfaces as normal EOF or malformed JSON] → Use declared `Content-Length`, flush before closing, assert the server failure is observed as body read I/O by both clients; fail the test if fixture does not exercise this path.
- [Pinned Node error cause differs across runtimes] → Assert exact pinned client's observable retryability under the real test server; if it does not classify the chosen failure as retryable, investigate the transport code without inventing parity or broadening Go errors.
- [Leaked private details via error prose] → Only put static bounded text in primary error message; retain original read error as cause for `errors.Is`/diagnostics and avoid dumping response bodies or secret headers.
- [Shared helper changes affect discovery/non-2xx] → Run explicit regression checks for those paths; leave their existing classification unchanged and assess independent changes separately.

## Migration Plan

No migration. Land tests and implementation together, run scoped Go and differential tests plus registered baseline/parity checks; revert the change if a boundary assertion fails. No pin or service migration.

## Open Questions

None for this change. Owner scope decision: HTTP 200 successful unary `DoGenerate` only; non-2xx and discovery are unchanged, not silently promised retryability.
