# Spec Delta

## MODIFIED Requirements

### Requirement: Independent evidence and capture control
Protocol test host tests SHALL exercise both clients, native projection and response-derived continuation. Returned metadata SHALL NOT authorize operator capture; metadata-only generations, logs and metrics SHALL omit supplied tool data and MCP credentials. Caller-owned request bodies remain sensitive. Synthetic deferred-result tests and native fakes SHALL NOT be represented as recorded provider responses or proof of live hosted MCP egress, deployed readiness or published-module adoption.

#### Scenario: Credential-bearing native round trip
- **WHEN** MCP configuration and responses pass through the protocol test host and native fake
- **THEN** configured native authentication SHALL remain authoritative, public developer data SHALL remain opaque, and metadata-only telemetry SHALL not capture submitted values
