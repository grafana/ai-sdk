# Spec Delta

## MODIFIED Requirements

### Requirement: Strict model configuration and public IDs
A bounded strict YAML document SHALL own named providers and public model routes. YAML decoding SHALL reject unknown fields, duplicate mapping keys, and trailing documents. Configuration SHALL reject empty names, unknown provider types, missing provider or backend model references, duplicate or colliding canonical IDs and aliases, missing required presentation names, an empty model set, an empty effective route, and duplicate candidate tuples within a route.

#### Scenario: Ordered Anthropic configuration is valid
- **WHEN** YAML defines multiple named Anthropic instances and a public model with one primary plus distinct ordered fallback provider/backend references
- **THEN** startup SHALL accept the route without contacting Anthropic and SHALL preserve the declared candidate order

#### Scenario: Minimal direct Anthropic configuration remains valid
- **WHEN** YAML defines one named Anthropic provider by API-key environment reference and one header-safe public route with only a primary provider/backend reference
- **THEN** startup SHALL construct the direct route without requiring a fallback list or contacting Anthropic

#### Scenario: Route references are invalid
- **WHEN** any route candidate has an empty field, names an unknown provider, repeats an earlier provider/backend tuple, attempts to name a public route, or contains a nested fallback field
- **THEN** startup SHALL fail before secret resolution, provider construction, or serving requests

#### Scenario: Unsafe public ID is configured
- **WHEN** a canonical ID or alias contains whitespace, control bytes, commas, non-ASCII bytes, or exceeds 128 bytes
- **THEN** startup SHALL fail before catalog construction

#### Scenario: Referenced credential is unavailable
- **WHEN** `apiKeyEnv` names an unset or empty environment variable
- **THEN** startup SHALL fail before constructing the catalog or serving requests
- **AND** logs and errors SHALL NOT contain any environment-variable value
