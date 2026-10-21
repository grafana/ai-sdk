# Spec Delta

## MODIFIED Requirements

### Requirement: Vercel-shaped credential ingestion
The host SHALL validate the complete call-level gateway.byok map with ordinary typed Go decoding, including unused accounts, canonical controls and required keys. Only selected-provider accounts SHALL become eligible, and only they SHALL be subject to the account attempt bound. Request bounds SHALL come from the shared route request-body bound and native-header bounds from the native transport.

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
- **AND** Each decoded key SHALL be nonempty. Decoding SHALL NOT apply an account-count bound; the eight-account bound SHALL be execution policy for the selected provider only.
- **AND** Decoding SHALL use provider-to-account maps and ordinary typed Go decoding without blanket unknown-field rejection.
- **AND** The only accepted Gateway control SHALL be exactly lowercase byok, as emitted by the supported clients; BYOK and other case variants SHALL fail.
- **AND** Account fields SHALL use the canonical client names apiKey, baseURL, organization and project.
- **AND** Account struct fields SHALL retain standard Go case-insensitive matching; namespace/map keys and discriminator values SHALL NOT be normalized.
- **AND** Null strings SHALL retain their ordinary Go value, and duplicate members SHALL follow ordinary Go decoding.
- **AND** Malformed earlier duplicate members MAY fail typed decoding even when a later member is valid; no JSON normalization layer SHALL be introduced.
- **AND** The shared route request-body bound SHALL govern BYOK input without separate subtree or per-field byte ceilings.
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
- **WHEN** the selected provider array contains 1 to 8 accounts
- **THEN** otherwise valid input SHALL be accepted
- **WHEN** any supplied array is empty
- **THEN** the request SHALL fail before I/O without echoing the rejected value
- **WHEN** an unused provider array contains more than 8 otherwise valid accounts
- **THEN** the request SHALL be accepted without constructing those accounts

#### Scenario: Shared request bound
- **WHEN** a BYOK request reaches the shared route request-body bound
- **THEN** otherwise valid input SHALL reach selection without a separate BYOK byte ceiling
- **WHEN** that bound is exceeded by one byte
- **THEN** the request SHALL be rejected as invalid before selection

#### Scenario: Native header validity
- **WHEN** a decoded account contains an invalid outgoing HTTP header value
- **THEN** the native HTTP transport SHALL refuse the attempt before sending the request, using existing fallback/error handling

#### Scenario: Selected provider lacks credentials
- **WHEN** the header selects OpenAI but only Anthropic credentials are supplied
- **THEN** selection SHALL fail without changing provider or checking configured/environment keys

## ADDED Requirements

### Requirement: Selected-provider account attempt bound
Request-scoped model construction SHALL accept at most eight ordered accounts for the selected provider, one physical attempt candidate each. A selected array above the bound SHALL fail as the fixed BYOK credentials invalid-request error before any native I/O. The bound SHALL NOT truncate the array or silently ignore accounts.

#### Scenario: Selected array at the bound
- **WHEN** the selected provider supplies exactly eight valid accounts
- **THEN** construction SHALL create eight ordered candidates under the shared execution deadline

#### Scenario: Selected array above the bound
- **WHEN** the selected provider supplies nine valid accounts
- **THEN** the request SHALL fail with the fixed BYOK credentials invalid-request error before any native call, without echoing account values
