## ADDED Requirements

### Requirement: Configured provider state cannot influence BYOK
All startup provider-instance configuration, secret resolution, custom endpoints and constructed account-bound models SHALL belong exclusively to configured-account access. BYOK provider construction SHALL use supported built-in adapters, fixed native destinations and supplied request credentials without consulting those instances or ambient SDK credentials. Credential-independent transport/lifecycle infrastructure SHALL remain reusable without shared mutable authentication state.

#### Scenario: Configured provider of the same type exists
- **WHEN** an OpenAI BYOK request arrives and an OpenAI instance has a configured custom endpoint and key
- **THEN** the request SHALL use the native OpenAI destination and its supplied key, never the configured instance

#### Scenario: Ambient credentials are present
- **WHEN** ambient Anthropic/OpenAI keys, base URLs or account headers conflict with a BYOK request
- **THEN** they SHALL NOT affect the destination, authentication or account headers

#### Scenario: Configured request includes BYOK
- **WHEN** a configured-access caller supplies providerOptions.gateway.byok
- **THEN** the host SHALL reject it explicitly before provider I/O rather than replace or ignore the configured credential source
