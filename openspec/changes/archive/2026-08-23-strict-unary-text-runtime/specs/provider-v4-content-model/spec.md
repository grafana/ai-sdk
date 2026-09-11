## MODIFIED Requirements

### Requirement: CallOptions.Reasoning field

`CallOptions` SHALL include a `Reasoning ReasoningEffort` value field for controlling model reasoning effort. The explicit wire values SHALL be `"provider-default"`, `"none"`, `"minimal"`, `"low"`, `"medium"`, `"high"`, and `"xhigh"`. The provider package SHALL define typed constants for these values, and `ReasoningProviderDefault` SHALL be the Go zero value so omission and explicit provider-default normalize to the same provider-domain value.

#### Scenario: Reasoning field set
- **WHEN** `CallOptions` is constructed with `Reasoning: ReasoningHigh`
- **THEN** the field SHALL carry the reasoning effort level for the provider to interpret

#### Scenario: Reasoning field omitted
- **WHEN** `CallOptions` is constructed without setting `Reasoning`
- **THEN** the field SHALL equal `ReasoningProviderDefault` and SHALL request no reasoning override

#### Scenario: Explicit wire provider-default is mapped
- **WHEN** a strict wire adapter receives the explicit value `"provider-default"`
- **THEN** it SHALL map that value to `ReasoningProviderDefault`
