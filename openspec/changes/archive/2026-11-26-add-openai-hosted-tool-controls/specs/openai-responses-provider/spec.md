## ADDED Requirements

### Requirement: Code interpreter auto-container controls

The Responses adapter SHALL accept optional `memoryLimit` and `networkPolicy` in `openai.code_interpreter` auto-container arguments. It SHALL keep omitted containers in auto mode and preserve named containers and supplied file IDs.

#### Scenario: Configure auto-container resources

- **WHEN** an auto container specifies a memory limit of `1g`, `4g`, `16g`, or `64g`
- **THEN** the tool declaration SHALL emit `memory_limit` with that value
- **AND** a supplied disabled policy SHALL emit `network_policy: {"type":"disabled"}`
- **AND** a supplied allowlist policy SHALL emit `allowed_domains` and, only when provided, `domain_secrets`
- **AND** explicitly supplied empty arrays SHALL be preserved

#### Scenario: Reject malformed container controls

- **WHEN** a provided container or supported control is malformed, a memory tier or policy discriminator is unknown, an allowlist lacks its domain array, or a domain secret lacks a string domain, name, or value
- **THEN** both unary and streaming calls SHALL reject the request before HTTP without including argument values in the error

### Requirement: Secure MCP tunnel addressing

The Responses adapter SHALL accept `tunnelId` in `openai.mcp` arguments and emit it as `tunnel_id`. It SHALL require a string target and SHALL NOT require mutual exclusion between the legacy `serverUrl` and `connectorId` arguments.

#### Scenario: Project a tunnel and preserve MCP configuration

- **WHEN** an MCP declaration has a tunnel ID and no server URL or connector ID
- **THEN** the request SHALL emit only `tunnel_id` for addressing
- **AND** existing approval rules, filters, and credentials SHALL retain their request semantics

#### Scenario: Validate target presence and combinations

- **WHEN** no MCP target is present, a provided target is not a string, or a tunnel is combined with either legacy target
- **THEN** the adapter SHALL reject the request before HTTP without including argument values
- **AND** an explicitly supplied string, including an empty string, SHALL count as present

### Requirement: Credential-safe hosted-tool diagnostics

The adapter SHALL transmit configured code interpreter and Secure MCP credentials unchanged. On requests bearing these credentials, it SHALL omit raw captures, provider finish metadata, and untrusted API error details while retaining normalized outcomes, numeric usage, HTTP status, retryability, and validated retry hints. The SDK logger SHALL omit the credential fields from captured tool arguments before invoking its redactor.

#### Scenario: Provider failure echoes a tool credential

- **WHEN** HTTP, unary response, or stream failure diagnostics echo a credential from these hosted tool declarations
- **THEN** API errors, response headers, raw stream parts, and finish diagnostics SHALL NOT expose the echoed value
- **AND** requests lacking these credential fields SHALL keep their existing diagnostics

#### Scenario: Request options add credential-bearing declarations

- **WHEN** SDK request options add a supported credential-bearing declaration to the captured outgoing JSON body
- **THEN** the same diagnostic protection SHALL apply
