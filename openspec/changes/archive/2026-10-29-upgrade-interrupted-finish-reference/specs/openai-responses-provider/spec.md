# Spec Delta

## ADDED Requirements

### Requirement: Shell execution controls round-trip

Shell tool calls SHALL expose `action.timeout_ms` and `action.max_output_length` as `timeoutMs` and `maxOutputLength` in the tool input, omitting null or absent values, in both unary and streaming responses. Provider-executed `shell_call_output` results SHALL include `maxOutputLength` when present. Shell tool results carrying `maxOutputLength` SHALL be sent as `shell_call_output.max_output_length` in stored and stateless history.

#### Scenario: Model-requested execution controls
- **WHEN** a `shell_call` carries `timeout_ms: 1000` and `max_output_length: 2048`
- **THEN** the tool input SHALL be `{"action":{"commands":[...],"timeoutMs":1000,"maxOutputLength":2048}}`

#### Scenario: Null execution controls
- **WHEN** both controls are null or absent
- **THEN** the tool input SHALL contain only `commands`

#### Scenario: Output length round trip
- **WHEN** a shell tool result contains `maxOutputLength: 10000`
- **THEN** the next request's `shell_call_output` SHALL contain `max_output_length: 10000`, and SHALL omit it when the result has none
