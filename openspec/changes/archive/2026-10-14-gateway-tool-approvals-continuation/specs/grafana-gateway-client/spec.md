## ADDED Requirements

### Requirement: Provider approval consumption and continuation
The independent Go client SHALL decode unary and streamed provider approval requests, retain opaque metadata and tool/approval IDs, and encode required false approval decisions and presence-aware execution-denied outputs compatibly with the registered Vercel client. Client encoding SHALL preserve provider prompt role boundaries.

#### Scenario: Approval output round trip
- **WHEN** a provider approval request is received and a valid V4 decision continuation is submitted
- **THEN** the independent client SHALL preserve the IDs, decision and registered metadata/options without adding application-only approval requests to provider prompts

#### Scenario: High-level Go continuation
- **WHEN** Go GenerateText or StreamText resumes actual response history containing a provider-owned approval and decision
- **THEN** core prompt conversion SHALL retain only registered approval-response fields at the provider boundary after handling ownership, without mutating the caller's history, and the next Gateway/native request SHALL succeed
