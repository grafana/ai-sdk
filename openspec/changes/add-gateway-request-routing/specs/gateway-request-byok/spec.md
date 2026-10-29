# Spec Delta

## MODIFIED Requirements

### Requirement: Vercel-shaped credential ingestion
The host SHALL validate the complete call-level gateway.byok map with ordinary typed Go decoding, including unused accounts, canonical controls, required keys and the selected provider's account count. Only accounts of the selected provider and of the providers named by gateway-request-routing fallback models SHALL become eligible; request and native-header bounds SHALL remain shared.

#### Scenario: Complete credential validation policy
- **WHEN** call-level BYOK controls are decoded before execution
- **THEN** The host SHALL accept only call-level providerOptions.gateway.byok, shaped as a provider-name map of ordered nonempty credential-object arrays.
- **AND** Account objects SHALL require apiKey and MAY contain baseURL.
- **AND** OpenAI accounts MAY also contain organization and project; nonempty organization/project SHALL be rejected for Anthropic accounts.
- **AND** Unrelated additive account fields SHALL be ignored by ordinary Go decoding without applying them to native construction.
- **AND** Optional null values SHALL follow ordinary Go string decoding.
- **AND** Omitted/empty baseURL SHALL select the native default; omitted/empty organization/project SHALL remain unset without ambient defaults.
- **AND** Multiple supported provider entries SHALL be accepted and validated, but only the credentials of the header-selected provider and of providers named by `models` entries SHALL be eligible.
- **AND** Empty/missing BYOK or missing credentials for that provider SHALL fail before provider I/O.
- **AND** The entire supplied BYOK map SHALL be validated before execution, including unused entries.
- **AND** Supported provider-map keys SHALL be exactly anthropic and openai.
- **AND** Each decoded key SHALL be nonempty. The shared provider attempt bound of 8 under gateway-resource-policy SHALL apply to the selected provider's array, and to the composed routed chain under gateway-request-routing, as execution attempt policy rather than JSON grammar; an unused provider array SHALL NOT be rejected for its length.
- **AND** Decoding SHALL use provider-to-account maps and ordinary typed Go decoding without blanket unknown-field rejection.
- **AND** The accepted Gateway controls SHALL be byok plus the routing controls defined by gateway-request-routing, matched as ordinary Go decoding matches struct fields; the supported clients emit the lowercase names.
- **AND** Account fields SHALL use the canonical client names apiKey, baseURL, organization and project.
- **AND** Account struct fields SHALL retain standard Go case-insensitive matching; namespace/map keys and discriminator values SHALL NOT be normalized.
- **AND** Null strings SHALL retain their ordinary Go value, and duplicate members SHALL follow ordinary Go decoding.
- **AND** Malformed earlier duplicate members MAY fail typed decoding even when a later member is valid; no JSON normalization layer SHALL be introduced.
- **AND** The platform request size under gateway-resource-policy SHALL govern BYOK input without separate subtree or per-field byte ceilings.
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
- **WHEN** the selected provider's array contains 1 to 8 accounts
- **THEN** otherwise valid input SHALL be accepted
- **WHEN** an unused provider's array contains 9 or more otherwise valid accounts
- **THEN** otherwise valid input SHALL be accepted and those accounts SHALL NOT be eligible
- **WHEN** any supplied array is empty, or the selected provider's array contains more than 8 accounts
- **THEN** the request SHALL fail with the fixed BYOK credentials invalid-request document before I/O, with zero native calls and without echoing the rejected value

#### Scenario: Shared request bound
- **WHEN** a BYOK request is above 1 MiB and within the host route message size
- **THEN** otherwise valid input SHALL reach selection without a separate BYOK byte ceiling
- **WHEN** a BYOK request exceeds the host route message size
- **THEN** the host SHALL reject it before selection, with zero native calls

#### Scenario: Native header validity
- **WHEN** a decoded account contains an invalid outgoing HTTP header value
- **THEN** the native HTTP transport SHALL refuse the attempt before sending the request, using existing fallback/error handling

#### Scenario: Selected provider lacks credentials
- **WHEN** the header selects OpenAI but only Anthropic credentials are supplied
- **THEN** selection SHALL fail without changing provider or checking configured/environment keys

#### Scenario: Fallback model provider accounts become eligible
- **WHEN** the header selects anthropic/x, `models` is ["openai/y"], and byok supplies Anthropic and OpenAI accounts
- **THEN** both providers' accounts SHALL be eligible, each only for its own provider's models, in array order

### Requirement: Unsupported controls fail explicitly
Unsupported credentials, mappings and controls SHALL fail with non-secret diagnostics before I/O. Native options and headers SHALL NOT override the selected account, model or destination.

#### Scenario: Unsupported-control policy
- **WHEN** BYOK input contains provider, mapping or routing controls
- **THEN** Unsupported provider selection, meaningful modelMappings, Azure/Vertex/Bedrock/compatible credential families and requested unsupported Gateway controls SHALL fail before provider I/O with actionable diagnostics. Unrelated additive account fields SHALL NOT cause failure. models, order and only SHALL follow gateway-request-routing; every other Gateway control, including providerTimeouts, SHALL remain unsupported.
- **AND** Nested gateway namespaces SHALL NOT be interpreted as credential controls.
- **AND** Native options or body headers SHALL NOT override credential source, destination or the selected model.

#### Scenario: Unsupported credential family or mapping
- **WHEN** a request supplies a known but unsupported credential family or modelMappings
- **THEN** it SHALL receive a capability-specific rejection rather than silently ignore the form or use another account

#### Scenario: Canonical credential control
- **WHEN** a request supplies BYOK or another case variant, alone or alongside canonical byok
- **THEN** it SHALL decode as the byok control under ordinary Go decoding, with the last duplicate member winning
- **AND** the call-level gateway namespace, not the member name, SHALL remain the capture boundary: the runtime SHALL remove it before any model, middleware or telemetry

#### Scenario: Routing controls accompany valid BYOK
- **WHEN** valid credentials accompany gateway.order, gateway.only or gateway.models
- **THEN** the request SHALL be routed under gateway-request-routing

#### Scenario: Unsupported control accompanies valid BYOK
- **WHEN** valid credentials accompany gateway.sort, gateway.providerTimeouts or another unsupported control
- **THEN** the request SHALL fail before I/O without suggesting that the control was applied

### Requirement: Ordered credentials reuse default fallback
Ordered request accounts, for the selected model and any routed fallback models, SHALL reuse the default fallback policy without a second retry layer. Attempts SHALL share the deadline and stop after cancellation or commitment; account failures SHALL retain safe native classification without substitution or exactly-once claims.

#### Scenario: Credential fallback policy
- **WHEN** a request-only model executes its ordered account candidates
- **THEN** The host SHALL compose selected-provider credentials with the existing fallback module and its default decider, one request-scoped candidate per key in array order, for the selected provider/model and then for each routed fallback model under gateway-request-routing.
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
- **AND** No configured account, provider without a routed model, another request's key or ambient credential SHALL become eligible.
- **AND** Unknown pre-commit failures may follow provider-side work; no exactly-once execution or charging guarantee SHALL be claimed.

#### Scenario: Retryable failure advances
- **WHEN** the first candidate returns a retryable API error such as a classified 429 or 503 before unary success or any stream part and the second succeeds
- **THEN** the second candidate SHALL run once with the same provider/model and supported request content

#### Scenario: Last account of a model fails before a fallback model
- **WHEN** every account of the selected model fails eligibly and a routed fallback model has accounts
- **THEN** the fallback model's first account SHALL run next with its own provider, model and key

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
