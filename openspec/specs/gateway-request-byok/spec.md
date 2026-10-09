# gateway-request-byok Specification

## Purpose

Define catalog-independent native account selection, destination approval, request isolation and bounded fallback for the internal Gateway BYOK engine, separately from authenticated service admission and deployment activation.

## Requirements

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
- **AND** Unknown fields SHALL fail through standard Go decoder validation.
- **AND** Optional null values SHALL follow ordinary Go string decoding.
- **AND** Omitted/empty baseURL SHALL select the native default; omitted/empty organization/project SHALL remain unset without ambient defaults.
- **AND** Multiple supported provider entries SHALL be accepted and validated, but only the header-selected provider's credentials SHALL be eligible.
- **AND** Empty/missing BYOK or missing credentials for that provider SHALL fail before provider I/O.
- **AND** The entire supplied BYOK map SHALL be validated before execution, including unused entries.
- **AND** Supported provider-map keys SHALL be exactly anthropic and openai.
- **AND** Each array SHALL contain 1 to 8 accounts; each decoded key SHALL be nonempty.
- **AND** Decoding SHALL use a Gateway-control map containing provider-to-account maps with typed account structs and the standard Go JSON decoder with DisallowUnknownFields.
- **AND** The only accepted Gateway control SHALL be exactly lowercase byok, as emitted by the supported clients; BYOK and other case variants SHALL fail.
- **AND** Account fields SHALL use the canonical client names apiKey, baseURL, organization and project.
- **AND** No case-insensitive compatibility contract SHALL be introduced for account fields.
- **AND** Null strings SHALL retain their ordinary Go value, and duplicate members SHALL follow ordinary Go decoding.
- **AND** Malformed earlier duplicate members MAY fail typed decoding even when a later member is valid; no JSON normalization layer SHALL be introduced.
- **AND** The existing complete request bound SHALL govern BYOK input without separate subtree or per-field byte ceilings.
- **AND** Native HTTP transport SHALL enforce outgoing header validity; request decoding SHALL NOT implement an additional credential/account-header grammar.
- **AND** Existing protected call-header and native-option controls SHALL remain unchanged.

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
- **THEN** Unknown credential fields/providers, modelMappings, Azure/Vertex/Bedrock/compatible credential families and unsupported Gateway controls SHALL fail with actionable non-secret diagnostics before provider I/O. models/order/only and providerTimeouts.byok SHALL remain explicitly unsupported until their separate routing delivery.
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
