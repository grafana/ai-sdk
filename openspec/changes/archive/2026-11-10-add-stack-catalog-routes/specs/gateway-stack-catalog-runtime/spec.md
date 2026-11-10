## ADDED Requirements

### Requirement: Catalog language-model route
A catalog route's `language-model` call SHALL select from its parent catalog as a stack-catalog selection: the parent's spec, the decrypted values the platform passed with the request, keyed by provider name, and the parent's name. The Gateway SHALL keep no catalog, key or built model between requests, so the next request after a catalog edit uses the edited catalog and keys.

#### Scenario: Decrypted key reaches the provider
- **WHEN** a catalog route call selects a model of an `anthropic` provider
- **THEN** the native request SHALL carry that provider's decrypted value as its API key

#### Scenario: Catalog edited between requests
- **WHEN** a provider's key or a model's primary changes, and the platform passes the new object and values
- **THEN** the next call SHALL use the new key or model

#### Scenario: Unary and streaming calls
- **WHEN** a registered client calls a catalog route with the catalog as its base URL, unary or streaming
- **THEN** the response SHALL use the same ProviderWire format as a namespace route call
