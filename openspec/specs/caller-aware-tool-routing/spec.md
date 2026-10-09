# caller-aware-tool-routing Specification

## Purpose

Define opt-in per-step visibility and execution of tools routed through local or provider callers, aligned with the registered upstream AI SDK.

## Requirements

### Requirement: Caller configuration is opt-in and validated

StreamText/GenerateText SHALL accept optional ToolRoutes keyed by callee, separating Direct from ordered Callers. With tools and routes present, unknown callees or callers not explicitly configured as local/provider caller tools SHALL cause configuration errors. Unlisted tools SHALL retain ordinary behavior; zero-value routes SHALL have no direct/provider-model visibility. Missing tools or mapping SHALL retain ordinary unconfigured behavior.

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

Each step SHALL filter by effective active tools, then prepare distinct model and execution sets. Active callees SHALL remain executable; deferred callees SHALL also require discovery at step start. Model visibility SHALL require direct access or an active provider caller, subject to discovery route validation; local-only callees SHALL be absent from provider definitions.

#### Scenario: Local-only callee hidden from model
- **WHEN** a caller list names only an active local caller and the callee is not deferred or was discovered before the step
- **THEN** the provider request SHALL contain the caller tool but not the callee tool
- **AND** the bound caller SHALL be able to invoke the callee

#### Scenario: Direct and local routes
- **WHEN** a route has `Direct` true and names an active local caller and the callee is not deferred or was discovered before the step
- **THEN** the callee SHALL remain model-visible and SHALL also be supplied to the local caller

#### Scenario: Per-step active-tool filtering
- **WHEN** a per-step override deactivates a configured callee or its sole local caller
- **THEN** neither a deactivated callee nor a local-only callee lacking an active local caller SHALL appear in that step's model tools
- **AND** the local caller's bound set SHALL not contain inactive callees

#### Scenario: Search eligibility precedes caller binding
- **WHEN** a local caller with PrepareModelMessage is routed to a search and an undiscovered deferred callee
- **THEN** its current binding/catalog SHALL contain the search but not the callee
- **AND** after matching search execution only its next-step binding/catalog SHALL include the active discovered callee while retaining its stable model definition and existing announcement deduplication

### Requirement: Caller preparation follows discovery eligibility

Opted-in generation discovery eligibility SHALL apply after active filtering and before caller preparation. Undiscovered deferred entries SHALL be absent from both prepared sets and local caller bindings/catalogs. Deferred discovery SHALL NOT change original-registry historical approval resume.

#### Scenario: Historical resume does not use the new-call snapshot
- **WHEN** a historical deferred call resumes after approval
- **THEN** original-registry resume SHALL remain unchanged even when the callee is absent from the new-step prepared sets.

### Requirement: Caller model and execution consumers stay separate

Step preparation, tool choice and provider calls SHALL use the effective step model set. Local parsing, approval and execution SHALL use the effective step execution set. Caller configuration SHALL NOT add independent caller-provenance rejection at local execution beyond upstream tool-set lookup.

#### Scenario: Local lookup uses execution tools
- **WHEN** a local call names a callee in the effective execution set
- **THEN** parsing, approval and execution SHALL use that set without an additional caller-provenance rejection.

### Requirement: Local caller is bound late

Each step SHALL bind active local callers to active routed callees eligible after discovery. Contextual catalog preparation SHALL use resolved eligible tool copies for the effective step context without mutating the registry. Execution SHALL use the bound tool; without a caller-message callback the model SHALL also use it.

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

### Requirement: Caller announcements retain the unbound model definition

With a caller-message callback, the original unbound tool SHALL remain the model definition. A returned string, including empty, SHALL be appended as a user message after step messages. Duplicate announcements matching latest user text or an already-appended announcement SHALL be omitted.

#### Scenario: Empty and repeated caller announcements
- **WHEN** a callback returns an empty string, or an announcement already appended
- **THEN** the empty string SHALL be appended unless duplicate, and already-appended announcements SHALL be omitted while the unbound model definition remains.

### Requirement: Provider caller prepares per-tool provider options

Active provider callers SHALL transform routed callee options via configured callbacks in caller-list order; callees SHALL remain model-visible without direct access. Without a callback, supplied options SHALL remain unchanged. Orchestration SHALL NOT mutate caller-owned options or impose generic conflict/authorization rules on manual allowedCallers; the callback SHALL control returned options and its own mutations.

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
