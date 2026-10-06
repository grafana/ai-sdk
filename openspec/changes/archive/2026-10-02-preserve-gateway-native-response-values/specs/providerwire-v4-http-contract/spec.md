## MODIFIED Requirements

### Requirement: Focused unary client-consumption evidence

The workspace SHALL exercise unary success through the exact registered client with an injected response. Probes SHALL assert representative supported content, finishReason, usage and Gateway response headers; SHALL prove replacement of server request/response with client-owned Gateway-hop information; and SHALL prove combination, not replacement, of server warnings followed by local warnings. At the currently registered Gateway version local warnings are empty; evidence SHALL exercise that actual behavior without inventing a local-warning-producing API.

Server native response id/modelId/timestamp SHALL remain in the raw response body even though they do not survive as typed response identity. Warning variants/active fields/order and required empty strings SHALL be asserted independently of raw/schema checks that govern strict server output. Client raw-body access SHALL NOT by itself be described as middleware capture or complete native diagnostic access.

#### Scenario: Unary result is consumed
- **WHEN** injected fetch returns a representative valid generate result
- **THEN** the registered client SHALL resolve with its content, finishReason and usage

#### Scenario: Client-owned unary transport fields replace server fields
- **WHEN** response JSON includes native server request/response fields and ordered warnings
- **THEN** typed request SHALL contain the submitted args and typed response SHALL contain Gateway HTTP headers/raw response data rather than native typed identity
- **AND** the raw response data SHALL retain server response identity
- **AND** resolved warnings SHALL preserve the server sequence before the registered local warning sequence, which is currently empty
