## ADDED Requirements

### Requirement: Catalog-independent request destination
When invoked by a host selector, the internal BYOK engine SHALL require a provider/model selector in the existing ai-language-model-id header. The selector SHALL be valid UTF-8, have a supported case-sensitive provider prefix before the first slash and a nonempty native-model suffix. Existing HTTP header limits SHALL apply without an additional engine-specific byte ceiling. The suffix SHALL be preserved, including additional slashes, as model data rather than a URL. Initial providers SHALL be anthropic and openai, using the native Messages and Responses adapters respectively.

The selector SHALL NOT resolve configured models, aliases, provider instances, endpoints, defaults or fallback routes. A separate service-owned destination approval map MAY authorize an explicitly submitted baseURL; it SHALL NOT supply a default endpoint or account. Advisory native model lists SHALL NOT act as a configured-catalog allowlist. Unknown native models SHALL be decided by the selected native provider without unsolicited inventory or validation calls.

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
The host SHALL accept only call-level providerOptions.gateway.byok, shaped as a provider-name map of ordered nonempty credential-object arrays. Account objects SHALL require apiKey and MAY contain baseURL. OpenAI accounts MAY also contain organization and project; nonempty organization/project SHALL be rejected for Anthropic accounts. Unknown fields SHALL fail through standard Go decoder validation. Optional null values SHALL follow ordinary Go string decoding. Omitted/empty baseURL SHALL select the native default; omitted/empty organization/project SHALL remain unset without ambient defaults. Multiple supported provider entries SHALL be accepted and validated, but only the header-selected provider's credentials SHALL be eligible. Empty/missing BYOK or missing credentials for that provider SHALL fail before provider I/O.

The entire supplied BYOK map SHALL be validated before execution, including unused entries. Supported provider-map keys SHALL be exactly anthropic and openai. Each array SHALL contain 1 to 8 accounts; each decoded key SHALL be nonempty. Decoding SHALL use a Gateway-control map containing provider-to-account maps with typed account structs and the standard Go JSON decoder with DisallowUnknownFields. The only accepted Gateway control SHALL be exactly lowercase byok, as emitted by the supported clients; BYOK and other case variants SHALL fail. Account fields SHALL use the canonical client names apiKey, baseURL, organization and project. No case-insensitive compatibility contract SHALL be introduced for account fields. Null strings SHALL retain their ordinary Go value, and duplicate members SHALL follow ordinary Go decoding. Malformed earlier duplicate members MAY fail typed decoding even when a later member is valid; no JSON normalization layer SHALL be introduced.

The existing complete request bound SHALL govern BYOK input without separate subtree or per-field byte ceilings. Native HTTP transport SHALL enforce outgoing header validity; request decoding SHALL NOT implement an additional credential/account-header grammar. Existing protected call-header and native-option controls SHALL remain unchanged.

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
Unknown credential fields/providers, modelMappings, Azure/Vertex/Bedrock/compatible credential families and unsupported Gateway controls SHALL fail with actionable non-secret diagnostics before provider I/O. models/order/only and providerTimeouts.byok SHALL remain explicitly unsupported until their separate routing delivery. Nested gateway namespaces SHALL NOT be interpreted as credential controls. Native options or body headers SHALL NOT override credential source, destination or the selected model.

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
A supplied nonempty baseURL SHALL match the native provider default or an exact service-owned approval for that provider before construction. Service composition SHALL validate approved destination syntax when loading its policy; the engine SHALL trust that host input rather than repeat URL grammar checks on every account. Approval SHALL NOT use host/path prefixes, wildcards or implicit port/trailing-slash normalization, supply account data or come from client controls. Approvals SHALL be independent of configured accounts/catalogs and SHALL NOT select an endpoint when the account omits it.

This is explicit destination authorization, not proof of public-address enforcement, DNS pinning or deployed proxy/network isolation. Service owners SHALL approve only intended destinations and control their DNS/proxy/network boundary. Transport and redirects SHALL remain service-owned. Approval/configuration activation belongs to the subsequent service change; the engine SHALL NOT expose BYOK HTTP admission in this change.

#### Scenario: Approved account endpoint
- **WHEN** an account supplies a valid exact service-approved baseURL for its provider
- **THEN** its native attempt SHALL use that endpoint with only its own key and account headers

#### Scenario: Unapproved endpoint
- **WHEN** any supplied account, including an unused provider, names an unapproved baseURL
- **THEN** host selection SHALL fail before native I/O with a non-secret diagnostic
- **AND** client-supplied approval controls SHALL NOT grant destination access

### Requirement: Request-scoped provider construction and control removal
Host selection SHALL decode and validate Gateway controls before passing the provider, native model and plain account configs to construction. Construction SHALL NOT parse raw Gateway JSON. Shared native constructors MAY serve configured and request-only execution, but SHALL receive explicit inputs without performing catalog, configuration or secret lookup. Missing model/accounts or an unsupported provider SHALL fail before native construction.

The BYOK selector SHALL construct account-bound models for the request using only the selected supplied accounts and native-default or service-approved destinations. It SHALL NOT inherit configured instances or ambient SDK key/base-URL/account defaults. Shared transports SHALL be credential-independent; redirects SHALL NOT forward credentials. The host SHALL remove gateway controls before generic model middleware and native provider options see them, and forward supported matching native options/content/history without using configured-catalog field allowlists or cross-provider intersections.

No key-bearing model, credential collection, raw request or request observer SHALL be stored in a global cache, catalog or background refresh service. Models and credential data SHALL follow bounded request execution/cleanup ownership; garbage-collection lifetime SHALL NOT be described as cryptographic zeroization.

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
Configured and BYOK construction SHALL share the existing consumption-backed
native-option checks. Anthropic BYOK invocation SHALL validate consumed MCP
configuration and provider-owned history under gateway-anthropic-mcp, and refuse
container skills and native server-side fallback before credential attempts in
both unary and streaming modes. Valid MCP configuration/history and inert local
markers SHALL retain their native semantics. These checks SHALL NOT introduce a
provider-field inventory or change the native SDK adapters.

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
The host SHALL compose selected-provider credentials with the existing fallback module and its default decider, one request-scoped candidate per key for the same provider/model in array order. Each candidate SHALL be invoked at most once, with native SDK retries disabled and one shared execution deadline. No BYOK-specific rejection decider or HTTP-rejection provenance gate SHALL be introduced. Client SDKs SHALL NOT add an account-retry layer.

Retryable API errors and unknown errors SHALL follow the default decider, including its existing context-window exclusion. Non-retryable API errors SHALL stop selection; HTTP 401/403 SHALL NOT receive special eligibility. Native preflight errors SHALL use the adapter's classification. Invalid results and premature stream closure before any part SHALL follow the existing fallback behavior. Cancellation or the shared deadline SHALL stop further attempts. Any first native stream part, including an error part, SHALL commit the candidate; later errors SHALL NOT trigger replay.

Stopped or exhausted attempts SHALL retain existing fallback error propagation and privacy-safe ProviderWire classification, without a blanket exhaustion status or exposure of raw joined errors. Native account failures SHALL NOT become Gateway-authentication failures. No configured account, another provider, another request's key or ambient credential SHALL become eligible. Unknown pre-commit failures may follow provider-side work; no exactly-once execution or charging guarantee SHALL be claimed.

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
