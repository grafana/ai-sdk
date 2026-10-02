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
Configuration loading and server projection SHALL enforce at most 1,024 expanded rows including aliases, 16 candidates per route, 128 aliases per route and 2,048 UTF-8 bytes per identity/display string. Consumers SHALL NOT duplicate these numeric policy ceilings; their resource limit SHALL be the complete document-byte budget. The existing 1–128 ASCII public-ID grammar SHALL remain unchanged. Required identities and names SHALL remain nonblank; optional description SHALL preserve existing empty/absent semantics. Server strings and raw consumer documents SHALL be valid UTF-8. Consumer strings SHALL use standard JSON decoding: Go normalizes escaped lone UTF-16 surrogates to U+FFFD, while TS retains the decoded UTF-16 value. Lossless cross-client identity agreement SHALL NOT be claimed for such escapes. Semantic checks SHALL apply to the decoded values. Complete discovery documents SHALL remain independently bounded by the existing server configurable byte limit, defaulting to 1,048,576 bytes (1 MiB), and the existing Go configurable byte limit, defaulting to 4,194,304 bytes (4 MiB). The TS helper SHALL default to and support at most 4,194,304 bytes (4 MiB), allowing smaller positive safe-integer maxBytes values. This feature SHALL NOT increase the server's existing default or treat either client's larger allowance as server capacity.

Server counts and raw string sizes SHALL be preflighted before row expansion, typed collection allocation and UTF-8 scanning; server encoding SHALL independently check the final complete encoded size including escaping and wrappers before HTTP 200. Consumers SHALL bound raw document reads before decoding and allocating typed results. Configured startup catalogs violating these bounds SHALL fail before readiness; invalid dynamic/custom lister output SHALL fail before success commitment. Malformed documents, duplicate IDs/aliases/candidate tuples, canonical/alias collisions, specification contradictions and inconsistent/incomplete configured route groups SHALL invalidate the entire result. Candidate tuple uniqueness SHALL use provider-instance plus model ID; distinct instances of the same provider/model SHALL remain valid. No path SHALL return a partial catalog or truncate strings/collections to fit. Standard JSON duplicate-member handling SHALL remain distinct from duplicate semantic entries.

#### Scenario: Server policy dimensions reach their boundaries
- **WHEN** a valid configured catalog reaches an exact row, candidate, alias or string ceiling
- **THEN** configuration loading and server projection SHALL accept it if the byte budget and semantic constraints hold
- **WHEN** any policy dimension exceeds its ceiling by one
- **THEN** configuration loading or server projection SHALL fail before readiness or success commitment

#### Scenario: Consumers do not duplicate configuration policy
- **WHEN** a structurally valid and consistent document fits the consumer byte budget but exceeds a server row, candidate, alias or string policy ceiling
- **THEN** the consumer SHALL return the complete catalog without imposing that policy ceiling
- **WHEN** the document exceeds the consumer byte budget by one
- **THEN** the consumer SHALL fail without returning any rows

#### Scenario: Escaped lone surrogates use standard JSON semantics
- **WHEN** a nonblank display or candidate string contains an escaped lone UTF-16 surrogate
- **THEN** Go SHALL decode it as U+FFFD and TS SHALL retain its standard decoded UTF-16 value
- **AND** invalid public IDs, duplicate candidate tuples and inconsistent route groups SHALL still fail based on each consumer's decoded values

#### Scenario: Client allowance exceeds the server budget
- **WHEN** a configured discovery document exceeds the server's existing 1 MiB default but fits the Go or TS helper's 4 MiB default allowance
- **THEN** the server SHALL reject the document atomically before success commitment unless its existing configurable limit was explicitly raised
- **AND** the client allowance SHALL NOT change the server limit

#### Scenario: Escaping expands the document
- **WHEN** individually bounded route facts fit raw-string preflight but repeated aliases and escaping push the final document over its byte limit
- **THEN** no HTTP 200 partial document SHALL be emitted

#### Scenario: Custom listing bypasses startup validation
- **WHEN** a custom lister returns duplicate public IDs, colliding aliases, malformed candidate facts or inconsistent configured groups
- **THEN** discovery SHALL reject the complete listing using the existing fixed internal-error response without exposing invalid data

#### Scenario: Client receives a late malformed row
- **WHEN** a bounded document contains valid initial rows followed by malformed, duplicated or contradictory configured facts
- **THEN** Go and TS configured access SHALL fail atomically without exposing the initial rows

### Requirement: Bounded TS companion access on the existing route
The repository SHALL provide a documented, typechecked and deterministically tested copyable `fetchConfiguredModels({baseURL, headers, fetch, signal, maxBytes})` consumer helper retaining typed configured-route facts from the existing authenticated `/config` document. It SHALL NOT be a new published TS package or endpoint. The helper SHALL preserve an HTTP(S) API-prefix base URL, reject URL credentials/query/fragment, use explicit selected JWT or CAP outer headers, issue one GET, refuse redirects, honor abort, check success and JSON media type, bound reads incrementally and validate the entire raw UTF-8 JSON document and recognized model/extension fields before return. It SHALL ignore unrelated unknown additive fields rather than expose arbitrary configuration. It SHALL release/cancel reader resources on success or failure and SHALL NOT embed auth headers or arbitrary response bodies in errors, cache catalogs, switch credentials or invoke inference.

Stock exact-pinned `getAvailableModels()` SHALL remain tested as compatible normalized discovery that strips the extension, not as an access path for candidate facts. The helper SHALL accept ordinary rows with no gateway extension; a present null, incomplete or malformed extension SHALL fail the complete document.

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
This feature SHALL ship independent server and Go/TS malformed/duplicate/oversized/boundary tests, immutable catalog tests, raw HTTP/schema tests and exact-pinned client/real-command discovery tests. Successful and denied discovery tests SHALL assert zero native inference requests. The TS example SHALL be registered in the existing ProviderWire workspace's typecheck/test commands, and Go client tests SHALL remain independent of AGPL implementation imports.

Guidance SHALL distinguish configured candidates from actual attempts/response identity, stock TS normalized discovery from helper access, and current startup-configured/internal-account visibility from future request-scoped Cloud/BYOK construction. Scoped fakes and dummy Cloud edges SHALL NOT be presented as deployed customer isolation or live CAP authorization proof. Operator telemetry configuration SHALL remain independent of developer discovery; no discovery feature SHALL enable payload capture or new topology labels. Applicable catalog/discovery/client specs, docs/navigation, parity and module checks SHALL ship with the feature.

#### Scenario: Command proves access without generation
- **WHEN** raw HTTP, pinned TS normalized discovery, the TS helper and Go ListModels exercise configured direct and fallback aliases against the real test command
- **THEN** normalized row compatibility and configured access SHALL be proven with intended authentication and zero native inference calls

#### Scenario: Current Cloud fixture is documented
- **WHEN** deterministic Cloud-auth command tests use the current static configured catalog
- **THEN** evidence SHALL identify that fixture's actual boundary without claiming customer account construction, internal-key exclusion for future BYOK or deployed cross-tenant isolation
