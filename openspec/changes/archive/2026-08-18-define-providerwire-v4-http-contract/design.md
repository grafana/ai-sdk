## Context

The existing `gateway/providerwire` package is a tolerant Go-oriented transport: it serializes flat `provider` structs, accepts legacy and selected upstream forms, serves requests, and is used by Grafana and the current stock-client interop suite. It remains the deployed rollback path. It is not suitable as the normative strict contract because the Go structs collapse some absent/empty/zero distinctions, represent discriminated unions as sibling fields, and apply custom JSON projections that reflection cannot describe accurately.

The registered compatibility baseline combines source commit `c527d7b3b26287598d2c80e7bce8f16b21653363` with exact npm packages. That commit contains `@ai-sdk/provider@4.0.4`, but its workspace manifests contain Gateway 4.0.30, provider-utils 5.0.14, and ai 7.0.40 rather than the registered Gateway 4.0.33, provider-utils 5.0.16, and ai 7.0.44. The relevant Gateway HTTP/error and provider-utils POST/response/SSE files are byte-identical between the commit and the registered package release tags; provider V4 types match provider 4.0.4. The `ai` orchestration package differs, so executable captures must run the exact installed npm pins.

The pinned Gateway client removes `abortSignal`, base64-encodes supported inline file bytes, retains body `headers`, `providerOptions`, and `includeRawChunks`, combines multiple HTTP header sources, and posts JSON to `/language-model`. It validates successful and failed response payloads with permissive runtime schemas. Captures therefore establish emitted requests and accepted responses, but response strictness must be identified as a curated local serialized projection rather than evidence of private Vercel server acceptance.

## Goals / Non-Goals

**Goals:**

- Establish a language-neutral, executable HTTP and JSON contract for the registered package set.
- Make standardized objects and selected union arms exact while preserving explicitly declared extension boundaries.
- Separate captured stock-client evidence, curated response projections, and locally authored negative fixtures by provenance.
- Resolve strict JSON syntax, OpenAPI validation, schema compilation, and code-generation decisions before production decoding.
- Preserve legacy provider wire behavior while clarifying that it is not the strict V4 contract authority.

**Non-Goals:**

- Add a production request decoder, handler, resolver, model adapter, host policy, stream server, Go client, or Grafana strict mode.
- Change the existing `gateway/providerwire` API, payloads, SSE lifecycle, or Grafana defaults.
- Claim compatibility with Vercel's private server or package versions outside the registered baseline.
- Expose public wire DTOs, generated production types, a generic union codec, or a reusable SSE framework.
- Define authentication, Gateway discovery, routing, credentials, billing, deployment, or provider disclosure policy.

## Decisions

### 1. Split source authority from executable package authority

Exact registered npm pins govern executable capture and consumption behavior. The registered commit remains the source reference, with path-level equivalence evidence recorded where its package manifest version differs. Capture provenance records the package versions, source commit, capture command, and normalized fields. A future baseline upgrade updates the manifest, package pins, source-equivalence record, captures, schemas, corpus, and lockfile together.

This avoids silently treating the commit's older `ai` orchestration as the registered client. Using only release tags was rejected because the repository baseline intentionally records a source commit; using only the commit was rejected because it would execute the wrong package set.

### 2. Keep H1 artifacts contract-only and protocol-local

The new directory contains package documentation, `openapi.yaml`, `schema/request.json`, `schema/generate-result.json`, `schema/stream-part.json`, `schema/error.json`, contract tests, and maintainer evidence. `schema/BOUNDARIES.md` records every standardized object and union arm, required/optional/omitted fields, nullable versus non-null opaque values, keyed map value constraints, explicit-empty behavior, and Go representability. `schema/GENERATION.md` records the bounded generator evaluation. It exports no handler, client, DTO, decoder, or adapter API.

Pinned capture tooling and evidence live under `test/interop/providerwire-v4`. An `INDEX.yaml` distinguishes stock-client request captures, locally authored response-consumption projections, and local negative fixtures. User-facing coexistence and upgrade guidance remains in `docs/` rather than a package README.

### 3. Give each artifact one authority boundary

OpenAPI 3.1 owns the method, path, routing headers, request media type, unary JSON success, streaming SSE success, and JSON error responses. External schema references remain relative and offline-resolvable.

JSON Schema 2020-12 owns serialized payload structure. H1 OpenSpec requirements and focused tests own behavior OpenAPI cannot express accurately: streaming-header response selection, detailed Accept handling, strict JSON syntax, JSON SSE event framing and EOF termination, capture provenance, and the absence of a production implementation. A later streaming-service capability owns server commitment, flush timing, cancellation, timeouts, and post-commit failure behavior.

`@redocly/cli@2.46.1` is pinned as test-only tooling to lint and bundle the complete OpenAPI document with local references and no network resolution. The existing `santhosh-tekuri/jsonschema/v6` dependency compiles every schema through a protocol-local registry that loads all checked-in resources by stable `$id`. Reusing only the existing single-resource schema helper was rejected because it cannot prove the complete reference graph.

### 4. Define a strict pinned HTTP envelope without inheriting legacy quirks

The contract permits only `POST /language-model`. It requires the three language-model routing headers. The model ID must be non-empty and have no leading or trailing whitespace; its remaining value is preserved. Specification version and streaming values are exact `4` and lowercase `true` or `false`. HTTP header names remain case-insensitive as required by HTTP.

Unary and streaming successes use HTTP 200 with `application/json` and `text/event-stream` respectively. Each stream event is exactly `data: <complete stream-part JSON>\n\n` without an `event:` discriminator; clean EOF terminates the protocol and `[DONE]` is tolerated-client evidence only. Server commitment, flushing, cancellation, timeouts, write failures, and post-commit errors remain deferred to the streaming-service phase. `Content-Type` is mandatory and must parse as `application/json`; media-type parameters are allowed. `Accept` may be omitted. When present, a syntactically valid exact or type-wildcard range with positive quality must permit `application/json` for unary or `text/event-stream` for streaming. Empty ranges, malformed values, incompatible values, and `q=0` do not permit a representation. This deliberately does not inherit the legacy handler's omitted content type, empty-entry, or `q=0` behavior.

Configured authorization, protocol, team, observability, custom, and user-agent headers are captured only through a safe allowlist or redacted presence marker. They remain host concerns. Only the three language-model headers belong to the reusable contract.

### 5. Curate schemas from serialized behavior, not language declarations

Every standardized object is closed with `additionalProperties: false` or an equivalently exact full selected arm. Discriminated unions use full-arm `oneOf` definitions with `const` discriminators; inactive siblings fail. Fragile inheritance compositions and schemas reflected from flat Go structs are avoided.

The request schema represents the post-`JSON.stringify` body: `abortSignal` is absent, undefined map values are omitted, supported `Uint8Array` file values are base64 strings, explicit empty arrays/maps remain present, and typed null is invalid unless the field is an opaque JSON value that permits null. It covers every call option, exact message role membership, function/provider tools, tool choice, response format, tool results, approvals, file-data variants, provider options, body headers, reasoning, and raw-chunk intent.

`JSONValue` and `JSONObject` remain distinct. Provider options and metadata are keyed maps whose values are JSON objects, provider-tool args are JSON objects, and provider references are string maps that may be empty but cannot contain a `type` member. Truly opaque values such as tool-call input, tool-result JSON output, provider raw values, and request/response bodies permit the appropriate full JSON value set. Streamed tool-result `result` excludes null even though prompt JSON tool-result output permits it.

The unary and stream schemas model serialized provider V4 fields, including omission caused by JavaScript `undefined`. Date values serialize as RFC 3339 date-time strings. Structural representability of raw parts, provider metadata, request/response bodies, or other sensitive values does not authorize a future public handler to disclose them.

### 6. Use a closed, Gateway-consumable error projection with explicit retryability

Non-2xx bodies use a closed top-level object with nested `error` and optional string `generationId`. The nested object requires a safe `message`, `statusCode`, and `isRetryable`. Its `type` is exactly one of the seven server-response classifications handled by the pinned client: `authentication_error`, `invalid_request_error`, `rate_limit_exceeded`, `model_not_found`, `internal_server_error`, `failed_dependency`, or `forbidden`. Client-originated `response_error` and `timeout_error` are not wire values. Only `model_not_found` may carry optional exact `param: {"modelId": string}`, and only `forbidden` may carry optional exact `param: {"ruleId": string}`; the model ID is the requested public ID, not a backend model ID. All other arms reject `param`. The initial safe projection rejects `code` because no safe server use is required. `statusCode` must equal the HTTP status, and explicit `isRetryable` is authoritative for the future Go client, including retryable 400 and non-retryable 500 projections.

The pinned stock client accepts the added status and retryability fields, validates the remaining nested Gateway shape, and infers its own retryability from HTTP status; response-consumption tests record that distinction. Carrying legacy URL, request values, response headers/body, provider identity, backend model ID, arbitrary backend data, or unrestricted `param`/`code` was rejected as unsafe. This envelope is classified as a local serialized projection extending the permissive pinned client so the later Go client can preserve explicit retryability without defining another wire format.

### 7. Capture requests independently from the Go handler

A deterministic recording server receives exact pinned client calls without importing or launching the legacy Go handler. Scenarios exercise direct `doGenerate`/`doStream` and orchestration-level `generateText`/`streamText`. Each capture stores method/path, normalized allowlisted headers, and parsed semantic JSON separately. Authorization values, volatile user-agent details, and environment observability values are never committed.

The matrix includes unary/streaming, all message roles, text and file content, nested tool-result files, function/provider tools, tool choice, structured response format, provider options and Gateway controls, body headers, raw-chunk intent, opaque null, explicit empty tools/stop sequences/maps, and header precedence/collision behavior. Captures are regenerated to a temporary directory and semantically compared by the normal verification command; an explicit update command is the only way to replace committed captures.

Locally authored responses test unary consumption, EOF-terminated SSE, tolerated `[DONE]`, raw filtering, timestamp conversion, representative non-2xx classifications, and explicit retryability behavior. They are labeled projections, not Vercel server captures or provider recordings.

### 8. Select `jsontext` for strict syntax evidence

H1 pins `github.com/go-json-experiment/json` at `v0.0.0-20260623181947-01eb4420fa68` and uses its `jsontext.Decoder` only in contract tests/tooling. The syntax check reads exactly one raw value, requires the next read to return EOF, and leaves the original bytes unchanged for schema validation. Default behavior rejects duplicate decoded names, escaped-equivalent duplicates, invalid UTF-8, and lone surrogate escapes while accepting valid surrogate pairs and trailing whitespace.

A standard `encoding/json.Decoder.Token` scanner was rejected because it replaces invalid UTF-8 and unpaired surrogates, which can also corrupt duplicate-name detection. Repository-wide JSON experiments were rejected. H2 may promote the narrow wrapper into production after reevaluating the pinned dependency; H1 itself provides no production decoder.

### 9. Bound the generator evaluation and default to deferral

The spike evaluates `github.com/atombender/go-jsonschema@v0.24.1` as the JSON-Schema-native candidate and `github.com/oapi-codegen/oapi-codegen/v2@v2.8.0` as the OpenAPI-oriented candidate. A standalone difficult schema fragment covers role/tool/file unions, absent versus empty values, optional booleans, non-null and nullable opaque JSON, keyed object maps, and exact inactive-arm rejection.

The report records commands, versions, deterministic clean regeneration, compilation, semantic round trips, presence preservation, discriminator behavior, raw JSON behavior, and whether manual edits are required. Generated output stays in temporary or ignored evaluation paths and is deleted after the run. Production generation remains deferred unless a candidate passes every gate; even a passing result does not add generated production types in H1.

### 10. Make validation and parity claims precise

`mise run validate-providerwire-v4-contract` validates the complete OpenAPI document and local references, compiles every schema as Draft 2020-12, validates all positive payloads, and checks each negative fixture against an expected structured schema/syntax category and instance path. `mise run test-interop-contract` verifies captures with exact package pins and runs stock-client response-consumption scenarios. Capture updates use a separate explicit task.

The parity map gains a ProviderWire V4 HTTP contract row. H1 claims exact pinned request emission, curated payload validation, and pinned response consumption only. It does not claim a V4 runtime, private-server acceptance, byte-canonical JSON, or provider recording provenance. Existing legacy interop rows remain unchanged.

## Risks / Trade-offs

- **Closed schemas require coordinated updates when upstream adds fields without changing specification version 4** → Treat every baseline movement as a governed upgrade of pins, captures, boundary ledger, schemas, corpus, and lockfiles.
- **The registered commit does not contain every registered package version** → Preserve explicit source-equivalence evidence and execute only exact npm pins.
- **Pinned clients permissively validate responses** → Maintain comprehensive curated response fixtures and label them local serialized projections.
- **The strict JSON dependency is pre-v1** → Pin one revision, isolate its use, avoid production exposure in H1, and reevaluate promotion in H2.
- **OpenAPI cannot express all content negotiation or SSE framing rules** → Keep H1 syntax, selection, framing, and EOF semantics in OpenSpec/tests, and assign server commitment/flushing/lifecycle to the later streaming-service capability.
- **Curated schemas can diverge from Go representability** → Complete the boundary ledger first and stop if a required distinction cannot be preserved by a future approved adapter.
- **Capture fixtures can leak credentials or volatile values** → Record only an allowlist, use synthetic credentials/headers, and scan committed fixtures for secrets and machine-local data.
- **Generator evaluation can become open-ended** → Limit it to two pinned candidates, one representative corpus, one durable report, and no generated production code.

## Migration Plan

This phase is additive. The legacy provider wire, server, Grafana path, and existing interop suite remain active and unchanged. Contract assets can be removed without runtime rollback because no production code depends on them yet. Before implementation begins, recheck the prerequisite provider behavior and registered baseline; stop and replan if either changed. At completion, validate and synchronize the new capability and legacy clarifications, archive the change, and confirm zero active changes before any unary-service phase begins.

## Open Questions

None are intentionally deferred. Any disagreement between exact pinned captures, registered source, curated schemas, or future Go representability is a stop condition requiring explicit classification rather than an adapter workaround.
