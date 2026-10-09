# gateway-request-byok Specification

## Purpose

Define catalog-independent native account selection, destination approval, request isolation and bounded fallback for the internal Gateway BYOK engine, separately from authenticated service admission and deployment activation.

## Requirements

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

### Requirement: Catalog-independent request destination
The BYOK engine SHALL select a supported native provider/model from request data, preserve the native suffix and remain independent of configured catalogs, accounts and defaults.

#### Scenario: Native destination policy
- **WHEN** a host selector processes a BYOK model destination
- **THEN** When invoked by a host selector, the internal BYOK engine SHALL require a provider/model selector in the existing ai-language-model-id header.
- **AND** The selector SHALL be valid UTF-8, have a supported case-sensitive provider prefix before the first slash and a nonempty native-model suffix.
- **AND** Existing HTTP header limits SHALL apply without an additional engine-specific byte ceiling.
- **AND** The suffix SHALL be preserved, including additional slashes, as model data rather than a URL.
- **AND** Initial providers SHALL be anthropic and openai, using the native Messages and Responses adapters respectively.
- **AND** The selector SHALL NOT resolve configured models, aliases, provider instances, endpoints, defaults or fallback routes.
- **AND** A separate service-owned destination approval map MAY authorize an explicitly submitted baseURL; it SHALL NOT supply a default endpoint or account.
- **AND** Advisory native model lists SHALL NOT act as a configured-catalog allowlist.
- **AND** Unknown native models SHALL be decided by the selected native provider without unsolicited inventory or validation calls.

#### Scenario: Native model absent from configured catalog
- **WHEN** a valid anthropic/model-name selector and Anthropic request credentials are supplied
- **THEN** the adapter SHALL receive model-name without consulting any catalog, regardless of whether it is configured internally

#### Scenario: Flat configured alias is supplied
- **WHEN** a BYOK caller names assistant or another selector without a provider/model split
- **THEN** selection SHALL fail before I/O rather than expand a configured alias

#### Scenario: Selector boundaries
- **WHEN** a selector has an empty provider/model component, an unsupported provider or invalid UTF-8
- **THEN** the request SHALL fail with a bounded capability/field diagnostic before provider construction

### Requirement: Vercel-shaped credential ingestion
The host SHALL validate the complete call-level gateway.byok map with ordinary typed Go decoding, including unused accounts, canonical controls, required keys and provider/count policy. Only selected-provider accounts SHALL become eligible; request and native-header bounds SHALL remain shared.

#### Scenario: Complete credential validation policy
- **WHEN** call-level BYOK controls are decoded before execution
- **THEN** The host SHALL accept only call-level providerOptions.gateway.byok, shaped as a provider-name map of ordered nonempty credential-object arrays.
- **AND** Account objects SHALL require apiKey and MAY contain baseURL.
- **AND** OpenAI accounts MAY also contain organization and project; nonempty organization/project SHALL be rejected for Anthropic accounts.
- **AND** Unrelated additive account fields SHALL be ignored by ordinary Go decoding without applying them to native construction.
- **AND** Optional null values SHALL follow ordinary Go string decoding.
- **AND** Omitted/empty baseURL SHALL select the native default; omitted/empty organization/project SHALL remain unset without ambient defaults.
- **AND** Multiple supported provider entries SHALL be accepted and validated, but only the header-selected provider's credentials SHALL be eligible.
- **AND** Empty/missing BYOK or missing credentials for that provider SHALL fail before provider I/O.
- **AND** The entire supplied BYOK map SHALL be validated before execution, including unused entries.
- **AND** Supported provider-map keys SHALL be exactly anthropic and openai.
- **AND** Each decoded key SHALL be nonempty. The existing eight-account bound SHALL remain temporarily as execution policy, not JSON grammar; #394/#317 own its later disposition.
- **AND** Decoding SHALL use provider-to-account maps and ordinary typed Go decoding without blanket unknown-field rejection.
- **AND** The only accepted Gateway control SHALL be exactly lowercase byok, as emitted by the supported clients; BYOK and other case variants SHALL fail.
- **AND** Account fields SHALL use the canonical client names apiKey, baseURL, organization and project.
- **AND** Account struct fields SHALL retain standard Go case-insensitive matching; namespace/map keys and discriminator values SHALL NOT be normalized.
- **AND** Null strings SHALL retain their ordinary Go value, and duplicate members SHALL follow ordinary Go decoding.
- **AND** Malformed earlier duplicate members MAY fail typed decoding even when a later member is valid; no JSON normalization layer SHALL be introduced.
- **AND** The existing complete request bound SHALL govern BYOK input without separate subtree or per-field byte ceilings.
- **AND** Native HTTP transport SHALL enforce outgoing header validity; request decoding SHALL NOT implement an additional credential/account-header grammar.
- **AND** Existing protected call-header and native-option controls SHALL remain unchanged.

#### Scenario: Ordinary account decoding accepts additive fields
- **WHEN** known account fields use ordinary struct-field casing alongside unrelated additive fields
- **THEN** known values SHALL decode normally, additive data SHALL not select credentials/models/transports, and duplicate/null behavior SHALL remain ordinary Go semantics

#### Scenario: Mapping control is inactive or meaningful
- **WHEN** modelMappings is absent, null or an empty list
- **THEN** native construction SHALL use the explicitly selected model
- **WHEN** a nonempty or malformed mapping control is supplied
- **THEN** decoding SHALL reject the unsupported capability before I/O without inspecting nested mapping entries or exposing rejected values

#### Scenario: Native account defaults
- **WHEN** an account omits optional configuration or supplies empty optional strings
- **THEN** native endpoint defaults SHALL apply and OpenAI organization/project headers SHALL remain unset, without reading environment variables

#### Scenario: OpenAI account overrides
- **WHEN** an OpenAI account supplies valid organization and project
- **THEN** only that account's native headers SHALL carry them in unary and streaming attempts, not the inference JSON body

#### Scenario: Multiple providers and ordered keys
- **WHEN** the map contains two OpenAI credentials and one Anthropic credential and the header selects openai/model
- **THEN** both entries SHALL validate, only OpenAI keys SHALL be eligible, and their array order SHALL be retained

#### Scenario: Malformed unused entry
- **WHEN** the selected provider's credentials are valid but another supplied entry is malformed
- **THEN** validation SHALL reject the request before any native call

#### Scenario: Account count
- **WHEN** a supplied provider array contains 1 to 8 accounts
- **THEN** otherwise valid input SHALL be accepted
- **WHEN** a supplied array is empty or contains more than 8 accounts, including an unused provider
- **THEN** the request SHALL fail before I/O without echoing the rejected value

#### Scenario: Shared request bound
- **WHEN** a BYOK request reaches the configured complete request byte limit
- **THEN** otherwise valid input SHALL reach selection without a separate BYOK byte ceiling
- **WHEN** that limit is exceeded by one byte
- **THEN** the wire handler SHALL reject the request before selection

#### Scenario: Native header validity
- **WHEN** a decoded account contains an invalid outgoing HTTP header value
- **THEN** the native HTTP transport SHALL refuse the attempt before sending the request, using existing fallback/error handling

#### Scenario: Selected provider lacks credentials
- **WHEN** the header selects OpenAI but only Anthropic credentials are supplied
- **THEN** selection SHALL fail without changing provider or checking configured/environment keys

### Requirement: Unsupported controls fail explicitly
Unsupported credentials, mappings and controls SHALL fail with non-secret diagnostics before I/O. Native options and headers SHALL NOT override the selected account, model or destination.

#### Scenario: Unsupported-control policy
- **WHEN** BYOK input contains provider, mapping or routing controls
- **THEN** Unsupported provider selection, meaningful modelMappings, Azure/Vertex/Bedrock/compatible credential families and requested unsupported Gateway controls SHALL fail before provider I/O with actionable diagnostics. Unrelated additive account fields SHALL NOT cause failure. models/order/only and providerTimeouts.byok remain unsupported pending separate routing delivery.
- **AND** Nested gateway namespaces SHALL NOT be interpreted as credential controls.
- **AND** Native options or body headers SHALL NOT override credential source, destination or the selected model.

#### Scenario: Unsupported credential family or mapping
- **WHEN** a request supplies a known but unsupported credential family or modelMappings
- **THEN** it SHALL receive a capability-specific rejection rather than silently ignore the form or use another account

#### Scenario: Canonical credential control
- **WHEN** a request supplies BYOK or another case variant, alone or alongside canonical byok
- **THEN** decoding SHALL fail before execution rather than accept an undocumented alias outside the capture boundary

#### Scenario: Routing controls accompany valid BYOK
- **WHEN** valid credentials accompany an unsupported gateway.order or gateway.providerTimeouts control
- **THEN** the request SHALL fail before I/O without suggesting that the control was applied

### Requirement: Service-approved account destinations
Account destinations SHALL use native defaults or exact provider-scoped host approval. Approval SHALL supply permission only, remain catalog-independent and retain service-owned transport and deployment boundaries.

#### Scenario: Destination approval policy
- **WHEN** a request supplies or omits an account baseURL
- **THEN** A supplied nonempty baseURL SHALL match the native provider default or an exact service-owned approval for that provider before construction.
- **AND** Service composition SHALL validate approved destination syntax when loading its policy; the engine SHALL trust that host input rather than repeat URL grammar checks on every account.
- **AND** Approval SHALL NOT use host/path prefixes, wildcards or implicit port/trailing-slash normalization, supply account data or come from client controls.
- **AND** Approvals SHALL be independent of configured accounts/catalogs and SHALL NOT select an endpoint when the account omits it.
- **AND** This is explicit destination authorization, not proof of public-address enforcement, DNS pinning or deployed proxy/network isolation.
- **AND** Service owners SHALL approve only intended destinations and control their DNS/proxy/network boundary.
- **AND** Transport and redirects SHALL remain service-owned.
- **AND** Approval/configuration activation belongs to the subsequent service change; the engine SHALL NOT expose BYOK HTTP admission in this change.

#### Scenario: Approved account endpoint
- **WHEN** an account supplies a valid exact service-approved baseURL for its provider
- **THEN** its native attempt SHALL use that endpoint with only its own key and account headers

#### Scenario: Unapproved endpoint
- **WHEN** any supplied account, including an unused provider, names an unapproved baseURL
- **THEN** host selection SHALL fail before native I/O with a non-secret diagnostic
- **AND** client-supplied approval controls SHALL NOT grant destination access

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
- **AND** no BYOK map, OpenAI key, Gateway JWT/CAP or configured key SHALL enter its native JSON body or headers

#### Scenario: Native content is preserved
- **WHEN** a BYOK request contains supported native options, reasoning/file/tool history or continuation
- **THEN** the chosen adapter SHALL receive its matching ordinary options and supported content unchanged, apart from existing documented provider mappings

#### Scenario: Concurrent customer isolation
- **WHEN** concurrent requests select the same provider/model with different keys
- **THEN** each native request SHALL use only its own key and SHALL NOT mutate shared clients/models

#### Scenario: Cancellation and late setup
- **WHEN** cancellation occurs during native setup or stream delivery
- **THEN** no later credential SHALL start, late results SHALL be cleaned up through bounded ownership/drain and no request credential SHALL be cached

### Requirement: Shared native execution protections
Configured and request-only models SHALL share consumption-backed native protection. Valid Anthropic MCP/history and inert markers SHALL survive; unsupported consumed execution controls SHALL fail before attempts without new field inventories or adapter changes.

#### Scenario: Native consumption policy
- **WHEN** an Anthropic request-only model receives native options and history
- **THEN** Configured and BYOK construction SHALL share the existing consumption-backed native-option checks.
- **AND** Anthropic BYOK invocation SHALL validate consumed MCP configuration and provider-owned history under gateway-anthropic-mcp, and refuse container skills and native server-side fallback before credential attempts in both unary and streaming modes.
- **AND** Valid MCP configuration/history and inert local markers SHALL retain their native semantics.
- **AND** These checks SHALL NOT introduce a provider-field inventory or change the native SDK adapters.

#### Scenario: Native execution control is rejected
- **WHEN** an Anthropic BYOK call supplies invalid consumed MCP configuration/history, container skills or native server-side fallback
- **THEN** the call SHALL return the existing unsupported-request error without native I/O or credential fallback

#### Scenario: Valid MCP configuration and history
- **WHEN** an Anthropic BYOK call supplies valid MCP configuration and matching provider-owned history
- **THEN** the native request SHALL preserve them without changing the inference endpoint, model or credential

#### Scenario: Harmless options are preserved
- **WHEN** native options contain an ordinary container ID, inert local MCP markers or protected-looking fields in non-consuming namespaces
- **THEN** the adapter SHALL receive ordinary options without changing the selected model, account or destination

### Requirement: Ordered credentials reuse default fallback
Ordered selected-provider accounts SHALL reuse the default fallback policy without a second retry layer. Attempts SHALL share the deadline and stop after cancellation or commitment; account failures SHALL retain safe native classification without substitution or exactly-once claims.

#### Scenario: Credential fallback policy
- **WHEN** a request-only model executes its ordered account candidates
- **THEN** The host SHALL compose selected-provider credentials with the existing fallback module and its default decider, one request-scoped candidate per key for the same provider/model in array order.
- **AND** Each candidate SHALL be invoked at most once, with native SDK retries disabled and one shared execution deadline.
- **AND** No BYOK-specific rejection decider or HTTP-rejection provenance gate SHALL be introduced.
- **AND** Client SDKs SHALL NOT add an account-retry layer.
- **AND** Retryable API errors and unknown errors SHALL follow the default decider, including its existing context-window exclusion.
- **AND** Non-retryable API errors SHALL stop selection; HTTP 401/403 SHALL NOT receive special eligibility.
- **AND** Native preflight errors SHALL use the adapter's classification.
- **AND** Invalid results and premature stream closure before any part SHALL follow the existing fallback behavior.
- **AND** Cancellation or the shared deadline SHALL stop further attempts.
- **AND** Any first native stream part, including an error part, SHALL commit the candidate; later errors SHALL NOT trigger replay.
- **AND** Stopped or exhausted attempts SHALL retain existing fallback error propagation and privacy-safe ProviderWire classification, without a blanket exhaustion status or exposure of raw joined errors.
- **AND** Native account failures SHALL NOT become Gateway-authentication failures.
- **AND** No configured account, another provider, another request's key or ambient credential SHALL become eligible.
- **AND** Unknown pre-commit failures may follow provider-side work; no exactly-once execution or charging guarantee SHALL be claimed.

#### Scenario: Retryable failure advances
- **WHEN** the first candidate returns a retryable API error such as a classified 429 or 503 before unary success or any stream part and the second succeeds
- **THEN** the second candidate SHALL run once with the same provider/model and supported request content

#### Scenario: Non-retryable authentication failure stops
- **WHEN** the first candidate returns a non-retryable 401/403 API error
- **THEN** no later key SHALL be attempted, using the default decider rather than a credential-rotation exception

#### Scenario: Native preflight error follows classification
- **WHEN** OpenAI returns an HTTP-200 initial SSE error, including one after response.in_progress, before exposing any native part
- **THEN** the returned adapter error SHALL be evaluated by the default decider
- **AND** a non-retryable synthesized authentication/permission error SHALL stop without requiring separate HTTP-status provenance

#### Scenario: Unknown failure or invalid stream setup
- **WHEN** a candidate returns an unknown setup error, invalid result or stream closure before any part
- **THEN** advancement SHALL match the existing fallback module under the shared execution deadline

#### Scenario: Context-window exclusion
- **WHEN** a candidate error matches the default decider's existing context-window exclusion
- **THEN** no later candidate SHALL be attempted

#### Scenario: Candidates are exhausted
- **WHEN** every candidate fails with an eligible error
- **THEN** attempts SHALL stop at the supplied array bound and returned errors SHALL follow existing safe classification without configured-account fallback

#### Scenario: Stream is already committed
- **WHEN** any native stream part has been observed, including an initial error part
- **THEN** that candidate SHALL remain selected and subsequent errors or closure SHALL NOT replay the request

#### Scenario: Request is canceled
- **WHEN** request cancellation or the shared deadline becomes observable during selection or an attempt
- **THEN** no later candidate SHALL start and existing bounded cleanup SHALL apply

#### Scenario: Continuation is a separate request
- **WHEN** a subsequent call supplies supported continuation and a new ordered credential array
- **THEN** it SHALL use that request's credentials without implicit persisted account affinity
