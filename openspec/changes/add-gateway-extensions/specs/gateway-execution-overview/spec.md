# Spec Delta

## ADDED Requirements

### Requirement: No overview for a call refused before the model
A call that an extension refuses before the selected model is invoked SHALL publish no overview and no routing summary, because no attempt was made. A call whose output an extension refuses after the model was invoked SHALL publish the overview of its observed attempts.

#### Scenario: Admit denial
- **WHEN** an admitter denies a routed configured call
- **THEN** the permission document SHALL carry no `gateway.execution` or `gateway.routing` metadata

#### Scenario: Output denial after fallback
- **WHEN** a call's first candidate fails, its second is selected and a response inspector denies the unary output
- **THEN** the permission document SHALL carry the overview with the failed and the selected attempt
