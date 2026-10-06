## Context

The repository currently exposes Grafana's product name through a nested Go
module, package identifier, Grafana provider option API, documentation paths,
automation tasks, diagnostics, and active specifications. The underlying
integration still depends on the Sigil SDK, whose module path and protocol
identifiers remain externally owned.

## Goals / Non-Goals

**Goals:**

- Rename every project-owned active surface from AI Observability to Agent
  Observability.
- Make the package and module path `middleware/agentobservability`.
- Preserve runtime behavior and exact Sigil-owned contracts.
- Leave the repository with no active references to project-owned old names.

**Non-Goals:**

- Rename `github.com/grafana/sigil-sdk`, `sigilsdk` aliases, `SIGIL_*`
  configuration, `sigil.*` metadata, the `"sigil"` JSON key, or the
  `aisdk.sigil.hooks.preflight` span.
- Provide compatibility aliases or a forwarding package.
- Rewrite archived OpenSpec history.
- Change Grafana documentation URL routes that still use `ai-observability`.

## Decisions

- Use `agentobservability` as the Go package identifier and
  `agent-observability` in prose and Markdown paths.
- Rename the Grafana provider's exported field and control type. The JSON tag
  remains `sigil` so hosted provider wire compatibility is unchanged.
- Rename project-owned diagnostics, test names, task names, tracer
  instrumentation scope, and Renovate descriptions. Use the existing
  `sigil-conformance-review` repository label for Sigil SDK dependency review.
- Replace the active `ai-observability-middleware` capability with
  `agent-observability-middleware` and update every active composition spec.
- Apply the same filename change to the still-active middleware documentation
  organization change. Archived changes retain the names current when they
  were completed.

## Risks / Trade-offs

- Existing consumers will stop compiling → document the exact import and symbol
  replacements and make the break atomic.
- A broad textual rename could alter owned protocol identifiers → validate an
  explicit allowlist of remaining `sigil` names and run conformance tests.
- Nested module paths can be missed by root validation → run tests, vet, build,
  and tidy-aware checks across all modules through Mise.
- External Grafana documentation routes still contain the old slug → retain the
  verified working URLs while changing their labels and surrounding prose.
