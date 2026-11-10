# Spec Delta

## ADDED Requirements

### Requirement: Denied model-call outcome
Logical logs and model metrics SHALL record a call that an extension denied with the distinct outcome `denied`, not as an error, so denials are excluded from error rates. A failure under `fail-closed` SHALL remain an error. Agent Observability SHALL record denied calls like other failed calls.

#### Scenario: Denial before the model
- **WHEN** an admitter denies a configured call
- **THEN** the terminal log record SHALL carry outcome `denied`, and the model request counter SHALL increase under status `denied` with no `error` series added

#### Scenario: In-band stream denial
- **WHEN** a response inspector aborts a stream
- **THEN** logs and metrics SHALL record that stream with outcome `denied`

### Requirement: Extension event metrics
The Gateway SHALL count extension denials and failures by extension name, phase and a fixed event: denied, failed under `fail-closed`, or skipped under `skip`. Labels SHALL take only registered extension names and fixed phase and event values, and SHALL NOT carry namespace, caller, model or error text.

#### Scenario: Mixed extension outcomes
- **WHEN** one extension denies at admit, another fails under `skip` in response inspection, and a third observer panics
- **THEN** the counter SHALL increase once for each, under that extension's name, its phase and its event

### Requirement: Extension failure logs
Every extension failure SHALL emit one warning log record with the extension name, phase, failure policy and a fixed class (error, panic or policy refusal), plus the call's correlation ID when one exists. Records SHALL NOT carry the error's text, request or response content, or configuration values.

#### Scenario: Failing transform with secret-bearing error
- **WHEN** a transform fails with an error whose text contains a credential
- **THEN** the log record SHALL carry only the approved fields, and no part of the error's text
