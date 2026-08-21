# gateway-providerwire-server Specification

## Purpose

Define the public provider-wire HTTP server contract and reusable handler behavior.
## Requirements
### Requirement: Public complete provider-wire package and dependency boundary

The repository SHALL provide a public `github.com/grafana/ai-sdk/gateway/providerwire` package that owns the complete tolerant remote `provider.LanguageModel` protocol and reusable server execution surface. The package SHALL co-locate route/header constants, a complete private request-only legacy representation and adapter, JSON response codecs, SSE framing/readers/writers, error envelopes, and the `net/http` handler. Its public model boundary SHALL use transport-neutral `provider` runtime values; the private request representation MUST remain inside the package and MUST NOT become a second public provider model or alter response codecs.

The package MUST NOT import a router, auth library, host catalog, billing, IAM, deployment, frontend orchestration, or future strict V4 package. The repository SHALL keep `github.com/grafana/ai-sdk/provider/wire` deleted and MUST NOT provide aliases, compatibility re-exports, or a forwarding shim at that path.

#### Scenario: Public handler import
- **WHEN** a Go host imports `github.com/grafana/ai-sdk/gateway/providerwire`
- **THEN** it can construct an `http.Handler` for provider language-model requests

#### Scenario: Canonical codecs are co-located
- **WHEN** the server decodes requests or encodes responses
- **THEN** it SHALL use the package's co-located exported codec helpers
- **AND** request helpers SHALL map between private legacy representations and provider runtime types rather than directly marshaling provider request structs

#### Scenario: Dependency graph remains one-way
- **WHEN** root and Grafana-provider imports are inspected
- **THEN** `gateway/providerwire` SHALL depend on `provider`, `providers/grafana` SHALL depend on both `provider` and `gateway/providerwire`, and `provider` SHALL NOT depend on gateway transport code

#### Scenario: Old package is absent
- **WHEN** the repository package tree is inspected
- **THEN** `provider/wire` SHALL be absent and no compatibility import path or re-export SHALL remain

#### Scenario: Host dependencies stay outside
- **WHEN** imports and public types in `gateway/providerwire` are inspected
- **THEN** no Gorilla mux, authlib, JWKS, Grafana Assistant catalog, IAM, billing, route-prefix, deployment, or future strict V4 type SHALL be present

### Requirement: Request-aware model resolver API

The package SHALL define a small resolver interface that receives the original `*http.Request` and the validated model ID and returns a `provider.LanguageModel` or an error. It SHALL provide a function adapter implementing that interface. `NewHandler` SHALL reject a nil resolver. The resolver MUST be invoked at most once per valid request and MUST NOT be invoked for a request that fails method, header, content negotiation, body-size, or body-decoding validation.

#### Scenario: Resolver receives request policy context

- **WHEN** a valid request carries authenticated tenant or caller values in its context and model ID header `model-a`
- **THEN** the resolver SHALL receive the original request with that context and the model ID `model-a`

#### Scenario: Function adapter

- **WHEN** a host supplies a resolver function through the package's function adapter type
- **THEN** the handler SHALL invoke it through the same resolver contract as any interface implementation

#### Scenario: Invalid request bypasses resolver

- **WHEN** a request has malformed JSON or invalid provider-wire headers
- **THEN** the handler SHALL return the validation error without invoking the resolver

#### Scenario: Nil resolver rejected

- **WHEN** `NewHandler` is called with a nil resolver
- **THEN** construction SHALL return an error and no handler

### Requirement: Handler configuration and defaults

The handler SHALL support options for total timeout, streaming idle timeout, and maximum request-body bytes. When omitted, their defaults SHALL be 120 seconds, 60 seconds, and 8 MiB respectively, preserving the Grafana Assistant server values. An explicitly supplied duration or byte limit MUST be positive; invalid options and nil options SHALL make construction fail rather than silently select a default.

#### Scenario: Defaults apply when options are omitted

- **WHEN** a host constructs a handler with a resolver and no options
- **THEN** model calls SHALL use a 120-second total timeout, streams SHALL use a 60-second idle timeout, and request bodies SHALL be limited to 8 MiB

#### Scenario: Host overrides limits

- **WHEN** a host supplies positive total-timeout, idle-timeout, and maximum-body options
- **THEN** the handler SHALL apply those values to subsequent requests

#### Scenario: Invalid explicit option

- **WHEN** a host supplies a zero or negative timeout or maximum-body value, or a nil option
- **THEN** `NewHandler` SHALL return an error

### Requirement: Provider-wire request validation

The handler SHALL accept only `POST`. It SHALL require a non-empty trimmed `providerwire.HeaderModelID`, `providerwire.HeaderSpecVersion` equal to `providerwire.SpecVersionV4`, and `providerwire.HeaderStreaming` equal to the trimmed literal `true` or `false`. If `Content-Type` is present, it MUST parse as `application/json`, with media-type parameters permitted. If `Accept` is present, at least one comma-separated entry MUST allow the selected response type through an exact match, `*/*`, a matching type wildcard, or an entry whose media type is empty after trimming and parameter stripping. To preserve the extracted Assistant behavior, matching SHALL strip and ignore all media-range parameters rather than perform quality-aware negotiation; consequently, a compatible exact or wildcard entry SHALL still allow the response when it includes `q=0`, and headers such as `,application/xml` or `;q=0` SHALL remain permissive because they contain an empty stripped entry. All validation failures SHALL be non-retryable JSON `provider.APICallError` responses.

#### Scenario: Method rejected

- **WHEN** a request method is not `POST`
- **THEN** the handler SHALL return HTTP 405 with a non-retryable `provider.APICallError`

#### Scenario: Required model header rejected

- **WHEN** `providerwire.HeaderModelID` is missing, empty, or whitespace-only
- **THEN** the handler SHALL return HTTP 400 with a non-retryable `provider.APICallError`

#### Scenario: Specification version rejected

- **WHEN** `providerwire.HeaderSpecVersion` is missing or is not `providerwire.SpecVersionV4`
- **THEN** the handler SHALL return HTTP 400 with a non-retryable `provider.APICallError`

#### Scenario: Streaming mode rejected

- **WHEN** `providerwire.HeaderStreaming` is missing or is neither `true` nor `false`
- **THEN** the handler SHALL return HTTP 400 with a non-retryable `provider.APICallError`

#### Scenario: Content type validated

- **WHEN** a request has a `Content-Type` that does not parse as `application/json`
- **THEN** the handler SHALL return HTTP 415 with a non-retryable `provider.APICallError`

#### Scenario: Omitted content type remains compatible

- **WHEN** a request omits `Content-Type` but otherwise has valid provider-wire headers and a valid JSON body
- **THEN** the handler SHALL continue to body decoding rather than reject the request

#### Scenario: Accept permits selected representation

- **WHEN** a streaming request accepts `text/event-stream`, `text/*`, or `*/*`, or a unary request accepts `application/json`, `application/*`, or `*/*`
- **THEN** content negotiation SHALL succeed

#### Scenario: Accept parameters preserve lenient matching

- **WHEN** a compatible exact or wildcard `Accept` entry has parameters, including `q=0`
- **THEN** the handler SHALL ignore those parameters and permit the selected response representation

#### Scenario: Empty Accept entry preserves lenient matching

- **WHEN** an `Accept` header contains a comma-separated entry whose media type is empty after trimming and parameter stripping, including `,application/xml` or `;q=0`
- **THEN** the handler SHALL permit the selected response representation

#### Scenario: Accept rejects selected representation

- **WHEN** every supplied `Accept` entry has a non-empty media type that is incompatible with the selected streaming or unary response type after parameters are stripped
- **THEN** the handler SHALL return HTTP 406 with a non-retryable `provider.APICallError`

### Requirement: Bounded canonical CallOptions decoding

After header validation, the handler SHALL read no more than the configured request-body limit and SHALL decode the body with the co-located `providerwire.DecodeCallOptions`. The explicit tolerant adapter SHALL preserve every redesigned provider-domain distinction represented by the request, including exact large integers, finite fractional numeric values, optional false and empty scalars, request-file filename presence, selected empty file data, opaque provider options, and nil versus non-nil empty collections. Invalid number values SHALL fail decoding. A body exceeding the limit SHALL produce HTTP 413. A body read failure or decode failure SHALL produce HTTP 400. These failures SHALL be non-retryable and SHALL occur before resolution or model invocation.

#### Scenario: CallOptions decoded losslessly
- **WHEN** a valid body produced by `providerwire.EncodeCallOptions` contains any supported historical or redesigned request values
- **THEN** the resolved model SHALL receive equivalent `provider.CallOptions`

#### Scenario: Empty collections retain presence
- **WHEN** the request explicitly carries an empty supported slice or map
- **THEN** the resolved model SHALL receive a non-nil empty collection

#### Scenario: Malformed JSON rejected
- **WHEN** the body cannot be decoded by `providerwire.DecodeCallOptions`
- **THEN** the handler SHALL return HTTP 400 with a non-retryable `provider.APICallError` and SHALL not invoke the resolver

#### Scenario: Oversized body rejected
- **WHEN** the body exceeds the configured maximum by at least one byte
- **THEN** the handler SHALL return HTTP 413 with a non-retryable `provider.APICallError` and SHALL not invoke the resolver

#### Scenario: Body read error rejected
- **WHEN** reading the request body fails before decoding
- **THEN** the handler SHALL return HTTP 400 with a non-retryable `provider.APICallError` and SHALL not invoke the resolver

### Requirement: Unary model dispatch and response

For `providerwire.HeaderStreaming: false`, the handler SHALL call the resolved model's `DoGenerate` exactly once with a context derived from the request and the decoded `provider.CallOptions`. It SHALL encode a successful non-nil result with the co-located `providerwire.EncodeGenerateResult` before committing HTTP 200 and SHALL respond with `Content-Type: application/json`. Returning a retryable HTTP 500 canonical error envelope for a nil or otherwise unencodable result is an intentional correction of the current Assistant bug that can leave an empty implicit HTTP 200 after `writeUnaryResult` fails.

#### Scenario: Successful unary dispatch

- **WHEN** a resolved model returns a non-nil `provider.GenerateResult`
- **THEN** the handler SHALL return HTTP 200 `application/json` whose body decodes through `providerwire.DecodeGenerateResult` to an equivalent result

#### Scenario: Unary mode does not stream

- **WHEN** `providerwire.HeaderStreaming` is `false`
- **THEN** the handler SHALL invoke `DoGenerate` and SHALL not invoke `DoStream`

#### Scenario: Unary model failure before commit

- **WHEN** `DoGenerate` returns an error
- **THEN** the handler SHALL return the normalized non-2xx JSON API-call error and SHALL not commit a success response

#### Scenario: Invalid unary result before commit

- **WHEN** `DoGenerate` returns a nil result without an error or successful-result encoding otherwise fails
- **THEN** the handler SHALL return a retryable HTTP 500 JSON `provider.APICallError` rather than an empty successful response

### Requirement: Streaming model dispatch and response commitment

For `providerwire.HeaderStreaming: true`, the handler SHALL call the resolved model's `DoStream` exactly once with a context derived from the request and the decoded `provider.CallOptions`. It MUST NOT commit an SSE success response until `DoStream` has returned a non-nil `provider.StreamResult` with a non-nil stream channel.

#### Scenario: Streaming mode dispatches stream call

- **WHEN** `providerwire.HeaderStreaming` is `true`
- **THEN** the handler SHALL invoke `DoStream` and SHALL not invoke `DoGenerate`

#### Scenario: Pre-stream error remains HTTP error

- **WHEN** `DoStream` returns an error before a valid stream is available
- **THEN** the handler SHALL return a normalized non-2xx JSON `provider.APICallError` rather than HTTP 200 SSE

#### Scenario: Nil stream rejected before commit

- **WHEN** `DoStream` returns a nil result or nil stream channel without an error
- **THEN** the handler SHALL return HTTP 500 with a retryable JSON `provider.APICallError`

#### Scenario: Successful stream headers

- **WHEN** `DoStream` returns a valid stream
- **THEN** the handler SHALL commit HTTP 200 with `Content-Type: text/event-stream`, `Cache-Control: no-cache, no-transform`, `Connection: keep-alive`, and `X-Accel-Buffering: no`

### Requirement: Canonical SSE framing, flushing, and termination

The handler SHALL encode each stream part with the co-located `providerwire.WriteSSEStreamPartTo` in received order. It SHALL flush once immediately after committing successful SSE headers and after every event when the response writer supports `http.Flusher`. A clean upstream channel close SHALL end the response without a synthetic event or `[DONE]` sentinel. A provider-emitted `provider.PartError` is a stream event and SHALL NOT terminate forwarding; the handler SHALL continue until the provider closes the stream, the request is canceled, a timeout expires, or a write fails.

#### Scenario: Initial headers are flushed

- **WHEN** a valid stream is established and the response writer implements `http.Flusher`
- **THEN** HTTP 200 and SSE headers SHALL be flushed before the first stream part is received

#### Scenario: Every part is immediately flushed

- **WHEN** the model emits stream parts over time and the writer implements `http.Flusher`
- **THEN** each canonical SSE event SHALL be flushed before the handler waits for the next part

#### Scenario: Stream order preserved

- **WHEN** the model emits multiple `provider.StreamPart` values
- **THEN** the client SHALL receive the same values in the same order through `providerwire.NewSSEReader`

#### Scenario: Clean stream closes without sentinel

- **WHEN** the model stream channel closes
- **THEN** the handler SHALL end the HTTP body without emitting `[DONE]` or any synthetic terminal part

#### Scenario: Upstream PartError does not terminate forwarding

- **WHEN** the model emits a `provider.PartError` followed by additional stream parts
- **THEN** the handler SHALL forward the error and every subsequent part unchanged and in order until the stream closes or another handler termination condition occurs

### Requirement: Error normalization and commit-aware encoding

The handler SHALL preserve any encodable `*provider.APICallError` reachable through `errors.As`, including status, retryability, URL, headers, body, and data. If malformed raw JSON makes that preserved envelope unencodable, or if its nonzero status cannot carry a final non-success JSON response because it is below 300, is the body-forbidden HTTP 304 status, or is above 999, the handler SHALL return an encodable retryable HTTP 500 API-call error rather than an empty implicit success, bodyless error, or panic. Other resolver or model errors SHALL become retryable HTTP 502 API-call errors carrying the original message. A nil resolved model SHALL become a retryable HTTP 500 API-call error. Before SSE commitment errors SHALL use the co-located `providerwire.WriteErrorResponse`; after commitment handler-generated terminal errors SHALL use a final `provider.PartError` SSE event.

#### Scenario: Resolver APICallError preserved

- **WHEN** the resolver returns a wrapped `*provider.APICallError`
- **THEN** the non-2xx response SHALL preserve its status, `IsRetryable`, URL, response metadata, and data

#### Scenario: Arbitrary resolver error normalized

- **WHEN** the resolver returns a non-API error
- **THEN** the handler SHALL return HTTP 502 with `IsRetryable: true` and the error message

#### Scenario: Unencodable API-call error falls back before commit

- **WHEN** a resolver or model returns an `APICallError` whose raw JSON fields prevent canonical error-envelope encoding
- **THEN** the handler SHALL return an encodable retryable HTTP 500 `provider.APICallError` rather than an empty implicit HTTP 200

#### Scenario: Invalid API-call error status falls back before commit

- **WHEN** a resolver or model returns an `APICallError` with a nonzero status below 300, equal to HTTP 304, or above 999
- **THEN** the handler SHALL return an encodable retryable HTTP 500 `provider.APICallError` without panicking or committing an implicit success or bodyless error

#### Scenario: Resolver returns nil model

- **WHEN** the resolver returns no error and a nil model
- **THEN** the handler SHALL return HTTP 500 with a retryable `provider.APICallError` without invoking a model method

#### Scenario: Model APICallError preserved before stream

- **WHEN** `DoGenerate` or pre-stream `DoStream` returns a wrapped `*provider.APICallError`
- **THEN** the non-2xx response SHALL preserve the API-call error fields rather than recategorize it

#### Scenario: Error after SSE commitment

- **WHEN** a handler timeout or cancellation occurs after HTTP 200 SSE has been committed
- **THEN** the handler SHALL make a best-effort write of one final `provider.PartError` carrying the normalized API-call error and then terminate

### Requirement: Request cancellation propagation

The model context SHALL derive from `r.Context()`. Request cancellation before response commitment SHALL be surfaced, when observable, as HTTP status 499 with a non-retryable API-call error message `consumer disconnected`. Cancellation after SSE commitment SHALL cancel the model promptly and make a best-effort final 499 non-retryable `PartError`. The handler SHALL not block waiting for a producer that honors context cancellation.

#### Scenario: Cancellation reaches unary model

- **WHEN** the request context is canceled during `DoGenerate`
- **THEN** the context passed to the model SHALL become done and a returned cancellation error SHALL normalize to status 499 and non-retryable when a response remains writable

#### Scenario: Cancellation reaches streaming model

- **WHEN** the consumer disconnects after streaming starts
- **THEN** the context passed to the model SHALL become done promptly and the handler SHALL return without leaking a handler-owned goroutine

#### Scenario: Cancellation after commit is best effort

- **WHEN** the request is canceled after HTTP 200 is committed and the socket can still accept a write
- **THEN** the final SSE part SHALL contain status 499, `IsRetryable: false`, and message `consumer disconnected`

### Requirement: Total timeout behavior

The handler SHALL derive one total-timeout context after successful resolution and use it for either `DoGenerate` or `DoStream` plus subsequent stream consumption. The default SHALL be 120 seconds unless configured otherwise. Total timeout SHALL NOT include method/header/body validation or resolver execution. The package SHALL expose an `ErrTotalTimeout` sentinel as the timeout cause. A total timeout SHALL normalize to HTTP status 504, `IsRetryable: true`, and message `total timeout exceeded`.

#### Scenario: Unary total timeout

- **WHEN** a unary model observes the total context deadline and returns its deadline error before response commitment
- **THEN** the handler SHALL return HTTP 504 with a retryable API-call error

#### Scenario: Total timeout before stream commitment

- **WHEN** `DoStream` observes the total deadline and returns before a valid stream is committed
- **THEN** the handler SHALL return HTTP 504 JSON with `IsRetryable: true`

#### Scenario: Total timeout during stream

- **WHEN** the total timeout expires after SSE commitment
- **THEN** the model context SHALL be canceled and the handler SHALL emit a final retryable 504 `PartError` when the writer remains available

#### Scenario: Resolution precedes total timeout

- **WHEN** request validation and resolution complete successfully
- **THEN** the total-timeout clock SHALL begin for model invocation rather than including the preceding work

### Requirement: Streaming idle timeout behavior

The handler SHALL enforce the idle timeout only after a valid stream is committed. The default SHALL be 60 seconds unless configured otherwise. The timer SHALL cover the wait for the first stream part and SHALL reset after every successfully forwarded part. On expiry, the handler SHALL cancel the model with an exported `ErrIdleTimeout` cause and emit a final API-call error with status 504, `IsRetryable: true`, and message `idle timeout: no stream parts produced within configured window`.

#### Scenario: Initial stream idle timeout

- **WHEN** a valid stream produces no first part within the idle window
- **THEN** the handler SHALL cancel the model and emit a final retryable 504 `PartError`

#### Scenario: Inter-part idle timeout

- **WHEN** a stream emits a part and then produces no next part within the reset idle window
- **THEN** the handler SHALL cancel the model and emit a final retryable 504 `PartError`

#### Scenario: Activity resets idle timer

- **WHEN** each part arrives within the configured idle interval
- **THEN** the handler SHALL reset the timer after each successful write and SHALL not time out solely because the total stream age exceeds one idle interval

#### Scenario: Idle timeout does not govern DoStream setup

- **WHEN** `DoStream` has not yet returned a valid stream
- **THEN** only request cancellation and the total-timeout context SHALL govern that call

### Requirement: Observable stream output failure handling

If canonical SSE encoding or writing to the response writer returns an error, the handler SHALL cancel the model context promptly with a cause wrapping that error and SHALL terminate without attempting to encode a second event on the failed writer. It SHALL clean up handler-owned timers and MUST NOT panic. Standard `http.Flusher.Flush()` has no error return, including when called by `providerwire.WriteSSEStreamPartTo`, so flush failures are not observable through this API and the handler MUST NOT introduce alternate SSE framing to detect them.

#### Scenario: SSE encoding or write fails mid-stream

- **WHEN** encoding a stream part or writing its canonical SSE bytes returns an error
- **THEN** the model context SHALL become done and the handler SHALL return without writing a synthetic `PartError` to the failed writer

#### Scenario: Timer cleanup on write failure

- **WHEN** a write failure terminates a streaming request
- **THEN** the handler SHALL stop its idle timer and SHALL not leave a handler-owned goroutine running

### Requirement: Real Grafana client/server conformance without module cycles

Tests SHALL exercise the public handler over a real `httptest.Server` using `providers/grafana.NewWithAccessToken` as the client. Such tests MUST live in the `providers/grafana` module or the independent conformance module, not in root-module tests that would create a dependency cycle. Coverage SHALL include unary success, ordered streaming success with clean EOF, and continued ordered forwarding after a retryable mid-stream `PartError`.

#### Scenario: Real client unary round-trip

- **WHEN** the Grafana client's `DoGenerate` calls an `httptest.Server` backed by `gateway/providerwire`
- **THEN** call options SHALL reach the resolved model and the equivalent generate result SHALL return through the canonical wire

#### Scenario: Real client streaming round-trip

- **WHEN** the Grafana client's `DoStream` calls the public handler
- **THEN** all model parts SHALL reach the client in order and the channel SHALL close cleanly without `[DONE]`

#### Scenario: Real client continues after retryable mid-stream error

- **WHEN** the resolved model emits a retryable `provider.PartError` followed by additional parts
- **THEN** the real Grafana client SHALL receive the error preserving status and retryability followed by every subsequent part in order

#### Scenario: Test dependency graph remains acyclic

- **WHEN** root and `providers/grafana` module tests list their imports
- **THEN** root tests SHALL not import the child Grafana module and the child or independent conformance tests MAY import the root public handler

### Requirement: Provider transport parity and Assistant migration boundary

The provider request-contract redesign SHALL be a source-breaking, parity-preserving Go adaptation. For every migrated request value accepted by the parent encoder, the public tolerant handler MUST preserve provider-wire headers, parent request bytes and shapes, response bytes and shapes, status/error envelopes, SSE framing, and frontend UI chunks. Previous-decoder acceptance applies only to the subset the parent decoder accepted. Private legacy request adapters MAY change implementation structure but not those historical outputs.

Newly representable fractional, explicit false/empty, and empty selected file-data values SHALL be tested as redesigned tolerant-adapter behavior and SHALL NOT be presented as historical byte compatibility. The handler SHALL NOT gain strict schema validation or mount a strict V4 dialect in this phase.

The existing intentional Assistant pre-commit response corrections remain in force: invalid unary results or unencodable API-call error envelopes, and API-call errors with invalid final HTTP statuses, produce retryable HTTP 500 responses using the existing canonical error envelope rather than an empty implicit HTTP 200 or panic. Otherwise the extraction and redesign MUST NOT change response bytes or behavior.

The Assistant host SHALL remain responsible for auth, caller and tenant identity, catalog/model policy, route prefix and Gorilla mount, IAM, billing, deployment, and host logging. During migration, Assistant SHALL continue to recognize `providerwire.ErrIdleTimeout` and `providerwire.ErrTotalTimeout` through `errors.Is`, or translate those causes to its existing timeout sentinels before classification.

#### Scenario: Existing Grafana conformance uses public server
- **WHEN** the Grafana fixture conformance path runs through the public handler
- **THEN** its output SHALL continue to match the existing shared `expected.jsonl` results without regenerating expectations for a wire change

#### Scenario: Historical requests remain byte-stable
- **WHEN** the handler receives and the Grafana client emits pre-redesign request values
- **THEN** request headers and canonical request bytes SHALL match the Phase 1 parent

#### Scenario: Previous decoder compatibility remains for the parent-decodable subset
- **WHEN** post-redesign encoding handles a migrated pre-redesign request whose corpus row was decoded successfully by parent commit `32e5ab7f1ab9e524477cc0ece04c690a89854a24`
- **THEN** its bytes SHALL equal the parent-produced fixture
- **AND** the recorded parent semantic projection SHALL establish previous-decoder acceptance independently from the redesigned decoder

#### Scenario: Parent decoder rejection is not overclaimed
- **WHEN** a parent-encoder-accepted corpus row records a parent decoder rejection
- **THEN** post-redesign encoding SHALL still preserve the parent bytes
- **AND** handler evidence SHALL retain the rejection outcome without claiming previous-decoder compatibility

#### Scenario: New request distinctions reach the model
- **WHEN** a redesigned tolerant request contains a supported new distinction
- **THEN** the handler SHALL preserve it through decoding and model dispatch

#### Scenario: Strict runtime remains absent
- **WHEN** the handler package is inspected
- **THEN** it SHALL expose only the existing tolerant dialect and SHALL NOT contain strict/tolerant branches

#### Scenario: Source compatibility changes without wire drift
- **WHEN** a consumer upgrades to the redesigned provider request types
- **THEN** it SHALL update affected Go construction sites while transmitted headers and bytes for pre-redesign values remain unchanged

#### Scenario: Frontend wire remains unaffected
- **WHEN** the change is inspected for UI message or `@ai-sdk/react` behavior
- **THEN** no UI chunk type, UI SSE framing, or frontend hook contract SHALL change

#### Scenario: Cancellation parity gap remains explicit
- **WHEN** handler cancellation unit tests pass
- **THEN** they SHALL provide local lifecycle confidence but SHALL NOT by themselves reclassify the upstream cancellation/abort conformance gap as automated parity

#### Scenario: Assistant integration remains thin
- **WHEN** Grafana Assistant uses the package
- **THEN** its integration SHALL consist of host auth/observability wrappers, a request-aware resolver adapter, timeout configuration, and route mounting rather than duplicated wire validation or dispatch

#### Scenario: Assistant adopts corrected invalid-unary expectation
- **WHEN** Assistant compares the public handler side by side with its existing handler for a nil or unencodable unary result
- **THEN** the migration SHALL expect the public handler's retryable HTTP 500 canonical error response instead of preserving the existing empty implicit HTTP 200 bug

#### Scenario: Assistant adopts corrected invalid-error expectation
- **WHEN** Assistant compares the public handler side by side with its existing handler for an API-call error containing malformed raw JSON or an invalid final HTTP status
- **THEN** the migration SHALL expect an encodable retryable HTTP 500 canonical error response instead of preserving the existing empty implicit HTTP 200 or panic bug

#### Scenario: Assistant preserves timeout classification
- **WHEN** the public handler cancels a model with `providerwire.ErrIdleTimeout` or `providerwire.ErrTotalTimeout`
- **THEN** Assistant's host-owned observability SHALL classify the causes with its existing backend-error and idle/total-timeout logging vocabulary rather than as generic cancellation
