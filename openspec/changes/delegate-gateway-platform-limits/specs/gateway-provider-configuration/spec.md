## MODIFIED Requirements

### Requirement: OpenAI Responses shared bounded transport
OpenAI Responses models SHALL use the shared model HTTP client and inherit its redirect rejection, fixed 10-second response-header timeout and fixed 16 MiB decompressed byte bound per non-SSE response and per SSE event under gateway-resource-policy. No provider-specific transport setting SHALL exist.

#### Scenario: OpenAI Responses shared bounded transport
- **WHEN** an OpenAI backend redirects, stalls its response headers, exceeds the byte bound in a unary response or in one stream event, or streams more than 16 MiB in total with every event within the bound
- **THEN** the shared client SHALL enforce the same bounds as for every other provider type, and only the long stream SHALL succeed


### Requirement: Compatible shared bounded transport
The compatible model HTTP client SHALL be the shared model HTTP client used for every provider type, so compatible backends inherit redirect rejection, the fixed 10-second response-header timeout and the fixed 16 MiB decompressed byte bound per non-SSE response and per SSE event. No provider-specific transport setting SHALL be added.

#### Scenario: Compatible shared bounded transport
- **WHEN** a compatible backend redirects, exceeds the byte bound in a unary response or in one stream event, stalls response headers, or streams more than 16 MiB in total with every event within the bound
- **THEN** the shared client SHALL enforce the same bounds without provider-specific settings, and only the long stream SHALL succeed

