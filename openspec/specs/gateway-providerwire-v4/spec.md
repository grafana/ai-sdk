# gateway-providerwire-v4 Specification

## Purpose

Define the strict, executable ProviderWire V4 HTTP and serialized JSON contract for the registered Vercel AI SDK baseline while preserving the active tolerant legacy transport.

## Requirements

### Requirement: Registered protocol authority

The contract SHALL use the source commit and exact package set registered in `test/conformance/upstream.yaml`. Executable evidence SHALL run those package pins, while source inspection SHALL use the registered commit. If a registered commit's workspace version differs from a package pin, parity claims SHALL require path-equivalence evidence to the corresponding release.

Pinned Gateway HTTP behavior and stock-client captures SHALL be the behavioral authority. OpenAPI and JSON Schema SHALL define the local strict projection. Language-specific types SHALL be implementation inputs rather than wire authority.

#### Scenario: Exact packages produce evidence
- **WHEN** the contract interop suite runs
- **THEN** baseline validation SHALL confirm that every imported AI SDK package matches `test/conformance/upstream.yaml`

#### Scenario: Source mismatch is explicit
- **WHEN** an inspected source path belongs to a mismatched workspace package
- **THEN** the contract SHALL prove release equivalence or stop without making a parity claim

#### Scenario: Private server behavior is not inferred
- **WHEN** request emission or response consumption succeeds
- **THEN** the repository SHALL claim pinned-client behavior only, not acceptance by Vercel's private server

### Requirement: Contract-only capability and legacy coexistence

`gateway/providerwire/v4` SHALL contain curated OpenAPI, JSON Schemas, test corpora, response projections, and validation tests. The pinned request capture SHALL be the only regenerated contract fixture and SHALL be identified as such in the interop index and GitHub review metadata.

This phase MUST NOT expose or implement a production decoder, handler, client, resolver, provider adapter, host policy, SSE server, or public wire DTO hierarchy. The existing `gateway/providerwire` package and Grafana transport SHALL remain active with unchanged defaults and behavior.

#### Scenario: No V4 invocation path exists
- **WHEN** production files and exported symbols under `gateway/providerwire/v4` are inspected
- **THEN** `doc.go` SHALL be the only production Go file and no V4 code SHALL invoke a language model

#### Scenario: Legacy behavior remains active
- **WHEN** existing provider-wire, Grafana, and interop tests run
- **THEN** they SHALL continue to use the parent `gateway/providerwire` package

#### Scenario: Contract artifacts are not generated DTOs
- **WHEN** the V4 package is inspected
- **THEN** it SHALL contain no generated or hand-written production request, result, stream-part, or error types

### Requirement: Language-model HTTP envelope

The OpenAPI 3.1 contract SHALL describe only `POST /language-model`. Requests SHALL require `Content-Type: application/json`, `ai-language-model-specification-version: 4`, a non-empty unpadded `ai-language-model-id`, and `ai-language-model-streaming: true` or `false`. Header names SHALL be case-insensitive; routing-header values SHALL be exact and case-sensitive.

Unary success SHALL be HTTP 200 `application/json`. Streaming success SHALL be HTTP 200 `text/event-stream`. Non-2xx responses SHALL use the JSON error schema. Content-type parameters SHALL be allowed. Optional `Accept` values SHALL be syntactically valid and permit the selected representation with positive quality through an exact or type-wildcard range.

Authorization, Gateway protocol, team, observability, custom, and user-agent headers SHALL remain host concerns rather than required language-model routing headers.

#### Scenario: Unary and streaming selection is exact
- **WHEN** a valid request selects streaming `false` or `true`
- **THEN** the response media type SHALL be `application/json` or `text/event-stream`, respectively

#### Scenario: Invalid envelopes fail predictably
- **WHEN** method, path, routing headers, content type, or Accept is invalid
- **THEN** envelope validation SHALL reject it with the corresponding stable category

#### Scenario: Host headers remain optional
- **WHEN** the pinned client emits broader Gateway or observability headers
- **THEN** captures MAY record safe normalized evidence without making those headers contract requirements

### Requirement: Offline exact payload schemas

The repository SHALL check in Draft 2020-12 schemas for shared values, requests, generate results, stream parts, and errors. Each resource SHALL have a stable identifier and SHALL resolve entirely from checked-in local resources. The schemas SHALL be the sole field-level authority.

Standard objects and selected union arms SHALL be closed. Unknown standard fields, inactive union siblings, and typed nulls SHALL fail. Open values SHALL be limited to declared JSON Schema values, opaque tool values, provider-tool arguments, object-valued provider option and metadata namespaces, provider raw values, and declared request or response bodies. `JSONValue` and `JSONObject` SHALL remain distinct.

The request schema SHALL represent post-Gateway semantic JSON: JavaScript `undefined` is absent, explicit empty collections and false or zero values remain present, `abortSignal` is absent, supported inline bytes are base64 strings, and file URLs are strings. Result schemas SHALL preserve required top-level fields, ordered content, undefined omission, and RFC 3339 timestamps.

#### Scenario: Schema graph validates offline
- **WHEN** contract validation runs without network access
- **THEN** OpenAPI SHALL lint and bundle and every JSON Schema SHALL compile with no unresolved external reference

#### Scenario: Selected union arms are exact
- **WHEN** a role, content part, tool, tool choice, response format, file, result, or stream part contains an inactive sibling or unknown field
- **THEN** schema validation SHALL reject the complete object

#### Scenario: Declared opaque values remain semantic JSON
- **WHEN** an allowed opaque value contains nested JSON or an allowed null
- **THEN** validation and comparison SHALL preserve its semantic value without requiring object-member order

#### Scenario: Explicit empty intent survives
- **WHEN** the pinned client emits an explicitly empty collection or object
- **THEN** capture comparison and schema validation SHALL distinguish it from absence

### Requirement: Stream projection and termination

The stream schema SHALL define exact arms for every registered LanguageModelV4 stream part and SHALL require complete JSON values. Each HTTP event SHALL be framed as `data: <JSON>\n\n` without an `event:` discriminator. Normal completion SHALL be response-body EOF after the final event; `[DONE]` SHALL not be required or schema-valid.

H1 SHALL define status, media type, event framing, and EOF only. Server commitment, flush timing, cancellation, timeout, write failure, and post-commit behavior SHALL belong to a later streaming-service capability.

#### Scenario: Every stream arm is covered
- **WHEN** the contract corpus is evaluated
- **THEN** every registered stream discriminator SHALL have positive and negative coverage

#### Scenario: Clean EOF terminates the stream
- **WHEN** the pinned client consumes a complete final event followed by EOF
- **THEN** it SHALL emit that event and complete without a sentinel

#### Scenario: DONE is tolerance evidence only
- **WHEN** a local response projection includes `data: [DONE]`
- **THEN** the pinned client MAY ignore it, but the contract SHALL not classify it as a stream part

#### Scenario: Runtime lifecycle remains deferred
- **WHEN** H1 artifacts are inspected
- **THEN** they SHALL make no claim about server commitment, flushing, cancellation, timeout, or post-commit failures

### Requirement: Safe error projection with explicit retryability

Non-2xx bodies SHALL be closed objects containing `error` and optional string `generationId`. The nested error SHALL require a safe message, integer `statusCode`, boolean `isRetryable`, and exactly one of `authentication_error`, `invalid_request_error`, `rate_limit_exceeded`, `model_not_found`, `internal_server_error`, `failed_dependency`, or `forbidden`.

Only `model_not_found` MAY carry exact `param: {modelId: string}` and only `forbidden` MAY carry exact `param: {ruleId: string}`. Other params, `code`, client-originated error types, backend URLs, credentials, provider identity, backend models, request values, response bodies, and arbitrary diagnostic data SHALL fail. The nested status SHALL equal the HTTP status. Explicit wire retryability SHALL remain distinct from the pinned Gateway client's status-based inference.

#### Scenario: Unsafe fields fail closed
- **WHEN** an error contains an unrecognized type, arm-inappropriate param, code, or backend diagnostic field
- **THEN** the error schema SHALL reject it

#### Scenario: Status correlation is enforced
- **WHEN** nested `statusCode` differs from the represented HTTP status
- **THEN** contract envelope validation SHALL reject the fixture

#### Scenario: Retryability differences remain observable
- **WHEN** local projections represent a retryable 400 and non-retryable 500
- **THEN** the wire values SHALL remain intact while pinned-client status inference is asserted separately

### Requirement: Strict JSON syntax policy

Before schema validation, contract tooling SHALL require exactly one top-level JSON value followed only by whitespace. It SHALL reject duplicate decoded names at every depth, escaped-equivalent names, invalid raw UTF-8, lone surrogate escapes, malformed escapes, truncation, and trailing values. It SHALL accept valid surrogate pairs and preserve the original bytes.

The strict decoder SHALL remain test-only during H1.

#### Scenario: Invalid syntax precedes schema validation
- **WHEN** input contains a duplicate name, invalid encoding, malformed escape, truncation, or trailing value
- **THEN** strict syntax validation SHALL reject it before schema evaluation

#### Scenario: Valid bytes remain unchanged
- **WHEN** a valid JSON value has optional trailing whitespace
- **THEN** syntax validation SHALL return the original bytes unchanged

#### Scenario: Production code cannot use the strict decoder
- **WHEN** production Go files are inspected
- **THEN** none SHALL import the experimental JSON dependency or call the test helper

### Requirement: Reproducible and privacy-safe interop evidence

A deterministic recorder independent of all Go provider-wire handlers SHALL capture direct `doGenerate` and `doStream` requests and orchestration-level `generateText` and `streamText` requests from the pinned packages. Coverage SHALL include unary and streaming selection, message roles, files, tools, tool execution, structured output, explicit empties, provider options, raw intent, and header precedence.

The fixture index SHALL identify regenerated request captures, curated semantic seeds, mutation recipes, derived response variants, reused provider-independent conformance inputs, and independent UI expectations. It SHALL record their authority, source, direction, update command, normalization, claims, and non-claims without duplicating baseline versions. Authorization values, credentials, volatile identifiers, machine-local paths, and provider-recording claims SHALL not be committed.

Response consumption evidence SHALL keep curated unary JSON and clean EOF-terminated SSE seeds. Tolerated `[DONE]` framing SHALL be derived from the clean stream seed, and safe error projections SHALL be selected from named curated positive error cases. Request-capture scenarios SHALL reuse the same response seeds where their behavior does not require a scenario-specific response.

Selected provider-independent conformance stream inputs SHALL be rendered in memory as ProviderWire SSE and consumed by the pinned Gateway client using each fixture's existing orchestration context. Their assembled UI chunks SHALL compare semantically with the existing pinned TypeScript `expected.jsonl` oracle. This lane SHALL be labeled a derived raw transport projection and SHALL NOT be described as provider recording, private-server behavior, host-policy enforcement, or a Go runtime.

#### Scenario: Recorder is implementation-independent
- **WHEN** request evidence is generated
- **THEN** requests SHALL terminate at the local recorder without importing or starting a Go handler

#### Scenario: Normal verification is non-mutating
- **WHEN** the aggregate ProviderWire V4 check runs, even with an ambient update variable
- **THEN** it SHALL recapture and derive evidence without rewriting committed fixtures

#### Scenario: Mechanical variants have one semantic source
- **WHEN** `[DONE]`, safe errors, schema failures, or envelope variants are tested
- **THEN** each variant SHALL identify its curated seed and deterministic derivation or mutation recipe

#### Scenario: Existing UI output remains the oracle
- **WHEN** a selected provider-independent conformance input is transported through ProviderWire SSE and the pinned Gateway client
- **THEN** the resulting UI chunks SHALL match its existing `expected.jsonl` without changing that oracle

#### Scenario: Ownership is explicit
- **WHEN** a reviewer inspects the fixture index or GitHub diff
- **THEN** regenerated captures, curated seeds, mutation recipes, derived projections, and reused conformance evidence SHALL be distinguishable

#### Scenario: Response projections prove consumption only
- **WHEN** the pinned client consumes a local or derived projection
- **THEN** the result SHALL not be described as Vercel server behavior or a live provider recording

### Requirement: Repeatable validation and coordinated evolution

`mise run validate-providerwire-v4-contract` SHALL validate OpenAPI, offline references, strict syntax, HTTP envelopes, schema compilation, positive payloads, mutation-derived negative failures, response seeds, and selected conformance transport inputs. `mise run test-interop-contract` SHALL validate baseline pins, type-check capture tooling, verify request captures, consume seeded and derived response projections, and compare selected transported conformance inputs with existing UI expectations.

`mise run check-providerwire-v4` SHALL aggregate those non-mutating checks. Committed artifact replacement SHALL require `mise run update-providerwire-v4-artifacts`, which SHALL validate generated content before atomically replacing the request capture.

The parity map SHALL classify V4 contract evidence separately from legacy transport. A baseline change SHALL update the manifest, dependency pins, required source-equivalence evidence, schemas, captures, semantic seeds, recipes, parity classification, and lockfiles together.

#### Scenario: Contract validation is one command
- **WHEN** a contributor runs `mise run validate-providerwire-v4-contract`
- **THEN** every machine-readable contract, curated seed, recipe, and selected transport input SHALL be checked without network access

#### Scenario: Interop verification uses baseline pins
- **WHEN** a contributor runs `mise run test-interop-contract`
- **THEN** baseline validation and all non-mutating client evidence SHALL run

#### Scenario: Aggregate verification does not update artifacts
- **WHEN** a contributor runs `mise run check-providerwire-v4`
- **THEN** contract and interop evidence SHALL be checked without changing committed files

#### Scenario: Artifact refresh is explicit and atomic
- **WHEN** a contributor runs `mise run update-providerwire-v4-artifacts`
- **THEN** generated request evidence SHALL be validated before replacing the committed capture atomically

#### Scenario: Legacy parity remains separate
- **WHEN** the parity coverage map is reviewed
- **THEN** V4 SHALL not claim handler, client, provider, Grafana, frontend runtime, private-server, or live-provider behavior

#### Scenario: Baseline drift is atomic
- **WHEN** a registered package or relied-on serialized behavior changes
- **THEN** all affected machine-readable artifacts and evidence SHALL change in the same parity-governed update
