## Context

`middleware/agentobservability` translates ai-sdk `provider.*` types into
`agento11y.*` SDK types through 18 unexported mappers, with a reverse path in
`hooks.go` for the transform round-trip. The 2026-07-15 rename left every
`Sigil`-spelled identifier, the `SIGIL_REGEN` test variable, the hooks span
telemetry, the `json:"sigil"` provider-options key, and the Renovate label in
place, and documented them as externally owned contracts.

The dependency migration in `47560e5` moved the module to
`github.com/grafana/agento11y/go v0.15.0`. Reading that tag directly:
`go/agento11y/client.go` emits `agento11y.generation.id`, and
`go/agento11y/env.go` treats `AGENTO11Y_*` as preferred with `SIGIL_*` as a
legacy fallback. No `sigil.*` name is owned by the SDK any more, so every
remaining occurrence in ai-sdk is project-owned.

## Goals / Non-Goals

**Goals:**

- Leave no `sigil` occurrence in active sources, active specifications,
  documentation, telemetry, or configuration.
- Keep the diff mechanically reviewable: one-to-one identifier renames, no
  behavior change beyond the two intentional wire and telemetry renames.
- Cover the hooks preflight span with tests, which it has never had, before
  changing its names.
- Correct the documentation and specification claims that agento11y v0.15.0
  falsifies.

**Non-Goals:**

- Sync the `grafana-assistant-app` git subtree at `../ai-sdk`, which still
  imports `github.com/grafana/ai-sdk/middleware/sigil` in 13 places and predates
  even `c401536`. It needs its own change.
- Migrate `grafana-assistant-app`'s independent legacy emitter on span
  `llm.claude.hooks.preflight`, which hardcodes its own `sigil.hooks.*`
  attributes and shares three keys with ai-sdk by coincidence, not by join.
- Change the Grafana documentation URL route that still reads
  `ai-observability`. It is externally owned and currently works; only its label
  matters and that is already correct.
- Implement the server-side reader for `GrafanaOptions`, which still does not
  exist.
- Migrate runtime `SIGIL_*` environment variables. ai-sdk never reads them,
  agento11y v0.15.0 still accepts them as a fallback, and `SIGIL_RUN_ID` has no
  `AGENTO11Y_` counterpart there at all.
- Rewrite archived OpenSpec history.

This change deliberately reverses the Non-Goals of
`openspec/changes/archive/2026-07-15-rename-agent-observability/`, which listed
`SIGIL_*` configuration, `sigil.*` metadata, the `"sigil"` JSON key, and the
`aisdk.sigil.hooks.preflight` span as names to preserve. That list was correct
while the dependency was `sigil-sdk`; it expired at `47560e5`.

## Decisions

- **Rename `json:"sigil"` to `json:"agentObservability"` outright.** No
  server-side parser exists: the hosted API decodes provider options into an
  opaque raw value, and capture mode is resolved server-side from tenant ID and a
  database opt-out bit with no `CallOptions` input. The archived design for the
  key states that the client half alone is inert. No gateway coordination is
  needed.
- **Use `agentObservability`, not `agent_observability`.** This matches the
  sibling keys and `captureMode` in `providers/grafana/options.go` and the
  repository's camelCase JSON-tag convention.
- **Use `aisdk.hooks.*`, not `agento11y.hooks.*`.** ai-sdk emits these
  attributes on its own span. Claiming the SDK's namespace for keys the SDK never
  produces would recreate the coupling this change removes.
- **Drop the product name from telemetry entirely.** Product names embedded in
  telemetry keys caused this problem once already; `aisdk.hooks.preflight` is
  immune to the next rename.
- **No dual emission and no tolerant decode.** No verified consumer exists for
  the old attribute keys or the old wire key. Dual-writing would pollute every
  hooks span for a hypothetical reader. Tolerant decoding can be added later as a
  small additive `UnmarshalJSON` if a reader appears.
- **Mechanical `Sigil` -> `Agento11y` mapper renames.** Intent-based names such
  as `messagesToRecord` or `finishReasonToStopReason` read better but turn a
  reviewable rename into a semantic change. The mechanical form matches
  `c401536` and the bare `agento11y` import alias.
- **Test the span before renaming it.** The span name and attribute keys are
  normative in the specification and had no test at all.
  `go.opentelemetry.io/otel/sdk` is already an indirect dependency, so a
  `tracetest` span recorder is cheap; it is promoted to a direct dependency.
- **One change, with the wire-key phase separable.** The identifier, telemetry,
  and documentation phases do not depend on the provider-options key, so that
  phase can be dropped without unpicking the rest.
- **Parity classification: this change sits outside the registered upstream
  baseline.** Provider options are a parity-sensitive layer under AGENTS.md, so
  the classification is recorded here. `test/conformance/upstream.yaml` registers
  no Grafana package because `GrafanaOptions` has no upstream analogue, and the
  only Grafana row in `test/conformance/PARITY.md` covers provider-wire
  transport rather than provider options. No conformance fixture, request
  snapshot, or UI chunk snapshot contains either the former or the new key, so no
  generated expectation moves. The hooks span and the mapper renames are
  internal to `middleware/agentobservability`, which has no upstream counterpart.
  `mise run validate-parity-baseline` is still run as a guard.

## Risks / Trade-offs

- A Grafana Cloud dashboard, alert rule, or TraceQL query outside the scanned
  repositories could select `aisdk.sigil.hooks.preflight` or `sigil.hooks.*` →
  the audit is a manual step; the investigation covered six local repositories
  and organization-wide code search, which cannot inspect dashboards or private
  unindexed repositories.
- Renovate would apply a nonexistent label if the configuration merges first →
  the GitHub label rename is an explicit human-owned gate on that phase.
- A hypothetical client already sending `{"sigil": ...}` loses the field
  silently → nothing reads it server-side today, so there is no behavior to lose.
- A broad textual rename could touch recorded fixtures → all 32 conformance
  fixtures were checked and none contains `sigil`; regeneration under the new
  variable must leave every `expected_*.json` snapshot byte-identical.
- Fixture regeneration is not fully idempotent today, independently of this
  change: at `ae6fc22`, running it rewrites five *input* fixtures
  (`generation/tool_call/result.json`, `generation/tool_use_stop/result.json`,
  and three `stream/*/stream.json`) because the hand-edited form of
  `provider.ToolCallPart.Input` and the JSON the writer marshals differ. The
  expected-output snapshots are stable. This change does not fix or worsen that;
  it is a separate defect in the fixture writer.
- The `grafana-assistant-app` subtree drifts further while it is out of scope →
  its exported symbols are unchanged by this work, so the eventual sync stays
  mechanical.
