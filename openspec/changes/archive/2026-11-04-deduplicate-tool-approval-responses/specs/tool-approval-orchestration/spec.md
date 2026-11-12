## ADDED Requirements

### Requirement: Repeated approval responses resolve each tool call once

StreamText, GenerateText and both ToolLoopAgent entrypoints SHALL collect only the first eligible approval response per tool call in the last tool-role message, including identical, metadata-differing or conflicting responses and different approval IDs. Existing validation and result suppression SHALL precede deduplication. Results in earlier messages SHALL NOT suppress collection.

#### Scenario: Approved response is replayed or contradicted
- **WHEN** an approved response is followed by another response for the same call
- **THEN** the local tool SHALL execute once and the next provider prompt SHALL contain one result for that call
- **AND** no denial SHALL be emitted for a later conflicting decision

#### Scenario: Denied response is replayed or contradicted
- **WHEN** a denied response is followed by another response for the same call
- **THEN** the local tool SHALL NOT execute and one denial event SHALL be emitted
- **AND** the next provider prompt SHALL contain one execution-denied result preserving the first reason

#### Scenario: Another approval ID resolves the same call
- **WHEN** two approval requests reference one tool call and their responses appear in the last tool message
- **THEN** only the first eligible response SHALL be collected

#### Scenario: Existing denied result and repeated denial
- **WHEN** the last tool message includes an execution-denied result and repeated denied responses for its call
- **THEN** one denial SHALL be collected and emitted without adding another result

#### Scenario: Invalid later response remains an error
- **WHEN** a collected decision is followed by an approval response with no approved value or an unknown approval ID, or a request whose tool call is missing
- **THEN** collection SHALL preserve the existing invalid-approval or missing-tool-call error

#### Scenario: A previous result does not extend the collection boundary
- **WHEN** a tool result appears in an earlier message and the last tool message repeats an approved response
- **THEN** collection SHALL consider the last message independently and collect the approved call once
