# Spec Delta

## ADDED Requirements

### Requirement: Interrupted steps end the invocation

A step whose provider finish is `interrupted` SHALL keep its partial output, usage, provider metadata and finish reason, emit no error solely for the interruption, and end the invocation. Required and named tool choice SHALL NOT be enforced for it. Local tools SHALL NOT execute, and no subsequent model request SHALL start, even when approval denials would otherwise satisfy continuation.

#### Scenario: Interrupted output without a required call
- **WHEN** a step with required or named tool choice finishes `interrupted` with text and no matching call
- **THEN** no tool-choice error SHALL be emitted, the text, usage and provider metadata SHALL be retained, step and invocation callbacks SHALL report `interrupted`, and the UI finish chunk SHALL carry `finishReason: "interrupted"`

#### Scenario: Interrupted step with tool calls
- **WHEN** a step finishes `interrupted` with local tool calls, whether approved, denied or without approval
- **THEN** no tool SHALL execute and no further model request SHALL start

#### Scenario: Frontend does not continue automatically
- **WHEN** `useChat` receives a Go UI stream ending in an `interrupted` finish with a client tool call and the caller later adds its tool output
- **THEN** `sendAutomaticallyWhen` SHALL NOT trigger another request
