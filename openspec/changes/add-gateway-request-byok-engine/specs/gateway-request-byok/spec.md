## ADDED Requirements

### Requirement: Catalog-independent request destination
When invoked by a host selector, the internal BYOK engine SHALL require a provider/model selector in the existing ai-language-model-id header. The selector SHALL be valid UTF-8, at most 2,048 bytes, have a supported case-sensitive provider prefix before the first slash and a nonempty native-model suffix without whitespace or control characters. The suffix SHALL be preserved, including additional slashes, as model data rather than a URL. Initial providers SHALL be anthropic and openai, using the native Messages and Responses adapters respectively.

The selector SHALL NOT resolve configured models, aliases, provider instances, endpoints, defaults or fallback routes. Advisory native model lists SHALL NOT act as a configured-catalog allowlist. Unknown native models SHALL be decided by the selected native provider without unsolicited inventory or validation calls.

#### Scenario: Native model absent from configured catalog
- **WHEN** a valid anthropic/model-name selector and Anthropic request credentials are supplied
- **THEN** the adapter SHALL receive model-name without consulting any catalog, regardless of whether it is configured internally

#### Scenario: Flat configured alias is supplied
- **WHEN** a BYOK caller names assistant or another selector without a provider/model split
- **THEN** selection SHALL fail before I/O rather than expand a configured alias

#### Scenario: Selector boundaries
- **WHEN** a selector has an empty provider/model component, an unsupported provider, forbidden characters or exceeds the byte ceiling
- **THEN** the request SHALL fail with a bounded capability/field diagnostic before provider construction

### Requirement: Vercel-shaped credential ingestion
The host SHALL accept only call-level providerOptions.gateway.byok, shaped as a provider-name map of ordered nonempty credential-object arrays. Initial credential objects SHALL contain only apiKey. Multiple supported provider entries SHALL be accepted and validated, but only the header-selected provider's credentials SHALL be eligible. Empty/missing BYOK or missing credentials for that provider SHALL fail before provider I/O.

The entire supplied BYOK map SHALL be validated before execution, including unused entries. Supported keys SHALL be exactly anthropic and openai. Each array SHALL contain at most 8 credentials; each decoded key SHALL be nonempty, at most 4,096 bytes and a valid credential header value without whitespace/control characters. The raw BYOK subtree SHALL be at most 65,536 bytes and remain within the existing complete request bound. Standard request JSON duplicate-member semantics SHALL remain unchanged.

#### Scenario: Multiple providers and ordered keys
- **WHEN** the map contains two OpenAI credentials and one Anthropic credential and the header selects openai/model
- **THEN** both entries SHALL validate, only OpenAI keys SHALL be eligible, and their array order SHALL be retained

#### Scenario: Malformed unused entry
- **WHEN** the selected provider's credentials are valid but another supplied entry is malformed
- **THEN** validation SHALL reject the request before any native call

#### Scenario: Credential limits
- **WHEN** array cardinality, key bytes or raw subtree bytes reaches its exact ceiling
- **THEN** otherwise valid input SHALL be accepted
- **WHEN** any ceiling is exceeded by one
- **THEN** the request SHALL fail before I/O without echoing the rejected value

#### Scenario: Selected provider lacks credentials
- **WHEN** the header selects OpenAI but only Anthropic credentials are supplied
- **THEN** selection SHALL fail without changing provider or checking configured/environment keys

### Requirement: Unsupported controls fail explicitly
Unknown credential fields/providers, modelMappings, Azure/Vertex/Bedrock/compatible credential families and unsupported Gateway controls SHALL fail with actionable non-secret diagnostics before provider I/O. models/order/only and providerTimeouts.byok SHALL remain explicitly unsupported until their separate routing delivery. Nested gateway namespaces SHALL NOT be interpreted as credential controls. Native options or body headers SHALL NOT override credential source, destination or the selected model.

#### Scenario: Unsupported credential family or mapping
- **WHEN** a request supplies a known but unsupported credential family or modelMappings
- **THEN** it SHALL receive a capability-specific rejection rather than silently ignore the form or use another account

#### Scenario: Routing controls accompany valid BYOK
- **WHEN** valid credentials accompany an unsupported gateway.order or gateway.providerTimeouts control
- **THEN** the request SHALL fail before I/O without suggesting that the control was applied

### Requirement: Request-scoped provider construction and control removal
The BYOK selector SHALL construct account-bound models for the request using only the selected supplied keys and supported fixed native destinations. It SHALL NOT inherit configured instances or ambient SDK key/base-URL/account defaults. Shared transports SHALL be credential-independent; redirects SHALL NOT forward credentials. The host SHALL remove gateway controls before generic model middleware and native provider options see them, and forward supported matching native options/content/history without using configured-catalog field allowlists or cross-provider intersections.

No key-bearing model, credential collection, raw request or request observer SHALL be stored in a global cache, catalog or background refresh service. Models and credential data SHALL follow bounded request execution/cleanup ownership; garbage-collection lifetime SHALL NOT be described as cryptographic zeroization.

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
native-option checks. Anthropic BYOK invocation SHALL refuse MCP servers, container
skills, native server-side fallback and MCP tool-use history before credential
attempts in both unary and streaming modes. These checks SHALL NOT introduce a
provider-field inventory or change the native SDK adapters.

#### Scenario: Native execution control is rejected
- **WHEN** an Anthropic BYOK call supplies a consumed MCP, skill or server-side fallback control
- **THEN** the call SHALL return the existing unsupported-request error without native I/O or credential fallback

#### Scenario: Harmless options are preserved
- **WHEN** native options contain an ordinary container ID or protected-looking fields in non-consuming namespaces
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
