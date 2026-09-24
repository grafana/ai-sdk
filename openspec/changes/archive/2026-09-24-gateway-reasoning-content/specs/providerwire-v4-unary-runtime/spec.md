## MODIFIED Requirements

### Requirement: Unsupported capability families

Custom content, provider tools and approvals, structured output, non-empty root provider options, body headers and raw output SHALL remain unsupported before resolution. File inputs SHALL execute only within gateway-file-inputs; function-tool requests SHALL execute only within their unary/streaming subsets. Assistant reasoning text and reasoning-file history SHALL execute within gateway-reasoning-content. Scoped continuation options SHALL not enable root options or headers.

#### Scenario: Reasoning continuation is supported
- **WHEN** a valid assistant reasoning part carries supported scoped continuation options
- **THEN** the model SHALL receive them without enabling deferred capability families

### Requirement: Minimal unary success response

A successful response SHALL contain only ordered supported text, function-tool-call, reasoning and reasoning-file content, finishReason and usage. Reasoning content metadata SHALL use the closed, bounded continuation projection in gateway-reasoning-content. All other response/request metadata, warnings, identity, headers and raw usage SHALL remain omitted. Required empty reasoning text and selected empty inline file data SHALL be retained. Invalid output SHALL fail safely before HTTP 200.

#### Scenario: Reasoning-only paid success
- **WHEN** a provider returns a valid reasoning-only result
- **THEN** the Gateway SHALL return success rather than a retryable adaptation error
- **AND** default high-level retries SHALL not invoke the provider again for that valid result
