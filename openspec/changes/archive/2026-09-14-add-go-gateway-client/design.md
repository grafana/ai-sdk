## Context

Work package 5 leaves an authenticated, runnable Gateway at `/api/v1/aisdk/config` and `/api/v1/aisdk/language-model`. Its ProviderWire V4 server is AGPL-3.0-only under `ai-gateway/`; the reusable Go SDK and clients remain Apache-2.0 and may not import or require that module. The previous `providers/grafana` module was intentionally deleted with the tolerant Go-to-Go wire. Git history is evidence for ergonomics and Grafana authentication only, not a codec to restore.

The registered compatibility baseline is `@ai-sdk/gateway@4.0.52` and `@ai-sdk/provider@4.0.7` in `test/conformance/upstream.yaml`. Existing captures prove its POST route, header composition, JSON presence rules and binary conversion. Consumption probes prove permissive unary parsing followed by replacement of `request`, `response`, and `warnings`; seven Gateway error classes; stream raw filtering; RFC 3339 timestamp conversion; tolerance of `[DONE]`; and clean EOF. The Go client must express those observations through `provider.LanguageModel` without claiming that Go shapes are TypeScript shapes or using the server validator as an oracle.

The client is immediately useful for text/scalar calls. Server support for tools, files, structured output, provider options, body-carried call headers, and raw output arrives in later work packages. WP7 must encode the current Go request model accurately enough that the strict server—not a client-maintained rollout table—owns capability rejection. Its initial response mapper is closed to the text response families WP5 can emit, plus bounded raw-part decoding needed to prove the pinned client's filtering behavior; later capability packages extend it deliberately.

## Goals / Non-Goals

**Goals:**

- Restore one registry-compatible `providers/grafana` module with cloud token exchange, pre-minted access-token, acting-user, base-URL, configured-header, HTTP-client, and bounded-read controls.
- Make supported Go text calls semantically equivalent to calls through the exact registered Vercel Gateway client.
- Decode discovery, unary, error, and text SSE responses into existing Apache provider-domain values with explicit validation and bounded work.
- Preserve cancellation from token exchange through request transport, SSE reads, and blocked result delivery.
- Establish differential and authenticated real-command evidence that future client extensions can reuse.

**Non-Goals:**

- A public `providerwire` transport package, shared server/client DTO package, restored legacy codec, or compatibility mode.
- An SDK import of `ai-gateway`, use of AGPL implementation code, or client-side calls to server schemas/validators.
- WP6 image/capacity behavior, WP8 telemetry/provider controls, WP9 fallback selection/attempt attribution, or WP10 deployment and migration smoke.
- Retrying requests inside the provider. Existing SDK retry/fallback orchestration consumes the returned error retryability; the client does not replay a call itself.
- Adding tools, files, structured output, provider options, or other post-text result families ahead of their owning work packages.

## Decisions

### 1. A separate Apache module is the sole public client

Create `providers/grafana` with module path `github.com/grafana/ai-sdk/providers/grafana`. `Provider` implements `registry.Provider`; returned models implement `provider.LanguageModel` and report specification `v4`, provider `grafana`, the requested public model ID, and no client-side URL support claims.

All request DTOs, response DTOs, error mapping, and SSE parsing remain unexported. Independent black-box tests outside the module may launch the Gateway command over HTTP, but client production code and `go.mod` cannot depend on `github.com/grafana/ai-sdk/ai-gateway`. This keeps the dependency direction Gateway → released SDK and prevents a second low-level client API.

Alternatives rejected: restoring `gateway/providerwire` recreates the retired Go-to-Go protocol; placing the client under `ai-gateway` makes it AGPL-coupled and unusable by normal SDK consumers; sharing server DTOs makes the implementation, rather than the pinned client, the compatibility authority.

### 2. Preserve proven constructors while centralizing client policy

Restore `NewWithCloudAuth(CloudAuthConfig)` and `NewWithAccessToken(AccessTokenConfig)`. Both configurations carry the Gateway base URL, optional `*http.Client`, optional configured outer headers, and optional validated client limits. Cloud configuration additionally carries CAP token, token-exchange URL, namespace, and audience (default `ai-sdk`); direct access-token configuration carries a short-lived token whose refresh remains the caller's responsibility. An unexported token source presents one context-aware `Token(ctx)` operation to the model.

`WithUserIDToken(ctx, token)` remains the explicit acting-user adaptation. Empty credentials, namespaces, audiences, model IDs, invalid URLs, unsafe/non-positive limits, nil option functions, and header names/values that cannot form a safe HTTP request fail before network I/O. Authlib owns CAP exchange and caching. A single injected HTTP client is used for exchange and Gateway calls unless the constructor explicitly separates those transports in a future change.

Alternatives rejected: a generic callback-only auth API discards useful domain validation; using `Authorization` would not authenticate WP5; minting or caching tokens locally duplicates authlib; environment-discovered defaults make credentials and endpoints non-auditable.

### 3. Base URL and header precedence are explicit

The configured base URL denotes the ProviderWire API prefix, normally `https://host/api/v1/aisdk`; it is normalized once, may include a path, and must not contain user info, query, or fragment. The client appends exactly `/config` or `/language-model` without allowing an absolute path to discard the prefix.

For model calls, configured headers are applied first, then `CallOptions.Headers`, then client-owned `Content-Type`/`Accept`, Grafana `X-Access-Token`, optional `X-Grafana-Id`, and the three `ai-language-model-*` headers. Client-owned fields therefore cannot be overridden. `CallOptions.Headers` remain in the JSON body as well as participating in outer composition, matching the registered client even while WP5 rejects non-empty body headers. Discovery carries only configured headers plus Grafana auth headers.

Alternatives rejected: treating call headers as outer-only changes body presence; accepting caller overrides of protocol/auth headers creates ambiguous or unsafe requests; `path.Join` on raw strings can rewrite escaped paths unexpectedly.

### 4. Explicit request projection follows the registered client, not server support

Use private client-side wire DTOs and explicit conversion from `provider.CallOptions`. The serializer preserves nil/absent versus explicit zero/false/empty collection where the Go type can express it, encodes selected binary data as base64, URLs as strings, opaque provider JSON without recursive interpretation, and carries call headers both in body and outer composition. It omits no field merely because WP5 cannot execute it. Invalid provider-domain discriminators, conflicting selected arms, non-finite values, invalid UTF-8, invalid JSON payloads, and values that cannot be represented in registered ProviderWire fail locally before authentication or HTTP.

The known parity-preserving Go adaptation remains explicit: zero-valued `ReasoningProviderDefault` serializes as omission because `provider.CallOptions` cannot distinguish it from absent; non-zero reasoning values use registered strings. Additions to `provider.CallOptions` fail a compile-time field witness, while additions or changes to Go's open finite string constants fail an executable, mutation-tested AST inventory until classified. This avoids silently inheriting behavior from generic `json.Marshal` while letting the server retain capability gating.

Alternatives rejected: marshaling `provider.CallOptions` directly can silently drift on Go tags or future fields; importing server request types crosses the license boundary; rejecting all post-text request families in the client would make the client a second rollout authority and needlessly diverge from Vercel emission.

### 5. Unary results are closed, bounded, and normalized client-side

On HTTP 2xx, require JSON media type, read at most the configured unary bytes plus one sentinel byte, require one complete JSON document, and map only the registered text content, finish reason, and usage fields currently emitted by WP5. Reject unknown discriminators, malformed required fields, unsafe token counts, trailing data, and oversized bodies.

As the pinned client does, ignore server-supplied `request`, `response`, and `warnings`. Return `Request.Body` from the locally encoded semantic request, `Response.Headers` from the HTTP response, `Response.Body` from the bounded received document, and an explicit empty warning slice. Do not adopt server model/provider identity or private metadata from unary JSON.

Alternatives rejected: unmarshalling directly into `provider.GenerateResult` accepts legacy fields and makes provider JSON tags wire authority; retaining server metadata differs from the pinned client and could expose physical topology.

### 6. Streaming uses one bounded reader and one cancellation-aware owner

`DoStream` completes authentication and HTTP setup synchronously. Non-2xx responses are mapped before a stream exists. A 2xx response must have SSE media type; then a single goroutine owns the body, incrementally reads within package limits (total bytes, complete event bytes, event count), parses `data:` events, maps the closed WP5 text stream family, and closes the result channel exactly once. It never buffers the whole response or allocates an unbounded line.

The client ignores exact `[DONE]`, filters `raw` unless `IncludeRawChunks` is true, converts valid response-metadata timestamps to `time.Time`, preserves required empty deltas and ordered error parts, forwards finish before closing, and treats transport clean EOF as clean EOF even if no finish was observed because the registered client does. Unsupported/malformed/oversized events become one terminal `PartError` carrying a non-retryable protocol `APICallError`, then close. Context cancellation stops reads and blocked sends, closes the body, and silently ends the channel; the caller's context remains the cancellation authority.

`StreamResult.Request` is the locally encoded request and `StreamResult.Response` contains response headers. No body, private service metadata, or hidden low-level stream handle is exported.

Alternatives rejected: `bufio.Scanner` with its default or merely enlarged token size obscures complete-event limits; a slice of all events is unbounded; treating missing finish as a protocol error differs from registered `[DONE]`/EOF behavior; internal automatic retries risk replay after visible output.

### 7. Gateway HTTP errors use a closed Go classification with the provider cause retained

Read non-2xx bodies through the configured error limit, validate the registered error envelope, and map the seven pinned categories: authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server. Preserve public code/status/message and status-derived retryability. Expose one idiomatic `GatewayError` with a typed category and `Unwrap()` to a bounded `*provider.APICallError`, so `errors.As` supports both Gateway classification and existing SDK retry behavior.

Malformed, oversized, wrong-media-type, or transport failures become locally worded `APICallError` values without copying arbitrary response bytes into the primary message. Context cancellation/deadline errors remain recognizable through `errors.Is`; non-context network failures are retryable. Stream `error` events are mapped to `provider.StreamPart{Type: PartError, APICallError: ...}` because the Go provider contract carries errors as parts; this is a parity-preserving Go adaptation to the TypeScript stream's error value.

Alternatives rejected: restoring legacy provider-error inference from arbitrary nested bodies would accept a second error dialect; strings alone lose retryability; class-per-category Go types add API surface without improving `errors.As` or typed switching.

### 8. Discovery is a public projection, not an AGPL catalog dependency

Expose `ListModels(ctx) ([]ModelInfo, error)` on `Provider`. `ModelInfo` contains only public ID, name, optional description, and the registered specification triple. The client reads `/config` within the discovery limit, validates required public fields and `v4`/`grafana` identity, rejects duplicate IDs and malformed rows, ignores unrecognized additive JSON members accepted by the pinned client, and returns server order. Aliases are ordinary rows exactly as served; the client does not infer canonical/backend mappings.

Alternatives rejected: importing `ai-gateway/catalog.ModelInfo` violates the boundary; returning arbitrary maps leaks schema discipline to callers; client-side catalog caching creates freshness and authorization questions outside WP7.

### 9. Exact-pinned differential evidence is scenario-based

Extend the existing `ai-gateway/test/providerwire-v4` workspace rather than adding another npm baseline. A test-only Go capture executable and the registered Vercel client make equivalent calls, then compare semantic method, path, effective protocol/custom headers, request-body presence, unary normalization, stream parts, error category/retryability, cancellation, discovery, `[DONE]`, and EOF behavior. The comparison ignores language-runtime-only representation differences after classifying them.

Client-module unit tests own hostile HTTP/SSE inputs and every configured boundary. A black-box repository test builds/spawns the WP5 command and proves both auth constructors where deterministic, discovery, unary, stream, acting user, cancellation, and privacy over HTTP. These tests may refer to the Gateway process but no Apache production source may import it.

Alternatives rejected: server-validator round trips alone can let client and server share the same bug; duplicating Vercel expectations by hand creates a second oracle; provider conformance fixtures are inappropriate because fake Gateway traffic is not real-provider provenance.

## Risks / Trade-offs

- **Explicit mapping is substantial and must evolve with the provider contract** → add a compile-time `CallOptions` field witness, an exhaustive executable finite-discriminator growth guard, and classify every baseline difference in `PARITY.md`; later capability PRs extend closed output families.
- **The client can encode requests the text-only server rejects** → preserve exact request semantics and surface the server's typed invalid-request response; document current executable capability separately from transport representation.
- **Restored authlib increases the client module graph** → isolate it in the separate module and prove root builds with `GOWORK=off` and the client/Gateway directories absent.
- **A malicious endpoint can stream forever or send pathological framing** → enforce total bytes, event bytes, event count, HTTP-client/context timeouts, incremental parsing, and cancellation-aware sends; do not invent a client wall-clock deadline that overrides caller context.
- **Go cannot perfectly mirror JavaScript object-presence semantics** → test all representable zero/false/empty/nil cases and document the reasoning-default adaptation; never claim parity for unrepresentable calls.
- **Error bodies can contain private service details** → accept only registered public fields into `GatewayError`, bound retained `APICallError` data, and keep arbitrary malformed bytes out of user-facing messages.
- **WP7 could absorb later roadmap concerns** → keep observability controls, fallback selection, image limits, deployment, migration, and rollout tests in WP8/WP9/WP6/WP10 respectively.
