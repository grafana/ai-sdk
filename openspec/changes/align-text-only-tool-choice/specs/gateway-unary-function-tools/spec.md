## MODIFIED Requirements

### Requirement: Unary function tools remain direct and stateless
The Gateway SHALL not execute application functions or persist tool-loop state. A direct route SHALL accept supported unary tool requests; streaming support SHALL follow gateway-streaming-function-tools. Fallback-configured routes SHALL reject nonempty definitions, non-automatic choices, and tool history before physical invocation. An otherwise supported text request with absent or empty tools and omitted or pure automatic choice SHALL remain eligible for fallback under gateway-ordered-text-fallback.

#### Scenario: Two-call unary continuation
- **WHEN** an application executes a returned tool locally and sends the call plus result in a second unary request
- **THEN** the real handler SHALL process two independent requests and the application SHALL receive final text

#### Scenario: Fallback route receives tool history
- **WHEN** a supported unary tool continuation targets a fallback-configured route
- **THEN** no candidate SHALL execute and the response SHALL be a fixed safe unsupported-request error

#### Scenario: Fallback route receives text-only automatic choice
- **WHEN** an otherwise supported text request contains automatic choice with no tool name and absent or empty tools
- **THEN** the original automatic choice SHALL reach each attempted physical candidate unchanged
