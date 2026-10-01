## MODIFIED Requirements

### Requirement: Separate model and execution tools per step

For each model step, orchestration SHALL filter the available tools by the effective active-tools setting, then prepare distinct model-visible and execution tool sets. When deferred discovery is opted into, orchestration SHALL apply that generation's discovery eligibility after active filtering and before caller preparation; undiscovered deferred entries SHALL be absent from both prepared sets and local caller bindings/catalogs. A configured callee SHALL remain executable in the execution set when active and, if deferred, discovered at the start of that step. It SHALL appear in the model set only when the caller list includes direct access or an active provider caller, subject to discovery route validation. Local-only callees SHALL be absent from the provider tool definitions. Step preparation, tool choice, and provider calls SHALL use the effective step model set; local call parsing, approval, and execution SHALL use the effective step execution set. Caller configuration SHALL NOT introduce an independent caller-provenance rejection at local execution beyond upstream's tool-set lookup. Deferred discovery SHALL NOT change the original-registry historical approval-resume path.

#### Scenario: Local-only callee hidden from model
- **WHEN** a caller list names only an active local caller and the callee is not deferred or was discovered before the step
- **THEN** the provider request SHALL contain the caller tool but not the callee tool
- **AND** the bound caller SHALL be able to invoke the callee

#### Scenario: Direct and local routes
- **WHEN** a route has Direct true and names an active local caller and the callee is not deferred or was discovered before the step
- **THEN** the callee SHALL remain model-visible and SHALL also be supplied to the local caller

#### Scenario: Per-step active-tool filtering
- **WHEN** a per-step override deactivates a configured callee or its sole local caller
- **THEN** neither a deactivated callee nor a local-only callee lacking an active local caller SHALL appear in that step's model tools
- **AND** the local caller's bound set SHALL not contain inactive callees

#### Scenario: Search eligibility precedes caller binding
- **WHEN** a local caller with PrepareModelMessage is routed to a search and an undiscovered deferred callee
- **THEN** its current binding/catalog SHALL contain the search but not the callee
- **AND** after matching search execution only its next-step binding/catalog SHALL include the active discovered callee while retaining its stable model definition and existing announcement deduplication

### Requirement: Local caller is bound late

An active local caller SHALL be bound on each step to the active tools that name it as an allowed caller and are eligible after deferred discovery preparation. When contextual descriptions are configured, caller catalog preparation SHALL receive resolved eligible tool copies for the effective step runtime context without mutating the original registry. The bound tool SHALL be used for local execution. Without a caller-message callback, the bound tool SHALL also be used as the model definition. When the caller has a caller-message callback, the original unbound tool SHALL remain the model definition, and a returned string (including an empty string) SHALL be appended as a user message after the step messages. Duplicate announcements matching the latest user text or an already-appended announcement SHALL be omitted.

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
