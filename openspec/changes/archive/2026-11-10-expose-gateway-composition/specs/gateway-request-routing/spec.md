# Spec Delta

## ADDED Requirements

### Requirement: Logical chain applied after selection
Selection SHALL return the selected model with its logical identity and no logical observers. The execution runtime SHALL apply the logical observer chain exactly once per call, after selection returns and before the model is invoked, for configured, routed and BYOK selections alike. Each selection SHALL name its source, `operator-catalog` or `request-byok`, and the source SHALL choose the observer configuration.

#### Scenario: Every selection path is observed once
- **WHEN** a configured call, a routed configured call and a BYOK call each run once
- **THEN** each SHALL traverse each logical observer exactly once, with the observer configuration of its source

#### Scenario: Selection without a known source
- **WHEN** a selector returns a selection whose source has no observer configuration
- **THEN** the call SHALL fail as internal with zero provider requests

#### Scenario: Selection built per request
- **WHEN** a selector builds a new model for each request, with credentials scoped to that request
- **THEN** each call SHALL invoke its own request's model under the logical chain of its source, and no credential SHALL enter logical logs, metric labels or Agent Observability metadata

## MODIFIED Requirements

### Requirement: Unchanged execution without routing controls
A call without `models`, `order` or `only` SHALL execute exactly as before this change: configured calls use their startup-composed route model and BYOK calls use their selected provider's accounts.

#### Scenario: Plain configured call
- **WHEN** a configured caller sends no gateway controls
- **THEN** the call SHALL invoke the prebuilt route with its configured order and no per-request candidate or fallback construction
- **AND** only the logical observer chain SHALL be applied for the call, after selection

### Requirement: One logical chain per routed call
A routed call SHALL run all physical attempts beneath one logical middleware chain, applied after selection. Its logical identity SHALL be the requested model's canonical public ID for configured calls, and the requested selector for BYOK calls. Fallback models SHALL NOT become the logical identity or add logical observations.

#### Scenario: Fallback model serves the call
- **WHEN** a routed configured call is served by a `models` entry's candidate
- **THEN** logical logs, metrics and Agent Observability SHALL record one call under the requested route's canonical ID

#### Scenario: Private physical records
- **WHEN** a routed configured call attempts candidates from two routes
- **THEN** each attempt SHALL produce one private physical record with its composed candidate index, provider instance and backend model, correlated with the one logical call
