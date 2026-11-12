## MODIFIED Requirements

### Requirement: Endpoint and compatibility fields
The kind schema SHALL keep `baseURL`, `providerName` and `supportsStructuredOutputs`, so pruning cannot drop them before admission. Admission SHALL apply the operator catalog's rules to them: `openai-compatible` requires `baseURL`, and only it may set the other two. A `baseURL` SHALL pass the operator endpoint form checks, and a custom one, other than its type's native default, the egress static checks.

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

#### Scenario: Native default endpoint
- **WHEN** an `openai` provider sets `baseURL: https://api.openai.com/v1` and the deny list holds `api.openai.com`
- **THEN** the write SHALL be accepted, and the provider SHALL count as having no `baseURL`
