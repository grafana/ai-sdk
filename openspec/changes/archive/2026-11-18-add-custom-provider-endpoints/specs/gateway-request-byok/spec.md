## MODIFIED Requirements

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
- **AND** An explicitly submitted baseURL SHALL be governed by the operator's custom endpoint list; no service configuration or default SHALL supply an endpoint or account.
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

### Requirement: Request-scoped provider construction and control removal
Validated plain accounts SHALL feed explicit native construction without raw-JSON parsing, catalog/configuration/secret lookup or ambient defaults. Request credentials and models SHALL remain request-owned; host controls SHALL be removed before native inference and middleware.

#### Scenario: Construction and ownership policy
- **WHEN** the host prepares request-only native execution
- **THEN** Host selection SHALL decode and validate Gateway controls before passing the provider, native model and plain account configs to construction.
- **AND** Construction SHALL NOT parse raw Gateway JSON.
- **AND** Shared native constructors MAY serve configured and request-only execution, but SHALL receive explicit inputs without performing catalog, configuration or secret lookup.
- **AND** Missing model/accounts or an unsupported provider SHALL fail before native construction.
- **AND** The BYOK selector SHALL construct account-bound models for the request using only the selected supplied accounts and native-default or listed custom destinations.
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
- **AND** no BYOK map, OpenAI key, platform access token or configured key SHALL enter its native JSON body or headers

#### Scenario: Native content is preserved
- **WHEN** a BYOK request contains supported native options, reasoning/file/tool history or continuation
- **THEN** the chosen adapter SHALL receive its matching ordinary options and supported content unchanged, apart from existing documented provider mappings

#### Scenario: Concurrent customer isolation
- **WHEN** concurrent requests select the same provider/model with different keys
- **THEN** each native request SHALL use only its own key and SHALL NOT mutate shared clients/models

#### Scenario: Cancellation and late setup
- **WHEN** cancellation occurs during native setup or stream delivery
- **THEN** no later credential SHALL start, late results SHALL be cleaned up through bounded ownership/drain and no request credential SHALL be cached

## ADDED Requirements

### Requirement: Request account endpoints
A request account's `baseURL` SHALL follow the custom endpoint rules of stack-catalog providers. Decoding SHALL check every supplied account, including unused providers, against the URL form checks and the custom endpoint list, without DNS. A failure SHALL be the BYOK credentials category before any provider request, without echoing the URL. An account whose `baseURL` is unset or its native default SHALL use the native endpoint.

#### Scenario: Listed endpoint
- **WHEN** an Anthropic account sets a `baseURL` the list holds
- **THEN** the attempt SHALL send only that account's key to that endpoint

#### Scenario: Unlisted endpoint
- **WHEN** any supplied account sets an unlisted custom `baseURL`, such as `https://169.254.169.254` or `http://llm.example.com`
- **THEN** the call SHALL fail with the BYOK credentials document before any provider request, and the response SHALL NOT contain the URL

#### Scenario: Explicit native endpoint
- **WHEN** an OpenAI account sets `baseURL: https://API.openai.com/v1/` and the list is empty
- **THEN** the account SHALL count as having no `baseURL` and reach the native endpoint

## REMOVED Requirements

### Requirement: Service-approved account destinations
**Reason**: Request account endpoints now follow the operator's custom endpoint list, the same rule as stack catalogs, and production never configured an approval map.
**Migration**: See "Request account endpoints" and the `gateway-custom-endpoints` capability.
