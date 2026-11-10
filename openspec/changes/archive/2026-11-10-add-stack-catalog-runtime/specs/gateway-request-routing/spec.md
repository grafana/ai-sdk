## MODIFIED Requirements

### Requirement: Logical chain applied after selection
Selection SHALL return the selected model with its logical identity and no logical observers. The execution runtime SHALL apply the logical observer chain exactly once per call, after selection returns and before the model is invoked, for configured, routed, stack-catalog and BYOK selections alike. Each selection SHALL name its source, `operator-catalog`, `stack-catalog` or `request-byok`, and the source SHALL choose the observer configuration.

#### Scenario: Every selection path is observed once
- **WHEN** a configured call, a routed configured call, a stack-catalog call and a BYOK call each run once
- **THEN** each SHALL traverse each logical observer exactly once, with the observer configuration of its source

#### Scenario: Selection without a known source
- **WHEN** a selector returns a selection whose source has no observer configuration
- **THEN** the call SHALL fail as internal with zero provider requests

#### Scenario: Selection built per request
- **WHEN** a selector builds a new model for each request, with credentials scoped to that request
- **THEN** each call SHALL invoke its own request's model under the logical chain of its source, and no credential SHALL enter logical logs, metric labels or Agent Observability metadata
