## MODIFIED Requirements

### Requirement: OpenAI Responses shared bounded transport
OpenAI Responses models SHALL use the shared model HTTP client, or the egress client for a stack-catalog `baseURL`, and inherit its redirect rejection, fixed 10-second response-header timeout and fixed 16 MiB decompressed byte bound per non-stream response under gateway-resource-policy, and leave streams to the execution time bounds. No provider-specific transport setting SHALL exist.

#### Scenario: OpenAI Responses shared bounded transport
- **WHEN** an OpenAI backend redirects, stalls its response headers, exceeds the byte bound in a unary response, or streams more than 16 MiB
- **THEN** the shared client SHALL enforce the same bounds as for every other provider type, and only the long stream SHALL succeed

### Requirement: Compatible shared bounded transport
The compatible model HTTP client SHALL be the shared model HTTP client used for every provider type, or the egress client for a stack-catalog provider, so compatible backends inherit redirect rejection, the fixed 10-second response-header timeout and the fixed 16 MiB decompressed byte bound per non-stream response, and leave streams to the execution time bounds. No provider-specific transport setting SHALL be added.

#### Scenario: Compatible shared bounded transport
- **WHEN** a compatible backend redirects, exceeds the byte bound in a unary response, stalls response headers, or streams more than 16 MiB
- **THEN** the shared client SHALL enforce the same bounds without provider-specific settings, and only the long stream SHALL succeed

### Requirement: Stack catalog provider keys
A stack catalog MAY supply provider keys as decrypted secure values keyed by provider name. Each key SHALL be used only for that catalog's candidates in the request that supplied it, sent to the provider's `baseURL` through the egress client when set, else to its type's native endpoint through the shared model client. Operator providers, their keys and endpoints, BYOK accounts and ambient SDK credentials SHALL NOT affect a stack-catalog call, and stack-catalog keys SHALL NOT affect other calls.

#### Scenario: Stack catalog and operator catalog share a provider name
- **WHEN** the operator catalog's `anthropic` provider has a custom endpoint and key, and a stack catalog's `anthropic` provider has its own key
- **THEN** a stack-catalog call SHALL send the stack catalog's key to the native Anthropic endpoint, and an operator-catalog call SHALL keep using the operator key and endpoint

#### Scenario: Ambient credentials are present
- **WHEN** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` or `OPENAI_BASE_URL` hold conflicting values
- **THEN** they SHALL NOT change the destination or authentication of a stack-catalog call

#### Scenario: Concurrent catalogs
- **WHEN** two requests for different stack catalogs with different keys for the same provider type run at once
- **THEN** each provider request SHALL carry only its own catalog's key

#### Scenario: Stack catalog with a custom endpoint
- **WHEN** a stack catalog's `openai-compatible` provider sets `baseURL: https://llm.example.com/v1`
- **THEN** its key SHALL be sent only to that endpoint, through the egress client
