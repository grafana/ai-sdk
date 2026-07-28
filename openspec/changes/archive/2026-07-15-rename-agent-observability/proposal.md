## Why

Grafana renamed AI Observability to Agent Observability. The SDK's public Go
surface and documentation must use the current product name consistently while
retaining Sigil-owned wire, SDK, configuration, and telemetry contracts.

## What Changes

- **BREAKING**: Rename the nested module and package from
  `middleware/aiobservability` to `middleware/agentobservability` without a
  compatibility package.
- **BREAKING**: Rename `AIObservabilityControl` to
  `AgentObservabilityControl` and `GrafanaOptions.AIObservability` to
  `GrafanaOptions.AgentObservability` while preserving the hosted wire key
  `"sigil"`.
- Rename documentation pages, package references, task names, test names,
  diagnostics, and active specifications to Agent Observability.
- Preserve Sigil SDK imports and aliases, `SIGIL_*` configuration,
  `sigil.*` metadata, the `"sigil"` provider JSON key, and
  `aisdk.sigil.hooks.preflight`.

## Capabilities

### New Capabilities

- `agent-observability-middleware`: Provider-agnostic Agent Observability
  recording, hook evaluation, mapping, context, and conformance contracts under
  the renamed module.

### Modified Capabilities

- `ai-observability-middleware`: Remove the former capability after its
  requirements move to `agent-observability-middleware`.
- `grafana-provider-options`: Rename the public per-request Agent Observability
  control API without changing its wire representation.
- `structured-logging-middleware`: Update composition contracts to the renamed
  Agent Observability middleware.
- `context-enrichment-middleware`: Update composition contracts to the renamed
  Agent Observability middleware.
- `prometheus-middleware`: Update composition contracts to the renamed Agent
  Observability middleware.

## Impact

Consumers must update imports, package qualifiers, Grafana option field names,
and control type names. The change affects the nested middleware module, the
Grafana provider, documentation, repository automation, and active OpenSpec
contracts. Runtime recording behavior and Sigil-owned external contracts do not
change.
