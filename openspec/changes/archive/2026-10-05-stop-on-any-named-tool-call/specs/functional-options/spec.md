## ADDED Requirements

### Requirement: Stop on any named tool call

The library SHALL provide `HasToolCall(toolNames ...string) StopCondition`. The condition SHALL report true when the most recent step contains a tool call whose name is any of `toolNames`. Tool calls in earlier steps SHALL NOT satisfy it, it SHALL report false when there are no steps, and with no names it SHALL never report true. A one-name call SHALL keep its previous behavior.

#### Scenario: Any of several names in the latest step

- **WHEN** the condition is `HasToolCall("search", "finalAnswer")` and the latest step called `weather` and `finalAnswer`
- **THEN** the condition reports true

#### Scenario: Name only in an earlier step

- **WHEN** the condition is `HasToolCall("finalAnswer")` and only an earlier step called `finalAnswer`
- **THEN** the condition reports false

#### Scenario: No names

- **WHEN** the condition is `HasToolCall()` and the latest step called any tool
- **THEN** the condition reports false
