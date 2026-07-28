## Context

ai-sdk currently owns the JSON/SSE provider codecs in `provider/wire` and the outbound client in the separate `providers/grafana` module. Grafana Assistant owns the matching inbound HTTP lifecycle in `api/internal/aisdkprovider`: protocol validation and bounded decoding, catalog resolution, unary/stream dispatch, response encoding, flushes, cancellation, and idle/total timeouts. That implementation also directly depends on Assistant caller identity, catalog, authlib, Gorilla mux, logging vocabulary, and timeout sentinels.

Issue #271 extracts the reusable lifecycle and consolidates the complete remote `provider.LanguageModel` protocol. `provider` remains the transport-agnostic in-process contract, `gateway/providerwire` owns route/header constants, JSON/SSE/error codecs, and server execution, and `providers/grafana` is the client. The former `provider/wire` package is deleted without aliases or re-exports. The registered upstream baseline is `ai@7.0.19`, but there is no upstream TypeScript equivalent for this internal Go-to-Go server. The relevant parity layer is provider implementation/transport: existing Grafana transport coverage is automated, while cancellation remains an explicit conformance gap.

## Goals / Non-Goals

**Goals:**

- Expose a public `net/http` handler co-located with the canonical provider-wire constants and codecs in `gateway/providerwire`.
- Preserve the Assistant implementation's observable request validation, response framing, error classification, flushing, cancellation, and timeout semantics except for explicitly classified pre-commit response corrections that replace empty implicit HTTP 200 responses or invalid-status panics with canonical retryable HTTP 500 errors.
- Let a host make request- and tenant-aware model decisions without introducing host identity or catalog types into ai-sdk.
- Prove the public server against both hand-written lifecycle tests and the real `providers/grafana` client without creating module cycles.
- Make the subsequent Assistant integration a thin auth, resolver, and route-mounting wrapper.

**Non-Goals:**

- A new provider protocol, DTO, wire version, frontend UI stream, or cross-language contract.
- Authentication, JWKS, IAM, billing, quotas, model catalogs, route prefixes, Gorilla mux, deployment wiring, or an observability framework.
- Moving Grafana client token exchange or gateway error-category normalization out of `providers/grafana`.
- Preserving source compatibility for the removed `provider/wire` import path; callers must migrate to `gateway/providerwire`.
- Implementing the Assistant migration in this repository change.

## Decisions

### D1. One package owns the complete remote protocol

Add `github.com/grafana/ai-sdk/gateway/providerwire` as the sole owner of the remote `provider.LanguageModel` protocol. Move the current route/header constants, request and response JSON codecs, SSE framing/readers/writers, error envelopes, and their tests from `provider/wire` into this package, then delete `provider/wire`. The handler uses the co-located `DecodeCallOptions`, `EncodeGenerateResult`, `WriteErrorResponse`, and `WriteSSEStreamPartTo` helpers. Canonical schemas, header values, JSON byte shapes, and SSE frame bytes remain unchanged. The only moved-helper semantic correction is the SSE reader EOF handling described below.

The dependency graph is deliberately one-way:

- `gateway/providerwire -> provider`
- `providers/grafana -> provider + gateway/providerwire`
- `provider` has no gateway or transport dependency

The gateway package imports only the standard library and `provider`; it introduces no router or host dependency. This makes protocol ownership match the server boundary while leaving the in-process contract transport-agnostic. The moved SSE reader additionally corrects its EOF handling so bytes returned together with `io.EOF` are processed rather than silently discarded; this changes no canonical server-emitted bytes.

The import move from `github.com/grafana/ai-sdk/provider/wire` to `github.com/grafana/ai-sdk/gateway/providerwire` is source-breaking. This trade-off is accepted to avoid permanent duplicate ownership and because the approved boundary makes the gateway package the complete remote protocol, not merely its server half.

Alternative: retain a separate leaf codec package and have `gateway/providerwire` import it. Rejected because it keeps one remote protocol split across two public ownership surfaces and prevents codecs from being co-located with the handler that validates and emits them. Alternative: leave an alias/forwarding package or compatibility re-exports at `provider/wire`. Rejected because a shim would preserve the obsolete boundary, create two discoverable public import paths, and weaken the single-owner decision. Alternative: copy rather than move the codecs. Rejected because two implementations could drift.

### D2. Public API and request-aware resolver

The package exposes this conceptual surface:

- `type ModelResolver interface { ResolveLanguageModel(*http.Request, string) (provider.LanguageModel, error) }`
- `type ModelResolverFunc func(*http.Request, string) (provider.LanguageModel, error)`, implementing the interface as a function adapter.
- `func NewHandler(ModelResolver, ...Option) (*Handler, error)`.
- `WithTotalTimeout(time.Duration)`, `WithIdleTimeout(time.Duration)`, and `WithMaxRequestBodyBytes(int64)`.
- `DefaultTotalTimeout = 120*time.Second`, `DefaultIdleTimeout = 60*time.Second`, and `DefaultMaxRequestBodyBytes = 8<<20`.
- exported `ErrTotalTimeout` and `ErrIdleTimeout` sentinels for `errors.Is`-based middleware classification.

`*http.Request` is deliberate: an authenticated host can read identity and tenant values from context or headers without the generic package importing those types. The resolver receives the original request and the trimmed wire model ID only after protocol and body validation. It must not retain the request after returning. Resolver `*provider.APICallError` values pass through; arbitrary errors use the common backend normalization.

`NewHandler` rejects a nil resolver, nil options, and explicitly configured non-positive durations or body limits. Defaults apply only when an option is omitted. This makes invalid configuration visible instead of silently turning it into a host-dependent default.

Alternative: accept only `context.Context`. Rejected because the issue requires request-aware policy and hosts may need request metadata beyond context. Alternative: accept an Assistant catalog. Rejected because catalog membership and tenant policy are host concerns. Alternative: add logger callbacks. Rejected because the host can wrap middleware around the handler and model; a generic observability API is not needed for extraction.

### D3. Validate and decode before host policy or model work

`ServeHTTP` performs the following order:

1. Require `POST`.
2. Validate non-empty model ID, specification version `4`, and streaming header exactly `true` or `false` after trimming.
3. If present, parse `Content-Type` and require `application/json`; validate an optional `Accept` against the selected JSON or SSE response, including exact, `*/*`, and matching type wildcards. Preserve Assistant's current parser exactly: ignore every media-range parameter without interpreting quality values, so a compatible entry still matches with `q=0`, and treat any comma-separated entry whose media type is empty after trimming and parameter stripping as permissive.
4. read at most the configured body size and decode it with the co-located `DecodeCallOptions`.
5. invoke the request-aware resolver.
6. derive a total-timeout context from the request context and invoke `DoGenerate` or `DoStream`.

This preserves Assistant's behavior that malformed traffic never reaches catalog/policy code and that the total timeout bounds the model call rather than authentication, validation, or resolution. As with Go context generally, a model method that ignores cancellation cannot be forcibly interrupted before it returns; streaming after `DoStream` returns is independently selected against the context.

The handler is path-agnostic. Hosts mount it at `providerwire.PathLanguageModel` under any host-owned prefix.

### D4. Unary and streaming commit points

Unary success is encoded before committing headers, then returned as HTTP 200 `application/json`. Model errors and encode failures that occur before commitment are non-2xx `provider.APICallError` JSON responses. In particular, nil or otherwise unencodable results return a retryable HTTP 500 canonical error envelope. A returned `provider.APICallError` is also validated and encoded before commitment; if its status cannot represent a final non-success HTTP response or its raw JSON fields make the preserved envelope unencodable, the handler replaces it with an encodable retryable HTTP 500 internal error. These are intentional corrections of current Assistant implementation bugs where ignored encoding failures can expose an empty implicit HTTP 200 or malformed status values can panic the HTTP server; they are not unchanged-extraction claims.

Streaming does not commit HTTP 200 until `DoStream` returns a non-nil result and channel. Pre-stream model errors and nil streams remain non-2xx JSON errors. Once a stream is available, the handler sets `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-transform`, `Connection: keep-alive`, and `X-Accel-Buffering: no`, writes status 200, and immediately flushes when the writer supports `http.Flusher`. Every event is written and flushed with the co-located `WriteSSEStreamPartTo`.

A clean channel close ends the HTTP body without `[DONE]`. An upstream `PartError` is forwarded unchanged and terminates forwarding. This keeps the same commit boundary consumed by `providers/grafana`.

### D5. One transport error normalization table

Existing `*provider.APICallError` values found through `errors.As` are preserved. Other errors are normalized consistently:

- `ErrIdleTimeout`: 504, retryable, `idle timeout: no stream parts produced within configured window`.
- `ErrTotalTimeout` or `context.DeadlineExceeded`: 504, retryable, `total timeout exceeded`.
- request `context.Canceled`: 499, non-retryable, `consumer disconnected`.
- nil stream, nil resolved model, nil/unencodable unary result, or an API-call error with a nonzero status below 300, equal to the body-forbidden HTTP 304 status, or above 999: 500, retryable, a stable internal-contract message.
- other resolver or model errors: 502, retryable, retaining the error message.
- invalid requests: the specific non-retryable 4xx status from validation.

Before commitment the normalized value is written through the co-located `WriteErrorResponse`. After SSE commitment timeout/cancellation errors are a final `provider.StreamPart{Type: PartError, APICallError: ...}`. Errors already emitted by the model are not categorized again; Grafana gateway category normalization remains client-side.

### D6. Cancellation, total timeout, idle timeout, and write failure

The total-timeout context is passed to both unary and streaming models. For a stream, a cancel-cause child context lets the handler cancel the producer with `ErrIdleTimeout` or a write error. The idle timer starts when the successful stream is committed and resets after each forwarded non-error part. Therefore the initial wait for a part and every inter-part gap use the same idle limit; time spent waiting for `DoStream` itself is governed only by the total context.

If the request is canceled or the total deadline expires after SSE commitment, the handler cancels the model and makes a best-effort final error event. On a real disconnect that event may be unobservable. If canonical SSE encoding or a response-writer write fails, the handler cancels the model promptly and returns without attempting another event on the broken writer. `providerwire.WriteSSEStreamPartTo` uses standard `http.Flusher.Flush()`, which has no error return, so flush failures are not observable through this API; the extraction MUST NOT add alternate SSE framing merely to manufacture flush-error detection. Timer cleanup and channel selection must not leak goroutines owned by the handler.

### D7. Tests move with protocol ownership and avoid module cycles

Move the existing `provider/wire` request, response, error, and SSE tests into `gateway/providerwire` with package declarations updated but assertions and byte expectations unchanged. Root tests under `gateway/providerwire` additionally use hand-written `provider.LanguageModel` and resolver doubles for exhaustive validation, dispatch, timeout, cancellation, flushing, and writer-failure behavior. The root module must not import `providers/grafana`, because that child module already depends on root.

A public client/server test lives in the `providers/grafana` module, or in the already independent `test/conformance` module, where importing both `providers/grafana` and root `gateway/providerwire` is acyclic. It uses `NewWithAccessToken`, an `httptest.Server` mounting the public handler, and a resolver-backed model to cover unary, streaming, and retryable mid-stream errors.

The existing Grafana fixture conformance server should also replace its hand-written provider-wire validation/dispatch with the public handler while retaining an outer host wrapper or mux guard that validates the exact `providerwire.PathLanguageModel` path and authorization header before dispatch, plus its Anthropic replay resolver. The public handler remains path-agnostic; exact-path validation stays host-owned and preserves the existing conformance contract. This keeps the established byte-identical `expected.jsonl` proof and tests the extracted server in the real transparent-transport path.

### D8. Parity and Assistant migration classification

This is a parity-preserving Go adaptation at the provider implementation/internal transport layer except for classified robustness fixes. Nil or unencodable unary results, unencodable API-call error envelopes, and API-call errors with invalid final HTTP statuses now produce a retryable HTTP 500 canonical error envelope instead of potentially exposing an empty implicit HTTP 200 or panicking the HTTP server. The SSE reader now processes a final data line returned together with `io.EOF` instead of silently treating it as clean completion. The source-breaking Go import move is not a wire-parity change: it changes no header, valid payload byte, canonical SSE event, error-envelope shape, or UI message chunk. There is no upstream TypeScript server equivalent. Updated `provider-wire`, `grafana-provider`, and `api-call-error` requirements remain authoritative. The cancellation gap in `test/conformance/PARITY.md` is not closed by unit tests alone and must not be reclassified as upstream conformance.

After ai-sdk publishes the handler, Assistant can separately:

1. keep `auth.go`, JWKS/dev-auth configuration, caller creation, catalog construction, IAM/billing/deployment wiring, and the `/api/v1/aisdk` Gorilla mount;
2. add a `ModelResolverFunc` adapter that reads authenticated caller/tenant data from the request and delegates model selection to its catalog/policy;
3. configure the public handler with current timeout values and wrap it with existing auth/host observability middleware;
4. update host-owned logging/classification to recognize `providerwire.ErrIdleTimeout` and `providerwire.ErrTotalTimeout` through `errors.Is`, or translate those causes to Assistant's existing timeout sentinels before classification;
5. remove duplicated validation, decode, dispatch, SSE, and timeout code only after side-by-side tests preserve existing behavior and explicitly adopt retryable HTTP 500 for nil or unencodable unary results and invalid API-call error envelopes.

The Assistant migration must verify both timeout causes retain its existing `outcome=backend_error` and `error_class=timeout:idle` / `timeout` classifications; leaving the current distinct Assistant sentinels unchanged would misclassify public-handler timeouts. The pre-commit response expectation changes are migration notes, not wire-shape migrations: clients now receive an existing canonical error envelope rather than an empty success or server panic when a unary result or API-call error envelope is invalid.

Rollback in ai-sdk is a normal revert because there is no persisted state or protocol-version migration. Assistant migration can remain on its old implementation until it deliberately adopts the new package.

## Risks / Trade-offs

- [A resolver can retain the request or block without honoring cancellation] → Document its lifetime and require hosts to use `r.Context()`; the generic handler cannot enforce host policy behavior.
- [A model ignores context and exceeds the nominal total timeout before returning] → Document Go context semantics and test prompt cancellation for compliant models; do not add goroutines that could leak around arbitrary provider calls.
- [Timeout/cancellation races with the next stream part] → Select behavior follows Go scheduling; tests assert terminal safety and classification rather than an impossible deterministic winner at the exact boundary.
- [A final 499 event cannot reach a disconnected client] → Treat it as best effort while prioritizing prompt upstream cancellation.
- [Moving codec tests or their package declarations could accidentally change bytes or claim broader upstream parity] → Preserve every assertion and fixture expectation, keep the extraction classified as provider transport, retain the documented cancellation gap, and run baseline validation plus existing Grafana conformance unchanged.
- [Removing `provider/wire` breaks source imports] → Treat the break as intentional, update every live repository consumer in the same implementation, document the new import, and provide no shim that would perpetuate dual ownership.
- [Assistant and ai-sdk land at different times] → Keep the old Assistant server until the public handler is released; no coordinated wire cutover is required.

## Open Questions

None for implementation. Exact unexported file layout and test helper names can follow repository conventions without changing the public or behavioral contract above.
