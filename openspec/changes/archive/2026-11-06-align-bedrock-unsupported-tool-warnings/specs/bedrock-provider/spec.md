## ADDED Requirements

### Requirement: Unsupported provider-tool warning parity

The Converse adapter SHALL match the registered upstream unsupported provider-tool warning contents and order. Unknown-tool warnings SHALL have no details. Unary and streaming calls SHALL expose identical warning lists. Unsupported web-tool filtering SHALL retain its specific warning before other tool warnings.

#### Scenario: Multiple unknown tools on Anthropic

- **WHEN** an Anthropic request contains unknown provider tools A and B
- **THEN** warnings SHALL be `provider-defined tool A`, `provider-defined tool B`, `tool A`, `tool B`, each of type `unsupported` with no details
- **AND** unknown tools SHALL remain excluded from request tool definitions

#### Scenario: Unsupported tool on another model family

- **WHEN** a non-Anthropic request contains a provider tool A
- **THEN** the warning SHALL be type `unsupported`, feature `tool A`, without details

#### Scenario: Named choice on a model rejecting forced tool use

- **WHEN** a named choice selects a provider tool on an Anthropic model that rejects forced tool use
- **THEN** shared Anthropic warnings SHALL precede the shared forced-choice warning, followed by Bedrock warnings for the selected tool only
- **AND** selecting only a function tool SHALL omit the shared provider-tool warnings and retain the Bedrock forced-choice warning
