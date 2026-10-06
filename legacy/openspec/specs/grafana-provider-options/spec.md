# grafana-provider-options Specification

## Purpose

Define a typed, per-request provider option (`GrafanaOptions`) for the
`providers/grafana` package that lets clients express intent for the Grafana
hosted backend's server-side middleware stack (e.g. Agent Observability capture), with
per-middleware hard-disable and graded controls, lossless wire round-trip via
the existing `provider.ProviderOptions` machinery, and client-side validation.

## Requirements

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

### Requirement: Per-middleware hard-disable knob

Every control sub-struct in `GrafanaOptions` SHALL carry a `Disabled` field of
type `*bool`. When `Disabled` is true, the corresponding server-side middleware
MUST run none of its behavior for that request and MUST NOT produce its
side-effects (for Agent Observability, no generation record is created). The `Disabled` knob is
orthogonal to any graded configuration on the same control struct: `Disabled`
suppresses the middleware's event entirely, whereas a graded field shapes the
payload of an event that still occurs. When `Disabled` is nil, the middleware
SHALL apply its server-side default behavior.

#### Scenario: Every control carries Disabled

- **WHEN** any control sub-struct in `GrafanaOptions` is inspected
- **THEN** it SHALL expose a `Disabled` field of type `*bool`

#### Scenario: Disabled suppresses the middleware entirely

- **WHEN** a request sets a control's `Disabled` to true
- **THEN** the corresponding server-side middleware SHALL short-circuit and produce none of its side-effects for that request

#### Scenario: Disabled takes precedence over graded config

- **WHEN** a control sets both `Disabled` true and a graded field on the same struct
- **THEN** the middleware SHALL be fully disabled and the graded field SHALL have no effect for that request

#### Scenario: Nil Disabled applies default behavior

- **WHEN** a control's `Disabled` field is nil
- **THEN** the corresponding middleware SHALL apply its server-side default behavior

### Requirement: Graded Agent Observability capture control

The `providers/grafana` package SHALL define an `AgentObservabilityControl` struct carrying
both the `Disabled *bool` hard-disable knob and a graded `CaptureMode` field of a
named string enum type. The enum SHALL be a named `string` type with typed
constants (never a bare `string`), and its values SHALL mirror the server-side
Agent Observability capture-mode set, including at least full capture, metadata-only capture,
and full-with-metadata-spans capture. The `CaptureMode` field SHALL express
graded capture intent and SHALL NOT be used as a full on/off switch, because the
Agent Observability capture-mode set has no "off" value; full suppression of Agent Observability for a
request SHALL be expressed via `Disabled`.

#### Scenario: Capture mode is a typed enum

- **WHEN** the `AgentObservabilityControl.CaptureMode` field type is inspected
- **THEN** it SHALL be a named string type with typed constants for the supported capture modes, not a bare `string`

#### Scenario: Graded modes available

- **WHEN** a client constructs an `AgentObservabilityControl`
- **THEN** it SHALL be able to select full capture, metadata-only capture, or full-with-metadata-spans capture

#### Scenario: Sensitive conversation downgrades capture

- **WHEN** a client sets `AgentObservabilityControl.CaptureMode` to the metadata-only value for a sensitive conversation
- **THEN** the option SHALL express that the request be recorded by Agent Observability in metadata-only mode while other requests retain their backend default

#### Scenario: Fully disabling Agent Observability uses Disabled, not CaptureMode

- **WHEN** a client wants no Agent Observability generation record at all for a sensitive request
- **THEN** it SHALL set `AgentObservabilityControl.Disabled` to true rather than relying on any `CaptureMode` value, since no capture mode suppresses the record

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

### Requirement: Lossless wire round-trip

`GrafanaOptions` SHALL round-trip losslessly across the provider-wire boundary
using the existing `provider.ProviderOptions` JSON machinery. When marshaled it
SHALL emit its concrete JSON under the `"grafana"` key; after crossing the wire
it SHALL be recoverable as the typed `GrafanaOptions` via
`provider.ResolveOption[GrafanaOptions]`. No new HTTP header and no new wire path
SHALL be introduced for this option.

#### Scenario: Marshal emits grafana namespace JSON

- **WHEN** a `provider.ProviderOptions` containing a `GrafanaOptions` value is marshaled to JSON
- **THEN** the output SHALL contain a `"grafana"` key holding the JSON encoding of the `GrafanaOptions` value

#### Scenario: ResolveOption recovers the typed view

- **WHEN** `provider.ResolveOption[GrafanaOptions]` is called on a `ProviderOptions` map decoded from the wire that contains the `"grafana"` key
- **THEN** it SHALL return the typed `GrafanaOptions` value including its set control fields and capture-mode value

#### Scenario: No new transport surface

- **WHEN** the provider transport for a request carrying `GrafanaOptions` is inspected
- **THEN** the option SHALL travel inside the JSON `CallOptions` body under `providerOptions` and SHALL NOT introduce a new HTTP header or wire path

### Requirement: Client-side validation of known fields

The `providers/grafana` package SHALL validate `GrafanaOptions` against its
known fields before a request is sent. An invalid value (for example a
`CaptureMode` that is not one of the defined constants) SHALL be surfaced to the
client as an error rather than sent on the wire.

#### Scenario: Invalid capture mode rejected client-side

- **WHEN** a client constructs an `AgentObservabilityControl` with a `CaptureMode` value that is not one of the defined constants and validation runs
- **THEN** the client SHALL receive an error and the request SHALL NOT be sent with the invalid value

#### Scenario: Valid options pass validation

- **WHEN** a client constructs a `GrafanaOptions` whose set fields use defined constant values
- **THEN** validation SHALL succeed and the option SHALL be eligible to send

### Requirement: Options carry intent for the server-side middleware stack

`GrafanaOptions` SHALL define a self-describing per-request control contract that
the Grafana hosted backend consumes. The semantics each control expresses SHALL
be: a nil control means "no client preference; the backend default applies"; a
non-nil control with `Disabled` true requests full suppression of that
middleware for the request; and a graded field (e.g. Agent Observability `CaptureMode`)
requests that the backend override its tenant default for that concern on that
request, with client preference taking precedence. The backend resolution and
enforcement of these semantics is delivered as a separate follow-up change in
`grafana-assistant-app` and is out of scope for the ai-sdk repo.

#### Scenario: Nil control expresses no preference

- **WHEN** a `GrafanaOptions` value leaves a control field nil
- **THEN** the option SHALL carry no preference for that middleware so the backend applies its own default

#### Scenario: Disabled control expresses full suppression intent

- **WHEN** a `GrafanaOptions` control sets `Disabled` to true
- **THEN** the option SHALL express that the corresponding middleware be fully suppressed for the request

#### Scenario: Graded control expresses an override intent

- **WHEN** a `GrafanaOptions.AgentObservability.CaptureMode` is set to a defined value
- **THEN** the option SHALL express that the backend override its tenant default capture mode with the client value for that request
