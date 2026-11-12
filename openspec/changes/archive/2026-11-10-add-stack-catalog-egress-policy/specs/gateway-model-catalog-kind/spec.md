## MODIFIED Requirements

### Requirement: ModelCatalog resource
The Gateway's API group SHALL serve a namespaced `ModelCatalog` kind in version `v1alpha1` that is not folder-scoped. Its `spec` SHALL describe providers, models, aliases, primaries and fallbacks with the same field names as the operator catalog document, without `apiKeyEnv`.

#### Scenario: Catalog saved without a folder
- **WHEN** a writer creates a `ModelCatalog` without a folder annotation
- **THEN** it SHALL be stored in the request namespace

#### Scenario: Catalog with a folder annotation
- **WHEN** a writer creates a `ModelCatalog` that names a folder
- **THEN** the platform SHALL refuse it

#### Scenario: Operator catalog document as a spec
- **WHEN** a valid operator catalog document without `apiKeyEnv`, whose provider endpoints pass the stack-catalog egress static checks, is used as a `spec` together with one secure value per provider
- **THEN** the catalog SHALL be accepted

### Requirement: Stack catalog provider policy
Admission SHALL accept only provider types `anthropic`, `openai` and `openai-compatible`, and provider names that are DNS labels: 1 to 63 lowercase ASCII letters, digits or `-`, starting and ending with a letter or digit.

#### Scenario: Compatible provider type
- **WHEN** a catalog provider has type `openai-compatible` with an `https` `baseURL` that passes the egress static checks
- **THEN** the write SHALL be accepted

#### Scenario: Unknown provider type
- **WHEN** a catalog provider has an unknown type
- **THEN** the write SHALL be refused

#### Scenario: Provider name outside the pattern
- **WHEN** a provider name contains an uppercase letter, `.`, `_`, `/` or a space, or exceeds 63 characters
- **THEN** the write SHALL be refused

## ADDED Requirements

### Requirement: Endpoint and compatibility fields
The kind schema SHALL keep `baseURL`, `providerName` and `supportsStructuredOutputs`, so pruning cannot drop them before admission. Admission SHALL apply the operator catalog's rules to them: `openai-compatible` requires `baseURL`, and only it may set the other two. A `baseURL` SHALL pass the operator endpoint form checks and the egress static checks.

#### Scenario: Custom Anthropic endpoint
- **WHEN** an `anthropic` provider sets `baseURL: https://anthropic-proxy.example.com`
- **THEN** the write SHALL be accepted

#### Scenario: Compatible provider with its fields
- **WHEN** an `openai-compatible` provider sets `baseURL: https://llm.example.com/v1`, `providerName: acme` and `supportsStructuredOutputs: true`
- **THEN** the write SHALL be accepted

#### Scenario: Compatible provider without an endpoint
- **WHEN** an `openai-compatible` provider has no `baseURL`
- **THEN** the write SHALL be refused

#### Scenario: Compatibility field on a native type
- **WHEN** an `openai` provider sets `providerName` or `supportsStructuredOutputs`
- **THEN** the write SHALL be refused

#### Scenario: Endpoint with credentials or a query
- **WHEN** a `baseURL` carries userinfo, a query or a fragment
- **THEN** the write SHALL be refused, and the reason SHALL NOT echo the URL

## REMOVED Requirements

### Requirement: Unsupported fields are refused, not pruned
**Reason**: Stack catalogs now accept these fields under the egress policy.
**Migration**: See "Endpoint and compatibility fields" and the `gateway-stack-catalog-egress` capability.
