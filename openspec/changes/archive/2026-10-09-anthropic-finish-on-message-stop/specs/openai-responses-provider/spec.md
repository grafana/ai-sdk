## ADDED Requirements

### Requirement: Tool-call input serialization matches upstream

Tool-call input strings that the OpenAI Responses adapter builds from structured data SHALL be compact JSON without HTML escaping of `<`, `>` and `&`, with fields in the order upstream serializes them.

#### Scenario: Code containing comparison operators
- **WHEN** a code interpreter call's code contains `<` or `>`
- **THEN** the tool-call input string SHALL contain those characters unescaped

#### Scenario: Apply-patch input field order
- **WHEN** an apply-patch call completes
- **THEN** its input SHALL be `{"callId":...,"operation":{"type":...,"path":...,"diff":...}}`, with no `diff` for a delete operation

#### Scenario: Local shell action field order
- **WHEN** a local shell call completes
- **THEN** its action SHALL serialize `type`, `command`, then any of `env`, `timeoutMs`, `user` and `workingDirectory`

### Requirement: Streaming finish reports the created response ID

The streaming finish part SHALL report the response ID from `response.created` as `responseId` provider metadata, even when a later event carries a different ID.

#### Scenario: Rotated response IDs
- **WHEN** `response.created` carries ID `resp_a` and `response.completed` carries ID `resp_b`
- **THEN** the finish part's `responseId` SHALL be `resp_a`
