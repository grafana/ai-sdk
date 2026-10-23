# Spec Delta

## MODIFIED Requirements

### Requirement: Authenticated request-only destination
Authenticated BYOK SHALL use the shared engine's native provider/model selection without configured lookup, new selector grammar or extra byte ceilings. Native model suffixes and provider-owned availability SHALL remain unchanged.

#### Scenario: Authenticated destination policy
- **WHEN** a platform-verified user or service-account caller with BYOK access in `cloud` mode requests native BYOK inference
- **THEN** Authenticated BYOK inference SHALL require a provider/model selector in the existing ai-language-model-id header.
- **AND** The selector SHALL be valid UTF-8, have a supported case-sensitive provider prefix before the first slash and a nonempty native-model suffix.
- **AND** The service SHALL reuse the request-only engine's decoder and the existing whole-request and HTTP header bounds without adding selector grammar or a separate byte ceiling.
- **AND** The suffix SHALL be preserved, including additional slashes, as model data rather than a URL.
- **AND** Initial providers SHALL be anthropic and openai, using the native Messages and Responses adapters respectively.
- **AND** The selector SHALL NOT resolve configured models, aliases, provider instances, endpoints, defaults or fallback routes.
- **AND** Advisory native model lists SHALL NOT act as a configured-catalog allowlist.
- **AND** Unknown native models SHALL be decided by the selected native provider without unsolicited inventory or validation calls.

#### Scenario: Native model absent from configured catalog
- **WHEN** a valid anthropic/model-name selector and Anthropic request credentials are supplied
- **THEN** the adapter SHALL receive model-name without consulting any catalog, regardless of whether it is configured internally

#### Scenario: Flat configured alias is supplied
- **WHEN** a BYOK caller names assistant or another selector without a provider/model split
- **THEN** selection SHALL fail before I/O rather than expand a configured alias

#### Scenario: Selector boundaries
- **WHEN** a selector has an empty provider/model component, invalid UTF-8 or an unsupported provider
- **THEN** the request SHALL fail with a bounded capability/field diagnostic before provider construction

### Requirement: BYOK capture policy is independent of returned data
Host authorization, explicit credential sources and request isolation SHALL remain intact. Operator capture SHALL be independent of returned values. Gateway-produced summaries SHALL apply gateway-attempt-failure-evidence's configured provider/exporter key policy while retaining BYOK-only echoes and catalog-independent eligibility. Missing BYOK observation SHALL remain an explicit capability gap.

#### Scenario: BYOK privacy and attribution policy
- **WHEN** the host processes request-only credentials and observes execution
- **THEN** Host controls SHALL be consumed for account selection rather than forwarded wholesale to native inference.
- **AND** Logging SHALL use configured field-aware redaction; other exporters SHALL own their capture policies, and metric labels SHALL NOT contain credential payloads.
- **AND** The host SHALL NOT inventory request credentials. Only Gateway-produced native summary fields SHALL apply the configured provider/exporter key policy; source errors, opaque metadata, ordinary content and unrelated BYOK echoes SHALL remain unchanged. Platform-delegated access tokens and other inbound platform credentials SHALL remain excluded.
- **AND** The host SHALL NOT synthesize diagnostics from an internal credential store or another request's state.
- **AND** Supported provider/model identity, application content and native response values SHALL remain available under their existing contracts.
- **AND** Configured execution attribution SHALL remain independent of logical BYOK observation.
- **AND** Event-local native SSE summaries SHALL remain eligible without catalog-owned attribution. BYOK unary/setup summaries and attempt overviews are currently unavailable; #317 owns reusing existing SDK observation with honest request identity, not a new collector.
- **AND** Independently registered SDK observers SHALL remain effective; original native metadata SHALL remain opaque and SHALL NOT establish configured provenance.
- **AND** Diagnostic messages SHALL use bounded approved capability names/schema paths and bounded indices/counts, never arbitrary rejected key names, values or serialized bodies.
- **AND** Detailed public attempt evidence SHALL remain owned by its separate contract; this capability SHALL NOT introduce a second metadata/error protocol.

#### Scenario: Provider echoes the rejected key
- **WHEN** a fake provider rejection includes the selected dummy API key in authentication-related error material
- **THEN** available native summaries SHALL retain that BYOK echo unless it exactly matches a configured provider/exporter key, without changing Gateway classification or account isolation
- **AND** logger sensitive-field policy and metadata-only exporter exclusions SHALL be tested independently from caller output

#### Scenario: Ordinary application text resembles a key
- **WHEN** supported prompt/output text contains a key-looking string unrelated to authentication material
- **THEN** the response SHALL preserve that content rather than apply token-pattern censorship

### Requirement: Evidence remains independent and provenance-correct
Acceptance SHALL retain independent pinned-client/native-HTTP/service/SDK/race witnesses with honest proof boundaries. Synthetic credential tests SHALL NOT become recorded provider evidence; baseline/parity checks and deferred support SHALL remain explicit.

#### Scenario: BYOK evidence policy
- **WHEN** the delivery's acceptance evidence is collected or summarized
- **THEN** Acceptance SHALL include exact-pinned Vercel HTTP captures, independent Go-client tests without AGPL imports, fake native HTTP requests, protocol test host tests, middleware sink tests and race/lifetime tests.
- **AND** Synthetic credential cases SHALL use dummy values in focused tests, not invented recorded/upstream conformance inputs.
- **AND** Existing authentic provider inputs SHALL remain unchanged.
- **AND** Applicable parity/module/frontend checks SHALL run, with coverage gaps and deferred families/routing explicitly recorded.

#### Scenario: Catalog is inaccessible to BYOK
- **WHEN** a spy catalog that fails on every call accompanies BYOK inference and discovery tests
- **THEN** inference SHALL work from request data and discovery SHALL be explicitly rejected, with zero catalog calls in both cases

#### Scenario: Provider behavior is claimed
- **WHEN** evidence is summarized
- **THEN** fake HTTP and registered-client evidence SHALL be distinguished from live native acceptance and Vercel private-service behavior

### Requirement: Request-scoped provider construction and control removal
Validated plain accounts SHALL feed explicit native construction without raw-JSON parsing, catalog/configuration/secret lookup or ambient defaults. Request credentials and models SHALL remain request-owned; host controls SHALL be removed before native inference and middleware.

#### Scenario: Construction and ownership policy
- **WHEN** the host prepares request-only native execution
- **THEN** Host selection SHALL decode and validate Gateway controls before passing the provider, native model and plain account configs to construction.
- **AND** Construction SHALL NOT parse raw Gateway JSON.
- **AND** Shared native constructors MAY serve configured and request-only execution, but SHALL receive explicit inputs without performing catalog, configuration or secret lookup.
- **AND** Missing model/accounts or an unsupported provider SHALL fail before native construction.
- **AND** The BYOK selector SHALL construct account-bound models for the request using only the selected supplied accounts and native-default or service-approved destinations.
- **AND** It SHALL NOT inherit configured instances or ambient SDK key/base-URL/account defaults.
- **AND** Shared transports SHALL be credential-independent; redirects SHALL NOT forward credentials.
- **AND** The host SHALL remove gateway controls before generic model middleware and native provider options see them, and forward supported matching native options/content/history without using configured-catalog field allowlists or cross-provider intersections.
- **AND** No key-bearing model, credential collection, raw request or request observer SHALL be stored in a global cache, catalog or background refresh service.
- **AND** Models and credential data SHALL follow bounded request execution/cleanup ownership; garbage-collection lifetime SHALL NOT be described as cryptographic zeroization.

#### Scenario: Decoded request owns credentials
- **WHEN** raw Gateway controls are discarded or modified after successful decoding
- **THEN** construction SHALL use the validated request's selected account configuration and authorized destination without rereading those controls

#### Scenario: Native endpoints observe selected credentials
- **WHEN** both provider entries contain distinct dummy keys and the request selects Anthropic
- **THEN** the Anthropic authentication transport SHALL contain only the selected Anthropic key
- **AND** no BYOK map, OpenAI key, platform access token or configured key SHALL enter its native JSON body or headers

#### Scenario: Native content is preserved
- **WHEN** a BYOK request contains supported native options, reasoning/file/tool history or continuation
- **THEN** the chosen adapter SHALL receive its matching ordinary options and supported content unchanged, apart from existing documented provider mappings

#### Scenario: Concurrent customer isolation
- **WHEN** concurrent requests select the same provider/model with different keys
- **THEN** each native request SHALL use only its own key and SHALL NOT mutate shared clients/models

#### Scenario: Cancellation and late setup
- **WHEN** cancellation occurs during native setup or stream delivery
- **THEN** no later credential SHALL start, late results SHALL be cleaned up through bounded ownership/drain and no request credential SHALL be cached
