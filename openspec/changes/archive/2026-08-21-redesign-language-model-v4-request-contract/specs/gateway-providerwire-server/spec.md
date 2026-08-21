## MODIFIED Requirements

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
