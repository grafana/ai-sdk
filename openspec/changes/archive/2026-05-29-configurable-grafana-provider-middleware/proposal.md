## Why

The Grafana provider is a transparent transport to the hosted ai-sdk endpoint in
`grafana-assistant-app`, where built-in middlewares (sigil, tracing, metrics,
usage tracking) are composed server-side around the catalog model. Today clients
have no way to influence those middlewares. Some clients need per-request control
-- for example, a sensitive conversation that should be recorded by sigil in
metadata-only mode while other conversations use full capture. Because the
middlewares live behind the wire, this control must be expressed as a typed,
per-request signal that crosses the provider-wire boundary and is consumed by the
server-side stack.

## What Changes

- Introduce a typed `GrafanaOptions` provider option in `providers/grafana` that
  implements `provider.ProviderOption` with `ProviderKey() == "grafana"`.
- `GrafanaOptions` carries per-middleware control sub-structs (e.g. `Sigil`,
  `Tracing`, `Metrics`, `Usage`), each an explicit, self-documenting type. There
  is no unified on/off knob; each concern is controlled independently.
- Sigil control is graded, not boolean: it carries a capture-mode value mirroring
  the server-side `sigil.ContentCaptureMode` set (e.g. full, metadata-only,
  full-with-metadata-spans).
- Clients attach the options per request through the ai-sdk-native
  `CallOptions.ProviderOptions` channel (via `WithProviderOptions`), not a
  provider-specific context helper. The provider already serializes
  `ProviderOptions` into the wire body, so no new transport plumbing is added on
  the client.
- The options encode a clear per-request intent for the server-side middleware
  stack: nil = backend default, `Disabled` = full suppression, graded field =
  override the backend tenant default (client preference wins).
- Client-side validation rejects invalid known fields (e.g. an unrecognized
  capture mode) before the request is sent.

This change is scoped to the ai-sdk repo. The `grafana-assistant-app` server-side
resolution and enforcement (reading `opts.ProviderOptions["grafana"]`, strict
unknown-field rejection, client-preference precedence in each middleware) is
delivered as a separate follow-up PR after this change merges.

## Capabilities

### New Capabilities
- `grafana-provider-options`: Typed, per-request Grafana provider options that
  express control intent for the server-side middleware stack (sigil capture
  mode, plus per-middleware disable for sigil/tracing/metrics/usage), their wire
  encoding via `ProviderOptions`, the client API for attaching them, and
  client-side validation. Backend resolution and enforcement is a follow-up
  change in `grafana-assistant-app`.

### Modified Capabilities
<!-- No spec-level requirement changes to existing capabilities. The grafana-provider
     transport contract already serializes ProviderOptions verbatim; this change
     adds a new typed option without altering that requirement. -->

## Impact

- `providers/grafana` (this repo): new option types (`GrafanaOptions` and
  per-middleware control sub-structs), client-side validation, and
  documentation. No changes to the transport, auth, or constructor surface.
- Wire contract: additive only. `GrafanaOptions` rides under the existing
  `providerOptions` JSON key keyed by `"grafana"`; no new headers and no
  breaking changes to the provider-wire schema.
- Depends on the existing `typed-provider-options` machinery (`ProviderOption`,
  `ProviderOptions`, `BuildProviderOptions`, `ResolveOption`).
- Out of scope (follow-up PR): `grafana-assistant-app` server-side resolution and
  enforcement -- reading `GrafanaOptions` from request `CallOptions`,
  client-preference precedence over tenant defaults, and strict
  unknown/invalid-field rejection. The `aisdkprovider.Handler` will require no
  changes because the options arrive inside the decoded `CallOptions` body.
