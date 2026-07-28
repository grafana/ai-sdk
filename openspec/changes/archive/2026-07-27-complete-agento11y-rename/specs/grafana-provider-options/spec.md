## MODIFIED Requirements

### Requirement: GrafanaOptions typed provider option

The `providers/grafana` package SHALL define a `GrafanaOptions` struct that
implements `provider.ProviderOption` with `ProviderKey()` returning `"grafana"`.
`GrafanaOptions` SHALL carry one pointer field per controllable server-side
middleware concern, so that a nil field means "no client preference; the backend
default applies" and a non-nil field expresses an explicit per-request control.
At minimum it SHALL include an `AgentObservability` field of type
`*AgentObservabilityControl`. The field SHALL serialize with the JSON key
`"agentObservability"`, matching the camelCase convention of its sibling keys.
No former key SHALL appear in a marshaled `GrafanaOptions` payload or populate
`AgentObservability` during unmarshaling. It MAY include additional
per-middleware control fields (for example `Tracing`, `Metrics`, `Usage`) as
those middlewares are introduced.

#### Scenario: Implements ProviderOption

- **WHEN** a `GrafanaOptions` value is used as a `provider.ProviderOption`
- **THEN** it SHALL satisfy the interface at compile time with `ProviderKey()` returning `"grafana"`

#### Scenario: Nil control field means backend default

- **WHEN** a `GrafanaOptions` value has a nil control field for a given middleware
- **THEN** the option SHALL carry no preference for that middleware and the backend SHALL apply its own default for that concern

#### Scenario: Per-middleware control is independent

- **WHEN** a `GrafanaOptions` value sets one control field and leaves others nil
- **THEN** only the set field SHALL influence its corresponding middleware and the others SHALL be unaffected

#### Scenario: Agent Observability control marshals under agentObservability

- **WHEN** a `GrafanaOptions` value with `AgentObservability: &AgentObservabilityControl{CaptureMode: CaptureModeFull}` is marshaled
- **THEN** the result SHALL equal `{"agentObservability":{"captureMode":"full"}}`

#### Scenario: Agent Observability control unmarshals from agentObservability

- **WHEN** `{"agentObservability":{"captureMode":"full"}}` is unmarshaled into a zero-value `GrafanaOptions`
- **THEN** `AgentObservability` SHALL be non-nil with `CaptureMode` equal to `CaptureModeFull`

#### Scenario: The former key is not decoded

- **WHEN** a payload nests the control under the pre-rename key instead of `"agentObservability"`
- **THEN** `AgentObservability` SHALL remain nil

### Requirement: Attach via ProviderOptions

Clients SHALL attach `GrafanaOptions` to a request through the ai-sdk-native
`provider.CallOptions.ProviderOptions` channel keyed by `"grafana"` (for example
via `provider.BuildProviderOptions` and the orchestration layer's
`WithProviderOptions`). The `providers/grafana` package SHALL NOT expose a
provider-specific `context.Context` helper for attaching these options.

#### Scenario: Option rides under the grafana key

- **WHEN** a client builds `provider.ProviderOptions` containing a `GrafanaOptions` value
- **THEN** the value SHALL be stored under the `"grafana"` key
- **AND** its `AgentObservability` field SHALL serialize under the nested
  `"agentObservability"` key

#### Scenario: No context helper for options

- **WHEN** the public API surface of `providers/grafana` is inspected
- **THEN** there SHALL be no exported context helper that attaches `GrafanaOptions` to a `context.Context`
