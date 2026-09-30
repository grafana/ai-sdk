## MODIFIED Requirements

### Requirement: Focused unary client-consumption evidence
The workspace SHALL exercise unary success with the exact registered public client and injected responses. Probes SHALL assert generated content, finish reason, usage, supported metadata, response headers and warning preservation/combination. Client-owned request and response SHALL overwrite server-native transport, while server warnings SHALL precede client warnings rather than be erased. Registered server response id/modelId/timestamp SHALL remain present in raw response.body but not typed unary response after overwrite. Evidence SHALL not claim typed unary identity parity.

#### Scenario: Unary result is consumed
- **WHEN** injected fetch returns a valid supported result with server warnings/metadata
- **THEN** the client SHALL resolve with content, finish reason, usage, supported metadata and combined warnings

#### Scenario: Client transport replaces server transport without erasing warnings
- **WHEN** the body includes server request/response plus ordered warnings
- **THEN** local request and Gateway response headers/body SHALL replace typed transport fields
- **AND** server warnings SHALL survive before client warnings and native response identity SHALL be inspectable only in raw body

### Requirement: Focused streaming client-consumption evidence

The workspace SHALL exercise streaming success through the registered client using SSE responses. The probes SHALL cover clean EOF after the final JSON event, tolerated `[DONE]`, raw-part filtering based on `includeRawChunks`, meaningful warning fields, ordinary supported providerMetadata through #280, source identity/display, actual response model identity and response-metadata timestamp conversion. Ordinary metadata SHALL not be filtered with raw parts.

#### Scenario: Finish followed by clean EOF is consumed
- **WHEN** the SSE response emits valid stream parts including `finish` and then closes without `[DONE]`
- **THEN** the registered client stream SHALL deliver the parts in order and close successfully

#### Scenario: DONE sentinel is tolerated
- **WHEN** the SSE response contains `data: [DONE]`
- **THEN** the registered client SHALL ignore the sentinel without emitting a stream part or failing the stream

#### Scenario: Raw parts are suppressed by default
- **WHEN** SSE contains raw parts and `includeRawChunks` is absent or false
- **THEN** the registered client SHALL omit those raw parts

#### Scenario: Requested raw parts are preserved
- **WHEN** SSE contains raw parts and `includeRawChunks` is true
- **THEN** the registered client SHALL preserve them in order

#### Scenario: Response metadata timestamp is converted
- **WHEN** a response-metadata part contains a timestamp string
- **THEN** the registered client SHALL expose that timestamp as a `Date` with the same instant

### Requirement: Focused non-success client-consumption evidence

The workspace SHALL exercise representative non-2xx JSON responses through the registered client and assert the public error classification, status, message and bounded code/param availability observable at the registered package boundary. Probes SHALL demonstrate cause/body access to string/number/null codes and projected details without claiming direct typed TS properties, plus native 408/409/429/5xx retryability and unrepresentable provider retry overrides. HTTP setup errors SHALL be distinguished from forwarded plain SSE error values. Any malformed-response coverage SHALL treat registered fallback behavior only as client evidence and SHALL NOT define the server error envelope.

#### Scenario: Structured non-2xx response is consumed
- **WHEN** the injected fetch returns a representative structured non-2xx Gateway response
- **THEN** unary and streaming setup calls SHALL reject with the registered public Gateway error behavior for that status and body

#### Scenario: Error probe remains client evidence
- **WHEN** a non-2xx response is accepted or normalized by the registered client
- **THEN** that result SHALL document client consumption only
- **AND** it SHALL NOT establish which fields the strict server may emit
