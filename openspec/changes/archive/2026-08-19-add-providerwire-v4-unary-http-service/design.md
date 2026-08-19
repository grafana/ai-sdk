## Context

`gateway/providerwire/v4` currently contains only the checked-in OpenAPI 3.1 envelope, Draft 2020-12 payload schemas, strict syntax corpus, pinned request captures, and response-consumption evidence. The active `gateway/providerwire` package remains a tolerant Go-oriented unary and streaming transport used by Grafana. This change adds the first V4 production path without changing that legacy package.

The registered source commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` contains `@ai-sdk/gateway@4.0.52` and `@ai-sdk/provider-utils@5.0.27` at the exact versions registered in `test/conformance/upstream.yaml`, so those source paths and the installed exact packages can support the unary parity claim. The pinned Gateway client sends post-serialization LanguageModelV4 call options to `POST /language-model`, accepts an arbitrary JSON generate result, and interprets non-2xx Gateway error categories while inferring retryability from HTTP status. The checked-in schemas remain stricter than that permissive consumer and are the local serialized authority.

The Go `provider` domain can represent the required unary values, but its flat structs and tolerant JSON methods are not strict wire DTOs. Important differences include system strings versus content slices, tagged file data, split tool-result values, typed provider options, required empty arrays, and model-returned metadata that must not be disclosed. The implementation therefore needs private adapters rather than direct unmarshalling into or marshaling from `provider` values.

The only provenance-valid unary provider fixture currently registered is the pinned Bedrock `json-tool-with-answer` fixture. H2 treats that as an explicit bounded evidence surface; broader provider coverage requires additional recorded or pinned upstream unary inputs, not synthetic provider payloads.

## Goals / Non-Goals

**Goals:**

- Serve strict unary requests from the pinned stock Gateway client through one resolved `provider.LanguageModel`.
- Guarantee envelope, byte limit, syntax, schema, extraction, policy, adaptation, and resource checks occur before resolution.
- Preserve accepted LanguageModelV4 semantics, including explicit empty collections, tagged file data, opaque JSON, and provider options.
- Keep host-owned Gateway controls, request headers, raw-chunk intent, backend diagnostics, raw usage, and provider metadata away from provider or public response boundaries as applicable.
- Encode and validate a complete bounded unary result or bounded safe error before HTTP commitment.
- Prove direct `doGenerate`, orchestration-level `generateText`, raw unary transport, policy-normalized runtime behavior, status/retry behavior, and legacy coexistence at the registered baseline.
- Expose only a small handler, resolver, option, constant, and timeout-error surface; keep wire values and policy implementation private.

**Non-Goals:**

- Streaming negotiation beyond safe rejection, `DoStream`, SSE, idle timeout, post-commit terminal events, or stream cancellation behavior.
- A reusable Go client, Grafana V4 adoption, or any change to legacy provider wire defaults or bytes.
- Authentication, credentials, catalog/discovery, routing, fallback, billing, accounting, deployment, or observability middleware.
- Support for Gateway routing controls, caller-supplied provider headers, or raw-chunk disclosure.
- Public DTOs/codecs, a generic policy engine, generic union decoder, generated production types, or a generic HTTP runtime.
- A claim about Vercel's private server or providers without provenance-valid unary fixtures.

## Decisions

### 1. Keep a sibling strict handler with a deliberately small public surface

`gateway/providerwire/v4` will export `Handler`, `NewHandler`, a request-aware `ModelResolver` plus function adapter, positive-valued functional options, and route/header/version/media constants that hosts or later clients need. The handler requires the exact `/language-model` path and is mounted directly or behind host prefix stripping. It imports only the standard library, `provider`, the existing schema validator, and the selected strict JSON package.

The initial options configure total timeout, maximum raw request bytes, maximum aggregate decoded inline-file bytes, maximum unary response bytes, and maximum error response bytes. Defaults are named constants: 120 seconds, 8 MiB, 8 MiB, 8 MiB, and 16 KiB respectively. Construction rejects a nil or typed-nil resolver, nil options, non-positive values, and a configuration too small to encode the fixed fallback error.

Reusing or wrapping the legacy handler was rejected because its envelope, decoding, error, and disclosure behavior is intentionally tolerant and conflicts with the strict contract. A shared generic handler framework was rejected because H2 needs only one unary protocol path and legacy behavior must remain isolated.

### 2. Promote the proven strict syntax mechanism and compile embedded schemas once

The existing pinned `jsontext.Decoder` wrapper will move from test-only code into a focused production syntax unit. It reads exactly one value, requires EOF after trailing whitespace, rejects duplicate decoded names and invalid Unicode/UTF-8, and returns the original bytes. The pre-v1 dependency is already pinned and its corpus proves behavior that `encoding/json` cannot preserve; replacing it with a custom scanner would add more protocol-critical code without evidence.

All five checked-in schemas will be embedded into the package and loaded into one protocol-local Draft 2020-12 compiler registry. Compilation occurs once and the immutable compiled schemas are shared concurrently. `NewHandler` fails if the embedded graph cannot compile. Contract tests use the same production syntax and registry boundaries rather than parallel implementations.

Generating structs or schemas was rejected because reflection over the flat Go types cannot express exact selected arms or presence rules. Loading schema files from the working directory was rejected because the production handler must work in a built binary.

### 3. Make request processing a fixed, test-visible pipeline

The handler executes these gates in order:

1. validate method, exact path, required routing headers, unary selection, required JSON content type, and H1 Accept semantics;
2. read at most the configured body limit plus one byte and close the body;
3. apply strict syntax validation to the original bytes;
4. validate semantic JSON against the embedded request schema;
5. decode into private exact wire values;
6. extract top-level Gateway controls, body headers, raw-chunk intent, and nested reserved namespaces;
7. apply the fixed H2 request policy;
8. adapt accepted values to `provider.CallOptions` while bounding and accounting decoded resource usage;
9. start the configured operation timeout, resolve once, and invoke `DoGenerate` once.

Every failure through step 8 is a non-retryable request error and bypasses the resolver. Streaming `true` is syntactically valid but unsupported by this phase, so it fails as an invalid request before body conversion or resolution. The original request context governs all stages; after policy acceptance the handler derives the operation-timeout context, passes a request carrying that context to the resolver, and passes the same context to `DoGenerate`. Host HTTP server settings remain responsible for transport-level header/read/write deadlines.

Direct decoding into `provider.CallOptions` was rejected because tolerant custom unmarshallers, flat union fields, and `omitempty` semantics would make schema validation and policy extraction easier to bypass or accidentally reorder.

### 4. Use private exact wire values and explicit adapters

Private wire types mirror only the validated request and unary-result projections. Optional scalar fields use pointers; absent arrays/maps remain nil while explicit empty arrays/maps remain non-nil. Union decoding selects a private exact arm only after schema validation. Opaque values stay as copied `json.RawMessage` and provider option namespaces become `provider.RawProviderOption` values only after host controls are removed.

The request adapter explicitly handles system text, role-specific content, tagged data/reference/text/URL variants, tool-result `value` arms, tools, tool choice, response format, and provider options. Base64 inline data is decoded with aggregate decoded-byte accounting before resolution. Integer-designated `maxOutputTokens`, `topK`, and `seed` values must be integral and within Go `int` range. Other standard numbers must remain finite, must not underflow a non-zero value to zero, and must survive canonical float64 decimal round-tripping; ordinary decimal values such as `0.1` remain accepted. Numeric lexeme and exponent work is bounded before arbitrary-precision conversion. Values outside those provider-domain bounds fail non-retryably before resolution and are never rounded or truncated. Optional absent versus explicit false or empty-string distinctions that the Go provider domain does not expose are canonicalized to the same provider value as a parity-preserving Go adaptation; private wire values still retain presence until adaptation.

The response adapter performs the inverse projection without using the legacy encoder. It selects the declared arm from each `provider.GenerateContentPart`, requires representable required values, preserves ordered content and safe public fields, makes required `content` and `warnings` arrays non-nil, and produces the H1 generate-result shape. Unknown or unrepresentable provider result arms fail before commitment.

### 5. Apply one fixed secure request policy before provider adaptation

H2 has an internal protocol-owned policy function, not a public pluggable policy interface. It applies these rules:

- remove an empty top-level `providerOptions.gateway` object and reject one containing any member;
- reject a reserved `gateway` object key nested anywhere in another provider option namespace;
- accept absent or explicitly empty body `headers`, remove them, and also remove the exact pinned orchestration marker `{"user-agent":"ai/7.0.65"}`; reject every other non-empty body header map;
- reject `includeRawChunks: true`;
- reject integer-designated values outside Go `int`, standard floats that overflow, underflow non-zero to zero, or fail canonical decimal round-tripping, and numeric lexemes or exponents outside bounded adaptation work;
- enforce the aggregate decoded inline-file limit;
- preserve all other schema-valid provider namespaces as opaque raw objects.

Rejecting unsupported call-option headers was selected over silently dropping them because it makes unsupported client intent observable and prevents callers from believing provider-bound authentication or routing headers were honored. Exact source inspection showed that pinned `ai.generateText` always adds `user-agent: ai/7.0.65` to call options before invoking the Gateway model. H2 therefore recognizes only that exact generated marker, removes it before adaptation, and keeps every caller-controlled or additional body header rejected. A configurable control/header allowlist was rejected as premature credential and routing policy.

The pinned stock Gateway client relies on platform fetch, whose default request carries `Accept: */*`. H1 therefore treats a syntactically valid positive-quality full wildcard as permitting the selected response representation, alongside exact and type-wildcard ranges. More specific quality-zero exclusions do not override another positive matching range; any malformed member still rejects the envelope.

Outer HTTP authorization, protocol, team, observability, and custom headers remain available to host wrappers and the request-aware resolver but are neither required by nor forwarded through `provider.CallOptions` by the V4 handler.

### 6. Bound resolution and unary invocation with one operation timeout

After request and policy acceptance, one `context.WithTimeoutCause` context covers resolver execution and the single `DoGenerate` call. A package sentinel identifies total-timeout expiry. Request cancellation is preserved as a distinct cause. The resolver receives the request and the validated public model ID, is invoked at most once, and may return a model or error; a nil model is an internal failure. The model is invoked exactly once and `DoStream` is never called.

Starting the timeout before untrusted body parsing was rejected because request bytes and decoded resources already have explicit limits while network read deadlines belong to the host server. Starting it after resolution was rejected because resolver work would otherwise be unbounded by the handler's advertised total timeout.

### 7. Enforce disclosure while constructing the response projection

The fixed unary response policy removes:

- top-level and per-content `providerMetadata`;
- `usage.raw`;
- request metadata and request bodies;
- response headers and response bodies;
- provider identity and backend model ID.

A response ID and non-zero timestamp may remain; they are protocol fields that do not identify the provider or backend model. Ordered content, standardized finish reason, standardized token counts, warnings, safe source/file values, tool calls/results, and other contract fields remain when representable.

The policy is applied while building the private result projection so prohibited fields never enter the public wire value. The projection is encoded into a bounded pre-commit buffer, validated against `generate-result.json`, and written only after encoding, schema validation, and the configured response limit succeed. Invalid, nil, unencodable, or oversized model results become safe non-2xx errors; no partial success body is committed.

Preserving all upstream-structurally-valid metadata was rejected because structural representability is not disclosure authorization. Removing the entire response object was rejected because safe response ID/timestamp fields can be useful and are independently representable.

### 8. Normalize all failures into the closed safe error projection

The handler never serializes `provider.APICallError` directly. It may inspect an error with `errors.As` for status and explicit retryability, but drops URL, request values, response headers/body, provider data, cause text, provider identity, and backend model details. Stable messages and the seven H1 categories are selected from the failure stage and safe status:

- envelope, syntax, schema, policy, and request-size failures use `invalid_request_error` with their appropriate 4xx status and `isRetryable: false`;
- resolver model absence may use `model_not_found` and the requested public model ID;
- resolver authorization/policy failures may use the matching authentication or forbidden category without backend details;
- provider 429 uses `rate_limit_exceeded`; other provider 4xx failures use `invalid_request_error`; provider 5xx and arbitrary provider failures use `failed_dependency`;
- timeout uses retryable 504 `failed_dependency`; consumer cancellation uses non-retryable 499 `failed_dependency` when a response can still be written;
- nil models, internal adapter defects, and fallback encoding failures use `internal_server_error`.

A preserved explicit `APICallError.IsRetryable` remains the local wire value when the status is usable; pinned stock Gateway behavior is asserted separately because it infers retryability from status. Invalid statuses normalize to 500. Safe schema instance paths may appear in validation messages, but prompt values, invalid fragments, control values, and validator diagnostics may not.

Every error is encoded, status-correlated, schema-validated, and size-checked before commitment. If a detailed safe error cannot fit, a fixed minimal 500 error is used; handler construction guarantees that fallback fits. A response-writer failure after commitment is returned from the write path without attempting a second response.

### 9. Keep raw transport and policy-runtime conformance independent

The TypeScript conformance tooling will process the selected provenance-valid unary fixture through the exact pinned provider package and observe the full raw `LanguageModelV4GenerateResult`. A non-mutating check validates that raw value against H1, serves it to the pinned Gateway client, and compares the consumed semantic result with the existing `expected-generate.json` outcome. This is the raw transport lane and makes no host-policy or Go-runtime claim.

A small TypeScript policy projector, specified by the H2 disclosure rules and independent of Go runtime code, removes prohibited fields and produces the policy-normalized ProviderWire expectation needed by the Go-only runtime test. The committed expectation records source fixture, baseline authority, policy profile, and generation command. Provider `input.response.json` remains byte-unchanged and retains its existing upstream provenance.

The conformance Go module will run the real Bedrock provider against its replay input behind the real V4 handler, then compare the handler's decoded JSON with the independently generated normalized expectation. Separately, the existing cross-language interop test server will expose V4 unary scenarios so exact pinned `doGenerate` and `generateText` calls exercise the real Go handler, including safe 429/5xx errors and pre-resolution policy rejection. The legacy interop route remains unchanged.

Using the Go handler to generate expected values was rejected because it would make the implementation its own oracle. Inventing additional unary provider fixtures was rejected because it cannot expand provider-boundary parity.

### 10. Evolve validation and documentation without broadening claims

`mise run check-providerwire-v4` will remain non-mutating and expand to cover contract validation, focused V4 runtime tests, exact-package unary interop, and the unary raw/policy checks. Artifact regeneration remains explicit and validates staged outputs before replacement. The parity map will move the V4 row from contract-only to strict unary runtime coverage while retaining explicit gaps for streaming, Go client, Grafana adoption, frontend runtime, private-server behavior, and providers without authentic unary inputs.

The provider-wire server guide will distinguish the active legacy complete transport from the V4 strict unary-only handler, show where host authentication and routing wrappers belong, and warn that streaming requests are intentionally rejected until the streaming phase.

## Risks / Trade-offs

- **The pre-v1 strict JSON dependency becomes production code** → Keep one pinned revision behind a tiny wrapper and retain the full syntax corpus; replacing it requires corpus-equivalent evidence.
- **Private adapters duplicate the wire inventory** → Keep them unexported and schema-driven, test every union arm, and fail on unrepresentable values rather than adding a public DTO hierarchy.
- **Schema validation and pre-commit buffering add unary CPU and memory cost** → Compile schemas once, cap request/decoded/output bytes, and accept this cost for a strict public boundary.
- **Rejecting body headers and Gateway controls narrows stock-client inputs** → Remove only the exact pinned orchestration user-agent marker needed by `generateText`, return stable non-retryable errors for every other non-empty map, and add individual typed controls only through later explicit requirements.
- **Metadata removal changes observable raw provider results** → Maintain separate raw and policy-normalized oracles and compare the real handler only with the independently normalized expectation.
- **Only Bedrock has a provenance-valid unary fixture** → State the provider-boundary coverage gap and add providers only through real recordings or matching pinned upstream fixtures.
- **Safe error normalization loses backend detail** → Preserve detail only in in-process causes for host observability outside the response; never expose it on the public wire.
- **A write can fail after headers are committed** → Precompute the entire bounded body and avoid any second write; network write recovery remains a host/HTTP concern.

## Migration Plan

This change is additive. Hosts may construct and mount the V4 handler separately, but Grafana and all existing callers remain on `gateway/providerwire`. Rollback consists of unmounting the V4 route or reverting this package; no data migration or legacy protocol change is involved. Before merge, the implementation will update and verify the V4 contract/runtime evidence, synchronize the modified capability specs, archive the change, and confirm no active OpenSpec change remains.

## Open Questions

None. A contract mismatch, unrepresentable provider value, source/package equivalence failure, need for service/auth dependencies, or need to weaken the fixed policy is a stop condition requiring a new decision rather than an implementation workaround.
