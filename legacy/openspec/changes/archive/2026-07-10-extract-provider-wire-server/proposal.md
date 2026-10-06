## Why

The reusable server half of the provider wire currently lives in Grafana Assistant, while ai-sdk splits the remote protocol across the `provider/wire` codecs and the `providers/grafana` client. Co-locating the complete remote `provider.LanguageModel` protocol and its reusable server handler in `gateway/providerwire` establishes one public ownership boundary: `provider` remains the transport-agnostic in-process contract, `gateway/providerwire` owns the remote JSON/HTTP/SSE protocol, and `providers/grafana` is its client.

## What Changes

- Add a public `net/http` provider-wire handler under `gateway/providerwire`.
- Move all existing route/header constants, JSON request/response codecs, SSE framing/readers/writers, error envelopes, and their tests from `provider/wire` into `gateway/providerwire`, co-located with the handler.
- Delete `provider/wire` and update all live consumers to import `github.com/grafana/ai-sdk/gateway/providerwire`; do not add an alias package, compatibility shim, or re-export.
- Classify the Go import path change as source-breaking while preserving canonical encoded bytes and protocol shapes except for the documented robustness corrections.
- Define a small request-aware model resolver boundary so hosts can apply authenticated tenant and model policy before returning a `provider.LanguageModel`.
- Validate provider-wire methods, headers, content negotiation, and bounded JSON bodies before resolving or invoking a model.
- Dispatch unary and streaming model calls and encode success and failure responses exclusively through the co-located `gateway/providerwire` contract.
- Preserve Assistant behavior for immediate SSE flushing, clean EOF without `[DONE]`, pre-stream HTTP errors, mid-stream `PartError`, cancellation, total timeouts, stream idle timeouts, and lenient `Accept` parameter stripping (including accepting a matching media range with `q=0`), while correcting the SSE reader to process a final data line returned together with `io.EOF`.
- Intentionally correct current Assistant pre-commit response bugs: encode unary results and validate API-call error envelopes before commitment, and fall back to a retryable HTTP 500 canonical error envelope when a result cannot be encoded or an error envelope has unencodable fields or an invalid final HTTP status.
- Add handler tests and a real `providers/grafana` client/server conformance test without introducing a Go module dependency cycle.
- Define the Assistant migration boundary: Assistant retains auth, catalog/policy resolution, route prefix/mounting, billing/deployment concerns, and host observability; its provider-wire transport becomes a thin wrapper around the public handler, with host logging updated to recognize or translate the public timeout sentinels.
- Classify the extraction and package move as parity-preserving provider-transport work: canonical encoded bytes, protocol shapes, and frontend UI SSE behavior remain stable except for the documented robustness corrections.

## Capabilities

### New Capabilities
- `gateway-providerwire-server`: Public complete provider-wire protocol package, including co-located route/header constants, JSON/SSE/error codecs, server validation, request-aware model resolution, unary/stream dispatch, response encoding, flushing, cancellation, and timeout behavior.

### Modified Capabilities
- `provider-wire`: Move complete JSON/HTTP/SSE protocol ownership and its codec tests from `provider/wire` to `gateway/providerwire`, delete the old package without a compatibility shim, preserve canonical encoded bytes and protocol shapes, and apply the documented robustness corrections.
- `grafana-provider`: Change the client to import and use `gateway/providerwire` while preserving its existing request, response, streaming, and error behavior.
- `api-call-error`: Update the wire-reconstruction requirement to name `gateway/providerwire` without changing fallback behavior.

## Impact

- New public package and sole remote-protocol owner: `github.com/grafana/ai-sdk/gateway/providerwire`.
- Removed public package: `github.com/grafana/ai-sdk/provider/wire`. This is an intentional source-breaking import-path change with no aliases, forwarding package, or compatibility re-exports; consumers must update imports.
- `github.com/grafana/ai-sdk/provider` remains the transport-agnostic in-process model/type contract and does not depend on gateway transport code.
- `providers/grafana` continues as a separate client module and depends on both root packages `provider` and `gateway/providerwire`.
- Existing codec tests move with their implementation; new root-module handler/lifecycle tests and nested-module/conformance coverage use the same co-located protocol implementation.
- Follow-up Assistant work can replace `api/internal/aisdkprovider` validation/dispatch code with auth middleware, a request-aware resolver adapter, and its existing Gorilla route mount. Its side-by-side expectations must adopt retryable HTTP 500 for invalid unary results and invalid API-call error envelopes, and its logging/classification must recognize `providerwire.ErrIdleTimeout` and `providerwire.ErrTotalTimeout` or translate them to the existing host sentinels.
- No new router, auth, IAM, billing, catalog, deployment, or provider SDK dependency is added to the root module.
