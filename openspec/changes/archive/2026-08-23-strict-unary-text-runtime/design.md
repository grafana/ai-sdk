## Context

Work package 2 established the complete request schema and executable request/consumption evidence for `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. The registered client posts the serialized `LanguageModelV4CallOptions` projection to `/language-model`, uses three protocol headers to carry version, model identity, and mode, and accepts unary JSON permissively. It also replaces server `request`, `response`, and `warnings` fields after parsing, so successful client consumption cannot prove server privacy, identity, or output completeness.

The Go repository currently has the complete request schema under `gateway/providerwire/v4`, transport-neutral catalog resolution under `gateway/catalog`, and provider-domain V4 models. It has no ProviderWire V4 handler or response authority. `provider.CallOptions.Reasoning` is still pointer-valued even though absence and provider-default are operationally equivalent; this complicates strict mapping and is inconsistent with the plan's provider-domain presence rules.

This change crosses HTTP processing, schema validation, provider-domain mapping, catalog resolution, provider implementations, error normalization, response encoding, parity evidence, and TypeScript integration. It is security-sensitive because untrusted JSON and provider-controlled output cross a public protocol boundary.

## Goals / Non-Goals

**Goals:**

- Execute the supported unary text subset through one strict, ordered, bounded production handler.
- Distinguish malformed input from complete-schema-valid but unsupported capabilities.
- Preserve exact supported prompt order, required empty text, scalar presence, and canonical public identity.
- Prevent provider and transport internals from crossing unary success or error responses.
- Make response schemas, raw HTTP tests, and bounded encoding authoritative independently of permissive client behavior.
- Normalize reasoning effort to a value enum whose zero value is provider default without changing provider behavior.
- Provide replay and cross-language evidence against the exact registered package baseline.

**Non-Goals:**

- Streaming setup, SSE commitment, stream state, event framing, timeouts, or drain behavior.
- Authentication, discovery, service mount prefixes, process configuration, or concrete provider construction.
- Go client or `providers/grafana` support.
- Executing tools, files, reasoning/custom content, provider options, body-carried provider headers, structured output, raw output, or non-text generated content.
- Restoring the retired tolerant transport or using provider-domain JSON methods as HTTP authority.
- Claiming compatibility with Vercel's private Gateway service.

## Decisions

### Build one immutable handler from explicit dependencies and limits

`gateway/providerwire/v4` will expose a constructor around an immutable handler. Configuration will contain a `catalog.ModelResolver`, an optional policy interface, and named unary limits. The constructor will embed and compile request, unary-success, and error schemas once and validate `limit+1` arithmetic for every byte limit. Only the error-response limit must contain fallback bytes: construction proves the canonical internal-error document fits before serving traffic. The unary-success limit has no fallback document because an oversized success transitions to the separately bounded error path. A nil policy becomes an internal no-op implementation so runtime sequencing remains uniform.

The package will own only relative ProviderWire routes and protocol behavior. It will not import service auth, concrete providers, environment configuration, or deployment code.

Alternative: package-level defaults and a directly constructible handler. Rejected because resource safety would depend on hidden values and invalid fallback limits could fail after traffic starts.

### Use a fixed request pipeline with stage-local errors

The handler will run these stages in order:

1. exact method, relative path, JSON media type, and protocol-header validation;
2. bounded body read, close, and UTF-8 validation;
3. complete request-schema validation;
4. explicit supported/unsupported mapping;
5. host policy;
6. exact-once catalog resolution and result validation;
7. exact-once `DoGenerate` under request cancellation and total timeout;
8. explicit response mapping, bounded encoding, schema validation, and commitment.

Each stage returns a private categorized failure. Tests will use recording policy, resolver, and model implementations to prove that later stages are not reached after earlier failures.

Alternative: decode directly into `provider.CallOptions` and validate afterward. Rejected because it loses field-presence distinctions and cannot distinguish malformed registered unions from valid unsupported branches.

### Use bounded standard JSON processing before explicit mapping

The handler bounds the complete request body and rejects invalid UTF-8 before passing the raw bytes to the existing complete-schema validator. Supported SDK clients serialize structured values through standard JSON encoders, so a second protocol-local JSON parser would add substantial maintenance and parser-divergence risk without improving compatibility. Malformed syntax and trailing values remain safe schema-instance decoding failures.

Shared `schema.CompiledSchema.Validate` remains unchanged and may reject a numerically unrepresentable value while decoding the schema instance. The ProviderWire runtime does not pass `json.Number` to the schema library. For requests that pass schema validation, private ProviderWire DTOs retain scalar fields as raw JSON lexemes and the mapper uses checked `strconv.ParseInt` and `strconv.ParseFloat`; it rejects non-canonical integer syntax, integer range errors, and non-finite floating-point results. No requirement forces a value such as `1e309` to reach the mapper: it may fail safely during schema-instance decoding.

Alternative: add a custom scanner for duplicate members, surrogate pairing, depth, token count, and number length. Rejected because supported clients do not emit malformed lexical JSON, the request byte limit bounds parser input, and the standard Go/schema decoding path fails safely. Alternative: use `json.Decoder.UseNumber` for shared or protocol schema values. Rejected because shared validation behavior remains unchanged and the protocol mapper already preserves only the scalar lexemes whose exact syntax affects behavior.

### Decode schema-valid values through private raw DTOs and explicit switches

After schema validation, private DTOs will retain `json.RawMessage` for nested union objects and values whose support decision depends on discriminators or presence. The mapper will explicitly switch over every known role and discriminator. Supported text branches map to provider constructors and typed values; known later branches return `UnsupportedCapability` with stable capability names.

No generated support registry or schema introspection will decide runtime support. Empty tools, provider-options maps whose namespace objects are all empty, an exactly empty `{}` headers map, disabled raw chunks, and text response format normalize to no-op. A headers map containing any member is non-empty even when that member's value is `""`, and activates the deferred body-header capability. Any activated deferred branch fails before policy or resolution. The mapper uses a fixed traversal and capability-priority order so a request activating multiple deferred branches reports one deterministic first capability. Focused one-capability cases, not a multi-capability golden, prove each branch independently. This preserves a complete contract while keeping the executable subset reviewable.

Alternative: define a text-only schema. Rejected because it would collapse valid future V4 input into malformed input and make phase-by-phase capability additions harder to reason about.

### Preserve only meaningful provider-domain presence

Optional integer and continuous scalar settings remain pointers because explicit zero can affect provider behavior. Required text remains a string and is never dropped when empty. Stop-sequence omission and an empty list normalize to no stop sequence.

`provider.CallOptions.Reasoning` changes from `*ReasoningEffort` to `ReasoningEffort`. `ReasoningProviderDefault` becomes the empty-string zero value. The wire mapper converts both omitted reasoning and explicit `"provider-default"` to that value; explicit operational levels remain distinct. Root option/config structs may remain pointer-valued where they need to represent “option not supplied” during merge, but they will assign a value at the provider call boundary. Anthropic, Bedrock, OpenAI Responses, OpenAI-compatible, middleware, conformance, and tests will be updated to treat the zero value as the existing no-op.

Alternative: preserve the pointer solely to distinguish wire omission. Rejected because the provider domain has no behavioral distinction to preserve, while the private wire DTO can validate and normalize presence explicitly.

### Keep policy and catalog separate and ordered

The handler policy receives only a successfully mapped request and returns approved provider call options or a categorized safe failure. It cannot infer support from raw JSON. Catalog resolution receives the exact untrimmed requested header value after policy succeeds. The handler validates a non-empty canonical ID, non-nil model, and V4 specification before invocation. It does not mutate the resolved model or derive public identity from `ModelID()`.

This phase's mapper removes semantically empty host-owned values and rejects activated deferred values, so the default policy is intentionally inert. The interface exists now to lock the required mapping-policy-resolution order for later service work.

Alternative: resolve before policy. Rejected because future authentication and host controls must reject requests without revealing catalog behavior or constructing provider effects.

### Bound model duration independently of provider cooperation

The handler will derive a child context from the HTTP request and configured total duration, invoke `DoGenerate` in a single model-call goroutine, and select among result, caller cancellation, and deadline. The goroutine will recover every panic before it can escape the `net/http` recovery boundary and convert it to an internal model result without formatting the recovered value publicly. A `nil, nil` model return is also an internal failure. The result channel will be buffered so a provider that returns after the handler exits does not block on delivery, including a late return after cancellation or timeout.

Cancellation will win when already observable before timeout; otherwise the selected terminal condition determines the safe category. A non-nil model result received before those conditions is authoritative. Providers remain required to observe context. A provider that blocks forever cannot hold the HTTP handler open, but it can retain one model-call goroutine forever; this phase bounds request latency, not retained resources owned by a non-compliant provider.

Alternative: call `DoGenerate` synchronously and rely entirely on providers. Rejected because the configured total duration would not bound handler latency for a defective model. Alternative: rely on `net/http` panic recovery. Rejected because the model call executes in a child goroutine outside that recovery scope.

### Normalize failures through a closed safe-error layer

A private safe-error value will carry only a category and, where applicable, a stable unsupported-capability identifier. Every raw error document has exactly the nested shape `{"error":{"message":"...","type":"...","param":null,"code":"..."}}`; no retryability member is serialized. Fixed retryability is represented by the HTTP status and matches the registered client's status-based `GatewayError.isRetryable` calculation. The authoritative category table is:

| Category | HTTP | `error.type` | `error.code` | `param` | Retryable | Pinned client class |
| --- | ---: | --- | --- | --- | --- | --- |
| invalid request | 400 | `invalid_request_error` | `invalid_request` | `null` | no | `GatewayInvalidRequestError` |
| authentication | 401 | `authentication_error` | `authentication_error` | `null` | no | `GatewayAuthenticationError` |
| permission | 403 | `forbidden` | `forbidden` | `null` | no | `GatewayForbiddenError` |
| not found | 404 | `model_not_found` | `model_not_found` | `null` | no | `GatewayModelNotFoundError` |
| rate limit | 429 | `rate_limit_exceeded` | `rate_limit_exceeded` | `null` | yes | `GatewayRateLimitError` |
| overload | 503 | `internal_server_error` | `overloaded` | `null` | yes | `GatewayInternalServerError` |
| failed dependency | 424 | `failed_dependency` | `failed_dependency` | `null` | no | `GatewayFailedDependencyError` |
| upstream failure | 502 | `internal_server_error` | `upstream_error` | `null` | yes | `GatewayInternalServerError` |
| timeout | 504 | `internal_server_error` | `timeout` | `null` | yes | `GatewayInternalServerError` |
| cancellation | 499 | `internal_server_error` | `canceled` | `null` | no | `GatewayInternalServerError` |
| internal failure | 500 | `internal_server_error` | `internal_error` | `null` | yes | `GatewayInternalServerError` |

The pinned client recognizes only the seven type values used above. Overload, upstream failure, timeout, and cancellation intentionally use `internal_server_error` for stable client classification while their code and status preserve the strict dialect category. Messages come from one closed category table, except invalid request may use the closed `unsupported capability: <typed-capability>` pattern. Arbitrary causes remain available only for internal observability and are never formatted into output.

Failure reduction first checks the request-derived context chain: errors matching `context.Canceled` become cancellation and errors matching `context.DeadlineExceeded` become timeout. Remaining `APICallError` values map by status: 408 or 504 becomes timeout; 429 becomes rate limit; 503 or 529 becomes overload; 401, 403, 404, and every other 4xx become failed dependency; status zero becomes upstream failure; every other 5xx becomes upstream failure; and any remaining status becomes upstream failure. For remaining non-`APICallError` values, a timeout-capable `net.Error` or `*url.Error` whose `Timeout()` is true becomes timeout; any other error identifiable through `net.Error` or `*url.Error` becomes upstream failure; and only the remaining arbitrary non-transport errors become internal failure. A panic or `nil, nil` model return becomes internal failure. Catalog unknown-model errors map to public model-not-found.

The error encoder will validate its closed DTO. If an attempted safe message, encoding, schema, or bound check fails, it will emit prevalidated canonical internal-error bytes.

Alternative: expose `APICallError` fields. Rejected because those values may contain credentials, URLs, bodies, headers, backend identity, provider metadata, and unsafe messages.

### Use private unary DTOs and a deliberately narrower response dialect

The unary DTO will contain ordered text content, finish reason, normalized usage, generic warnings, and response metadata. It will not reuse `provider.GenerateResult.MarshalJSON`. Warning switches will emit exactly the registered fields for all four variants. Because `provider.Warning` uses non-pointer strings, required `feature`, `setting`, and `message` keys are always emitted for their known variants even when the value is empty; missing and explicitly empty cannot be distinguished and are both valid registered strings. Only an unknown warning discriminator fails provider-domain mapping. Usage values will be checked for non-negativity and JavaScript-safe integer range; raw usage is dropped. Only text content is accepted in this phase.

Response metadata always exists and always sets `modelId` from `catalog.ResolvedModel.ID`. Provider response ID and timestamp may survive; provider name, backend model ID, headers, body, request data, provider metadata, and content-part metadata do not. The unary schema will be closed around this strict normalized dialect even though the registered client accepts a wider arbitrary object.

Alternative: serialize the provider result and delete known private fields. Rejected because future provider fields or custom marshalers could silently become public protocol.

### Incrementally encode complete bounded documents before writing headers

Success and error DTOs will use protocol-local JSON writers that append incrementally into a bounded buffer, including incremental string escaping, rather than `json.Marshal` on provider-controlled values. Encoding stops immediately at `limit+1`. Only bounded complete bytes are passed to the compiled response schema. The handler commits status and headers only after encoding and validation succeed.

An oversized success therefore becomes a bounded safe error. A malformed or oversized ordinary error becomes the fixed canonical internal error. This provides a real precommit boundary and avoids making a full oversized encoded copy.

Alternative: `json.Marshal` followed by a length check. Rejected because it allocates the complete encoded response before enforcing the configured boundary. Alternative: stream HTTP 200 while encoding. Rejected because later mapping, size, or schema failures could no longer change status safely.

### Combine Go authority with registered-client compatibility evidence

Focused Go tests will own envelope, bounded UTF-8 body processing, schema, mapper, sequencing, error, DTO, privacy, and boundary assertions. The committed phase 2 goldens will be replayed without modifying them according to an explicit stage matrix:

| Golden record | Expected stage/result |
| --- | --- |
| `streaming.json` | reject at unary envelope because streaming is `true` |
| `sequence.json` record 1 | supported unary text execution |
| `sequence.json` record 2 | reject at unary envelope because streaming is `true` |
| `scalar-presence.json` | schema succeeds; mapper reports `body-headers` first because a header member with value `""` is still present |
| `headers.json` records 1 and 2 | schema succeeds; mapper reports `body-headers` first |
| `comprehensive-unions.json` | schema succeeds; fixed traversal reports message-level `provider-options` first |

A dedicated supported scalar request, separate from the committed scalar golden, will prove exact scalar presence and execution. Focused authored requests will activate one deferred capability at a time and prove every unsupported branch independently. No single multi-capability request is expected to report more than its deterministic first capability. No provider fixture inputs will be invented or placed under provider conformance directories.

A deterministic cross-language scenario will run the pinned Gateway client against the real Go handler. It will prove request emission and successful consumption, plus representative non-2xx classification. Because the client overwrites unary response metadata and warnings, raw Go HTTP tests will independently assert server warnings, canonical model identity, response schema, and privacy.

The parity map will move strict Go envelope/body/schema decoding, unsupported mapping, and unary runtime evidence from deferred to automated/mixed while retaining the client-consumption evidence boundary.

Alternative: rely only on the TypeScript client test. Rejected because its permissive schema and overwrite behavior cannot establish server correctness.

## Risks / Trade-offs

- [Schema validation rejects malformed JSON or a numerically unrepresentable value before mapping] → Keep one byte-bounded UTF-8 body, leave shared schema decoding unchanged, preserve behavior-relevant scalar lexemes in private DTOs, and accept safe schema-stage rejection when ordinary decoding cannot represent a value.
- [Unsupported-capability names drift as features land] → Centralize typed capability constants and remove one switch branch and its tests with each implementing work package.
- [Reasoning value migration changes provider behavior] → Update every provider and middleware call site together, add zero/provider-default equivalence tests, and run provider request snapshots plus full module tests.
- [Provider-controlled warning strings contain sensitive material] → Emit only registered warning variants/fields, omit all metadata and error material, and keep privacy regression cases with hostile provider structs; later policy may further restrict warning content without widening the DTO.
- [A provider panics or ignores context forever] → Recover inside the model-call goroutine, validate nil results, and bound HTTP latency with a single buffered result handoff. A forever-blocked provider can still retain that goroutine, so record it as a provider lifecycle defect; do not claim retained-resource bounds, add retries, or wait unboundedly.
- [A strict response is accepted by raw tests but masked by the client] → Validate closed schemas before commitment and retain raw HTTP assertions as authority.
- [Error type behavior differs across registered Gateway classes] → Keep public codes/statuses authoritative, use recognized client types where available, and pin non-2xx TypeScript probes to the registered package.
- [The large phase introduces too many simultaneous invariants] → Implement in testable layers: domain migration, body/schema pipeline, mapper sequencing, safe errors, response mapping, then integration.

## Migration Plan

1. Change the reasoning provider-domain field and all root/provider/middleware call sites in one buildable step, preserving request snapshots.
2. Add embedded response/error schemas, immutable handler configuration, and bounded body/schema processing without model invocation.
3. Add explicit text mapping, policy sequencing, catalog resolution, and recording-model tests.
4. Add safe provider/model failure normalization and bounded unary response commitment.
5. Replay committed request goldens, add raw boundary/privacy tests, and add pinned-client integration.
6. Update parity coverage and run baseline validation, ProviderWire checks, provider conformance, integration, and every Go module test.

Rollback removes the new V4 handler and schemas and restores the reasoning pointer migration together. No deployed service or client depends on this phase yet, and the phase 2 contract workspace remains valid independently.

## Open Questions

None. Streaming lifecycle, service authentication/policy, approved body headers, provider options, and additional request/response families remain explicitly assigned to later work packages.
