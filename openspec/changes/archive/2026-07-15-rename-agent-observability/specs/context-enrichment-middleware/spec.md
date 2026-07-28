## REMOVED Requirements

### Requirement: Composition with registry, AI Observability, and Grafana hosted provider
**Reason**: Grafana renamed the product and the integration package. The old requirement name and package-qualified examples are no longer part of the public contract.
**Migration**: Use Agent Observability terminology and `agentobservability.Stack(...)`.

## ADDED Requirements

### Requirement: Composition with registry, Agent Observability, and Grafana hosted provider

The enrichment module SHALL require no registry changes. It SHALL be usable with `registry.WithLanguageModelMiddleware` because registry already accepts `middleware.Middleware` values.

The enrichment module SHALL document middleware ordering with Agent Observability. When enrichment appears before `agentobservability.Stack(...)` in the middleware slice, Agent Observability hooks and recording SHALL observe enriched `CallOptions`; when enrichment appears after Agent Observability, enrichment SHALL be transport-only from Agent Observability's perspective.

The enrichment module SHALL document Grafana hosted provider usage through provider options, not hosted middleware control headers. The recommended Grafana sidecar shape SHALL be `Options{ProviderOptions: ProviderOptionsConfig{ProviderKey: "grafana", ObjectKey: "enrichment"}}`. Enrichment SHALL NOT reinterpret or modify `grafana.sigil`, `grafana.tracing`, `grafana.metrics`, or `grafana.usage` unless callers explicitly configure provider-options fields to those exact names.

#### Scenario: Registry applies enrichment to resolved model

- **WHEN** a provider registry is configured with `registry.WithLanguageModelMiddleware(enrichment.Middleware(opts))`
- **THEN** every model resolved by that registry SHALL receive enrichment behavior

#### Scenario: Enrichment before Agent Observability is visible to Agent Observability

- **WHEN** a model is wrapped with enrichment middleware before `agentobservability.Stack(...)`
- **THEN** subsequent Agent Observability middleware SHALL observe the enriched call options

#### Scenario: Enrichment after Agent Observability is transport-only for Agent Observability

- **WHEN** a model is wrapped with Agent Observability middleware before enrichment middleware
- **THEN** Agent Observability middleware SHALL observe the original call options and the inner provider SHALL observe enriched call options

#### Scenario: Grafana hosted controls remain separate

- **WHEN** enrichment writes to `grafana.enrichment`
- **THEN** Grafana hosted middleware controls under `grafana.sigil`, `grafana.tracing`, `grafana.metrics`, and `grafana.usage` SHALL remain separate and unchanged
