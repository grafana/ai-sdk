## MODIFIED Requirements

### Requirement: Explicit configured route projection
Configured-access-authorized `GET /api/v1/aisdk/config` SHALL return one row per visible canonical model, in ascending canonical-ID order, retaining its public `id` and `specification.modelId`, specification version `v4`, provider `grafana` and existing name/description. Each command-configured row SHALL include `gateway: {aliases, primary: {providerInstance, provider, providerModelId}, fallbacks: [{providerInstance, provider, providerModelId}]}`. Alias and fallback order SHALL preserve configuration; empty aliases and fallbacks SHALL be `[]`. Aliases SHALL remain callable without appearing as separate rows. `providerModelId` SHALL identify the configured native invocation model, never a public selection ID or response identity. Ordinary catalog entries without candidate metadata SHALL omit the extension rather than infer it.

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
Discovery SHALL authenticate and require configured-account access before listing and SHALL pass the request context to the existing model lister. It SHALL project only entries and candidate facts visible at the same account/route boundary as resolution, without supplementing a scoped list from global configuration. The explicit projection SHALL exclude API keys, environment/secret credential references, signing/workload credential material, caller-auth headers and unrelated account/provider state. Authorized route, candidate, provider-instance, provider and model identifiers SHALL NOT be categorically concealed. This feature SHALL NOT introduce account provisioning or a new policy engine.

#### Scenario: Scoped host listing matches resolution
- **WHEN** a host decorator restricts a request to one route and its configured candidates
- **THEN** discovery SHALL contain only that route's canonical row, aliases and authorized facts, consistent with resolution
- **AND** another request's entries SHALL NOT enter the projection or retained shared state

#### Scenario: Known credential sources are configured
- **WHEN** provider configuration contains distinct dummy API keys, secret references and unrelated provider entries
- **THEN** discovery SHALL expose only the approved visible route/candidate fields, without those credential sources or unrelated entries
- **AND** ordinary key-looking display strings and authorized provider/model identifiers SHALL remain intact

#### Scenario: Authentication fails
- **WHEN** the existing authentication layer rejects a discovery request
- **THEN** no catalog listing or provider work SHALL occur and the existing bounded authentication response SHALL apply

#### Scenario: Authenticated BYOK discovery
- **WHEN** a trusted Cloud/BYOK caller requests /api/v1/aisdk/config
- **THEN** the host SHALL return HTTP 400 with type invalid_request_error, code invalid_request, param null and a fixed message that catalog discovery is unsupported for BYOK
- **AND** it SHALL NOT list or resolve the catalog, return an empty success, enumerate providers or perform inference

#### Scenario: Authentication still precedes unsupported operation
- **WHEN** an unauthenticated Cloud request asks for discovery
- **THEN** authentication SHALL fail before the application returns an authenticated unsupported-operation response

### Requirement: Bounded TS companion access on the existing route
The repository SHALL provide a documented, typechecked and deterministically tested copyable `fetchConfiguredModels({baseURL, headers, fetch, signal, maxBytes})` consumer helper retaining typed configured-route facts from the existing authenticated `/config` document. It SHALL NOT be a new published TS package or endpoint. The helper SHALL preserve an HTTP(S) API-prefix base URL, reject URL credentials/query/fragment, use explicit private-endpoint JWT outer headers, issue one GET, refuse redirects, honor abort, check success and JSON media type, bound reads incrementally and validate the entire raw UTF-8 JSON document and recognized model/extension fields before return. It SHALL ignore unrelated unknown additive fields rather than expose arbitrary configuration. It SHALL release/cancel reader resources on success or failure and SHALL NOT embed auth headers or arbitrary response bodies in errors, cache catalogs, switch credentials or invoke inference.

Stock exact-pinned `getAvailableModels()` SHALL remain tested as compatible normalized discovery listing canonical rows and stripping the extension, including aliases and provider choices. It SHALL NOT be presented as an access path for these facts; known alias IDs SHALL remain callable. The helper SHALL accept ordinary rows with absent/null gateway; a non-null extension with a JSON shape or field type incompatible with ConfiguredRoute SHALL fail the complete document. It SHALL NOT check route semantics.

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
- **WHEN** the helper is pointed at the Cloud/BYOK endpoint with Cloud credentials
- **THEN** it SHALL surface the non-success discovery response without switching credentials/endpoints or returning an empty catalog

### Requirement: Independent evidence and honest support guidance
This feature SHALL ship startup route-policy tests, server complete-projection tests and independent Go/TS malformed/type-error/oversized/boundary tests, immutable catalog tests, raw HTTP/schema tests and exact-pinned client/real-command discovery tests. Successful and denied discovery tests SHALL assert zero native inference requests. The TS example SHALL be registered in the existing ProviderWire workspace's typecheck/test commands, and Go client tests SHALL remain independent of AGPL implementation imports.

Guidance SHALL distinguish configured candidates from actual attempts/response identity, stock TS normalized discovery from helper access, and configured-account visibility from catalog-independent Cloud/BYOK execution. Scoped fakes and dummy Cloud edges SHALL NOT be presented as deployed customer isolation or live CAP authorization proof. Operator telemetry configuration SHALL remain independent of developer discovery; no discovery feature SHALL enable payload capture or new topology labels. Applicable catalog/discovery/client specs, docs/navigation, parity and module checks SHALL ship with the feature.

#### Scenario: Command proves access without generation
- **WHEN** raw HTTP, pinned TS normalized discovery, the TS helper and Go ListModels exercise configured direct and fallback aliases against the real test command
- **THEN** normalized row compatibility and configured access SHALL be proven with intended authentication and zero native inference calls

#### Scenario: Current Cloud fixture is documented
- **WHEN** deterministic Cloud-auth command tests run against the unified service
- **THEN** discovery SHALL be rejected without catalog access and inference SHALL require BYOK
- **AND** no fixture SHALL preserve the retired Cloud-to-configured-account behavior as a compatibility requirement
