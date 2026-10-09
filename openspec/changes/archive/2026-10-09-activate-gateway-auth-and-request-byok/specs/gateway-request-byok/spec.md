## ADDED Requirements

### Requirement: Authenticated request-only destination
Authenticated BYOK SHALL use the shared engine's native provider/model selection without configured lookup, new selector grammar or extra byte ceilings. Native model suffixes and provider-owned availability SHALL remain unchanged.

#### Scenario: Authenticated destination policy
- **WHEN** a Cloud caller requests native BYOK inference
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

### Requirement: Request-only native consumption guards
Request-only invocation SHALL retain shared consumption-backed guards before attempts. Valid bounded MCP/history, inert markers and unrelated namespaces SHALL survive; invalid consumed state, skills and native fallback SHALL fail without catalog inventories.

#### Scenario: Request-only consumption policy
- **WHEN** native options and history reach the BYOK model
- **THEN** BYOK invocation SHALL retain gateway-native-provider-options protections at the actual consuming namespace and scope without restoring catalog inventories.
- **AND** Invalid consumed Anthropic MCP configuration/history, container skills and provider-side fallback controls SHALL fail before native I/O in both invocation modes.
- **AND** Valid bounded MCP configuration and configured provider-executed history SHALL remain supported.
- **AND** Harmless ignored fields and unrelated namespaces SHALL NOT be rejected by blanket spelling rules.

#### Scenario: Consumed execution bypass is refused
- **WHEN** an Anthropic BYOK request supplies invalid consumed MCP configuration/history, a container skill or a provider-side fallback control
- **THEN** it SHALL return the fixed invalid-request error without any credential attempt

#### Scenario: Ordinary native fields survive
- **WHEN** native options carry a safe container ID or ignored protected-looking fields in non-consuming scopes
- **THEN** the request SHALL reach the adapter without changing its selected model, account or destination

#### Scenario: Valid MCP configuration and history survive
- **WHEN** an Anthropic BYOK request supplies valid bounded MCP servers and corresponding provider-executed history
- **THEN** the native adapter SHALL receive the MCP configuration and history without consulting configured accounts

### Requirement: BYOK capture policy is independent of returned data
Host account authorization, explicit credential sources and request isolation SHALL remain intact. Logger field redaction and other exporter capture SHALL be independent of returned provider data, including producer credential echoes. Native summaries SHALL not require catalog provenance; missing request-account observation SHALL remain an explicit capability gap.

#### Scenario: BYOK privacy and attribution policy
- **WHEN** the host processes request-only credentials and observes execution
- **THEN** Host controls SHALL be consumed for account selection rather than forwarded wholesale to native inference.
- **AND** Logging SHALL use configured field-aware redaction; other exporters SHALL own their capture policies, and metric labels SHALL NOT contain credential payloads.
- **AND** Returned provider data SHALL not be rewritten or matched against credential inventories, including producer-originated caller/service credential echoes.
- **AND** The host SHALL NOT synthesize diagnostics from an internal credential store or another request's state.
- **AND** Supported provider/model identity, application content and native response values SHALL remain available under their existing contracts.
- **AND** Configured execution attribution SHALL remain independent of logical BYOK observation.
- **AND** Event-local native SSE summaries SHALL remain eligible without catalog-owned attribution. BYOK unary/setup summaries and attempt overviews are currently unavailable; #317 owns reusing existing SDK observation with honest request identity, not a new collector.
- **AND** Independently registered SDK observers SHALL remain effective; original native metadata SHALL remain opaque and SHALL NOT establish configured provenance.
- **AND** Diagnostic messages SHALL use bounded approved capability names/schema paths and bounded indices/counts, never arbitrary rejected key names, values or serialized bodies.
- **AND** Detailed public attempt evidence SHALL remain owned by its separate contract; this capability SHALL NOT introduce a second metadata/error protocol.

#### Scenario: Provider echoes the rejected key
- **WHEN** a fake provider rejection includes the selected dummy API key in authentication-related error material
- **THEN** available native summaries SHALL retain that echo without changing Gateway classification or account isolation
- **AND** logger sensitive-field policy and metadata-only exporter exclusions SHALL be tested independently from caller output

#### Scenario: Ordinary application text resembles a key
- **WHEN** supported prompt/output text contains a key-looking string unrelated to authentication material
- **THEN** the response SHALL preserve that content rather than apply token-pattern censorship

### Requirement: Evidence remains independent and provenance-correct
Acceptance SHALL retain independent pinned-client/native-HTTP/service/SDK/race witnesses with honest proof boundaries. Synthetic credential tests SHALL NOT become recorded provider evidence; baseline/parity checks and deferred support SHALL remain explicit.

#### Scenario: BYOK evidence policy
- **WHEN** the delivery's acceptance evidence is collected or summarized
- **THEN** Acceptance SHALL include exact-pinned Vercel HTTP captures, independent Go-client tests without AGPL imports, fake native HTTP requests, real unified-command tests, middleware sink tests and race/lifetime tests.
- **AND** Synthetic credential cases SHALL use dummy values in focused tests, not invented recorded/upstream conformance inputs.
- **AND** Existing authentic provider inputs SHALL remain unchanged.
- **AND** Applicable parity/module/frontend checks SHALL run, with coverage gaps and deferred families/routing explicitly recorded.

#### Scenario: Catalog is inaccessible to BYOK
- **WHEN** a spy catalog that fails on every call accompanies BYOK inference and discovery tests
- **THEN** inference SHALL work from request data and discovery SHALL be explicitly rejected, with zero catalog calls in both cases

#### Scenario: Provider behavior is claimed
- **WHEN** evidence is summarized
- **THEN** fake HTTP and registered-client evidence SHALL be distinguished from live native acceptance and Vercel private-service behavior
