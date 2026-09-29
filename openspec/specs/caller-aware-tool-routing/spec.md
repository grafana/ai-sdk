# caller-aware-tool-routing Specification

## Purpose

Define opt-in per-step visibility and execution of tools routed through local or provider callers, aligned with the registered upstream AI SDK.

## Requirements

### Requirement: Caller configuration is opt-in and validated

`StreamText` and `GenerateText` SHALL accept an optional `ToolRoutes` mapping keyed by callee tool name. Each `ToolRoute` SHALL separate a `Direct` flag from an ordered `Callers` list of tool names. Caller names SHALL refer to tools explicitly configured as local or provider callers in the supplied tool set. When a tool set and mapping are present, unknown callee names or names that are not configured caller tools SHALL cause a configuration error. An unlisted tool SHALL retain ordinary behavior; a listed tool with a zero-value route SHALL have no direct or provider-model visibility. A missing tool set or missing mapping SHALL retain ordinary unconfigured behavior.

#### Scenario: Unknown callee
- **WHEN** a configured mapping names a callee not in the tool set
- **THEN** orchestration SHALL surface a configuration error before making a provider request

#### Scenario: Invalid caller
- **WHEN** a configured caller name exists in the tool set but is not defined as a caller tool
- **THEN** orchestration SHALL surface a configuration error before making a provider request

#### Scenario: Unconfigured tools remain visible
- **WHEN** a tool has no caller-list entry
- **THEN** it SHALL retain its existing provider visibility and execution behavior

#### Scenario: Zero-value route
- **WHEN** a tool has a configured route with `Direct` false and no callers
- **THEN** it SHALL not appear among the model tools for that step

### Requirement: Separate model and execution tools per step

For each model step, orchestration SHALL filter the available tools by the effective active-tools setting, then prepare distinct model-visible and execution tool sets. A configured callee SHALL remain executable in the execution set when active. It SHALL appear in the model set only when the caller list includes direct access or an active provider caller. Local-only callees SHALL be absent from the provider tool definitions. Step preparation, tool choice, and provider calls SHALL use the effective step model set; local call parsing, approval, and execution SHALL use the effective step execution set. Caller configuration SHALL NOT introduce an independent caller-provenance rejection at local execution beyond upstream's tool-set lookup.

#### Scenario: Local-only callee hidden from model
- **WHEN** a caller list names only an active local caller
- **THEN** the provider request SHALL contain the caller tool but not the callee tool
- **AND** the bound caller SHALL be able to invoke the callee

#### Scenario: Direct and local routes
- **WHEN** a route has `Direct` true and names an active local caller
- **THEN** the callee SHALL remain model-visible and SHALL also be supplied to the local caller

#### Scenario: Per-step active-tool filtering
- **WHEN** a per-step override deactivates a configured callee or its sole local caller
- **THEN** neither a deactivated callee nor a local-only callee lacking an active local caller SHALL appear in that step's model tools
- **AND** the local caller's bound set SHALL not contain inactive callees

### Requirement: Local caller is bound late

An active local caller SHALL be bound on each step to the active tools that name it as an allowed caller. The bound tool SHALL be used for local execution. Without a caller-message callback, the bound tool SHALL also be used as the model definition. When the caller has a caller-message callback, the original unbound tool SHALL remain the model definition, and a returned string (including an empty string) SHALL be appended as a user message after the step messages. Duplicate announcements matching the latest user text or an already-appended announcement SHALL be omitted.

#### Scenario: Late binding uses step's allowed set
- **WHEN** a model calls a local caller that has one active allowed callee
- **THEN** the caller's bound execution SHALL receive that callee and no unrelated tools

#### Scenario: Announcement preserves model definition
- **WHEN** a local caller supplies a model-message callback and a bound tool with a different description
- **THEN** the provider SHALL see the original caller description and the appended user announcement
- **AND** execution SHALL use the bound tool

#### Scenario: Duplicate announcement omitted
- **WHEN** the latest user text already equals a caller announcement
- **THEN** the provider prompt SHALL not contain a duplicate announcement

### Requirement: Provider caller prepares per-tool provider options

An active provider caller SHALL transform each routed callee's provider options using its configured preparation callback in caller-list order. That callee SHALL remain model-visible without requiring direct access. Explicitly supplied provider options SHALL remain unchanged in the absence of such a callback. Orchestration SHALL not itself mutate caller-owned options or impose a generic conflict or authorization rule on manually supplied `allowedCallers`; the configured callback controls its returned options and any mutation it performs.

#### Scenario: Provider-only route
- **WHEN** a callee lists only an active provider caller
- **THEN** the callee SHALL remain in the provider tool definitions with the callback-prepared options

#### Scenario: Direct and provider routes
- **WHEN** a route has `Direct` true and names a provider caller
- **THEN** it SHALL remain directly model-visible and carry the callback-prepared options

#### Scenario: Manual provider options
- **WHEN** a tool has manually supplied `allowedCallers` options and no provider caller configuration
- **THEN** those options SHALL be forwarded unchanged by caller preparation

#### Scenario: Callback owns option precedence
- **WHEN** a provider caller preparation callback receives manually supplied provider options
- **THEN** its returned options SHALL determine the routed tool's provider options without a generic merge or conflict rejection

### Requirement: Provider-executed and dynamic calls retain their existing lifecycle

Caller preparation SHALL NOT turn provider-executed calls into local executions. Dynamic calls not present in the effective execution set SHALL retain their existing unresolved/provider-driven behavior. Approval and tool-result handling for those calls SHALL remain subject to their existing provider-executed and dynamic rules.

#### Scenario: Provider-executed tool call
- **WHEN** the provider reports a provider-executed tool call for a configured tool
- **THEN** local execution SHALL not run for that call

#### Scenario: Unknown dynamic tool
- **WHEN** a dynamic provider call names a tool absent from the effective execution set
- **THEN** no local execution callback SHALL run for that call
