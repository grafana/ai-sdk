## MODIFIED Requirements

### Requirement: Client composition and credential privacy

The registered low-level Gateway client MUST work through a test-only edge shim with the real command. The shim MUST use fixed dummy credentials and scope outcomes, not production CAP verification or policy evaluation. It SHALL strip Cloud and internal credentials, replace `X-Scope-OrgID` with its stack assertion, and strip other client identity headers. It SHALL forward the unchanged path.

Low-level model-call evidence SHALL continue to use `doGenerate` and `doStream` with explicit `maxOutputTokens`. Equivalent high-level Go `StreamText` and registered TypeScript `streamText` text-only calls with no tools or explicit choice SHALL also work through the test-only edge shim and real command when their remaining options are within the supported subset. Each incoming request SHALL preserve the automatic choice prepared by its SDK. High-level TypeScript `generateText` body headers and default unary token limits SHALL remain documented as separate compatibility gaps. Tests SHALL NOT rewrite client requests to conceal rejected body headers or tool-choice defaults.

Inbound credentials and assertions MUST NOT become provider credentials or appear in public application diagnostics. Application authentication failures SHALL use the fixed ProviderWire authentication document. Telemetry SHALL retain one registry, existing HTTP metrics, and fixed authentication source/outcome values without credential or customer-ID labels. Middleware SHALL preserve `responseWriter.Unwrap` for streaming flush support.

#### Scenario: Client-to-edge outcomes

- **WHEN** the registered low-level client sends discovery or model requests through the shim using the cases below
- **THEN** the fixture observes the corresponding result

| Case | Expected result |
| --- | --- |
| Valid dummy credentials and read scope | Discovery returns the configured public catalog without a token exchange. |
| Valid dummy credentials and write scope | Unary generation and streaming return the fake provider's expected results with explicit `maxOutputTokens`. |
| Invalid dummy credentials | The shim rejects the request; application and provider call counts remain zero. |
| Read-only inference or write-only discovery | The shim rejects the configured scope denial; application and provider call counts remain zero. |
| Spoofed client assertions with valid dummy credentials | The application receives the shim's stack assertion; the shim strips other client identity headers. |

#### Scenario: Outbound authentication

- **WHEN** an authorized client sends dummy credentials, incoming provider keys, and spoofed identity assertions through the local edge shim
- **THEN** fake Anthropic receives only the configured provider credential
- **AND** it receives no CAP credential, internal JWT, incoming provider key, or forwarded identity assertion

#### Scenario: Application authentication diagnostics

- **WHEN** application middleware rejects an assertion or internal token and the fixture captures the response, logs, and metrics
- **THEN** the response uses the fixed ProviderWire authentication document
- **AND** those outputs contain no test credential or customer identity value
- **AND** authentication observations use only fixed source and outcome values

#### Scenario: Edge denials are separate evidence

- **WHEN** the shim denies dummy credentials or a configured scope outcome
- **THEN** the fixture does not treat that response as an application authentication error
- **AND** the result does not establish a deployed proxy's credential verification or access-policy enforcement

#### Scenario: Streaming through authentication and telemetry

- **WHEN** the registered client streams through the shim and the real command
- **THEN** incremental server-sent events flush through the authentication and telemetry composition
- **AND** client cancellation cancels provider work
- **AND** the response wrapper preserves `Unwrap`

#### Scenario: Compatibility claims match test evidence

- **WHEN** command coverage is recorded after implementation
- **THEN** the parity map identifies the registered baseline, low-level calls with explicit output-token limits, and the proven high-level text-only streaming cases
- **AND** it records high-level TypeScript `generateText` body headers, default unary token limits, and actual tools as remaining compatibility work
- **AND** fake-provider fixtures are not presented as recorded provider conformance evidence

#### Scenario: High-level text-only streaming through the edge

- **WHEN** Go `StreamText` and registered TypeScript `streamText` send equivalent text prompts through the shim and real command with no tools or explicit choice
- **THEN** each observed inbound model request SHALL contain automatic tool choice without request rewriting
- **AND** each call SHALL reach the fake provider and return the expected text
- **AND** the existing authentication, credential stripping, identity, and telemetry privacy guarantees SHALL remain intact
