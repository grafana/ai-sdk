## ADDED Requirements

### Requirement: Completed responses satisfy the effective tool choice

Shared text orchestration SHALL validate a completed model response against the effective per-step tool choice resolved for that provider invocation, including `PrepareStepResult.ToolChoice` precedence and later-step reset to configured choice or default auto. This validation SHALL apply to `StreamText`, `GenerateText`, `ToolLoopAgent.Stream`, and `ToolLoopAgent.Generate` without changing option or result signatures.

For required choice, a response SHALL satisfy the choice if it contains at least one parsed tool call. For named choice, a response SHALL satisfy the choice if at least one parsed tool call has the selected name. Invalid and provider-executed parsed calls SHALL count toward this presence test while retaining their existing validity and execution semantics. Named choice SHALL NOT prohibit additional calls when a selected call is present. Auto and none SHALL retain their existing response behavior. Text resembling a tool call SHALL NOT count as a parsed call.

A completed response that fails this predicate SHALL be a terminal semantic failure. Enforcement SHALL occur before new local approval or execution processing for that step. It SHALL NOT depend on effective tool count, local executability, provider request rewriting, or continuation eligibility. Incomplete/canceled model calls and existing provider error paths SHALL retain their existing behavior rather than acquiring a missing-required-call error.

#### Scenario: Required choice with no parsed call
- **WHEN** a model call completes with required choice and reasoning/text but no parsed tool call
- **THEN** the response SHALL fail with a tool-choice semantic error
- **AND** tool-call-shaped text SHALL not satisfy the choice

#### Scenario: Named choice with only an unrelated call
- **WHEN** a model call completes under named choice lookup and contains only a call to another configured tool
- **THEN** the response SHALL fail before that tool's local execution or approval processing

#### Scenario: Valid required and named controls
- **WHEN** a completed required response contains a parsed call or a completed named response contains the selected parsed call
- **THEN** tool-choice validation SHALL succeed
- **AND** existing tool validation, approval, finish eligibility and stop conditions SHALL still apply

#### Scenario: Matching and unrelated calls coexist
- **WHEN** a named-choice response contains the selected call and another tool call
- **THEN** the choice SHALL be satisfied
- **AND** the additional call SHALL not be rejected merely because its name differs

#### Scenario: Invalid or provider-executed call satisfies presence
- **WHEN** a required response contains an invalid or provider-executed parsed call, or a named response contains such a call with the selected name
- **THEN** that call SHALL satisfy choice presence
- **AND** its existing invalid-input error or provider-executed handling SHALL remain unchanged

#### Scenario: PrepareStep strengthens or relaxes enforcement
- **WHEN** configured auto is overridden by required or named choice for the current step
- **THEN** completed-response validation SHALL enforce the override
- **AND** when configured required or named is overridden by auto or none, the configured requirement SHALL not be enforced on that step

#### Scenario: Choice override resets on later step
- **WHEN** a successful step overrides choice and a subsequent step supplies nil choice
- **THEN** the later response SHALL be validated against configured choice or default auto, not the previous override

#### Scenario: Empty effective tools do not disable response validation
- **WHEN** a required or named choice is forwarded with absent, empty or filtered-empty tools and the model completes without a satisfying call
- **THEN** the response SHALL still fail choice validation

#### Scenario: Auto and none remain unchanged
- **WHEN** a completed response uses auto or none choice
- **THEN** this validation SHALL not introduce a missing-call error or a new prohibition on returned calls
