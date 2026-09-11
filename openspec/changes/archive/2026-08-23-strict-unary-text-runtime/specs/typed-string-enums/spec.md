## MODIFIED Requirements

### Requirement: ReasoningEffort typed string enum

The `provider` package SHALL define a `ReasoningEffort` typed string with constants `ReasoningProviderDefault`, `ReasoningNone` (`"none"`), `ReasoningMinimal` (`"minimal"`), `ReasoningLow` (`"low"`), `ReasoningMedium` (`"medium"`), `ReasoningHigh` (`"high"`), and `ReasoningXHigh` (`"xhigh"`). `ReasoningProviderDefault` SHALL have the empty-string value so it is the Go zero value. The `CallOptions.Reasoning` field SHALL be typed as `ReasoningEffort`, and the existing constant names SHALL be preserved.

#### Scenario: Reasoning field uses typed value
- **WHEN** a caller sets `CallOptions.Reasoning`
- **THEN** it SHALL assign a `ReasoningEffort` value, for example `opts.Reasoning = ReasoningHigh`

#### Scenario: Anthropic reasoning maps use typed keys
- **WHEN** the Anthropic provider's reasoning maps are defined
- **THEN** their key type SHALL be `ReasoningEffort`, not bare `string`

#### Scenario: Explicit reasoning JSON round-trip
- **WHEN** `CallOptions{Reasoning: ReasoningHigh}` is marshaled to JSON
- **THEN** it SHALL produce `"reasoning":"high"`

#### Scenario: Provider-default is omitted from provider-domain JSON
- **WHEN** a zero-valued `CallOptions` is marshaled to JSON
- **THEN** its `reasoning` member SHALL be omitted
- **AND** a strict wire adapter SHALL remain responsible for normalizing explicit wire `"provider-default"` to that zero value
