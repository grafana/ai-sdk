# Gateway Configured Discovery

## Purpose

Define authenticated, bounded discovery of explicitly configured route candidates, independent Go and TS consumer access, and the distinction between configured possibilities and runtime evidence.

## Requirements

### Requirement: Explicit configured route projection
Authenticated `GET /api/v1/aisdk/config` SHALL extend each command-configured canonical and alias model row with `gateway: {canonicalModelId, aliases, candidates: [{providerInstance, provider, modelId}]}`. Candidate order SHALL be primary followed by configured fallbacks; alias order SHALL preserve configuration. Canonical and alias rows SHALL carry matching route facts while retaining their individual public `id` and `specification.modelId`, specification version `v4`, provider `grafana`, existing names/descriptions and ascending public-row-ID ordering. Empty aliases SHALL be `[]`; configured candidates SHALL be nonempty. Ordinary catalog entries without candidate metadata SHALL omit the extension rather than infer it.

#### Scenario: Alias and mixed-provider candidates are inspected
- **WHEN** an authorized caller discovers an alias for a route with an Anthropic primary and an OpenAI fallback
- **THEN** canonical and alias rows SHALL expose the same canonical ID, explicit aliases and candidate order/provider-instance/provider/model facts
- **AND** neither row's public specification SHALL be replaced with a native candidate or response identity

#### Scenario: Direct compatible route is inspected
- **WHEN** a configured compatible provider has an explicit providerName or uses its constructor default
- **THEN** its candidate SHALL expose the effective adapter/provider identifier, configured instance reference and exact configured invocation model ID
- **AND** discovery SHALL NOT derive these values from canonical middleware or provider responses

#### Scenario: No inference is needed
- **WHEN** raw HTTP, Go discovery or the TS helper inspects configured candidates
- **THEN** no provider generation, streaming, inventory enumeration or credential-check inference SHALL run

### Requirement: Account-authorized projection excludes credential sources
Discovery SHALL authenticate before listing and SHALL pass the request context to the existing model lister. It SHALL project only entries and candidate facts visible at the same account/route boundary as resolution, without supplementing a scoped list from global configuration. The explicit projection SHALL exclude API keys, environment/secret credential references, signing/workload credential material, caller-auth headers and unrelated account/provider state. Authorized route, candidate, provider-instance, provider and model identifiers SHALL NOT be categorically concealed. This feature SHALL NOT introduce account provisioning or a new policy engine.

#### Scenario: Scoped host listing matches resolution
- **WHEN** a host decorator restricts a request to one route and its configured candidates
- **THEN** discovery SHALL contain only that route's canonical/alias rows and authorized facts, consistent with resolution
- **AND** another request's entries SHALL NOT enter the projection or retained shared state

#### Scenario: Known credential sources are configured
- **WHEN** provider configuration contains distinct dummy API keys, secret references and unrelated provider entries
- **THEN** discovery SHALL expose only the approved visible route/candidate fields, without those credential sources or unrelated entries
- **AND** ordinary key-looking display strings and authorized provider/model identifiers SHALL remain intact

#### Scenario: Authentication fails
- **WHEN** the existing authentication layer rejects a discovery request
- **THEN** no catalog listing or provider work SHALL occur and the existing bounded authentication response SHALL apply

### Requirement: Atomic discovery resource and consistency limits
Configuration loading SHALL validate route semantics before readiness: at most 1,024 expanded rows including aliases, 16 candidates per route, 128 aliases per route and 2,048 UTF-8 bytes per identity/display string. The existing 1–128 ASCII public-ID grammar SHALL remain unchanged. Required identities and names SHALL be nonblank; strings SHALL be valid UTF-8; optional description SHALL preserve existing empty/absent semantics. Duplicate IDs/aliases/candidate tuples, canonical/alias collisions and unknown provider references SHALL fail startup. Candidate tuple uniqueness SHALL use provider-instance plus model ID; distinct instances of the same provider/model SHALL remain valid.

The discovery handler SHALL project the complete visible configured catalog without revalidating route semantics or imposing a discovery response-byte cap. Hosts supplying custom listers SHALL own catalog validity before serving. The response SHALL retain closed credential-safe fields and consistent canonical/alias specifications by construction. No path SHALL truncate strings or collections.

Consumers SHALL decode typed metadata without duplicating server ID, nonblank-string, cardinality, uniqueness or route-consistency policy. They SHALL independently bound raw UTF-8 JSON reads before decoding: Go defaults to a configurable 4,194,304 bytes (4 MiB); the TS helper defaults to and supports at most 4,194,304 bytes (4 MiB), allowing smaller positive safe-integer maxBytes values. Byte overflow, malformed JSON and type-decoding errors SHALL return no partial catalog. Standard JSON string and duplicate-member semantics SHALL apply: Go normalizes escaped lone UTF-16 surrogates to U+FFFD, while TS retains the decoded UTF-16 value. Lossless cross-client identity agreement SHALL NOT be claimed for such escapes.

#### Scenario: Server policy dimensions reach their boundaries
- **WHEN** a configured catalog reaches an exact row, candidate, alias or string ceiling
- **THEN** configuration loading SHALL accept it if semantic constraints hold
- **WHEN** any policy dimension exceeds its ceiling by one
- **THEN** configuration loading SHALL fail before readiness

#### Scenario: Invalid routes fail startup
- **WHEN** configuration contains invalid IDs, duplicate aliases/candidate tuples, canonical/alias collisions or blank required identifiers
- **THEN** startup SHALL fail before listener creation or provider inference
- **AND** response building SHALL NOT repeat those checks

#### Scenario: Configured catalog exceeds the former response budget
- **WHEN** a valid configured discovery document exceeds 1 MiB after alias expansion and JSON escaping
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
The repository SHALL provide a documented, typechecked and deterministically tested copyable `fetchConfiguredModels({baseURL, headers, fetch, signal, maxBytes})` consumer helper retaining typed configured-route facts from the existing authenticated `/config` document. It SHALL NOT be a new published TS package or endpoint. The helper SHALL preserve an HTTP(S) API-prefix base URL, reject URL credentials/query/fragment, use explicit selected JWT or CAP outer headers, issue one GET, refuse redirects, honor abort, check success and JSON media type, bound reads incrementally and validate the entire raw UTF-8 JSON document and recognized model/extension fields before return. It SHALL ignore unrelated unknown additive fields rather than expose arbitrary configuration. It SHALL release/cancel reader resources on success or failure and SHALL NOT embed auth headers or arbitrary response bodies in errors, cache catalogs, switch credentials or invoke inference.

Stock exact-pinned `getAvailableModels()` SHALL remain tested as compatible normalized discovery that strips the extension, not as an access path for candidate facts. The helper SHALL accept ordinary rows with absent/null gateway; a non-null extension with a JSON shape or field type incompatible with ConfiguredRoute SHALL fail the complete document. It SHALL NOT check route semantics.

#### Scenario: Both TS discovery surfaces are demonstrated
- **WHEN** the registered Gateway client and shipped helper read the same configured response
- **THEN** normalized discovery SHALL preserve its existing public fields and discard gateway extras
- **AND** the helper SHALL expose typed canonical/alias/candidate facts through an executable documented access expression

#### Scenario: TS bounded read is interrupted
- **WHEN** the byte cap is crossed, the caller aborts, the transport fails or the response is invalid
- **THEN** the helper SHALL release/cancel its body reader and return no document, without retry or inference

#### Scenario: Endpoint attempts credential forwarding
- **WHEN** discovery redirects to another endpoint
- **THEN** the helper SHALL refuse the redirect without sending credentials to the redirect target

### Requirement: Independent evidence and honest support guidance
This feature SHALL ship startup route-policy tests, server complete-projection tests and independent Go/TS malformed/type-error/oversized/boundary tests, immutable catalog tests, raw HTTP/schema tests and exact-pinned client/real-command discovery tests. Successful and denied discovery tests SHALL assert zero native inference requests. The TS example SHALL be registered in the existing ProviderWire workspace's typecheck/test commands, and Go client tests SHALL remain independent of AGPL implementation imports.

Guidance SHALL distinguish configured candidates from actual attempts/response identity, stock TS normalized discovery from helper access, and current startup-configured/internal-account visibility from future request-scoped Cloud/BYOK construction. Scoped fakes and dummy Cloud edges SHALL NOT be presented as deployed customer isolation or live CAP authorization proof. Operator telemetry configuration SHALL remain independent of developer discovery; no discovery feature SHALL enable payload capture or new topology labels. Applicable catalog/discovery/client specs, docs/navigation, parity and module checks SHALL ship with the feature.

#### Scenario: Command proves access without generation
- **WHEN** raw HTTP, pinned TS normalized discovery, the TS helper and Go ListModels exercise configured direct and fallback aliases against the real test command
- **THEN** normalized row compatibility and configured access SHALL be proven with intended authentication and zero native inference calls

#### Scenario: Current Cloud fixture is documented
- **WHEN** deterministic Cloud-auth command tests use the current static configured catalog
- **THEN** evidence SHALL identify that fixture's actual boundary without claiming customer account construction, internal-key exclusion for future BYOK or deployed cross-tenant isolation
