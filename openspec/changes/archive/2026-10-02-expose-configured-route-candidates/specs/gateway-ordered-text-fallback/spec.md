## MODIFIED Requirements

### Requirement: Public topology confidentiality
Authenticated configured discovery SHALL expose authorized candidate count/order, provider-instance/provider identifiers and configured model IDs through the gateway-configured-discovery projection at the same visibility boundary as resolution. Credentials, secret references and unrelated account configuration SHALL remain excluded. Discovery SHALL describe configured possibilities, not whether fallback actually occurred or which candidate completed a request.

Public model resolution errors, ProviderWire unary/SSE runtime output, HTTP access logs, WP8 logical logs/metrics/Agent Observability and runtime client-visible metadata SHALL retain their separately owned identity/error/capture requirements; configured discovery SHALL NOT change them. Operator capture restrictions SHALL NOT suppress authorized discovery facts or require operator payload capture to be enabled.

#### Scenario: Primary and secondary produce equivalent success
- **WHEN** otherwise equivalent calls are served by different physical candidates
- **THEN** configured discovery SHALL remain unchanged and SHALL NOT invent actual attempt or selected-response facts
- **AND** this feature SHALL leave their separately governed runtime identity/protocol behavior unchanged

#### Scenario: Fallback chain is exhausted
- **WHEN** every candidate fails before commitment
- **THEN** the existing safe fixed public error mapping SHALL continue to apply to the aggregate error without listing candidates or copying provider error text as a consequence of this discovery feature

#### Scenario: Discovery lists fallback route
- **WHEN** authorized authenticated discovery lists a route backed by multiple configured candidates
- **THEN** it SHALL emit one canonical public row with aliases, primary and configured fallbacks in declared order
- **AND** it SHALL exclude credentials, secret references and unrelated account state without invoking any candidate
