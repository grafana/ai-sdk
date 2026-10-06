## 1. Middleware module rename

- [x] 1.1 Rename the nested module directory, module path, package declarations, and package-qualified examples to `middleware/agentobservability`.
- [x] 1.2 Rename project-owned Agent Observability prose, diagnostics, tracer scope, comments, and test names inside the module while preserving Sigil-owned contracts.

## 2. Public provider API rename

- [x] 2.1 Rename `AIObservabilityControl` to `AgentObservabilityControl` and `GrafanaOptions.AIObservability` to `GrafanaOptions.AgentObservability` while preserving `json:"sigil,omitempty"`.
- [x] 2.2 Update Grafana provider tests and documentation for the renamed API.

## 3. Documentation and automation

- [x] 3.1 Rename the middleware guide to `agent-observability.md` and update every active documentation and godoc reference.
- [x] 3.2 Rename Mise tasks and module paths, and update Renovate configuration for Agent Observability.

## 4. Active specifications

- [x] 4.1 Replace the active `ai-observability-middleware` specification with `agent-observability-middleware`.
- [x] 4.2 Update active composition, provider-option, and in-progress documentation-organization specifications without rewriting archived history.

## 5. Validation

- [x] 5.1 Format code and confirm only allowlisted Sigil-owned and external URL identifiers retain the old naming or Sigil terminology.
- [x] 5.2 Run all tests, build, vet, lint, docs lint, Agent Observability conformance tests, parity baseline validation, and strict OpenSpec validation.
- [x] 5.3 Review the complete diff for accidental contract changes and confirm the working tree is ready to commit.
