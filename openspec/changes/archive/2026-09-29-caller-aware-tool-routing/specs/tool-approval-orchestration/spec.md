## ADDED Requirements

### Requirement: Approved streaming local tools use final results for approval continuation

For a streaming local tool requiring approval, both same-invocation automatic approval and later approved approval-response resumption SHALL use the same streamed execution semantics as an unblocked call. A pending or denied request SHALL not execute the streaming function. On approval, preliminary outputs SHALL be emitted to the stream but SHALL not be appended to the model prompt; only the final output or final error SHALL resolve the call. Existing approval response correlation and provider-executed handling SHALL remain unchanged.

#### Scenario: Pending streaming tool waits for user approval
- **WHEN** a streaming local tool requires user approval
- **THEN** its execution function SHALL not run during the request-emitting invocation
- **AND** no preliminary or final tool result SHALL be emitted for it

#### Scenario: Automatically approved streaming tool
- **WHEN** a streaming local tool receives automatic approval
- **THEN** preliminary outputs SHALL be visible after the approval events
- **AND** only its final result SHALL satisfy tool-result completion

#### Scenario: Resumed approved streaming tool
- **WHEN** an approved response to a prior streaming-tool approval is supplied in the input messages
- **THEN** preliminary outputs SHALL be visible before the next model call
- **AND** only the final result SHALL be appended to the continuation prompt

#### Scenario: Denied streaming tool
- **WHEN** approval is denied for a streaming local tool
- **THEN** the streaming execution function SHALL not run
- **AND** existing execution-denied prompt behavior SHALL apply
