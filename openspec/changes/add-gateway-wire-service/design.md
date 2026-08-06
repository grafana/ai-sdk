## Context

The registered transport baseline is `@ai-sdk/provider@4.0.4`, `@ai-sdk/gateway@4.0.33`, and `ai@7.0.44`. The stock gateway client sends both operations to `POST /language-model`, selects generate versus stream with headers, uses LanguageModelV4 JSON, consumes stream parts as SSE data events, and treats clean EOF as completion. It provides no upstream server implementation for routing, authorization, lifecycle, flushing, limits, or error disclosure.

Today `gateway/providerwire` delegates most serialization to custom JSON methods on `provider` domain types. Those methods combine canonical V4 encoding, legacy Go decoding, and normalization of flat unions. The handler also resolves models through an HTTP-aware resolver rather than `gateway/catalog`, so canonical identity and gateway-configured middleware depend on host-written adapters. The current branch has already corrected forwarding after `PartError`, required empty deltas, and several tool-result/file union validation defects; those corrections are part of the compatibility baseline rather than work to repeat.

The existing package and Grafana client are deployed contracts. The rewrite therefore adds a new implementation beside them. Both handlers speak the same external LanguageModelV4 protocol and use the same `/language-model` relative path, so side-by-side deployment requires distinct base-URL prefixes or hosts rather than in-band version negotiation.

## Goals / Non-Goals

**Goals:**

- Give gateway façades that map losslessly to the provider LanguageModelV4 domain one transport-neutral execution runtime with normalized gateway control metadata.
- Apply ordered provider-bound input policy and transformation before model resolution, including inspection of caller headers, provider options, and gateway routing controls.
- Preserve requested, canonical public, and model-reported resolved identities through an invocation without claiming they identify the backend actually selected by wrappers such as fallback.
- Put protocol, gateway request ID, public identities, and host-authenticated attributes into model middleware context before invocation.
- Attach the runtime-configured middleware chain once and enter it once for the requested operation.
- Make strict bidirectional LanguageModelV4 serialization explicit, validated, and independent of all transport-specific `provider` JSON methods.
- Preserve canonical V4 bytes and stream ordering where the current implementation is correct.
- Treat provider `PartError` values as repeatable data while separating runtime termination from provider data.
- Prevent public errors from disclosing backend URLs, request values, response headers/bodies, provider data, or arbitrary internal messages.
- Bound request/client read buffering, enforce encoded-response transport limits before commitment, and make stream flushing observable through `http.ResponseController`.
- Keep backend request bodies and response headers/bodies out of strict public results.
- Keep the legacy Go package and transport usable during an explicit migration with a documented strict-canonical end state.

**Non-Goals:**

- Chat Completions, Responses, Anthropic Messages, embeddings, discovery, or additional public façades.
- A universal execution runtime for native protocol features that cannot be represented by `provider.CallOptions`, `provider.GenerateResult`, and `provider.StreamPart`.
- A new external provider-wire version, automatic protocol negotiation, or retrying a streaming POST.
- A generic SSE abstraction shared across protocols.
- Host authentication, tenant identity derivation, billing, concrete routing/fallback algorithms, or deployment-specific logging.
- Chat-specific streaming state such as choices, assistant-role deltas, accumulated tool arguments, usage chunks, or `[DONE]`.
- Detecting middleware already embedded in a catalog model or constraining middleware-managed retries.
- A bounded streaming JSON encoder; encoded-response limits in this change may allocate the encoded value before rejecting it.
- Removing provider custom JSON methods or legacy decoding while `gateway/providerwire` still depends on them; deprecation and deletion remain follow-up work.

## Decisions

### 1. Add three one-way package boundaries

The new dependency graph is:

```text
gateway/providerwire/v4  ->  gateway/runtime  ->  gateway/catalog
             |                     |
             +-----> gateway/failure <---------+
                          |             |
                      provider     middleware -> provider
```

`gateway/failure` contains typed failure kinds, category sentinels, a derived non-error classification value, classification helpers, and cause-preserving wrapping. `gateway/runtime` contains normalized gateway control types but no `net/http` or façade DTOs. `gateway/providerwire/v4` is declared as package `providerwirev4`; `v4` identifies the pinned LanguageModelV4 contract, not a new revision of the existing Go dialect.

The existing `gateway/providerwire` remains unchanged as the legacy-compatible complete transport. The new package does not import it, and neither new package is imported by `provider` or `gateway/catalog`.

Alternative: replace `gateway/providerwire` in place. Rejected because strict decoding, safe errors, the resolver API, and standards-consistent negotiation intentionally differ from deployed legacy behavior.

Alternative: name the new protocol `v2`. Rejected because clients still send LanguageModel specification version `4`; a transport-v2 label would imply a wire negotiation that does not exist.

### 2. The runtime accepts a normalized gateway call before resolution

Generate and stream calls accept a `GatewayCall` rather than separate model ID and call options. The call contains:

- `Protocol`, a typed protocol identifier;
- `RequestedModelID`, the exact public ID supplied by the caller;
- normalized `provider.CallOptions`;
- `GatewayOptions`, extracted from the `providerOptions.gateway` namespace and removed from provider-bound options;
- trusted `CallMetadata`, including gateway request ID and host-supplied authenticated attributes.

`GatewayOptions` models the full registered gateway control namespace needed before resolution: BYOK credentials, caching/privacy/capability filters, model fallbacks, provider order/allowlists/timeouts, quota/user/tags, service tier/sort intent, and service-owned extensions. Because the pinned type has a string index signature, unknown gateway keys are retained as valid opaque JSON in an extension map; policy/resolution may accept or reject them but codecs never discard them. Credential and attribution values remain private control data. This change defines the representation and consumption seam, not a built-in routing algorithm. A default catalog adapter MUST reject non-empty controls it cannot honor rather than silently passing them to a provider.

`runtime.New` accepts a call-aware resolver plus an ordered set of pre-resolution policies. A compatibility adapter turns an existing `catalog.ModelResolver` into a call-aware resolver for calls without unsupported routing controls. Each policy receives the normalized call and may reject it or return transformed call options, gateway options, or separate policy-derived metadata before resolution; `Protocol`, the original `RequestedModelID`, gateway request ID, and host-authenticated attributes remain immutable. This is where hosts prohibit provider headers such as downstream `Authorization`, reject provider options, and apply request-specific routing rules; host authentication itself remains outside the runtime.

After resolution the runtime preserves immutable identity containing:

- `RequestedModelID`, the immutable public ID originally supplied by the caller;
- `CanonicalModelID`, `catalog.ResolvedModel.ID`;
- `ResolvedProviderID` and `ResolvedModelID`, the values reported by the resolved model before runtime middleware is attached.

The resolved values are model-reported routing identity, not proof of the backend attempt that actually executed. Middleware can override identity, and fallback models report their first candidate independently of the selected attempt. Actual attempt attribution remains the responsibility of provider/fallback observability.

Before attaching model middleware, the runtime enriches the invocation context with protocol, a guaranteed non-empty gateway request ID, original/requested and canonical public model IDs, immutable host-authenticated attributes, and separately identified policy-derived metadata through typed accessors. Construction and accessors defensively copy metadata maps. It never copies caller-controlled headers into trusted metadata. A nil resolved model is an internal failure. The runtime applies its configured `[]middleware.Middleware` once and invokes only the selected `DoGenerate` or `DoStream` entry point once. Runtime outcomes return identity alongside every success or failure occurring after resolution; failures during policy or resolution preserve the safe call identity available at that point.

Alternative: let each protocol adapter call `catalog.ModelResolver` with only a model ID. Rejected because provider-bound policy and routing controls need normalized call data before resolution.

Alternative: pass `providerOptions.gateway` through as an opaque provider option. Rejected because gateway routing policy owns that namespace and providers must not receive it accidentally.

### 3. Streaming uses a runtime-owned invocation session

A stream call returns a deliberately small adapter-facing invocation containing immutable identity, a single-consumer receive-only `Parts` channel, `Wait() error`, and idempotent `Cancel(error)`. After `DoStream` returns a valid stream, an internal proxy forwards provider parts in order and owns the model context. Provider-emitted `PartError` values pass through unchanged as data and never set the invocation error. Provider request bodies and response headers from `provider.StreamResult` are not exposed on the public invocation; a private observability hook can be designed later if a concrete host requires one.

The adapter consumes `Parts` once, calls `Wait` after the channel closes, and may call `Cancel` to stop work after an adapter failure. The public contract does not promise concurrent/repeated `Wait`, concurrent cancellation arbitration, or a reusable stream session before real façades demonstrate a need. Internal synchronization still prevents goroutine/timer leaks and makes repeated cancellation harmless.

The runtime starts the configured total timeout after successful catalog resolution. It passes that context directly to the synchronous `DoStream` call and covers subsequent stream consumption. The provider contract remains cooperative: cancellation makes the context done, but a provider that blocks inside `DoStream` while ignoring context can delay the call's return. The runtime does not spawn an unbounded setup goroutine to pretend it can force such a provider to stop. Runtime-owned forwarding begins only after a valid stream exists and must terminate even if that established provider channel ignores cancellation.

The provider-wire adapter owns its idle timer because idleness is defined while waiting for the next runtime part after the previous event was written and flushed successfully. The timer is paused during synchronous response writes, so a blocked writer is not mislabeled as provider inactivity; server/transport write deadlines remain host-owned. On idle expiry or an observed write/flush failure the adapter cancels the invocation with a cause. Write/flush failure terminates without attempting a second event on the failed writer.

Alternative: encode runtime failures as synthetic provider parts inside the runtime. Rejected because the runtime must remain protocol-neutral and provider parts are model data.

Alternative: let every adapter call a raw `provider.StreamResult` directly. Rejected because total timeout, cancellation cause, and stream cleanup would again be convention rather than a shared guarantee.

### 4. Failure classification is derived and protocol projection stays local

`gateway/failure` defines category sentinels and a typed `Kind` for unauthenticated, invalid request, unknown model, forbidden, rate limited, timeout, canceled, failed dependency, and internal failure. Wrapping uses `errors.Join(category, cause)` so `errors.Is` reaches the category while `errors.As` reaches the private originating error. Deterministic precedence prevents a joined error from changing category based on traversal order.

The package also returns a non-error `Classification` value containing `Kind`, derived `Retryable`, private `Cause`, and a small allowlisted set of safe public parameters. Retryability is computed fresh at the active boundary from explicit classification context and trusted causes; it is not an inherited sentinel. An outer boundary can therefore classify a wrapped retryable cause as non-retryable without an uncleared `errors.Join` marker. `Classification` itself is never returned as an error and never selects an HTTP status or protocol envelope.

The runtime classifies catalog unknown-model errors, context termination, policy failures, and provider call failures where their meaning is known. Backend authentication/not-found errors and otherwise unattributed provider 4xx responses are failed dependencies, not caller authentication, invalid public input, or public catalog misses. Status code alone is not trusted caller attribution. Deterministic internal failures and permanent dependency failures classify as non-retryable; trusted transient provider/transport failures and timeouts classify as retryable.

Protocol adapters map a classification independently. The strict V4 adapter emits permanent failed dependencies as HTTP 424 and retryable failed dependencies as HTTP 502. For example:

```json
{"error":{"message":"upstream dependency failed","type":"failed_dependency","statusCode":424,"isRetryable":false,"param":null}}
```

The pinned TypeScript client uses `message`, `type`, and `param`, but derives retryability from HTTP status rather than the envelope's `isRetryable`: 408, 409, 429, and 5xx are retryable. The split 424/502 mapping therefore preserves permanent/transient failed-dependency behavior for both clients. An internal failure remains HTTP 500; consequently the pinned TypeScript client observes it as retryable even when Grafana receives `isRetryable: false`. This baseline limitation is documented and tested.

Adapter-local protocol failures do not pass through the execution-failure mapper. Method, negotiation, body-size, and media-type failures remain owned by the V4 adapter so it can emit 405, 406, 413, and 415; a future Chat adapter can emit its own OpenAI error `type`, `param`, and `code`. Model-not-found may include only the requested public model ID from allowlisted safe parameters. Stream `PartError` payloads are projected through the same V4 mapper, including safe structured category data for Grafana normalization, before later parts continue normally.

Alternative: serialize `provider.APICallError` directly. Rejected because it can expose backend identity, provider bodies, request data, and headers.

Alternative: carry retryability as an `errors.Join` marker. Rejected because wrapping can add the marker but cannot reliably clear inherited retryability at a more authoritative boundary.

Alternative: introduce a custom gateway error type. Rejected in favor of project conventions using sentinels, wrapped causes, and a separate non-error classification value.

### 5. Strict codecs use wire-native private DTOs to every polymorphic leaf

`gateway/providerwire/v4` defines unexported request, generate-result, stream-part, content, file-data, tool, usage, and error DTOs matching the registered baseline. No DTO embeds a provider type that has a transport-specific JSON method. Conversion is field by field; only intrinsically opaque `json.RawMessage` values such as provider options, provider metadata, schemas, raw chunks, and tool JSON cross unchanged after JSON validity checks.

Encoding and decoding reject unknown discriminators, absent required fields, contradictory union fields, malformed tool input JSON, invalid opaque JSON, and values not representable by the pinned V4 contract. Harmless additive object fields are ignored to reduce upgrade fragility, but unknown union variants fail. The semantic contract is that every valid supported LanguageModelV4 value preserves its meaning, not that every arbitrarily populated flat Go struct round-trips byte-for-byte.

The strict codec is bidirectional: it encodes and decodes canonical requests, generate results, and stream parts through private DTOs while exposing provider-domain conversion entry points needed by the server and Grafana client. Strict decoders accept only canonical V4 shapes. Legacy system arrays, split tool-result outputs, legacy file discriminators, and legacy response/event shapes remain accepted only by `gateway/providerwire`.

During request conversion, the codec extracts the `providerOptions.gateway` object into `runtime.GatewayOptions`, validates registered fields, retains unknown valid JSON keys in an opaque extension map, and removes the namespace from provider-bound `CallOptions.ProviderOptions`. During strict service result conversion, backend request bodies, response headers/bodies, and provider-reported backend identity are omitted from public output. LanguageModelV4 `response.modelId` means the model actually used, so the privacy-safe V4 adapter omits it rather than substituting a catalog alias; only allowlisted response ID/timestamp remain.

Alternative: reuse provider custom marshalers behind wrapper functions. Rejected because nested provider values would silently re-enter the old compatibility codec and make ownership unverifiable.

### 6. The V4 handler is thin and depends on a concrete runtime

`providerwirev4.NewHandler` accepts a non-nil `*runtime.Runtime`, preventing the new public constructor from bypassing call policy, resolution, and runtime middleware. It owns HTTP method/header/media validation, bounded request decoding, mode selection, response commitment, V4 conversion, safe error projection, idle timeout, writes, and flushes. For each valid request it constructs `GatewayCall{Protocol: ProtocolLanguageModelV4, RequestedModelID, CallOptions, GatewayOptions, CallMetadata}`. A host-configured metadata extractor may add immutable authenticated tenant/project attributes after HTTP authentication; body fields and caller provider headers never become trusted metadata. A configurable request-ID generator supplies a non-empty ID whenever the trusted extractor does not.

The handler preserves:

- `POST /language-model`;
- `ai-language-model-id`;
- `ai-language-model-streaming: true|false`;
- `ai-language-model-specification-version: 4`;
- JSON generate responses;
- `data: <json>\n\n` SSE events;
- ordered repeatable error parts;
- clean EOF without `[DONE]`.

`Content-Type` is required to parse as `application/json`. Missing `Accept` is allowed. Supplied `Accept` values use quality-aware matching; a compatible media range with `q=0` does not authorize the representation, and empty list entries do not match. This deliberately does not inherit the legacy permissive parser.

Streaming uses `http.NewResponseController(w).Flush()` initially and after every event. Wrappers can expose flushing through `Unwrap`. `Connection: keep-alive` is omitted; `Cache-Control: no-cache, no-transform` and `X-Accel-Buffering: no` remain. An unsupported or failed flush cancels the invocation and terminates; after commitment it cannot be converted back into JSON.

### 7. Limits are finite, configurable, and explicit about allocation guarantees

The strict handler defaults are:

- request body: 8 MiB;
- encoded unary success: 16 MiB;
- encoded SSE event: 8 MiB;
- streaming idle timeout: 60 seconds.

Positive handler options may override each value. Independently, `runtime.New` defaults its total invocation timeout to 120 seconds and accepts a positive runtime option to override it. Request decoding uses limit-plus-one before unbounded allocation. Unary and event values are encoded and then checked before response commitment or event write; these are encoded-response transport limits, not guarantees that the encoder never allocates the rejected value. A genuinely bounded encoder is deferred until demonstrated necessary.

The SSE limit uses one shared accounting rule on both server and Grafana client: the complete bytes of `data: ` plus canonical JSON plus the terminating `\n\n` are counted. An oversized part becomes a safe post-commit error when that error itself fits; otherwise the stream terminates.

The existing Grafana provider gains positive options with defaults of 16 MiB for unary success, 1 MiB for non-2xx/error bodies, and 8 MiB per complete framed SSE event. Its reader uses bounded incremental buffering and preserves final-line-plus-EOF behavior. Limit violations are non-retryable protocol errors. This is a deliberate security correction: constructors remain source-compatible, but previously accepted oversized responses can now fail and callers can explicitly raise the limits.

Alternative: rely on `http.Client` or server infrastructure limits. Rejected because neither bounds decoded bodies or individual SSE events in the client process.

### 8. Compatibility is proven against both handlers before cutover

Tests first freeze the current handler's external Go API and executable behavior, including public constants/functions/types, permissive request negotiation, legacy error detail, `Connection: keep-alive`, successful canonical calls, tool-result file input, and the already-correct `PartError` to later `PartFinish` sequence. Shared canonical success scenarios run against both handlers. Strict-only tests assert typed safe errors and disclosure prevention; legacy-only tests assert existing detail preservation and tolerant payload acceptance.

The stock TypeScript unary client overwrites server `warnings`, `request`, and `response` with client-owned transport values. Stock-client assertions therefore cover content, finish reason, usage, provider metadata, files, sources, and tools plus the expected client-owned transport fields. Strict codec/raw HTTP and Grafana strict-mode tests prove preservation of public warnings and allowlisted response ID/timestamp while proving omission of provider request bodies, backend headers/bodies, and backend `modelId`. A dual-handler interop harness uses distinct base URLs and handler-specific error expectations.

Grafana receives an explicit strict codec option whose request encoder and unary/SSE response decoders come entirely from `gateway/providerwire/v4`. Its default remains the legacy tolerant codec in this change. Cutover evidence must run Grafana in strict mode against the strict handler so future migration can remove its legacy codec dependency rather than merely relying on legacy tolerance.

Golden tests compare strict codec JSON to the exact registered npm sources and cover every pinned discriminator; Go encode/decode round trips alone are insufficient. Error matrices exercise the public categories and observed retry behavior through both pinned clients. No provider conformance input is invented: provider-independent transport cases use focused tests or `test/interop`, and existing recorded/upstream fixture provenance remains unchanged.

### 9. Future façades must fit the LanguageModel domain explicitly

The runtime is a shared LanguageModel execution layer, not a universal protocol runtime. A future Chat Completions or Responses adapter may reuse it only for features that can be represented without semantic loss by `provider.CallOptions`, `provider.GenerateResult`, and `provider.StreamPart`. The adapter owns protocol DTOs, terminal events such as `[DONE]`, named lifecycle events, and validation. It must reject unsupported native features before catalog resolution rather than burying them in provider options or silently dropping them.

Exact Responses features such as stored/background responses, conversations, continuation identifiers, stateful orchestration, and protocol-specific lifecycle events may require additions to the provider domain or a distinct execution layer. Embeddings and other non-LanguageModel operations are not forced through this runtime.

Speculative mock Chat/Responses DTO tests are not part of this change. They would lock in assumptions without exercising the real state machine. The immediate Chat Completions follow-up is the acceptance test for runtime adequacy and may add only demonstrated execution needs while keeping choice/tool state, finish mapping, usage chunks, and `[DONE]` in the Chat adapter.

## Risks / Trade-offs

- **[The DTO surface can drift from upstream]** → Cover every registered discriminator with canonical JSON goldens, stock-client interop, `mise run parity-check`, and baseline validation; update DTOs with future parity upgrades.
- **[The runtime proxy adds a goroutine and channel boundary]** → Start it only after synchronous `DoStream` returns a valid stream; use bounded or unbuffered forwarding, explicit `Cancel`, and tests for backpressure, request cancellation, provider close, timeout, and established streams that ignore provider contexts.
- **[A provider can block synchronously inside `DoGenerate` or `DoStream` while ignoring context]** → Make context cooperation an explicit provider-contract boundary, avoid leak-prone invocation goroutines, and test that the deadline context is canceled even though the provider must return cooperatively.
- **[The same relative route prevents same-prefix dual mounting]** → Migrate through separate hosts or base-URL prefixes; do not add implicit negotiation or stream retry.
- **[Safe projection reduces remote provider diagnostics]** → Preserve original causes for in-process logging/middleware and expose only stable public categories; hosts own private observability.
- **[Finite Grafana defaults reject very large historical responses]** → Provide named positive options, exact-boundary tests, and release notes describing the security correction.
- **[Encoded-response limits can still allocate the rejected value]** → Name them transport/commit limits, share exact framed-event accounting, and defer a bounded encoder until profiling or production evidence requires one.
- **[Gateway control metadata could mix trusted and caller-controlled values]** → Keep protocol/model/request fields typed, generate a non-empty request ID, populate immutable authenticated attributes only through a host callback after authentication, return defensive copies, and keep policy-derived metadata separate.
- **[Legacy coupling remains globally observable]** → Freeze its public symbols and permissive behavior in external-package and behavioral tests; new code must not import that package or rely on provider JSON methods. Removal is a later coordinated breaking change.
- **[Future façade reuse can become lossy abstraction]** → Name the runtime as LanguageModel-domain execution, require adapters to reject unsupported native features before resolution, and validate it with the real Chat implementation rather than speculative DTO mocks.
- **[Resolved model identity can be mistaken for the backend attempt]** → Use `ResolvedProviderID`/`ResolvedModelID`, document them as model-reported, and leave fallback-attempt attribution to observability hooks.
- **[A blocked response write is not interruptible by the idle timer]** → Pause idle accounting during writes and rely on host HTTP server/transport write deadlines; cancel promptly when a write or flush returns an error.
- **[A provider that ignores context can retain its own goroutine]** → Ensure post-establishment runtime-owned goroutines terminate and document that provider implementations remain responsible for honoring context.

## Migration Plan

1. Freeze legacy behavior, then implement `GatewayCall`, call policy, call-aware resolution, derived failure classification, identity, middleware context, and the minimal runtime lifecycle. Review this public seam with the Chat owner before building protocol adapters.
2. Implement the strict bidirectional request/result/stream codec and V4 handler from the registered upstream source and tests.
3. Add Grafana strict codec mode, complete error normalization, and bounded reads. Keep legacy mode as the default while running the strict Grafana client against the strict handler.
4. Run dual-handler Vercel/Grafana evidence, deploy the strict handler under a separate base URL, and cut controlled clients over. Rollback restores the legacy URL; no stream replay occurs.
5. In the immediate follow-up, implement real Chat Completions against `GatewayCall`, policy, resolver, identity, and the minimal stream session. Keep Chat schemas, HTTP errors, choice/tool state, finish reasons, usage, and `[DONE]` in that adapter; revise the runtime only for demonstrated cross-façade needs.
6. After strict Grafana adoption and usage evidence, make strict V4 canonical, deprecate legacy provider wire and provider JSON wire ownership, then remove them only through a coordinated breaking change.

## Chat Follow-up Feedback Disposition

**Applied in this change:**

- strict V4 becomes bidirectional and Grafana gains an opt-in strict mode;
- the runtime accepts `GatewayCall`, parses gateway control options separately, and runs provider-bound input policy before call-aware resolution;
- middleware receives typed protocol, request, public identity, and host-authenticated context;
- failure category wrapping remains separate from a derived retryability/classification value, while every adapter owns status and envelope mapping;
- the adapter-facing stream session is reduced to identity, ordered parts, terminal wait, and cancellation;
- strict public output omits provider request bodies, backend response headers/bodies, and backend `modelId`; canonical public model identity remains available in runtime context for protocols such as Chat whose public field has that meaning;
- response limits are described as encoded transport/commit limits with identical server/client SSE framing accounting;
- implementation is staged so runtime/control seams are reviewed before V4, Grafana, deployment, and the real Chat follow-up.

**Deferred to follow-up work:**

- flipping Grafana's default to strict and removing its legacy codec dependency after deployment evidence;
- deprecating/removing `gateway/providerwire` and provider custom JSON wire ownership through a coordinated breaking change;
- concrete routing/fallback algorithms, private provider observability hooks, and an allocation-bounded encoder;
- implementation-driven runtime adjustments discovered by the real Chat Completions façade.

**Intentionally excluded from this refactor:**

- Chat-specific request/response DTOs, choice/tool-call state, finish mapping, usage chunking, and `[DONE]`;
- speculative mock Chat/Responses fit tests;
- OpenAI-specific error envelopes or native/stateful Responses orchestration;
- forcing non-LanguageModel operations into this runtime.

## Deferred Work

- Concrete gateway routing/fallback algorithms beyond the call-aware resolver and policy seam.
- Host authentication and derivation of tenant/project identity.
- Private provider request/response observability APIs.
- A bounded streaming encoder that avoids allocating an oversized encoded value.
- Chat Completions and native/stateful Responses behavior.
- Flipping Grafana's default to strict, deprecating/removing `gateway/providerwire`, and removing provider custom JSON wire ownership.

## Open Questions

- Which deployment prefix or host will expose the strict handler during rollout is host-owned and must be selected by the consuming service.
- Which registered `GatewayOptions` fields the first host will honor is deployment policy; unsupported fields must fail rather than be silently ignored.

## Residual Risks

- Host policy remains responsible for gateway-control support and raw/provider-bound data exposure; the catalog adapter fails closed for controls it cannot honor.
- Providers that ignore context may keep synchronous generate or stream setup calls blocked after the runtime deadline, and blocked network writes still depend on host server deadlines.
- The pinned TypeScript client treats HTTP 500 as retryable even when the strict envelope and Grafana client classify the failure as non-retryable.
- Unary and event limits prevent partial commitment but may allocate the complete encoded value before rejection.
- Deployment routing must keep legacy clients on the legacy endpoint and strict clients on the strict endpoint; the service does not negotiate or replay streams automatically.
- Model-reported identity cannot identify the specific backend attempt selected inside a provider-managed fallback sequence.
