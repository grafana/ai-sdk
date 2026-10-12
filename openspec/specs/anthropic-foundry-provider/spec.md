# anthropic-foundry-provider Specification

## Purpose
Define Azure Foundry endpoint, authentication, deployment routing and canonical
model identity while preserving compatibility with published SDK modules.

## Requirements

### Requirement: Explicit Foundry transport configuration
Foundry SHALL require an explicit service endpoint and exactly one API key,
bearer token, or credential callback. It SHALL ignore ambient Anthropic and
Foundry settings and SHALL block HTTP redirects by default.

#### Scenario: Conflicting environment credentials
- **WHEN** ambient credentials, profiles, or endpoint variables are present
- **THEN** requests use only the explicit Foundry endpoint and credential

#### Scenario: Retried request after credential rotation
- **WHEN** a request is retried and a credential callback is configured
- **THEN** the callback is invoked for each HTTP attempt
- **AND** an empty, conflicting, or failed credential result prevents that attempt

### Requirement: Deployment routing preserves canonical identity
Foundry SHALL select model capabilities using the canonical model ID and SHALL
send the separately configured deployment name in the request model field.
Generation and streaming response metadata SHALL identify provider `azure` and
the canonical model ID.

#### Scenario: Custom deployment name returned by the server
- **WHEN** a Foundry response echoes a deployment name
- **THEN** generation and stream metadata retain the configured canonical ID
- **AND** tools, reasoning and usage use the shared Anthropic protocol adapter

### Requirement: Published-module compatibility
The Azure provider SHALL compose published SDK and Anthropic provider modules
without requiring an unrelated Gateway migration. Existing Anthropic protocol
conversion SHALL be reused, and application deployment approval and fallback
configuration SHALL remain consumer responsibilities.

#### Scenario: Standalone consumer
- **WHEN** tests run with GOWORK disabled and the declared module dependencies
- **THEN** the provider compiles and preserves the Foundry request and response contract

#### Scenario: Cancelled stream
- **WHEN** the caller cancels while the provider is forwarding stream parts
- **THEN** the provider drains the upstream stream and closes its output
