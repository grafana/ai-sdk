## MODIFIED Requirements

### Requirement: Outbound model transport bounds
Every Gateway-constructed model SHALL use the shared model HTTP client, except a stack-catalog provider with `baseURL`, which SHALL use the egress client. Both clients SHALL reject redirects and wait at most 10 seconds for response headers. Each SHALL fail a non-`text/event-stream` response once its decompressed body exceeds 16 MiB, and SHALL NOT bound the bytes of a `text/event-stream` response, which the execution time bounds end. No bound SHALL be an operator setting.

#### Scenario: Decompressed bytes are counted
- **WHEN** a provider returns a gzip-encoded body under 16 MiB compressed whose decompressed size exceeds 16 MiB
- **THEN** the read SHALL fail with the transport's response-too-large error once the decompressed bytes exceed the bound

#### Scenario: Each request has its own bound
- **WHEN** two sequential requests through either client each return just under 16 MiB
- **THEN** both SHALL succeed, because the bound applies to each response independently and not to the client or the logical call

#### Scenario: Response exceeds the bound
- **WHEN** a non-SSE provider response exceeds 16 MiB after decompression
- **THEN** the call SHALL fail through the existing fallback and safe-error classification without returning a partial result

#### Scenario: Stream above the response bound
- **WHEN** a `text/event-stream` response, plain or compressed and with any media-type parameters or case, totals more than 16 MiB, or carries one event above 16 MiB
- **THEN** the transport SHALL read it to its end without a response-too-large error

#### Scenario: Unparsable media type
- **WHEN** a response's media type cannot be parsed
- **THEN** the 16 MiB total bound SHALL apply

#### Scenario: Egress client bounds
- **WHEN** a custom stack-catalog endpoint redirects, stalls its response headers or exceeds 16 MiB in a unary response
- **THEN** the egress client SHALL enforce the same bounds as the shared client
