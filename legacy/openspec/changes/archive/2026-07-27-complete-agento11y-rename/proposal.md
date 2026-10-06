## Why

The 2026-07-15 rename (`89ba440`, `c401536`) froze five categories of `sigil`
names on the premise that they were owned by the Sigil SDK, the hosted provider
wire, or SDK telemetry. Commit `47560e5` (2026-07-21) replaced
`github.com/grafana/sigil-sdk/go` with `github.com/grafana/agento11y/go v0.15.0`
and invalidated that premise:

- At tag `go/v0.15.0` the SDK emits `agento11y.generation.id`, not
  `sigil.generation.id` (`go/agento11y/client.go`).
- `AGENTO11Y_*` is the preferred configuration prefix, with `SIGIL_*` kept as a
  legacy fallback (`go/agento11y/env.go`). One variable, `SIGIL_RUN_ID` in
  `go/agento11y/experiment.go`, still has no `AGENTO11Y_` counterpart; ai-sdk
  reads no environment variable under either prefix.
- The SDK produces no `sigil.hooks.*` span attribute, and no consumer of those
  attributes exists on the ai-sdk span.
- The `json:"sigil"` provider-options key has no server-side reader. The
  archived design for that key records that the backend half was deferred to a
  follow-up PR and that "PR 1 alone is inert"; that follow-up never shipped, so
  the key is free on the wire.

What is left in ai-sdk is therefore internal naming, project-owned telemetry, a
wire key nobody parses, one automation label, and documentation and
specifications that assert an SDK contract that no longer exists.

## What Changes

- Rename 18 unexported mappers, one loop variable, and 26 test functions in
  `middleware/agentobservability` from `Sigil` to `Agento11y`.
- Rename the fixture-regeneration environment variable `SIGIL_REGEN` to
  `AGENTO11Y_REGEN`, including every skip and failure message that names it.
- **BREAKING**: Rename the hooks preflight span from `aisdk.sigil.hooks.preflight`
  to `aisdk.hooks.preflight` and its attributes from `sigil.hooks.*` to
  `aisdk.hooks.*`. The product name is dropped from telemetry rather than
  replaced, so the keys survive the next product rename. No dual emission.
- Add the missing test coverage for the hooks span name and its allow / deny /
  transform attributes through an OpenTelemetry span recorder.
- **BREAKING**: Rename the `GrafanaOptions.AgentObservability` JSON key from
  `"sigil"` to `"agentObservability"`. No tolerant decoding of the old key.
- Correct `middleware/agentobservability/doc.go` and the active specifications,
  which claim SDK ownership of `sigil.*` identifiers that agento11y v0.15.0 does
  not produce, and that the provider-options key is required by the hosted
  provider contract.
- Rename the Renovate conformance-review label to
  `agento11y-conformance-review`, gated on the matching GitHub label rename.

## Capabilities

### Modified Capabilities

- `agent-observability-middleware`: New hooks span name and attribute keys with
  test coverage, `AGENTO11Y_REGEN` fixture regeneration, renamed internal
  mappers in normative prose, and `agento11y.generation.id` on the canonical
  generation span.
- `grafana-provider-options`: `AgentObservability` serializes under
  `"agentObservability"`; the hosted-contract justification for the old key is
  removed.
- `context-enrichment-middleware`: Hosted Grafana control fields enrichment must
  leave alone are `grafana.agentObservability`, `grafana.tracing`,
  `grafana.metrics`, and `grafana.usage`.

## Impact

Consumers that select the hooks preflight span or its attributes by name must
update their queries; nothing in the Grafana organization was found to do so.
Clients that set `GrafanaOptions.AgentObservability` emit a different JSON key,
which no server currently reads. Go identifiers, exported symbols, and recording
behavior do not change, and no conformance fixture changes. Archived OpenSpec
history is untouched.
