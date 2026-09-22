## MODIFIED Requirements

### Requirement: Tool options
The library SHALL provide `WithTools(ToolSet)`, `WithToolChoice(provider.ToolChoice)`, `WithActiveTools(...string)`, and `WithStopWhen(...StopCondition)` as shared options.

Before each provider invocation, shared text orchestration SHALL resolve tool choice independently of the effective tools list. A non-nil `PrepareStepResult.ToolChoice` SHALL override the configured choice for that step. Otherwise the configured choice SHALL apply. If neither supplies a choice, `CallOptions.ToolChoice` SHALL contain `provider.ToolChoice{Type: provider.ToolChoiceAuto}`, including when tools are absent, empty, or filtered to empty. Explicit auto, none, required, and named choices SHALL be preserved without replacement or tool-count-based omission. A step override SHALL NOT mutate the configured choice or leak into later steps. These rules SHALL apply to `StreamText`, `GenerateText`, `ToolLoopAgent.Stream`, and `ToolLoopAgent.Generate` through their shared orchestration path.

#### Scenario: WithTools configures available tools
- **WHEN** `WithTools` is passed with a `ToolSet`
- **THEN** the tools are converted to provider tools and included in `CallOptions`

#### Scenario: WithActiveTools filters available tools
- **WHEN** `WithActiveTools("search", "calculate")` is passed alongside `WithTools`
- **THEN** only the named tools are included in the `CallOptions` sent to the provider

#### Scenario: WithStopWhen configures stop conditions
- **WHEN** `WithStopWhen` is passed with one or more `StopCondition` values
- **THEN** the step loop evaluates those conditions to determine when to stop

#### Scenario: Omitted choice without tools
- **WHEN** a shared text entry point is called without tool choice and with tools either absent or explicitly empty
- **THEN** the provider call SHALL contain a non-nil automatic tool choice and no tools

#### Scenario: Omitted choice with tools
- **WHEN** tools are configured and active but no tool choice is supplied
- **THEN** the provider call SHALL contain those tools and an automatic tool choice

#### Scenario: Active tools disable every tool
- **WHEN** configured tools are filtered to empty by an explicit empty global active-tools selection or a non-nil empty `PrepareStepResult.ActiveTools`
- **AND** neither configuration nor that step supplies a tool choice
- **THEN** the provider call SHALL contain no tools and a non-nil automatic choice

#### Scenario: Explicit choice survives empty tools
- **WHEN** the configured choice is auto, none, required, or a named tool and the effective tools list is empty
- **THEN** the provider call SHALL preserve that exact choice, including the named tool name

#### Scenario: Step choice takes precedence
- **WHEN** configuration supplies a choice and `PrepareStep` supplies a different non-nil choice for the current step
- **THEN** the provider call SHALL use the step choice regardless of the effective tool count
- **AND** a nil step choice SHALL instead use the configured choice or default auto if configuration omitted it

#### Scenario: Step override does not persist
- **WHEN** one step supplies a choice override and a later step omits the override
- **THEN** the later provider call SHALL use the configured choice or automatic default rather than the earlier override

#### Scenario: Shared generate and Agent calls prepare the same default
- **WHEN** `GenerateText`, `ToolLoopAgent.Stream`, or `ToolLoopAgent.Generate` is invoked without tools or choice
- **THEN** its shared provider streaming call SHALL contain the same automatic choice as `StreamText`
- **AND** configured, Agent per-call, and step-specific explicit choices SHALL retain their existing precedence
