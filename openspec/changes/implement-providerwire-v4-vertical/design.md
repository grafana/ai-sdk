## Context

The registered `@ai-sdk/gateway@4.0.52` client emits a strict LanguageModelV4 request envelope, but successful unary and streaming responses are parsed with permissive `z.any()` handlers. Phase 1 therefore established an independent strict request schema and only smoke-level response consumption probes. Phase 2 made `provider.CallOptions` transport-neutral and presence-correct while preserving the deployed tolerant `gateway/providerwire` dialect.

The current `gateway/providerwire` handler provides useful lifecycle precedent, but it deliberately accepts and emits the legacy dialect, receives `*http.Request` in its resolver, and serializes provider errors. Adding a strict flag would couple incompatible contracts and risk its parent-pinned byte compatibility. The new vertical must instead consume `gateway/catalog.ModelResolver`, explicitly map wire values, and keep backend details outside normalized output.

The exact baseline remains Vercel AI SDK commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/provider-utils@5.0.27`. The installed package sources confirm the minimal unary result shape, the text stream lifecycle, and the recognized public error types. Because the pinned success parser is permissive, local schemas, explicit encoders, goldens, and Go tests remain the response authority.

## Goals / Non-Goals

**Goals:**

- Prove an isolated strict V4 handler from validated HTTP input through policy, canonical catalog resolution, provider invocation, and bounded output.
- Support ordered system, user-text, and assistant-text prompts plus presence-sensitive scalar call settings without using provider JSON as wire authority.
- Define one small protocol-neutral safe failure value and one explicit V4 failure mapping.
- Support bounded unary text and an ordered SSE lifecycle containing stream start, optional normalized response metadata, text blocks, finish, and clean EOF.
- Exercise the handler through deterministic models and the exact pinned Gateway client.
- Keep the request schema and public maintenance commands single-sourced.

**Non-Goals:**

- Full prompt, file, reasoning-content, tool, result, approval, structured-output, provider-option, body-header, raw-part, warning, metadata, or stream-part support.
- Comprehensive timeout, cancellation, partial-write, and post-commit recovery hardening beyond safe bounded behavior required by the vertical.
- Authlib, `/config`, service prefixes, concrete provider construction, Docker, or a Go V4 client.
- Any strict-mode branch, behavior change, or dependency from the tolerant legacy package to V4.
- A generic policy framework, protocol registry, response AST, or shared streaming runtime.

## Decisions

### Isolate strict V4 as a sibling package

`gateway/providerwire/v4` will own private wire DTOs, embedded schemas, strict parsing, mapping, V4 errors, SSE framing, handler lifecycle, and only the relative path/header/MIME constants real hosts and clients require. It will consume `provider` and `gateway/catalog`; legacy `gateway/providerwire` will neither be imported for codecs nor changed to depend on V4.

This preserves the one-dialect rule and legacy byte stability. Adding strict options to the existing handler was rejected because its validation, request representation, resolver, errors, and response privacy rules are intentionally incompatible.

### Use one embedded request schema and separate syntax validation

The normative request schema will move from the test workspace beneath `gateway/providerwire/v4/schema` so production can embed it. The TypeScript evidence workspace will read those same committed bytes; no copied schema remains under `test/providerwire-v4`.

A bounded raw body will first pass `utf8.Valid` and then a duplicate-aware token walk that accepts exactly one JSON value. It rejects invalid raw UTF-8 before Go can replace it with `U+FFFD`, and rejects duplicate members within the same object, comments, trailing commas, invalid numbers, and trailing data. Equal member names in separate object scopes remain valid. Only then will a `UseNumber` semantic value be validated with the existing `santhosh-tekuri/jsonschema/v6` draft-2020-12 tooling and mapped through private DTOs. Schema validation alone was rejected because ordinary JSON decoding collapses duplicate keys and reinterprets invalid UTF-8. A new parser or code-generation dependency was rejected because the standard decoder plus explicit UTF-8 validation, a small scope-aware duplicate tracker, and the existing schema library cover this vertical more directly.

The envelope requires exactly one bare `Content-Type: application/json` value. Parameters are rejected because the exact pinned client emits the bare media type and the claim does not extend to unobserved variants. Phase 5 must make the reusable Go client emit the same bare value rather than weakening this server boundary.

### Validate the complete contract, then gate the Phase 3 subset

Every request must satisfy the complete Phase 1 schema. A separate subset validator will then allow:

- system messages with string content;
- user and assistant messages containing only text parts;
- `maxOutputTokens`, `temperature`, `topP`, `topK`, `presencePenalty`, `frequencyPenalty`, `stopSequences`, `seed`, and `reasoning`;
- absent versus explicit zero and explicit empty values supported by those fields.

Any presence of tools, tool choice, non-text content, response format, raw-chunk selection, body headers, provider options, or message/part provider options will return a safe invalid-request failure before policy or resolution, including explicitly empty unsupported values. Rejecting rather than silently dropping them keeps the vertical truthful and leaves their complete behavior to Phase 4.

### Keep host policy narrow and protocol-neutral

The handler will expose an optional pre-resolution policy interface over `context.Context`, requested public model ID, unary/stream mode, and normalized `provider.CallOptions`. Policy is a trusted internal seam: implementations contractually must not mutate or retain the aliased slices, maps, pointers, or raw JSON in `provider.CallOptions`, and the handler provides no defensive-copy guarantee. The policy may allow the call or return `gateway/failure.Failure`; it does not receive `*http.Request`, V4 DTOs, authlib values, a catalog entry, or concrete service configuration.

The order is fixed: envelope and body bounds, strict syntax, full schema, subset mapping, host policy, one catalog resolution, then one model invocation. Request-scoped caller data can already travel through `context.Context`; future host controls can extend the narrow policy request when Phase 4 extracts them.

### Represent safe failures without raw causes

A new `gateway/failure` package will expose a typed category and an immutable value containing only category, approved safe message, and retryability. Constructors reject unknown categories and empty messages. The value has no cause, provider error, URL, body, headers, provider identity, backend model ID, or arbitrary metadata.

The initial categories are invalid request, authentication, permission, not found, rate limit, overload, failed dependency, upstream failure, timeout, cancellation, and internal failure. Invalid request, authentication, permission, not found, failed dependency, and cancellation are non-retryable; rate limit, overload, upstream failure, timeout, and internal failure are retryable. Protocol or host boundaries reduce richer errors into this value; the shared package does not become an error-classification framework. Because Go permits a zero `Failure` value despite constructors, every renderer validates the value and maps an invalid or zero value to the canonical internal failure.

The V4 encoder uses this complete table:

| Safe category | HTTP status | Public `type` | Public `code` |
| --- | ---: | --- | --- |
| invalid request | 400 | `invalid_request_error` | `invalid_request` |
| authentication | 401 | `authentication_error` | `authentication_error` |
| permission | 403 | `forbidden` | `forbidden` |
| not found | 404 | `model_not_found` | `model_not_found` |
| rate limit | 429 | `rate_limit_exceeded` | `rate_limit_exceeded` |
| overload | 503 | `internal_server_error` | `overloaded` |
| failed dependency | 424 | `failed_dependency` | `failed_dependency` |
| upstream failure | 502 | `internal_server_error` | `upstream_error` |
| timeout | 504 | `internal_server_error` | `timeout` |
| cancellation | 499 | `internal_server_error` | `canceled` |
| internal failure | 500 | `internal_server_error` | `internal_error` |

The recognized Gateway `type` values preserve pinned-client classification; HTTP status preserves retryability because the pinned client derives `isRetryable` from status. The strict envelope is `{"error":{"message", "type", "param":null, "code"}}`; generation IDs are omitted until a host owns them. Structural validation may use audited messages without raw values, while reductions from resolver/provider errors use fixed category messages rather than copying error text. Catalog unknown-model errors reduce to not found and other resolver errors reduce to internal failure unless request cancellation or deadline takes precedence. Provider-call context cancellation and deadlines reduce to cancellation or timeout; provider HTTP 429 reduces to rate limit, HTTP 503 reduces to overload, and HTTP 408/504 reduces to timeout. After those special cases, a retryable `provider.APICallError` reduces to upstream failure and a non-retryable one reduces to failed dependency; other provider failures reduce to upstream failure. This avoids exposing backend authentication, permission, and model-not-found classifications as caller failures without making their permanent failures retryable.

### Make output mapping explicit, bounded, and privacy-preserving

Private response DTOs and hand-authored response schemas will cover only the Phase 3 output families. Runtime encoders will map and validate before writing; goldens and focused tests provide an independent check of their bytes.

Unary output accepts text content, a valid finish reason, and usage. It emits the required `warnings` member as an empty array while rejecting non-empty provider warnings. It strips provider metadata, raw usage, request bodies, response bodies and headers, provider identity, backend model IDs, and unsupported warnings. Its normalized `response.modelId` is always the canonical `ResolvedModel.ID`. A nil, unsupported, unrepresentable, schema-invalid, or oversized result becomes a safe non-2xx failure before success is committed.

A non-nil `StreamResult` with a non-nil channel is the SSE commitment boundary. The handler commits HTTP 200 with `Content-Type: text/event-stream` and `Cache-Control: no-cache, no-transform`, then immediately emits exactly one server-owned `stream-start` with empty warnings. A provider `stream-start` is validated and consumed rather than emitted, and its absence is valid. Non-empty provider start warnings produce a terminal safe error. Empty finish-carried warnings are ignored as a Go representation detail; non-empty finish warnings remain unsupported and produce a terminal safe error. Invalid first channel parts are post-commit SSE failures, not non-2xx responses.

After normalization, streaming accepts optional `response-metadata`, sequential text start/delta/end blocks, and one finish. Response metadata strips provider data and replaces any backend model ID with the canonical public ID. Each complete JSON event is schema-validated, bounded, framed as one `data:` event, and flushed. The server never emits `[DONE]`; a valid finish followed by provider close produces clean EOF. Provider or handler failures after commitment are reduced to at most one bounded `error` part whose closed `error` object contains the same safe message, public type, public code, and null parameter as the non-2xx mapping plus numeric `statusCode` and boolean `retryable`; the stream then ends. Phase 4 will complete the broader stream taxonomy and harden all races and write-failure cases.

### Configure protocol-local bounds and timeouts

Construction will expose positive options for request bytes, unary-response bytes, error-response bytes, complete-event bytes, total model-call timeout, and stream idle timeout. Named defaults retain the legacy 8 MiB request, 120 second total, and 60 second idle values; the new text vertical uses named conservative defaults of 8 MiB unary output, 64 KiB error output, and 1 MiB per SSE event. Request and unary limits need only be positive. The error-response limit must fit the complete canonical JSON fallback, while the event limit must independently fit the larger complete `data: <terminal-error-json>\n\n` fallback frame. Event size always includes the entire SSE frame, not only its JSON payload.

The total timeout begins after policy and resolution and covers provider invocation plus stream consumption. Idle timeout begins only after a stream is established and resets after each successful event. Request cancellation or deadline takes precedence whenever observed during body reading, policy, resolution, or provider execution; resolver cancellation therefore never degrades to internal failure. Pre-commit timeout and cancellation use non-2xx safe errors; post-commit cases use a best-effort bounded safe error event. More exhaustive lifecycle behavior remains Phase 4 scope.

### Use exact-pinned client tests as consumers, not oracles

The integration test server will mount the relative handler beneath a test-owned prefix and use a deterministic catalog with a canonical ID and alias. `test/integration` will add the exact registered Gateway package and call its public `doGenerate` and `doStream` paths directly. The unary client result can prove request mapping, content, finish reason, usage, and consumption. It cannot prove server ownership of `warnings` or `response.modelId` because `@ai-sdk/gateway@4.0.52` replaces both after spreading the server response. Raw HTTP assertions plus Go schemas and goldens therefore prove unary `warnings: []` and canonical `response.modelId`. Streaming client assertions continue to prove canonical identity through `response-metadata`, ordered parts through finish, and clean EOF.

The existing local response probes remain smoke-only. Strict response schemas, golden bytes, state-machine tests, raw response bytes, and error-table tests establish server correctness because the pinned successful-response parser is permissive and ignores `[DONE]`. For `authentication_error`, raw response bytes assert the server-owned message because the pinned client replaces it contextually; pinned-client assertions cover its class, status, and retryability instead. `check-providerwire-v4` will run focused Go runtime tests and the pinned runtime scenario without mutating artifacts; normal integration and parity commands retain broader coverage.

## Risks / Trade-offs

- **[Two-stage JSON processing adds code and CPU]** → Keep the duplicate tracker small, bound the body before both passes, reuse the existing schema compiler, and test nested duplicates and exact-number mapping.
- **[The Phase 3 subset can appear to support full V4 because it validates the full schema]** → Gate unsupported presence explicitly, return safe invalid-request errors before resolution, and retain Phase 4 gaps in the parity map.
- **[Locally authored success schemas can drift from provider V4 types]** → Compare against the exact installed type sources, use typed deterministic fixtures, validate golden bytes, and keep pinned-client consumption as an additional independent signal.
- **[Error messages or provider metadata could leak backend details]** → Use a cause-free safe value, fixed reduction messages, explicit response DTOs, closed schemas, and negative privacy assertions.
- **[SSE errors occur after HTTP commitment]** → Bound and validate each event before writing, emit at most one safe terminal error event when possible, and leave comprehensive transport-failure hardening to Phase 4.
- **[Moving the request schema could create duplicate authorities]** → Move rather than copy it and make evidence and runtime checks compare the same path.
- **[Adding Gateway to the integration workspace creates another parity consumer]** → Pin it exactly to `upstream.yaml`; existing baseline validation and upgrade tooling already cover integration dependencies.

## Migration Plan

1. Move the normative request schema to its production-owned location and update the evidence workspace to consume it without changing generated captures.
2. Add safe failures and the isolated strict package behind tests; do not mount it in any production service.
3. Add deterministic Go and pinned-client integration coverage, then extend the existing V4 check and parity classification.
4. Preserve all legacy golden bytes and interop tests. Rollback removes the new packages/tests and restores the schema path; no deployed route or stored data requires migration.

## Open Questions

None. Full request/response families, raw disclosure, comprehensive lifecycle hardening, and service-owned controls remain explicitly assigned to later phases.
