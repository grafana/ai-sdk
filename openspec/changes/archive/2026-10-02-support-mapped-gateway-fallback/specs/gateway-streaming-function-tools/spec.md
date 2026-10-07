## MODIFIED Requirements

### Requirement: Stateless multi-step client loops
Registered Vercel and independent Go client orchestration SHALL complete multi-step local function loops through independent real-handler requests with the existing supported continuation history on direct and configured-fallback routes. The Gateway SHALL execute no local function and retain no cross-request workflow state. Each request SHALL produce one canonical metadata-only logical observation. Configured fallback SHALL accept the same mapped definitions/history/choices/options as direct streaming invocation, with existing eligibility and cancellation. Every first provider part, including stream-start, error, tool input or call, SHALL irrevocably select that candidate; later failures SHALL NOT replay the selected stream or start a later candidate. Each new continuation request SHALL begin at the configured primary. Local execution evidence SHALL NOT imply an exactly-once provider guarantee.

#### Scenario: Two-client multi-step acceptance
- **WHEN** each client receives a function call on a direct or configured-fallback route, executes its deterministic local function once and sends the result in the next request
- **THEN** both SHALL receive final text, native request history SHALL match and logical records SHALL count one generation per request

#### Scenario: Secret-bearing tool data
- **WHEN** tool input/result/name/ID/schema or provider metadata contains hostile markers
- **THEN** public allowlisted tool content SHALL retain required semantics but exported logs/metrics/metadata-only AO SHALL omit those markers and all private provider metadata

#### Scenario: Streaming tool on fallback route
- **WHEN** a supported tool request or continuation targets a configured-fallback route and the primary has an eligible setup failure before any part
- **THEN** the next candidate SHALL receive the same mapped request and the existing bounded tool-input/call lifecycle SHALL reach the consumer without a blanket effect refusal

#### Scenario: Tool events commit without replay
- **WHEN** the selected stream emits tool input or a complete call and then errors, closes prematurely or exceeds an encoding bound
- **THEN** existing strict terminal behavior SHALL apply without invoking a later candidate, duplicating tool events or adding a Gateway executor

#### Scenario: Stream-start precedes tool failure
- **WHEN** a primary emits stream-start before a tool-related error and the secondary would succeed
- **THEN** the primary SHALL remain selected and the secondary SHALL NOT run even if the error is retryable
