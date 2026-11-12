## MODIFIED Requirements

### Requirement: Configured provider state cannot influence BYOK
Startup provider-instance and candidate-policy configuration, secret resolution, configured endpoints and account-bound models SHALL belong only to configured-account access. BYOK construction SHALL use built-in adapters, native destinations or the account's policy-admitted baseURL, and request credentials, without that configuration or ambient SDK credentials. Credential-independent transport infrastructure SHALL stay reusable without shared mutable authentication state.

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

### Requirement: OpenAI Responses shared bounded transport
OpenAI Responses models SHALL use the shared model HTTP client, or the egress client for a stack-catalog or request-account `baseURL`, and inherit its redirect rejection, fixed 10-second response-header timeout and fixed 16 MiB decompressed byte bound per non-stream response under gateway-resource-policy, and leave streams to the execution time bounds. No provider-specific transport setting SHALL exist.

#### Scenario: OpenAI Responses shared bounded transport
- **WHEN** an OpenAI backend redirects, stalls its response headers, exceeds the byte bound in a unary response, or streams more than 16 MiB
- **THEN** the shared client SHALL enforce the same bounds as for every other provider type, and only the long stream SHALL succeed
