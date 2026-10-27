## ADDED Requirements

### Requirement: Execution-denied reason presence
Provider-domain execution-denied results SHALL distinguish absent reason from explicit empty reason through JSON and native conversion. A provider default denial message SHALL apply only to absence. Approval-response absent and empty reasons SHALL normalize consistently without changing this output distinction.

#### Scenario: Canonical Go representation
- **WHEN** Go represents an execution-denied result
- **THEN** it SHALL use a single optional `ToolResultOutput.Reason *string`: nil for absent and non-nil for supplied empty or nonempty text, and synthesis from an empty normalized approval-response reason SHALL produce absent denial reason

#### Scenario: Three reason values
- **WHEN** execution-denied results contain absent, empty or nonempty reason
- **THEN** JSON round-trip and supported native requests SHALL preserve absence, empty and supplied text respectively, with native defaults applied only for absence
