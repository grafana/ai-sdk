## MODIFIED Requirements

### Requirement: Provider options output merge semantics

The package SHALL provide provider-options enrichment through `Options.ProviderOptions`. `ProviderOptionsConfig` SHALL include `ProviderKey string`, `ObjectKey string`, `Map map[string]string`, and `Conflict ConflictPolicy`. Provider-options output SHALL be disabled when `ProviderKey` is empty.

The output SHALL write string enrichment values into `provider.CallOptions.ProviderOptions` under `ProviderKey`. Values selected by `ProviderOptionsConfig.Map` SHALL write to the corresponding JSON field names. Globally included values not present in the map SHALL write under their original keys. Values selected solely by the header mapping SHALL NOT be emitted into provider options. If `ObjectKey` is non-empty, the output SHALL write values into a nested object named by `ObjectKey`; otherwise it SHALL write values into the top-level provider option object.

The output SHALL preserve unrelated existing fields. When the provider key exists, it SHALL marshal the existing typed or raw provider option to JSON, require object-shaped JSON for merging, and store the merged result as `provider.RawProviderOption{Key: ProviderKey, Raw: mergedJSON}`. Field conflicts SHALL obey `ConflictPolicy`, with `ConflictCallerWins` as the default.

#### Scenario: Absent provider key creates raw option

- **WHEN** provider options are nil or do not contain the configured provider key
- **THEN** the provider-options output SHALL allocate provider options as needed
- **AND** it SHALL store a `provider.RawProviderOption` containing the enrichment JSON under the configured provider key

#### Scenario: Existing raw object is merged

- **WHEN** provider options contain a `provider.RawProviderOption` with object JSON for the configured provider key
- **THEN** enrichment fields SHALL be shallow-merged into that JSON object or nested object
- **AND** unrelated existing fields SHALL be preserved

#### Scenario: Existing typed option is merged through JSON

- **WHEN** provider options contain a typed provider option for the configured provider key
- **THEN** the provider-options output SHALL marshal the typed option to JSON, merge enrichment into the object JSON, and store the result as `provider.RawProviderOption`

#### Scenario: Existing non-object obeys conflict policy

- **WHEN** provider options contain non-object JSON for the configured provider key
- **THEN** `ConflictCallerWins` SHALL preserve the existing option without enrichment
- **AND** `ConflictEnrichmentWins` SHALL replace it with the enrichment object
- **AND** `ConflictError` SHALL return an error

#### Scenario: Grafana controls are preserved

- **WHEN** provider options contain existing `grafana.agentObservability`, `grafana.tracing`, `grafana.metrics`, or `grafana.usage` fields
- **AND** enrichment writes to `ProviderKey: "grafana"` and `ObjectKey: "enrichment"`
- **THEN** those existing Grafana hosted middleware control fields SHALL be preserved unchanged

#### Scenario: ResolveOption remains usable after merge

- **WHEN** a provider option is merged and stored as `provider.RawProviderOption`
- **THEN** downstream code SHALL be able to recover typed views using `provider.ResolveOption` for compatible option structs

### Requirement: Composition with registry, Agent Observability, and Grafana hosted provider

The enrichment module SHALL require no registry changes. It SHALL be usable with `registry.WithLanguageModelMiddleware` because registry already accepts `middleware.Middleware` values.

The enrichment module SHALL document middleware ordering with Agent Observability. When enrichment appears before `agentobservability.Stack(...)` in the middleware slice, Agent Observability hooks and recording SHALL observe enriched `CallOptions`; when enrichment appears after Agent Observability, enrichment SHALL be transport-only from Agent Observability's perspective.

The enrichment module SHALL document Grafana hosted provider usage through provider options, not hosted middleware control headers. The recommended Grafana sidecar shape SHALL be `Options{ProviderOptions: ProviderOptionsConfig{ProviderKey: "grafana", ObjectKey: "enrichment"}}`. Enrichment SHALL NOT reinterpret or modify `grafana.agentObservability`, `grafana.tracing`, `grafana.metrics`, or `grafana.usage` unless callers explicitly configure provider-options fields to those exact names.

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
- **THEN** Grafana hosted middleware controls under `grafana.agentObservability`, `grafana.tracing`, `grafana.metrics`, and `grafana.usage` SHALL remain separate and unchanged
