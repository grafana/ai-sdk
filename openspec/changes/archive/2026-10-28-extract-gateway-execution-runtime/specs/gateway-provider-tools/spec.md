# Spec Delta

## MODIFIED Requirements

### Requirement: Provider result and continuation transport
Unary/SSE output SHALL preserve ordered calls and non-null JSON results, including error, dynamic and preliminary semantics. Assistant provider calls and basic assistant-side results SHALL map into continuation. Tool-part options SHALL retain opaque object namespaces, including `gateway`, `grafana` and `grafana-ai-sdk`, because only call-level `gateway` controls are host-owned.

#### Scenario: Selected result values
- **WHEN** output results contain empty string, false, zero, empty object or array
- **THEN** those values SHALL survive; output null SHALL fail without changing nullable prompt JSON result arms

#### Scenario: Native continuation
- **WHEN** either client replays an aliased provider call and result in assistant history
- **THEN** native conversion SHALL preserve the pairing without a Gateway session or executor

#### Scenario: Gateway namespace on a provider tool call
- **WHEN** an assistant provider-executed tool call in history carries `gateway` and `anthropic` options
- **THEN** the model SHALL receive both namespaces on the call, and native MCP interpretation of the `anthropic` namespace SHALL be unchanged
