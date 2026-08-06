## Context

The registered baseline is `@ai-sdk/provider@4.0.4`, `@ai-sdk/gateway@4.0.33`, and `ai@7.0.44`. The stock gateway client sends generate and stream requests to `POST /language-model`, selects the operation with headers, uses LanguageModelV4 JSON, reads stream parts as SSE data events, and treats clean EOF as completion. Upstream does not provide a server implementation for catalog resolution, lifecycle, limits, flushing, or disclosure policy.

The deployed `gateway/providerwire` package intentionally remains tolerant of historical Go wire shapes and delegates serialization to provider-domain JSON methods. Strict behavior therefore belongs beside it rather than in an in-place rewrite.

## Goals and Non-Goals

**Goals:**

- own the pinned V4 wire through private DTOs and explicit conversion;
- compose the strict handler directly with `catalog.ModelResolver` and `provider.LanguageModel`;
- preserve request context, exact requested model ID, ordering, repeatable `PartError` data, flushing, clean EOF, and commit-aware failures;
- own positive total and idle timeouts in the handler without invocation goroutines or stream proxies;
- expose only stable redacted public errors and allowlisted response metadata;
- bound strict request, unary response, Grafana response, and complete SSE event reads/writes;
- preserve legacy deployment and Grafana's explicit strict migration mode.

**Non-goals:**

- a public gateway runtime, failure-classification package, policy engine, middleware chain, trusted identity/metadata, request IDs, call-aware routing, or reusable stream session;
- modeling or honoring `providerOptions.gateway` routing controls;
- raw backend chunk exposure;
- Chat Completions, Responses, Anthropic Messages, discovery, authentication, billing, retries, fallback policy, or a generic SSE framework;
- changing prerequisite provider APIs, legacy provider-wire behavior, or Grafana's default mode.

## Decisions

### 1. Keep two independent provider-wire implementations

The dependency graph is:

```text
gateway/providerwire/v4 -> gateway/catalog -> provider
             |
             +---------------------------> provider

providers/grafana -> gateway/providerwire       (default legacy mode)
providers/grafana -> gateway/providerwire/v4    (explicit strict mode)
```

`gateway/providerwire/v4` is package `providerwirev4`; `v4` names the LanguageModelV4 contract, not a new negotiated transport version. It does not import legacy `gateway/providerwire`. Both handlers retain `/language-model`, so dual deployment uses separate hosts or prefixes.

### 2. Resolve and invoke directly in the handler

`NewHandler` accepts a non-nil `catalog.ModelResolver`. After adapter-local HTTP and DTO validation, the handler derives a positive total-timeout context from the request, calls `ResolveModel` with the exact header value, rejects a nil resolved model as an internal adapter defect, and directly calls `DoGenerate` or `DoStream` once.

The default total timeout is 120 seconds. It covers resolution, synchronous model setup, and stream consumption, but enforcement remains cooperative while a resolver or model call is executing synchronously. The handler does not launch a goroutine to pretend it can interrupt a provider that ignores context.

For established streams, the handler selects directly over the provider channel, total/request context, and a 60-second default idle timer. A new idle timer exists only while waiting for the next provider part, so synchronous response writes are not counted as provider inactivity. There is no proxy channel or public stream session. Write or flush failure cancels the model context and terminates without a second write.

### 3. Keep strict request control fail-closed and small

The handler's private decoder returns `provider.CallOptions`; there is no public decoded-call or gateway-control type. Top-level `providerOptions.gateway` is removed when absent or `{}` and rejected when it contains any key. Nested reserved namespaces remain rejected. This happens before catalog resolution. `includeRawChunks: true` is also rejected before resolution because no current host policy authorizes backend raw exposure; provider-emitted raw parts are never published by the handler.

### 4. Keep safe failure logic private to V4

Small unexported helpers classify only failures the current adapter can produce:

- `catalog.ErrUnknownModel` -> HTTP 404 `model_not_found`, non-retryable, with the requested public model ID;
- provider HTTP 429 -> HTTP 429 `rate_limit_exceeded`, retryable;
- total or deadline expiry -> HTTP 504 `internal_server_error`, retryable;
- request cancellation -> HTTP 499 `internal_server_error`, non-retryable;
- permanent provider dependency -> HTTP 424 `failed_dependency`, non-retryable;
- transient provider dependency -> HTTP 502 `failed_dependency`, retryable;
- adapter defects such as nil models/results or encoding failures -> HTTP 500 `internal_server_error`, non-retryable.

Generic model invocation errors are failed dependencies rather than internal defects. `provider.APICallError` remains privately reachable as a cause while public projection never copies provider URLs, request values, response headers/body, provider data, backend identity, or arbitrary messages. Stream error parts use the same safe projection and remain repeatable data; later parts continue.

The strict decoder continues accepting registered Grafana error types such as `forbidden` even when the simplified handler has no producer for a policy-only category.

### 5. Preserve private DTO ownership and pinned wire semantics

Unexported request, result, stream, content, file-data, tool, usage, metadata, and error DTOs own every polymorphic wire shape. Conversion is field-by-field and does not embed or marshal provider types with transport JSON behavior. Only intrinsically opaque valid JSON crosses as `json.RawMessage`.

Unknown discriminators, missing required or active fields, invalid active-field types, malformed tool JSON, invalid opaque JSON, invalid provider references, and known legacy/private fields fail closed. A canonical discriminator selects its arm, so unrelated additive fields and inactive sibling-arm properties are ignored. Provider-domain values without a trustworthy discriminator still fail encoding when ambiguous or unrepresentable. Valid empty inline-text file data decodes and re-encodes as the same tagged V4 variant.

Strict service output omits provider request bodies, backend response headers/bodies, stream setup metadata, provider identity, and backend `modelId`. Response ID and timestamp, warnings, usage, finish reason, content, and provider metadata remain available where allowed by V4.

### 6. Preserve HTTP, SSE, and limit behavior

The handler retains strict method, header, media type, and quality-aware `Accept` validation. Adapter-owned failures preserve 405, 406, 413, and 415 statuses. Unary data is encoded and checked before HTTP 200 commitment. Stream setup failures remain non-2xx JSON; after commitment lifecycle failures become one best-effort safe error event when the writer remains usable.

Streaming uses `http.ResponseController` for initial and per-event flushes, writes exactly `data: <json>\n\n`, emits no `event:` field or `[DONE]`, preserves part order, and ends on clean provider-channel EOF. It omits `Connection: keep-alive`.

Defaults remain 8 MiB request, 16 MiB encoded unary success, and 8 MiB complete framed SSE event. Encoding may allocate a rejected value. Opt-in strict Grafana retains 16 MiB unary, 1 MiB error, and 8 MiB complete-event read defaults and counts canonical strict events identically. Legacy Grafana continues using its original unbounded readers.

### 7. Preserve migration and evidence

Legacy compile and behavior fixtures remain unchanged. Shared canonical TypeScript and Grafana scenarios run against distinct legacy and strict base URLs. Strict-only evidence covers canonical DTOs, redaction, metadata allowlisting, error continuation, limits, flushing, gateway/raw rejection before resolution, and current safe error mappings. Provider-independent transport tests do not invent provider conformance inputs.

Grafana remains legacy by default and selects strict request/result/stream codecs only through `WithStrictProviderWire()`. It exposes no general mode enum. Cutover changes both codec selection and base URL; rollback restores legacy routing. Flipping the default or deleting legacy/provider JSON ownership is deferred.

## Risks and Trade-offs

- Providers or resolvers that ignore context can block synchronous calls after the deadline; the handler intentionally avoids leak-prone invocation goroutines.
- A blocked response write still depends on host HTTP write deadlines; idle time measures provider waits only.
- HTTP 500 remains retryable to the pinned TypeScript client even when the strict envelope and Grafana classify the internal defect as non-retryable.
- Encoded transport limits prevent partial commitment but can allocate the complete rejected value.
- Treating an otherwise zero exported `DataContent` at the strict file-text boundary as empty inline text is the Go adaptation required to preserve the pinned tagged union without changing prerequisite provider APIs.
- Deployment routing must keep legacy and strict base URLs explicit; there is no negotiation or stream replay.

## Migration Plan

1. Keep legacy behavior frozen and deploy the catalog-backed strict handler under a distinct base URL.
2. Run stock TypeScript and Grafana strict-mode evidence against the strict endpoint while legacy clients remain unchanged.
3. Move controlled Grafana clients by changing both base URL and codec option; rollback restores legacy configuration.
4. Consider making strict canonical and removing legacy/provider JSON ownership only in a separate breaking change after adoption evidence.

## Residual Risks

- Context cooperation and server write deadlines remain provider/host responsibilities.
- The pinned TypeScript client derives retryability from HTTP status rather than `isRetryable`.
- Response encoding is size-checked after allocation.
- Legacy and strict endpoints intentionally coexist until a later coordinated migration.
