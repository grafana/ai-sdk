## MODIFIED Requirements

### Requirement: Direct tools do not enable effectful fallback
Enabling function tools on direct routes in WP11 or WP12 SHALL NOT enable them on fallback-configured routes. Non-empty tools, tool-call/result history, and any tool choice other than an absent choice or pure automatic choice with no tools SHALL activate the route guard even when no tool is selected by the model. A pure automatic choice SHALL have type auto and no tool name. Otherwise supported text-only requests with absent or empty tools and an automatic choice SHALL pass the guard, and every attempted physical candidate SHALL receive the original choice unchanged. All other unsupported-effect checks SHALL remain in force. The guard SHALL use a fixed safe non-retryable unsupported-request error and SHALL NOT reveal topology or silently route to primary only. This exception SHALL NOT change fallback ordering, retry eligibility, or commitment semantics.

#### Scenario: Unary or streaming tool request after WP11 and WP12
- **WHEN** a schema-valid supported function-tool request targets a fallback-configured public model
- **THEN** no physical candidate SHALL run and the client SHALL receive a fixed safe unsupported-request error

#### Scenario: Text-only automatic choice reaches a candidate
- **WHEN** an otherwise supported text request with absent or empty tools and pure automatic choice targets a fallback-configured route through either unary or streaming invocation
- **THEN** the guard SHALL allow the call and the physical candidate SHALL receive the same automatic choice

#### Scenario: Failover preserves automatic choice
- **WHEN** a permitted text-only automatic-choice request encounters a retry-eligible primary failure before commitment
- **THEN** the next configured candidate SHALL receive the same original automatic choice and other call options
- **AND** the ordinary ordering, retry, and commitment rules SHALL still apply

#### Scenario: Other choices remain guarded
- **WHEN** a direct unary or streaming guard call supplies none, required, named, an unknown choice type, or an automatic choice carrying a tool name
- **THEN** it SHALL return the fixed unsupported-request error before any physical candidate executes

#### Scenario: Automatic choice does not bypass effect guards
- **WHEN** a unary or streaming call combines automatic choice with nonempty tools, tool history, or another unsupported control
- **THEN** it SHALL fail before any physical candidate executes rather than selecting a fallback or silently removing those options
