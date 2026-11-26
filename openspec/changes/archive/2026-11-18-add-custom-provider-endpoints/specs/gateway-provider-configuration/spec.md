## MODIFIED Requirements

### Requirement: Configured provider state cannot influence BYOK
Startup provider-instance and candidate-policy configuration, secret resolution, configured endpoints and account-bound models SHALL belong only to configured-account access. BYOK construction SHALL use built-in adapters, native destinations or the account's listed baseURL, and request credentials, without that configuration or ambient SDK credentials. Credential-independent transport infrastructure SHALL stay reusable without shared mutable authentication state.

#### Scenario: Configured provider of the same type exists
- **WHEN** an OpenAI BYOK request arrives and an OpenAI instance has a configured custom endpoint and key
- **THEN** the request SHALL use the native OpenAI destination and its supplied key, never the configured instance

#### Scenario: Ambient credentials are present
- **WHEN** ambient Anthropic/OpenAI keys, base URLs or account headers conflict with a BYOK request
- **THEN** they SHALL NOT affect the destination, authentication or account headers

#### Scenario: Configured request includes BYOK
- **WHEN** a configured-access caller supplies providerOptions.gateway.byok
- **THEN** the host SHALL reject it explicitly before provider I/O rather than replace or ignore the configured credential source

#### Scenario: Configured candidate policy exists
- **WHEN** an Anthropic BYOK request selects a model that a configured candidate also serves with a skills policy
- **THEN** BYOK admission SHALL be decided without that policy, under the BYOK rules of gateway-anthropic-skills

### Requirement: Stack catalog provider keys
A stack catalog MAY supply provider keys as decrypted secure values keyed by provider name. Each key SHALL be used only for that catalog's candidates in the request that supplied it, with the provider's listed `baseURL` when set, otherwise its type's native endpoint, through the shared model HTTP client. Operator providers, their keys and endpoints, BYOK accounts and ambient SDK credentials SHALL NOT affect a stack-catalog call, and stack-catalog keys SHALL NOT affect other calls.

#### Scenario: Stack catalog and operator catalog share a provider name
- **WHEN** the operator catalog's `anthropic` provider has a custom endpoint and key, and a stack catalog's `anthropic` provider has its own key
- **THEN** a stack-catalog call SHALL send the stack catalog's key to the native Anthropic endpoint, and an operator-catalog call SHALL keep using the operator key and endpoint

#### Scenario: Ambient credentials are present
- **WHEN** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` or `OPENAI_BASE_URL` hold conflicting values
- **THEN** they SHALL NOT change the destination or authentication of a stack-catalog call

#### Scenario: Concurrent catalogs
- **WHEN** two requests for different stack catalogs with different keys for the same provider type run at once
- **THEN** each provider request SHALL carry only its own catalog's key

#### Scenario: Stack catalog with a listed endpoint
- **WHEN** a stack catalog's `openai-compatible` provider sets a listed `baseURL: https://llm.example.com/v1`
- **THEN** its key SHALL be sent only to that endpoint, through the shared model client
