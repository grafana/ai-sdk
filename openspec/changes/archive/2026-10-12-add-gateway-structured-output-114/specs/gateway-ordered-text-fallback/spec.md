## MODIFIED Requirements

### Requirement: Mapped capability fallback boundary
Configured candidates SHALL compose directly through reusable fallback beneath the single logical middleware chain. Every attempted candidate SHALL receive the same supported mapped CallOptions, retaining function definitions/history/choices, ordinary file selections and filename presence, reasoning content/controls, ordinary headers and opaque options at their original supported scopes.

#### Scenario: Scoped native options reach all attempts
- **WHEN** a mapped request carries options for multiple ordinary native namespaces at supported call/message/content/function-tool/file-result scopes and a primary fails eligibly before commitment
- **THEN** each attempted candidate SHALL receive those options unchanged at the same scopes and consume only according to its existing native namespace rules

#### Scenario: Empty and active ordinary namespaces remain eligible
- **WHEN** an otherwise supported request carries empty namespace objects or nested ordinary JSON containing null, false, zero, empty strings, arrays or objects
- **THEN** configured fallback SHALL NOT reject the request on semantic-emptiness or namespace-inventory grounds

#### Scenario: Setup failure and empty pre-part EOF
- **WHEN** setup fails eligibly or a candidate channel closes before any part while the request remains live
- **THEN** reusable fallback SHALL evaluate the failure using its existing decider and advance only to the next configured candidate when eligible

#### Scenario: Noneligible failure or cancellation
- **WHEN** a mapped request encounters a noneligible failure or cancellation before selection
- **THEN** no later candidate SHALL be invoked and cancellation SHALL preserve the existing context cause and bounded cleanup

#### Scenario: Stream-start or error commits the candidate
- **WHEN** a candidate emits stream-start or a provider error as its first part and later fails or closes
- **THEN** that first part SHALL select the candidate without looking ahead and no later candidate SHALL run

#### Scenario: Selected tool output cannot be replayed
- **WHEN** a selected unary result or streaming candidate supplies a function call and response adaptation subsequently fails or later provider output errors
- **THEN** configured fallback SHALL NOT re-enter selection or replay the call on another candidate

#### Scenario: Deferred or protected request remains refused
- **WHEN** a request activates an unsupported provider-tool, provider-executed history, approval, raw/custom/generated codec, unconsumed host control or concrete native bypass
- **THEN** its existing owning validation/protection boundary SHALL remain effective before prohibited native I/O
- **AND** removal of the text guard SHALL NOT enable the deferred feature

#### Scenario: Capture policy does not govern admission
- **WHEN** a supported tool/file/reasoning/options request runs through configured fallback under metadata-only operator capture
- **THEN** capture policy SHALL NOT reject the mapped content or alter supported responses and one logical observation SHALL cover the physical attempts

#### Scenario: Structured output is mapped
- **WHEN** a valid JSON response format is supplied on a configured fallback route
- **THEN** it SHALL remain eligible under gateway-structured-output with candidate-native validation and existing commitment rules
