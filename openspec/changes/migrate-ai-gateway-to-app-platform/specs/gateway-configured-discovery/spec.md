# Spec Delta

## MODIFIED Requirements

### Requirement: Explicit configured route projection
Configured-access-authorized discovery SHALL expose ordered canonical public rows with typed aliases, primary and ordered fallback facts, without generation, inferred candidates or substitution of native identity.

#### Scenario: Explicit configured route projection policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Configured-access-authorized `GET` on the plugin `config` route SHALL return one row per visible canonical model, in ascending canonical-ID order, retaining its public `id` and `specification.modelId`, specification version `v4`, provider `grafana` and existing name/description. Each command-configured row SHALL include `gateway: {aliases, primary: {providerInstance, provider, providerModelId}, fallbacks: [{providerInstance, provider, providerModelId}]}`. Alias and fallback order SHALL preserve configuration; empty aliases and fallbacks SHALL be `[]`. Aliases SHALL remain callable without appearing as separate rows. `providerModelId` SHALL identify the configured native invocation model, never a public selection ID or response identity. Ordinary catalog entries without candidate metadata SHALL omit the extension rather than infer it.

#### Scenario: Alias and mixed-provider candidates are inspected
- **WHEN** an authorized caller discovers an alias for a route with an Anthropic primary and an OpenAI fallback
- **THEN** one canonical row SHALL expose its explicit aliases, primary and ordered fallbacks with provider-instance/provider/providerModelId facts
- **AND** its public specification SHALL NOT be replaced with a native candidate or response identity

#### Scenario: Direct compatible route is inspected
- **WHEN** a configured compatible provider has an explicit providerName or uses its constructor default
- **THEN** its candidate SHALL expose the effective adapter/provider identifier, configured instance reference and exact configured invocation model ID
- **AND** discovery SHALL NOT derive these values from canonical middleware or provider responses

#### Scenario: No inference is needed
- **WHEN** raw HTTP, Go discovery or the TS helper inspects configured candidates
- **THEN** no provider generation, streaming, inventory enumeration or credential-check inference SHALL run

### Requirement: Account-authorized projection excludes credential sources
Discovery SHALL require configured-account authorization before listing and project only request-visible route/candidate facts. Explicit keys, secret references, caller credentials and unrelated account state SHALL not enter the projection; ordinary authorized identifiers SHALL remain available.

#### Scenario: Account-authorized projection excludes credential sources policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Discovery SHALL authenticate and require configured-account access before listing and SHALL pass the request context to the existing model lister. It SHALL project only entries and candidate facts visible at the same account/route boundary as resolution, without supplementing a scoped list from global configuration. The explicit projection SHALL exclude API keys, environment/secret credential references, signing/workload credential material, caller-auth headers and unrelated account/provider state. Authorized route, candidate, provider-instance, provider and model identifiers SHALL NOT be categorically concealed. This feature SHALL NOT introduce account provisioning or a new policy engine.

#### Scenario: Scoped host listing matches resolution
- **WHEN** a host decorator restricts a request to one route and its configured candidates
- **THEN** discovery SHALL contain only that route's canonical row, aliases and authorized facts, consistent with resolution
- **AND** another request's entries SHALL NOT enter the projection or retained shared state

#### Scenario: Known credential sources are configured
- **WHEN** provider configuration contains distinct dummy API keys, secret references and unrelated provider entries
- **THEN** discovery SHALL expose only the approved visible route/candidate fields, without those credential sources or unrelated entries
- **AND** ordinary key-looking display strings and authorized provider/model identifiers SHALL remain intact

#### Scenario: Authentication fails
- **WHEN** the platform rejects a discovery request before it reaches the plugin
- **THEN** no catalog listing or provider work SHALL occur, and the Gateway SHALL NOT be required to produce the response

#### Scenario: Authenticated BYOK discovery
- **WHEN** a platform-verified user or service-account caller with BYOK access in `cloud` mode requests the plugin `config` route
- **THEN** the host SHALL return HTTP 400 with type invalid_request_error, code invalid_request, param null and a fixed message that catalog discovery is unsupported for BYOK
- **AND** it SHALL NOT list or resolve the catalog, return an empty success, enumerate providers or perform inference

#### Scenario: Authentication still precedes unsupported operation
- **WHEN** a discovery request lacks a verified delegated identity or its identity is refused
- **THEN** the request SHALL fail before the application returns an authenticated unsupported-operation response or lists the catalog

### Requirement: Atomic discovery resource and consistency limits
Configuration loading SHALL validate route semantics before readiness: at most 1,024 callable public IDs including aliases, 16 candidates per route, 128 aliases per route and 2,048 UTF-8 bytes per identity/display string. The existing 1–128 ASCII public-ID grammar SHALL remain unchanged. Required identities and names SHALL be nonblank; strings SHALL be valid UTF-8; optional description SHALL preserve existing empty/absent semantics.

#### Scenario: Server policy dimensions reach their boundaries
- **WHEN** a configured catalog reaches an exact public-ID, candidate, alias or string ceiling
- **THEN** configuration loading SHALL accept it if semantic constraints hold
- **WHEN** any policy dimension exceeds its ceiling by one
- **THEN** configuration loading SHALL fail before readiness

#### Scenario: Invalid routes fail startup
- **WHEN** configuration contains invalid IDs, duplicate aliases/candidate tuples, canonical/alias collisions or blank required identifiers
- **THEN** startup SHALL fail before serving requests or provider inference
- **AND** response building SHALL NOT repeat those checks

#### Scenario: Configured catalog exceeds the former response budget
- **WHEN** a valid configured discovery document exceeds 1 MiB after JSON encoding and escaping
- **THEN** the server SHALL serve every visible configured row without a discovery response-size rejection

#### Scenario: Consumers do not duplicate configuration policy
- **WHEN** a document fits the consumer byte budget and decodes into its typed metadata
- **THEN** the consumer SHALL retain supplied facts without checking route semantics, uniqueness or numeric configuration policy
- **WHEN** the document exceeds the consumer byte budget by one
- **THEN** the consumer SHALL fail without returning any rows

#### Scenario: Escaped lone surrogates use standard JSON semantics
- **WHEN** a display or candidate string contains an escaped lone UTF-16 surrogate
- **THEN** Go SHALL decode it as U+FFFD and TS SHALL retain its standard decoded UTF-16 value

#### Scenario: Client receives a late type error
- **WHEN** a bounded document contains valid initial rows followed by a field that cannot decode into its declared type
- **THEN** configured access SHALL fail atomically without exposing the initial rows

### Requirement: Bounded TS companion access on the existing route
The existing copyable typed TS discovery helper SHALL read configured facts from authenticated /config using explicit platform credential headers, bounded reads, ordinary recognized-field decoding and deterministic cleanup. It SHALL not be a new package/endpoint or grant account access, switch credentials, cache catalogs or invoke inference.

#### Scenario: Bounded TS companion access on the existing route policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** The repository SHALL provide a documented, typechecked and deterministically tested copyable `fetchConfiguredModels({baseURL, headers, fetch, signal, maxBytes})` consumer helper retaining typed configured-route facts from the existing authenticated `/config` document. It SHALL NOT be a new published TS package or endpoint. The helper SHALL preserve an HTTP(S) API-prefix base URL, reject URL credentials/query/fragment, use explicit platform credential outer headers, issue one GET, refuse redirects, honor abort, check success and JSON media type, bound reads incrementally and validate the entire raw UTF-8 JSON document and recognized model/extension fields before return. It SHALL ignore unrelated unknown additive fields rather than expose arbitrary configuration. It SHALL release/cancel reader resources on success or failure and SHALL NOT embed auth headers or arbitrary response bodies in errors, cache catalogs, switch credentials or invoke inference.
- **AND** Stock exact-pinned `getAvailableModels()` SHALL remain tested as compatible normalized discovery listing canonical rows and stripping the extension, including aliases and provider choices. It SHALL NOT be presented as an access path for these facts; known alias IDs SHALL remain callable. The helper SHALL accept ordinary rows with absent/null gateway; a non-null extension with a JSON shape or field type incompatible with ConfiguredRoute SHALL fail the complete document. It SHALL NOT check route semantics.

#### Scenario: Both TS discovery surfaces are demonstrated
- **WHEN** the registered Gateway client and shipped helper read the same configured response
- **THEN** normalized discovery SHALL preserve its existing public fields and discard gateway extras
- **AND** the helper SHALL expose the canonical public ID and typed aliases/primary/fallbacks through an executable documented access expression

#### Scenario: TS bounded read is interrupted
- **WHEN** the byte cap is crossed, the caller aborts, the transport fails or the response is invalid
- **THEN** the helper SHALL release/cancel its body reader and return no document, without retry or inference

#### Scenario: Endpoint attempts credential forwarding
- **WHEN** discovery redirects to another endpoint
- **THEN** the helper SHALL refuse the redirect without sending credentials to the redirect target

#### Scenario: Helper cannot grant configured access
- **WHEN** the helper is used with user or service-account credentials in `cloud` mode
- **THEN** it SHALL surface the non-success discovery response without switching credentials/endpoints or returning an empty catalog

### Requirement: TS discovery URL and bounded transport validation
The helper SHALL preserve an HTTP(S) API-prefix base URL, reject URL credentials/query/fragment, use explicit selected platform credential outer headers, issue one GET, refuse redirects, honor abort, check success and JSON media type, bound reads incrementally and validate the entire raw UTF-8 JSON document and recognized model/extension fields before return. It SHALL ignore unrelated unknown additive fields rather than expose arbitrary configuration.

#### Scenario: TS discovery URL and bounded transport validation
- **WHEN** the helper receives an API-prefix URL and a response with unknown additive fields
- **THEN** it SHALL issue one authenticated nonredirecting GET, validate recognized fields and raw UTF-8 within the byte bound, and ignore unrelated additive fields

### Requirement: Independent evidence and honest support guidance
Configured discovery SHALL retain independent Go/TS/server/protocol-test-host tests, module boundaries and honest support guidance. Successful/denied discovery SHALL assert no inference; BYOK user and service-account discovery in `cloud` mode SHALL be authenticated but unsupported without catalog lookup.

#### Scenario: Independent evidence and honest support guidance policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** This feature SHALL ship startup route-policy tests, server complete-projection tests and independent Go/TS malformed/type-error/oversized/boundary tests, immutable catalog tests, raw HTTP/schema tests and exact-pinned client/protocol-test-host discovery tests. Successful and denied discovery tests SHALL assert zero native inference requests. The TS example SHALL be registered in the existing ProviderWire workspace's typecheck/test commands, and Go client tests SHALL remain independent of AGPL implementation imports.
- **AND** Guidance SHALL distinguish configured candidates from actual attempts/response identity, stock TS normalized discovery from helper access, and configured-account visibility from catalog-independent BYOK execution. Scoped fakes and test-signed platform tokens SHALL NOT be presented as deployed customer isolation or live platform authorization proof. Operator telemetry configuration SHALL remain independent of developer discovery; no discovery feature SHALL enable payload capture or new topology labels. Applicable catalog/discovery/client specs, docs/navigation, parity and module checks SHALL ship with the feature.

#### Scenario: Command proves access without generation
- **WHEN** raw HTTP, the TS helper and Go ListModels as an internal service in `cloud` mode, and pinned TS normalized discovery as a service account in `self-hosted` mode, exercise configured direct and fallback aliases through the protocol test host
- **THEN** normalized row compatibility and configured access SHALL be proven with intended authentication and zero native inference calls

#### Scenario: Current Cloud fixture is documented
- **WHEN** deterministic protocol-test-host tests authenticate as a user or service account in `cloud` mode
- **THEN** discovery SHALL be rejected without catalog access and inference SHALL require BYOK
- **AND** no `cloud` mode fixture SHALL preserve a user-to-configured-account behavior as a compatibility requirement

### Requirement: Discovery support and production evidence boundaries
Guidance SHALL distinguish configured candidates from actual attempts/response identity, stock TS normalized discovery from helper access, and current startup-configured/internal-account visibility from request-scoped BYOK construction. Scoped fakes and test-signed platform tokens SHALL NOT be presented as deployed customer isolation or live platform authorization proof.

#### Scenario: Discovery support and production evidence boundaries
- **WHEN** guidance describes static configured discovery exercised through the protocol test host
- **THEN** it SHALL distinguish helper access, configured possibilities and current visibility from runtime attempts, BYOK execution and deployed isolation or platform authorization proof
