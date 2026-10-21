# Spec Delta

## ADDED Requirements

### Requirement: Adapter route registration
Host composition SHALL serve every manifest route through one registration table. Each entry SHALL name its method, route path, metric label, the adapter handler and the adapter's host-error writer. Composition SHALL apply the same request-body bound, access middleware and metric labeling to every entry, and SHALL construct each adapter once from the shared execution runtime. Adding an adapter SHALL require a manifest path and a table entry, not edits to dispatch, access or telemetry code.

#### Scenario: Registered routes match the manifest
- **WHEN** a test compares the registered route table with the committed manifest paths
- **THEN** every manifest path SHALL have exactly one entry and every entry SHALL have a manifest path

#### Scenario: Test adapter is registered
- **WHEN** a test registers a minimal non-ProviderWire adapter on an additional route
- **THEN** it SHALL receive the shared body bound, access decision, metric label and runtime without changes to composition code

### Requirement: Adapter-formatted host errors
Failures the host produces for a route, such as access refusal, unsupported BYOK discovery and internal errors, SHALL be written by that route's adapter in its own error format, selected from closed host categories. Host code SHALL NOT write another adapter's error document.

#### Scenario: Refused caller on a ProviderWire route
- **WHEN** access middleware refuses a caller on the `language-model` route
- **THEN** the response SHALL be the fixed ProviderWire permission document

#### Scenario: Refused caller on a test adapter route
- **WHEN** access middleware refuses a caller on the test adapter's route
- **THEN** the response SHALL be written by the test adapter's host-error writer, not as a ProviderWire document

### Requirement: Catalog listing is host-owned
The host SHALL own catalog listing and the access decision for discovery. Each adapter that serves discovery SHALL own only the projection of visible models into its document, and SHALL report BYOK discovery as unsupported in its own format.

#### Scenario: ProviderWire discovery
- **WHEN** a configured-account caller requests `config`
- **THEN** the ProviderWire projection SHALL encode the host-supplied visible catalog unchanged from today's document

#### Scenario: BYOK discovery
- **WHEN** a BYOK caller requests `config`
- **THEN** the ProviderWire adapter SHALL write its fixed unsupported-discovery document without catalog access
