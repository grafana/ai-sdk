## ADDED Requirements

### Requirement: Stack catalog provider keys
Provider keys MAY also come from a stack catalog, as the decrypted secure value keyed by the provider name. Such a key SHALL be used only for that catalog's candidates in the request that supplied it, with the provider type's native endpoint and the shared model HTTP client. Operator providers, their keys and endpoints, BYOK accounts and ambient SDK credentials SHALL NOT affect a stack-catalog call, and stack-catalog keys SHALL NOT affect other calls.

#### Scenario: Stack catalog and operator catalog share a provider name
- **WHEN** the operator catalog's `anthropic` provider has a custom endpoint and key, and a stack catalog's `anthropic` provider has its own key
- **THEN** a stack-catalog call SHALL send the stack catalog's key to the native Anthropic endpoint, and an operator-catalog call SHALL keep using the operator key and endpoint

#### Scenario: Ambient credentials are present
- **WHEN** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` or `OPENAI_BASE_URL` hold conflicting values
- **THEN** they SHALL NOT change the destination or authentication of a stack-catalog call

#### Scenario: Concurrent catalogs
- **WHEN** two requests for different stack catalogs with different keys for the same provider type run at once
- **THEN** each provider request SHALL carry only its own catalog's key

### Requirement: Serving without an operator catalog
Without an operator catalog, configured callers SHALL get a discovery document with no models, and every model ID SHALL fail as model not found with zero provider requests. BYOK SHALL behave as with an operator catalog.

#### Scenario: Configured discovery without an operator catalog
- **WHEN** a configured caller requests `config` on a Gateway started without `model_catalog`
- **THEN** the response SHALL be a successful discovery document with an empty `models` list

#### Scenario: Configured call without an operator catalog
- **WHEN** a configured caller requests any model, with or without routing controls
- **THEN** the call SHALL fail with the model not found document, with zero provider requests

#### Scenario: BYOK without an operator catalog
- **WHEN** a BYOK caller sends valid accounts in `cloud` mode on a Gateway started without `model_catalog`
- **THEN** the call SHALL succeed as it would with an operator catalog
