# Spec Delta

## MODIFIED Requirements

### Requirement: Compact observed execution overview

The Gateway SHALL provide a pure projector for one invocation's ordered observed fallback decisions.

#### Scenario: Compact observed execution overview policy
- **WHEN** an invocation is projected into an optional execution overview
- **THEN** It SHALL include the requested route identity once, the canonical route identity once when a catalog resolution supplied one, and each observed candidate's provider/backend model, optional configured instance, outcome and optional failure summary. It SHALL NOT emit unrun configured candidates, a selected index, timestamps, completion/replay claims, fallback-intent flags, raw diagnostic trees or disposition taxonomies. Only the last attempt SHALL be eligible for selected or canceled outcome. Selected SHALL mean accepted unary result or first stream part, not successful completion. Invalid, incomplete or mixed invocation observation SHALL omit the optional overview rather than invent attribution.
- **AND** Configured overviews SHALL always include the canonical route identity. Overviews without a catalog resolution SHALL omit the canonical identity and configured instances rather than substitute the requested identity, a native identity or a credential-derived value.
- **AND** The owned namespace schema SHALL require the requested identity and attempts, and SHALL accept an absent canonical identity.

#### Scenario: Earlier candidate fails and later candidate is selected
- **WHEN** one invocation reports a failed primary followed by a selected secondary
- **THEN** projection SHALL include those two candidates in order with the primary summary and no duplicated selected index

#### Scenario: Intent does not establish invocation
- **WHEN** a failure permits fallback but no later candidate decision is observed
- **THEN** projection SHALL NOT invent a subsequent attempt or project an incomplete advance as a complete invocation

#### Scenario: Error part selects a stream
- **WHEN** fallback accepts an error part as the first stream part
- **THEN** the overview SHALL report selection without a completion claim or a duplicated stream-error history

#### Scenario: Invocation without catalog resolution
- **WHEN** a request-account invocation for `openai/gpt-5` reports a failed then a selected native attempt
- **THEN** the overview SHALL carry `requestedModelId` `openai/gpt-5`, no `canonicalModelId`, and two attempts with native provider `openai`, model `gpt-5`, no `providerInstance`, the first attempt's own failure summary and the second's selected outcome
