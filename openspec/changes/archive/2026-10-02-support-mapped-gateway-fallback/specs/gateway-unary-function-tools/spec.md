## MODIFIED Requirements

### Requirement: Unary function tools remain direct and stateless
The Gateway SHALL NOT execute application functions or persist tool-loop state. Direct and configured-fallback routes SHALL accept the existing supported unary function definitions, choices and history/results through the same strict mapper. Configured candidates SHALL receive the same mapped request without tool/choice/history-specific admission guards, namespace translation or candidate intersections. A successful unary result SHALL select the candidate; later response adaptation failure SHALL NOT restart fallback. Eligible pre-success failures, noneligible errors and cancellation SHALL follow the reusable fallback contract. Unsupported codecs and concrete protocol/credential/account/execution bypass protections SHALL remain effective. Streaming function support SHALL follow gateway-streaming-function-tools rather than a unary-only restriction.

#### Scenario: Two-call unary continuation
- **WHEN** an application executes a returned tool locally and sends the call plus result in a second unary request through a direct or configured-fallback route
- **THEN** the real handler SHALL process two independent requests and the application SHALL receive final text
- **AND** the deterministic function SHALL execute once for its returned call with no Gateway executor or persisted workflow state

#### Scenario: Fallback route receives tool history
- **WHEN** a supported unary tool continuation targets a fallback-configured route and the primary fails eligibly before success
- **THEN** the next configured candidate SHALL receive the same supported definitions, history/results, choices and scoped options without omission or reinterpretation

#### Scenario: Fallback route receives text-only automatic choice
- **WHEN** an otherwise supported text request contains automatic choice with no tool name and absent or empty tools
- **THEN** the original automatic choice SHALL reach each attempted physical candidate unchanged

#### Scenario: All supported choices remain eligible
- **WHEN** a schema-valid supported unary request supplies auto, none, required or named function choice
- **THEN** a configured-fallback route SHALL NOT reject it merely because of the choice and every attempted candidate SHALL receive the original choice

#### Scenario: Selected unary call is not replayed
- **WHEN** a candidate returns a successful unary function call and the strict response encoder rejects an accompanying unsupported output or exceeds its response budget
- **THEN** the existing safe response failure SHALL apply without invoking a later candidate or executing a consumer function in the Gateway

### Requirement: Unary client and provider acceptance evidence
Exact registered Vercel and independent Go clients SHALL complete authenticated unary function round trips through direct and configured-fallback production routes. Native request tests SHALL verify schemas, examples, strict presence, all supported choices, supported continuation and candidate-specific scoped native options. Tests SHALL prove configured order, unchanged failure eligibility and cancellation, consumer-owned execution and one logical observation per request. Apache modules SHALL remain independent of the Gateway module, and fixture provenance SHALL be preserved. Synthetic request evidence SHALL NOT claim live backend acceptance, full output-derived continuation or exactly-once provider execution.

#### Scenario: Capability verification
- **WHEN** unary function verification runs
- **THEN** both-client direct/fallback HTTP semantics, native request assertions, unsupported family rejection, privacy and response bounds SHALL pass without claiming invented provider payloads as recorded evidence
